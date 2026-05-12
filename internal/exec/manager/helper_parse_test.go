package indexer

import (
	"testing"

	"stake_indexer/conf"
)

func TestNormalizeIndexerIDAtHeight_AllowsOutsideAllowlistWindow(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	backup := conf.StakeRewardCfg
	t.Cleanup(func() {
		conf.StakeRewardCfg = backup
	})

	cfg := conf.DefaultConfig()
	cfg.IndexerAllowlistWindows = []conf.IndexerAllowlistWindow{
		{StartHeight: 100, EndHeight: 200, IndexerIDs: []string{"12:3"}},
	}
	conf.StakeRewardCfg = cfg

	id, err := m.normalizeIndexerIDAtHeight("12:4", 200)
	if err != nil {
		t.Fatalf("normalizeIndexerIDAtHeight returned error: %v", err)
	}
	if id != "12:4" {
		t.Fatalf("unexpected indexer id: %q", id)
	}
}

func TestNormalizeIndexerIDAtHeight_AllowlistWindow(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	backup := conf.StakeRewardCfg
	t.Cleanup(func() {
		conf.StakeRewardCfg = backup
	})

	cfg := conf.DefaultConfig()
	cfg.IndexerAllowlistWindows = []conf.IndexerAllowlistWindow{
		{StartHeight: 100, EndHeight: 200, IndexerIDs: []string{"12:3"}},
	}
	conf.StakeRewardCfg = cfg

	if _, err := m.normalizeIndexerIDAtHeight("12:3", 100); err != nil {
		t.Fatalf("allowlisted id should pass at start height, got error: %v", err)
	}
	if _, err := m.normalizeIndexerIDAtHeight("12:4", 100); err == nil {
		t.Fatalf("non-allowlisted id should fail inside allowlist window")
	}
	if _, err := m.normalizeIndexerIDAtHeight("12:4", 199); err == nil {
		t.Fatalf("non-allowlisted id should fail before end height")
	}
	if _, err := m.normalizeIndexerIDAtHeight("12:4", 200); err != nil {
		t.Fatalf("end height should be excluded from allowlist window, got error: %v", err)
	}
}
