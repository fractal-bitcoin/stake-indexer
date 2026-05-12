package protocolparser

import (
	"strings"
)

type OpReturnPayload struct {
	Tag    string
	Fields map[string]string
	Values []string
}

const (
	OpFieldIndexerID   = "INDEXER_ID"
	OpFieldIndexerName = "INDEXER_NAME"
	OpFieldIndexRatio  = "INDEX_RATIO"
	OpFieldPubKey      = "PUBKEY"
	OpFieldAddressType = "ADDRESS_TYPE"
	OpFieldBlockHeight = "BLOCK_HEIGHT"
	OpFieldBlockHash   = "BLOCK_HASH"
	OpFieldStakeAddr   = "STAKE_ADDRESS"
	OpFieldRewardAddr  = "REWARD_ADDR"
	OpFieldActorPubKey = "ACTOR_PUBKEY"
	OpFieldActorAddr   = "ACTOR_ADDRESS"
)

func (p *OpReturnPayload) Get(keys ...string) string {
	for _, key := range keys {
		normalized := normalizeFieldKey(key)
		if value, ok := p.Fields[normalized]; ok && value != "" {
			return value
		}
	}
	return ""
}

func (p *OpReturnPayload) ValueAt(idx int) string {
	if idx < 0 || idx >= len(p.Values) {
		return ""
	}
	return p.Values[idx]
}

func normalizeFieldKey(key string) string {
	key = strings.TrimSpace(strings.ToUpper(key))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, " ", "_")
	return key
}
