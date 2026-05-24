package pgdb

import (
	"database/sql"
	"fmt"
	logger "stake_indexer/internal/component/log"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var StakeDB *sql.DB

func InitAll(configFile string) error {
	if err := Init(configFile); err != nil {
		return fmt.Errorf("init pg failed: %w", err)
	}
	if err := InitStakeSchema(); err != nil {
		return fmt.Errorf("init pg stake schema failed: %w", err)
	}
	return nil
}

func Init(filename string) error {
	vp := viper.New()
	vp.SetConfigFile(filename)
	if err := vp.ReadInConfig(); err != nil {
		return fmt.Errorf("read pg config failed: %w", err)
	}

	dsn := vp.GetString("dsn")
	if dsn == "" {
		host := vp.GetString("host")
		port := vp.GetInt("port")
		user := vp.GetString("user")
		password := vp.GetString("password")
		dbname := vp.GetString("dbname")
		sslmode := vp.GetString("sslmode")
		if sslmode == "" {
			sslmode = "disable"
		}

		if host == "" || port == 0 || user == "" || dbname == "" {
			return fmt.Errorf("invalid pg config, require host/port/user/dbname or dsn")
		}

		dsn = fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			host, port, user, password, dbname, sslmode,
		)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open pg failed: %w", err)
	}

	maxOpenConns := vp.GetInt("max_open_conns")
	maxIdleConns := vp.GetInt("max_idle_conns")
	connMaxLifetime := vp.GetDuration("conn_max_lifetime")
	connMaxIdleTime := vp.GetDuration("conn_max_idle_time")

	if maxOpenConns > 0 {
		db.SetMaxOpenConns(maxOpenConns)
	}
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}
	if connMaxLifetime > 0 {
		db.SetConnMaxLifetime(connMaxLifetime)
	} else {
		db.SetConnMaxLifetime(30 * time.Minute)
	}
	if connMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(connMaxIdleTime)
	}

	if err = db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("ping pg failed: %w", err)
	}

	StakeDB = db
	logger.Log.Info("pg initialized", zap.String("config", filename))
	return nil
}

func InitStakeSchema() error {
	if StakeDB == nil {
		return fmt.Errorf("pg not initialized")
	}

	ddl := `
CREATE TABLE IF NOT EXISTS stake_proofs (
    id BIGSERIAL PRIMARY KEY,
    indexer_id TEXT NOT NULL,
    prove_block_height BIGINT NOT NULL,
    prove_data_hash TEXT NOT NULL,
    txid TEXT NOT NULL UNIQUE,
    height BIGINT NOT NULL,
    tx_idx INT NOT NULL,
    verify_status SMALLINT NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS idx_stake_proofs_indexer_id ON stake_proofs(indexer_id);
CREATE INDEX IF NOT EXISTS idx_stake_proofs_prove_height ON stake_proofs(prove_block_height);
CREATE INDEX IF NOT EXISTS idx_stake_proofs_verify_status ON stake_proofs(verify_status);
CREATE INDEX IF NOT EXISTS idx_stake_proofs_indexer_status_height ON stake_proofs(indexer_id, verify_status, height DESC, tx_idx DESC);

CREATE TABLE IF NOT EXISTS stake_indexer_registers (
    indexer_id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    reward_address TEXT NOT NULL,
    user_address TEXT NOT NULL DEFAULT '',
    index_ratio DOUBLE PRECISION NOT NULL,
    last_update_height BIGINT NOT NULL,
    txid TEXT NOT NULL UNIQUE,
    height BIGINT NOT NULL,
    tx_idx INT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_stake_indexer_registers_reward_address ON stake_indexer_registers(reward_address);
CREATE INDEX IF NOT EXISTS idx_stake_indexer_registers_user_address ON stake_indexer_registers(user_address);
CREATE UNIQUE INDEX IF NOT EXISTS uq_stake_indexer_registers_user_address ON stake_indexer_registers(user_address);

CREATE TABLE IF NOT EXISTS stake_claimed_rewards (
    txid TEXT PRIMARY KEY,
    user_address TEXT NOT NULL,
    amount BIGINT NOT NULL,
    height BIGINT NOT NULL,
    tx_idx INT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_stake_claimed_rewards_user_address ON stake_claimed_rewards(user_address);
CREATE INDEX IF NOT EXISTS idx_stake_claimed_rewards_height ON stake_claimed_rewards(height);

CREATE TABLE IF NOT EXISTS stake_allocated_rewards (
    id BIGSERIAL PRIMARY KEY,
    user_address TEXT NOT NULL,
    indexer_id TEXT NOT NULL,
    stake_address TEXT NOT NULL,
    reward_type TEXT NOT NULL DEFAULT 'stake',
    height BIGINT NOT NULL,
    stake_amount_snapshot BIGINT NOT NULL,
    stake_amount_effective BIGINT NOT NULL DEFAULT 0,
    total_effective_stake BIGINT NOT NULL DEFAULT 0,
    release_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    block_reward_amount BIGINT NOT NULL DEFAULT 0,
    indexer_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
    allocate_amount BIGINT NOT NULL,
    UNIQUE (height, indexer_id, stake_address)
);
CREATE INDEX IF NOT EXISTS idx_stake_allocated_rewards_user_address ON stake_allocated_rewards(user_address);
CREATE INDEX IF NOT EXISTS idx_stake_allocated_rewards_stake_address ON stake_allocated_rewards(stake_address);
CREATE INDEX IF NOT EXISTS idx_stake_allocated_rewards_reward_type ON stake_allocated_rewards(reward_type);
CREATE INDEX IF NOT EXISTS idx_stake_allocated_rewards_indexer_height ON stake_allocated_rewards(indexer_id, height DESC);

CREATE TABLE IF NOT EXISTS stake_pending_rewards (
    id BIGSERIAL PRIMARY KEY,
    user_address TEXT NOT NULL,
    indexer_id TEXT NOT NULL,
    stake_address TEXT NOT NULL,
    reward_type TEXT NOT NULL DEFAULT 'stake',
    height BIGINT NOT NULL,
    stake_amount_snapshot BIGINT NOT NULL,
    stake_amount_effective BIGINT NOT NULL DEFAULT 0,
    total_effective_stake BIGINT NOT NULL DEFAULT 0,
    release_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    block_reward_amount BIGINT NOT NULL DEFAULT 0,
    indexer_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
    pending_amount BIGINT NOT NULL,
    UNIQUE (height, indexer_id, stake_address)
);
CREATE INDEX IF NOT EXISTS idx_stake_pending_rewards_user_address ON stake_pending_rewards(user_address);
CREATE INDEX IF NOT EXISTS idx_stake_pending_rewards_stake_address ON stake_pending_rewards(stake_address);
CREATE INDEX IF NOT EXISTS idx_stake_pending_rewards_reward_type ON stake_pending_rewards(reward_type);
CREATE INDEX IF NOT EXISTS idx_stake_pending_rewards_indexer_height ON stake_pending_rewards(indexer_id, height DESC);

CREATE TABLE IF NOT EXISTS stake_bindings (
    stake_address TEXT PRIMARY KEY,
    user_address TEXT NOT NULL,
    indexer_id TEXT NOT NULL,
    address_type TEXT NOT NULL,
    height BIGINT NOT NULL,
    txid TEXT NOT NULL,
    tx_idx INT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_stake_bindings_indexer_id ON stake_bindings(indexer_id);
CREATE INDEX IF NOT EXISTS idx_stake_bindings_user_address ON stake_bindings(user_address);
CREATE INDEX IF NOT EXISTS idx_stake_bindings_address_type ON stake_bindings(address_type);
DROP INDEX IF EXISTS uq_stake_bindings_indexer_user;
CREATE UNIQUE INDEX IF NOT EXISTS uq_stake_bindings_indexer_user_type ON stake_bindings(indexer_id, user_address, address_type);
CREATE INDEX IF NOT EXISTS idx_stake_bindings_height ON stake_bindings(height);
CREATE TABLE IF NOT EXISTS sync_blocks (
    height BIGINT PRIMARY KEY,
    block_hash TEXT NOT NULL,
    parent_hash TEXT NOT NULL,
    version BIGINT NOT NULL,
    coinbase_reward BIGINT NOT NULL,
    state TEXT NOT NULL,
    is_reward_block_version BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_sync_blocks_hash ON sync_blocks(block_hash);
CREATE INDEX IF NOT EXISTS idx_sync_blocks_state ON sync_blocks(state);
CREATE INDEX IF NOT EXISTS idx_sync_blocks_reward_ver_height ON sync_blocks(is_reward_block_version, height);

CREATE TABLE IF NOT EXISTS indexer_addr_deltas (
    height BIGINT NOT NULL,
    address TEXT NOT NULL,
    delta BIGINT NOT NULL,
    PRIMARY KEY (height, address)
);
CREATE INDEX IF NOT EXISTS idx_indexer_addr_deltas_height ON indexer_addr_deltas(height);
CREATE INDEX IF NOT EXISTS idx_indexer_addr_deltas_address ON indexer_addr_deltas(address);

CREATE TABLE IF NOT EXISTS indexer_undo_new (
    height BIGINT NOT NULL,
    outpoint TEXT NOT NULL,
    utxo_raw BYTEA NOT NULL,
    PRIMARY KEY (height, outpoint)
);
CREATE INDEX IF NOT EXISTS idx_indexer_undo_new_height ON indexer_undo_new(height);

CREATE TABLE IF NOT EXISTS indexer_undo_spent (
    height BIGINT NOT NULL,
    outpoint TEXT NOT NULL,
    utxo_raw BYTEA NOT NULL,
    PRIMARY KEY (height, outpoint)
);
CREATE INDEX IF NOT EXISTS idx_indexer_undo_spent_height ON indexer_undo_spent(height);

CREATE TABLE IF NOT EXISTS stake_mempool_events (
    txid TEXT PRIMARY KEY,
    op TEXT NOT NULL,
    height BIGINT NOT NULL DEFAULT 0,
    inscription_content TEXT NOT NULL DEFAULT '',
    indexer_id TEXT NOT NULL DEFAULT '',
    user_address TEXT NOT NULL DEFAULT '',
    reward_address TEXT NOT NULL DEFAULT '',
    stake_address TEXT NOT NULL DEFAULT '',
    amount BIGINT NOT NULL DEFAULT 0,
    index_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
    indexer_name TEXT NOT NULL DEFAULT '',
    prove_block_height BIGINT NOT NULL DEFAULT 0,
    prove_data_hash TEXT NOT NULL DEFAULT '',
    biz_invalid_flags BIGINT NOT NULL DEFAULT 0,
    tx_idx INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_stake_mempool_events_op ON stake_mempool_events(op);
CREATE INDEX IF NOT EXISTS idx_stake_mempool_events_height ON stake_mempool_events(height);
CREATE INDEX IF NOT EXISTS idx_stake_mempool_events_indexer_id ON stake_mempool_events(indexer_id);
CREATE INDEX IF NOT EXISTS idx_stake_mempool_events_user_address ON stake_mempool_events(user_address);
CREATE INDEX IF NOT EXISTS idx_stake_mempool_events_reward_address ON stake_mempool_events(reward_address);

CREATE TABLE IF NOT EXISTS fip101_inscription_events (
    txid TEXT PRIMARY KEY,
    op TEXT NOT NULL,
    height BIGINT NOT NULL DEFAULT 0,
    inscription_content TEXT NOT NULL DEFAULT '',
    indexer_id TEXT NOT NULL DEFAULT '',
    user_address TEXT NOT NULL DEFAULT '',
    reward_address TEXT NOT NULL DEFAULT '',
    stake_address TEXT NOT NULL DEFAULT '',
    amount BIGINT NOT NULL DEFAULT 0,
    index_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
    indexer_name TEXT NOT NULL DEFAULT '',
    prove_block_height BIGINT NOT NULL DEFAULT 0,
    prove_data_hash TEXT NOT NULL DEFAULT '',
    biz_invalid_flags BIGINT NOT NULL DEFAULT 0,
    tx_idx INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_fip101_inscription_events_op ON fip101_inscription_events(op);
CREATE INDEX IF NOT EXISTS idx_fip101_inscription_events_height ON fip101_inscription_events(height);
CREATE INDEX IF NOT EXISTS idx_fip101_inscription_events_indexer_id ON fip101_inscription_events(indexer_id);
CREATE INDEX IF NOT EXISTS idx_fip101_inscription_events_user_address ON fip101_inscription_events(user_address);
CREATE INDEX IF NOT EXISTS idx_fip101_inscription_events_reward_address ON fip101_inscription_events(reward_address);
`

	if _, err := StakeDB.Exec(ddl); err != nil {
		return fmt.Errorf("init stake schema failed: %w", err)
	}
	return nil
}
