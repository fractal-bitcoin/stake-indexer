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

func TestIsIndexerAllowedAtHeight_AllowlistWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IndexerAllowlistWindows = []IndexerAllowlistWindow{
		{StartHeight: 100, EndHeight: 200, IndexerIDs: []string{"12:3"}, indexerSet: stringSet([]string{"12:3"})},
	}

	if !cfg.IsIndexerAllowedAtHeight("12:4", 99) {
		t.Fatalf("outside window should allow valid indexer")
	}
	if !cfg.IsIndexerAllowedAtHeight("12:3", 100) {
		t.Fatalf("start height should allow listed indexer")
	}
	if cfg.IsIndexerAllowedAtHeight("12:4", 100) {
		t.Fatalf("start height should reject unlisted indexer")
	}
	if cfg.IsIndexerAllowedAtHeight("12:4", 199) {
		t.Fatalf("height before end should reject unlisted indexer")
	}
	if !cfg.IsIndexerAllowedAtHeight("12:4", 200) {
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
