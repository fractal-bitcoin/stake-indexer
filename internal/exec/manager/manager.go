package indexer

import (
	"context"
	"fmt"
	"stake_indexer/conf"
	pgdb "stake_indexer/internal/component/pg"
	protocolparser "stake_indexer/internal/parser/protocol"
	"stake_indexer/model"
	"strings"
)

type StakeAddressInfo struct {
	Address   string
	IndexerID string
}

type WaitForUpsert struct {
	StakeIndexerRegisterList []pgdb.StakeIndexerRegister
	StakeProofList           []pgdb.StakeProof
	StakeClaimedRewardList   []pgdb.StakeClaimedReward
	StakeBindingList         []pgdb.StakeBinding
	FIP101InscriptionEvents  []pgdb.FIP101InscriptionEvent
}

type mempoolOutpointInfo struct {
	Address string
	Satoshi uint64
}

type Manager struct {
	ctx context.Context

	slowState *slowWriteState

	pendingRewardMode bool

	stakeAddrToIndexer        map[string]StakeAddressInfo
	indexerToAddrStakeAmount  map[string]map[string]uint64
	indexerToUserStakeAddress map[string]map[string]string
	stakeBindingsLoadedHeight uint32
	mempoolOutpointCache      map[string]mempoolOutpointInfo

	registerOwnerSeen map[string]struct{}

	WaitForUpsert         WaitForUpsert
	BlocksOfChainByHeight map[uint32]*model.BlockIndexInfo
	MainChainHeight       uint32
}

func NewManager(cfg conf.StakeRewardConfigInfo) *Manager {
	if cfg.ProofWindow == 0 {
		cfg.ProofWindow = 144
	}

	return newManagerWithMode(cfg, false)
}

func NewPendingManager(cfg conf.StakeRewardConfigInfo) *Manager {
	return newManagerWithMode(cfg, true)
}

func newManagerWithMode(cfg conf.StakeRewardConfigInfo, pendingMode bool) *Manager {
	return &Manager{
		ctx:                       context.Background(),
		slowState:                 newSlowWriteState(),
		pendingRewardMode:         pendingMode,
		stakeBindingsLoadedHeight: 0,
		stakeAddrToIndexer:        make(map[string]StakeAddressInfo),
		indexerToAddrStakeAmount:  make(map[string]map[string]uint64),
		indexerToUserStakeAddress: make(map[string]map[string]string),
		mempoolOutpointCache:      make(map[string]mempoolOutpointInfo),
		registerOwnerSeen:         make(map[string]struct{}),

		BlocksOfChainByHeight: make(map[uint32]*model.BlockIndexInfo),
	}
}

func (m *Manager) SubmitBalanceBlock(block protocolparser.BlockSnapshot) error {
	if m == nil {
		return nil
	}
	if err := m.handleBlockReward(&block); err != nil {
		return fmt.Errorf("handle block reward failed at height %d: %w", block.Height, err)
	}
	return nil
}

func (m *Manager) shouldTrackStakeAddress(address string) bool {
	if m == nil {
		return false
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return false
	}
	_, ok := m.stakeAddrToIndexer[address]
	return ok
}

func (m *Manager) cacheStakeBinding(userAddress, indexerID, addressType, stakeAddress string) {
	if m == nil {
		return
	}

	userAddress = strings.TrimSpace(userAddress)
	indexerID = strings.TrimSpace(indexerID)
	addressType = strings.TrimSpace(addressType)
	stakeAddress = strings.TrimSpace(stakeAddress)
	if userAddress == "" || indexerID == "" || addressType == "" || stakeAddress == "" {
		return
	}

	m.stakeAddrToIndexer[stakeAddress] = StakeAddressInfo{
		Address:   userAddress,
		IndexerID: indexerID,
	}
	userMap := m.indexerToUserStakeAddress[indexerID]
	if userMap == nil {
		userMap = make(map[string]string)
		m.indexerToUserStakeAddress[indexerID] = userMap
	}
	userMap[userAddress] = stakeAddress
}
