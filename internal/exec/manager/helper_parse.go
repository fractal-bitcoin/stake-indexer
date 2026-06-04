package indexer

import (
	"fmt"
	"strings"

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
	return id, nil
}
