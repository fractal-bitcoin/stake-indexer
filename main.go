package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"stake_indexer/conf"
	api "stake_indexer/interfaces/http"
	idxbootstrap "stake_indexer/internal/component/init"
	logger "stake_indexer/internal/component/log"
	"stake_indexer/internal/component/node"
	pgdb "stake_indexer/internal/component/pg"
	rdb "stake_indexer/internal/component/redis"
	"stake_indexer/internal/component/stateapi"
	entrymempool "stake_indexer/internal/entry/mempool"
	idxrollback "stake_indexer/internal/entry/rollback"
	entryslow "stake_indexer/internal/entry/slow"
	indexer "stake_indexer/internal/exec/manager"
	"stake_indexer/lib/midware"
	"stake_indexer/model"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/pprof"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var listenAddress = os.Getenv("LISTEN")

const (
	retryInterval       = time.Second
	mempoolLoopInterval = 2 * time.Second
)

var (
	stakeManager        *indexer.Manager
	stakeRewardSyncOnce sync.Once
)

func bootstrap() error {
	if err := logger.Init(); err != nil {
		return err
	}
	if err := conf.Init("conf/config.yaml"); err != nil {
		return fmt.Errorf("init config failed: %w", err)
	}
	if err := node.Init("conf/chain.yaml"); err != nil {
		return fmt.Errorf("init node rpc failed: %w", err)
	}
	if err := stateapi.Init(
		conf.StakeRewardCfg.StateAPIBaseURL,
		conf.StakeRewardCfg.StateAPIAuth,
		conf.StakeRewardCfg.StateAPITimeout,
	); err != nil {
		return fmt.Errorf("init state api failed: %w", err)
	}
	if err := pgdb.InitAll("conf/pg.yaml"); err != nil {
		return fmt.Errorf("init pg failed: %w", err)
	}
	if err := rdb.InitAll("conf/rdb_balance.yaml", "conf/rdb_utxo.yaml"); err != nil {
		return fmt.Errorf("init redis failed: %w", err)
	}
	if err := idxbootstrap.ResetMempoolSnapshotOnStartup(context.Background()); err != nil {
		return fmt.Errorf("reset mempool snapshot on startup failed: %w", err)
	}
	if err := idxbootstrap.CleanupLegacyIndexerArtifactsOnStartup(context.Background()); err != nil {
		return fmt.Errorf("cleanup legacy indexer artifacts on startup failed: %w", err)
	}
	if err := idxbootstrap.InitIndexerStatusCache(context.Background()); err != nil {
		return fmt.Errorf("init indexer status cache failed: %w", err)
	}
	return nil
}

func startStakeRewardSyncAfterCommitted(committedHeight uint32) {
	stakeRewardSyncOnce.Do(func() {
		logger.Log.Info("starting stake reward sync", zap.Uint32("committed_height", committedHeight))
		go entryslow.SyncStakeRewardIndexer()
	})
}

func startStakeRewardSyncIfCommitted() {
	committedHeight, exists, err := pgdb.GetLatestCommittedSyncBlockHeight(context.Background())
	if err != nil {
		logger.Log.Warn("check committed block before starting stake reward sync failed", zap.Error(err))
		return
	}
	if !exists {
		return
	}
	startStakeRewardSyncAfterCommitted(committedHeight)
}

func syncBlockIndexer() {
	stakeManager = indexer.NewManager(conf.StakeRewardCfg)
	indexStartHeight := conf.StakeRewardCfg.IndexStartHeight
	logger.Log.Info("index start height", zap.Uint32("index_start_height", indexStartHeight))

	for {
		if model.NeedStop {
			break
		}

		nextHeight, err := indexer.EnsureIndexerDataConsistency()
		if err != nil {
			logger.Log.Warn("syncBlockIndexer check data heights failed", zap.Error(err))
			time.Sleep(retryInterval)
			continue
		}
		if nextHeight < indexStartHeight {
			nextHeight = indexStartHeight
		}

		rolledBack, rollbackFrom, err := idxrollback.RollbackCheckWithFloor(100, indexStartHeight)
		if err != nil {
			logger.Log.Warn("syncBlockIndexer reconcile indexer recent blocks failed", zap.Error(err))
			time.Sleep(retryInterval)
			continue
		}
		if rolledBack {
			logger.Log.Warn("syncBlockIndexer rollback detected",
				zap.Uint32("rollback_from", rollbackFrom))
			nextHeight = rollbackFrom
			if nextHeight < indexStartHeight {
				nextHeight = indexStartHeight
			}
		}
		startStakeRewardSyncIfCommitted()

		loadBindingHeight := uint32(0)
		if nextHeight > 0 {
			loadBindingHeight = nextHeight - 1
		}
		if err := stakeManager.LoadStakeBindingsToHeight(loadBindingHeight, false); err != nil {
			logger.Log.Warn("syncBlockIndexer load stake bindings failed", zap.Error(err), zap.Uint32("height", loadBindingHeight))
			time.Sleep(retryInterval)
			continue
		}

		if ok := stakeManager.InitLatestBlockFromRPC(nextHeight, conf.StakeRewardCfg.BatchBlockCount); !ok {
			logger.Log.Warn("syncBlockIndexer init latest block from rpc failed")
			time.Sleep(retryInterval)
			continue
		}

		if err := indexer.UpdateLatestBlockHeightStatus(stakeManager.MainChainHeight); err != nil {
			logger.Log.Warn("syncBlockIndexer update latest_block_height failed", zap.Error(err))
		}

		currentEndBlockHeight := stakeManager.MainChainHeight + 1
		if currentEndBlockHeight <= nextHeight {
			stats, err := entrymempool.Sync(stakeManager, stakeManager.MainChainHeight)
			if err != nil {
				logger.Log.Warn("syncBlockIndexer sync mempool proofs failed", zap.Error(err))
				time.Sleep(retryInterval)
				continue
			}
			if stats.Upserted > 0 || stats.Removed > 0 {
				logger.Log.Info("syncBlockIndexer mempool proofs synced",
					zap.Int("mempool_txs", stats.MempoolTxs),
					zap.Int("proof_txs", stats.ProofTxs),
					zap.Int("upserted", stats.Upserted),
					zap.Int("removed", stats.Removed))
			}
			time.Sleep(mempoolLoopInterval)
			continue
		}

		startHeight := nextHeight
		if err := idxbootstrap.ResetMempoolSnapshotOnStartup(context.Background()); err != nil {
			logger.Log.Warn("syncBlockIndexer reset mempool snapshot before parsing blocks failed", zap.Error(err))
			time.Sleep(retryInterval)
			continue
		}
		idxrollback.ResetIndexerArtifactStage()
		_, stageBlockHeight, txCount := stakeManager.ParseLongestChain(nextHeight, currentEndBlockHeight)
		if model.NeedStop && txCount == 0 {
			idxrollback.ResetIndexerArtifactStage()
			break
		}
		if txCount == 0 {
			idxrollback.ResetIndexerArtifactStage()
			time.Sleep(retryInterval)
			continue
		}
		if stageBlockHeight < startHeight {
			idxrollback.ResetIndexerArtifactStage()
			logger.Log.Error("syncBlockIndexer stage height invalid",
				zap.Uint32("start", startHeight),
				zap.Uint32("stage", stageBlockHeight))
			time.Sleep(retryInterval)
			continue
		}

		if err := indexer.SubmitBlocks(stakeManager, startHeight, stageBlockHeight); err != nil {
			logger.Log.Error("syncBlockIndexer submit blocks failed",
				zap.Error(err),
				zap.Uint32("start", startHeight),
				zap.Uint32("end", stageBlockHeight))
			time.Sleep(retryInterval)
			continue
		}
		if model.NeedStop {
			break
		}
		startStakeRewardSyncAfterCommitted(stageBlockHeight)

		logger.Log.Info("syncBlockIndexer block range finished",
			zap.Uint32("start", startHeight),
			zap.Uint32("end", stageBlockHeight),
			zap.Int("tx_count", txCount))
	}

	logger.Log.Info("syncBlockIndexer stopped")
}

func main() {
	if err := bootstrap(); err != nil {
		panic(err)
	}
	entryslow.SetManagerFactory(func(cfg conf.StakeRewardConfigInfo) entryslow.RewardSyncManager {
		return indexer.NewManager(cfg)
	})
	if err := entryslow.EnsureManagerFactoryConfigured(); err != nil {
		panic(err)
	}

	router := gin.New()
	router.Use(ginzap.Ginzap(logger.Log, time.RFC3339, true))
	router.Use(ginzap.RecoveryWithZap(logger.Log, true))
	router.Use(midware.Metrics())
	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithDecompressFn(gzip.DefaultDecompressHandle)))
	router.UseRawPath = true
	router.UnescapePathValues = true
	if os.Getenv("ENABLE_PPROF") == "true" {
		pprof.Register(router)
	}
	if os.Getenv("ENABLE_METRICS") == "true" {
		midware.CreateMetricsEndpoint(router)
	}

	apiV1 := router.Group("/")
	apiV1.GET("/indexers", api.GetIndexersHandler)
	apiV1.GET("/indexer/status", api.GetIndexerStatusHandler)
	apiV1.GET("/indexers/by-address/:address", api.GetIndexerByAddressHandler)
	apiV1.GET("/indexers/:id/stakers", api.GetIndexerStakersHandler)
	apiV1.GET("/indexers/:id/proofs", api.GetIndexerProofsHandler)
	apiV1.GET("/users/:address/stakings", api.GetUserStakingsHandler)
	apiV1.GET("/users/:address/rewards", api.GetUserRewardRecordsHandler)
	apiV1.GET("/stake-reward/sync-status", api.GetStakeRewardSyncStatusHandler)
	apiV1.GET("/mempool/protocol-txs", api.GetMempoolProtocolTxsHandler)

	logger.Log.Info("LISTEN:", zap.String("address", listenAddress))
	svr := &http.Server{
		Addr:    listenAddress,
		Handler: router,
	}
	go func() {
		err := svr.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal("ListenAndServe:", zap.Error(err))
		}
	}()

	sigCtrl := make(chan os.Signal, 1)
	signal.Notify(sigCtrl, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		for s := range sigCtrl {
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				triggerStop()
			default:
				logger.Log.Info("other signal", zap.String("sig", s.String()))
			}
		}
	}()

	go func() {
		for {
			runtime.GC()
			time.Sleep(time.Second * 10)
		}
	}()

	syncBlockIndexer()

	logger.SyncLog()

	timeout := time.Duration(1) * time.Second
	sctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := svr.Shutdown(sctx); err != nil {
		logger.Log.Fatal("Shutdown:", zap.Error(err))
	}

	if model.NeedStop {
		os.Exit(1)
	}
}

func triggerStop() {
	logger.Log.Info("program exit...")
	model.NeedStop = true
}
