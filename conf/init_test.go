package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EnableMempoolIndexing_DefaultFalse(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "index_start_height: 1760000\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.EnableMempoolIndexing {
		t.Fatalf("expected enable_mempool_indexing default false when not configured")
	}
}

func TestLoad_EnableMempoolIndexing_True(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "enable_mempool_indexing: true\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if !cfg.EnableMempoolIndexing {
		t.Fatalf("expected enable_mempool_indexing true when configured")
	}
}

func TestLoad_DelaySubmitStage2Config(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "delay_submit_stage2_step_blocks: 50\ndelay_submit_stage2_step_percent: 20\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.DelaySubmitStage2StepBlocks != 50 {
		t.Fatalf("expected delay_submit_stage2_step_blocks 50, got %d", cfg.DelaySubmitStage2StepBlocks)
	}
	if cfg.DelaySubmitStage2StepPercent != 20 {
		t.Fatalf("expected delay_submit_stage2_step_percent 20, got %d", cfg.DelaySubmitStage2StepPercent)
	}
}

func TestLoad_DelaySubmitStage3Blocks(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "delay_submit_stage3_blocks: [10, 20, 30, 40, 50, 60, 70]\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	expected := []uint32{10, 20, 30, 40, 50, 60, 70}
	if len(cfg.DelaySubmitStage3Blocks) != len(expected) {
		t.Fatalf("expected delay_submit_stage3_blocks len %d, got %d", len(expected), len(cfg.DelaySubmitStage3Blocks))
	}
	for i := range expected {
		if cfg.DelaySubmitStage3Blocks[i] != expected[i] {
			t.Fatalf("delay_submit_stage3_blocks[%d] expected %d, got %d", i, expected[i], cfg.DelaySubmitStage3Blocks[i])
		}
	}
}

func TestLoad_DelaySubmitStage3BlocksRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "wrong length", content: "delay_submit_stage3_blocks: [10, 20]\n"},
		{name: "zero", content: "delay_submit_stage3_blocks: [0, 20, 30, 40, 50, 60, 70]\n"},
		{name: "not increasing", content: "delay_submit_stage3_blocks: [10, 20, 20, 40, 50, 60, 70]\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write config failed: %v", err)
			}
			if _, err := Load(configPath); err == nil {
				t.Fatalf("expected load config error")
			}
		})
	}
}

func TestLoad_CommissionActivationBlocks(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "commission_activation_blocks: 144\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.CommissionActivationBlocks != 144 {
		t.Fatalf("expected commission_activation_blocks 144, got %d", cfg.CommissionActivationBlocks)
	}
}

func TestLoad_CommissionActivationBlocks_Default(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "index_start_height: 1760000\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.CommissionActivationBlocks != 20160 {
		t.Fatalf("expected default commission_activation_blocks 20160, got %d", cfg.CommissionActivationBlocks)
	}
}

func TestRewardProofWindowByHeight(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProofWindow = 20160
	cfg.DelaySubmitStage2StepBlocks = 50
	cfg.DelaySubmitStage2StepPercent = 20
	cfg.DelaySubmitStage3Blocks = []uint32{10, 20, 30, 40, 50, 60, 70}
	cfg.Stage2StartHeight = 1000
	cfg.Stage3StartHeight = 2000

	if got := cfg.RewardProofWindowByHeight(999); got != 20160 {
		t.Fatalf("stage-1 proof window expected 20160, got %d", got)
	}
	if got := cfg.RewardProofWindowByHeight(1000); got != 250 {
		t.Fatalf("stage-2 proof window expected 250, got %d", got)
	}
	if got := cfg.RewardProofWindowByHeight(1999); got != 250 {
		t.Fatalf("before stage-3 proof window expected 250, got %d", got)
	}
	if got := cfg.RewardProofWindowByHeight(2000); got != 70 {
		t.Fatalf("stage-3 proof window expected 70, got %d", got)
	}
}

func TestLoad_RewardClaimSenderAddressKeys(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "reward_claim_sender_address_keys: [' addr-1 ', 'addr-2', 'addr-1', '']\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	expected := []string{"addr-1", "addr-2"}
	if len(cfg.RewardClaimSenderAddressKeys) != len(expected) {
		t.Fatalf("expected reward_claim_sender_address_keys len %d, got %d", len(expected), len(cfg.RewardClaimSenderAddressKeys))
	}
	for i := range expected {
		if cfg.RewardClaimSenderAddressKeys[i] != expected[i] {
			t.Fatalf("reward_claim_sender_address_keys[%d] expected %s, got %s", i, expected[i], cfg.RewardClaimSenderAddressKeys[i])
		}
	}
}

func TestLoad_FixLegacyIndexerClaimRewards_DefaultTrue(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "index_start_height: 1760000\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if !cfg.FixLegacyIndexerClaimRewards {
		t.Fatalf("expected fix_legacy_indexer_claim_rewards default true")
	}
}

func TestLoad_FixLegacyIndexerClaimRewards_False(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "fix_legacy_indexer_claim_rewards: false\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.FixLegacyIndexerClaimRewards {
		t.Fatalf("expected fix_legacy_indexer_claim_rewards false when configured")
	}
}

func TestLoad_Stage2StartHeight(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "stage2_start_height: 1800000\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Stage2StartHeight != 1800000 {
		t.Fatalf("expected stage2_start_height 1800000, got %d", cfg.Stage2StartHeight)
	}
}

func TestLoad_Stage3StartHeight(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := "stage3_start_height: 1925280\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Stage3StartHeight != 1925280 {
		t.Fatalf("expected stage3_start_height 1925280, got %d", cfg.Stage3StartHeight)
	}
}

func TestIsIndexerRewardAllowedAtHeight_AllowlistWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IndexerAllowlistWindows = []IndexerAllowlistWindow{
		{StartHeight: 100, EndHeight: 200, IndexerIDs: []string{"12:3"}, indexerSet: stringSet([]string{"12:3"})},
	}

	if !cfg.IsIndexerRewardAllowedAtHeight("12:4", 99) {
		t.Fatalf("outside window should allow reward allocation")
	}
	if !cfg.IsIndexerRewardAllowedAtHeight("12:3", 100) {
		t.Fatalf("start height should allow listed indexer reward allocation")
	}
	if cfg.IsIndexerRewardAllowedAtHeight("12:4", 100) {
		t.Fatalf("start height should reject unlisted indexer reward allocation")
	}
	if cfg.IsIndexerRewardAllowedAtHeight("12:4", 199) {
		t.Fatalf("height before end should reject unlisted indexer reward allocation")
	}
	if !cfg.IsIndexerRewardAllowedAtHeight("12:4", 200) {
		t.Fatalf("end height should be outside window")
	}
}

func TestParseIndexerAllowlistWindows(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{
			"start_height": "100",
			"end_height":   200,
			"indexer_ids":  []interface{}{"12:3", "12:3", "12:4"},
		},
	}

	windows, err := parseIndexerAllowlistWindows(raw)
	if err != nil {
		t.Fatalf("parseIndexerAllowlistWindows returned error: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("unexpected window count: %d", len(windows))
	}
	window := windows[0]
	if window.StartHeight != 100 || window.EndHeight != 200 {
		t.Fatalf("unexpected window bounds: %#v", window)
	}
	if len(window.IndexerIDs) != 2 {
		t.Fatalf("expected deduplicated indexer ids, got %#v", window.IndexerIDs)
	}
	if _, ok := window.indexerSet["12:4"]; !ok {
		t.Fatalf("expected indexer set to include 12:4")
	}
}

func TestParseIndexerAllowlistWindowsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		raw  interface{}
	}{
		{name: "not list", raw: map[string]interface{}{}},
		{name: "invalid end", raw: []interface{}{map[string]interface{}{"start_height": 100, "end_height": 100, "indexer_ids": []interface{}{"12:3"}}}},
		{name: "invalid id", raw: []interface{}{map[string]interface{}{"start_height": 100, "end_height": 200, "indexer_ids": []interface{}{"bad"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseIndexerAllowlistWindows(tt.raw); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}
