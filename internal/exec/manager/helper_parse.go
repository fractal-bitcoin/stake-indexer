package indexer

import (
	"fmt"
	"strings"

	"stake_indexer/conf"
	protocolparser "stake_indexer/internal/parser/protocol"
)

func (m *Manager) normalizeIndexerIDAtHeight(raw string, height uint32) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", nil
	}
	if !protocolparser.IsIndexerIDHeightTxIdx(id) {
		return "", fmt.Errorf("invalid indexer_id %q, expect format height:txidx", id)
	}
	if !conf.StakeRewardCfg.IsIndexerAllowedAtHeight(id, height) {
		return "", fmt.Errorf("indexer_id %q is not allowed by indexer_allowlist_windows at height %d", id, height)
	}
	return id, nil
}
