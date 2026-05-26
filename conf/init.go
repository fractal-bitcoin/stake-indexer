package conf

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	protocolparser "stake_indexer/internal/parser/protocol"

	"github.com/spf13/viper"
)

type RewardReleaseTier struct {
	StartHeight    uint64
	ReleasePercent float64
}

type IndexerAllowlistWindow struct {
	StartHeight uint32
	EndHeight   uint32
	IndexerIDs  []string
	indexerSet  map[string]struct{}
}

type StakeRewardConfigInfo struct {
	RetryInterval                uint32
	LoopInterval                 uint32
	BatchBlockCount              uint32
	SlowLagBlocks                uint32
	ProofWindow                  uint32
	DelaySubmitTriggerBlocks     uint32
	EnableMempoolIndexing        bool
	IndexStartHeight             uint32
	StartRewardHeight            uint32
	StateAPIBaseURL              string
	StateAPIAuth                 string
	StateAPITimeout              time.Duration
	RewardClaimSenderAddressKeys []string
	IndexerAllowlistWindows      []IndexerAllowlistWindow
	RewardReleaseTiers           []RewardReleaseTier
}

var (
	StakeRewardCfg = DefaultConfig()
)

func defaultRewardReleaseTiers() []RewardReleaseTier {
	return []RewardReleaseTier{
		{StartHeight: 1764000, ReleasePercent: 30},
		{StartHeight: 1784160, ReleasePercent: 37.5},
		{StartHeight: 1804320, ReleasePercent: 45},
		{StartHeight: 1824480, ReleasePercent: 52.5},
		{StartHeight: 1844640, ReleasePercent: 60},
		{StartHeight: 1864800, ReleasePercent: 70},
		{StartHeight: 1884960, ReleasePercent: 80},
		{StartHeight: 1905120, ReleasePercent: 90},
		{StartHeight: 1925280, ReleasePercent: 100},
	}
}

func DefaultConfig() StakeRewardConfigInfo {
	return StakeRewardConfigInfo{
		BatchBlockCount:              500,
		SlowLagBlocks:                20160,
		ProofWindow:                  20160,
		DelaySubmitTriggerBlocks:     120,
		IndexStartHeight:             1760000,
		StartRewardHeight:            1764000,
		StateAPITimeout:              5 * time.Second,
		RewardClaimSenderAddressKeys: nil,
		IndexerAllowlistWindows:      nil,
		RewardReleaseTiers:           defaultRewardReleaseTiers(),
	}
}

func Init(configFile string) error {
	cfg, err := Load(configFile)
	if err != nil {
		return err
	}
	StakeRewardCfg = cfg
	return nil
}

func Load(configFile string) (StakeRewardConfigInfo, error) {
	cfg := DefaultConfig()

	vp := viper.New()
	vp.SetConfigFile(configFile)
	if err := vp.ReadInConfig(); err != nil {
		return cfg, fmt.Errorf("read config failed: %w", err)
	}

	if retryInterval := vp.GetUint32("retry_interval"); retryInterval > 0 {
		cfg.RetryInterval = retryInterval
	}
	if loopInterval := vp.GetUint32("loop_interval"); loopInterval > 0 {
		cfg.LoopInterval = loopInterval
	}
	if batchBlockCount := vp.GetUint32("batch_block_count"); batchBlockCount > 0 {
		cfg.BatchBlockCount = batchBlockCount
	}
	if lag := vp.GetUint32("slow_lag_blocks"); lag > 0 {
		cfg.SlowLagBlocks = lag
	}
	if proofWindow := vp.GetUint32("proof_window"); proofWindow > 0 {
		cfg.ProofWindow = proofWindow
	}
	if delayBlocks := vp.GetUint32("delay_submit_trigger_blocks"); delayBlocks > 0 {
		cfg.DelaySubmitTriggerBlocks = delayBlocks
	}
	cfg.EnableMempoolIndexing = vp.GetBool("enable_mempool_indexing")
	if vp.IsSet("index_start_height") {
		cfg.IndexStartHeight = vp.GetUint32("index_start_height")
	}
	if startRewardHeight := vp.GetUint32("start_reward_height"); startRewardHeight > 0 {
		cfg.StartRewardHeight = startRewardHeight
	}
	cfg.StateAPIBaseURL = strings.TrimSpace(vp.GetString("state_api_base_url"))
	cfg.StateAPIAuth = strings.TrimSpace(vp.GetString("state_api_auth"))
	if timeout := vp.GetDuration("state_api_timeout"); timeout > 0 {
		cfg.StateAPITimeout = timeout
	}
	cfg.RewardClaimSenderAddressKeys = vp.GetStringSlice("reward_claim_sender_address_keys")

	if vp.IsSet("indexer_allowlist_windows") {
		windows, err := parseIndexerAllowlistWindows(vp.Get("indexer_allowlist_windows"))
		if err != nil {
			return cfg, err
		}
		cfg.IndexerAllowlistWindows = windows
	}

	if vp.IsSet("reward_release_tiers") {
		tiers, err := parseRewardReleaseTiers(vp.Get("reward_release_tiers"))
		if err != nil {
			return cfg, err
		}
		cfg.RewardReleaseTiers = tiers
	}

	return cfg, nil
}

func (c StakeRewardConfigInfo) IsIndexerAllowedAtHeight(indexerID string, height uint32) bool {
	indexerID = strings.TrimSpace(indexerID)
	if indexerID == "" {
		return false
	}

	matchedWindow := false
	for _, window := range c.IndexerAllowlistWindows {
		if height < window.StartHeight || height >= window.EndHeight {
			continue
		}
		matchedWindow = true
		if _, ok := window.indexerSet[indexerID]; ok {
			return true
		}
		for _, allowedID := range window.IndexerIDs {
			if allowedID == indexerID {
				return true
			}
		}
	}
	return !matchedWindow
}

func (c StakeRewardConfigInfo) RewardReleasePercentByHeight(height uint32) float64 {
	release := float64(0)
	for _, tier := range c.RewardReleaseTiers {
		if uint64(height) < tier.StartHeight {
			break
		}
		release = tier.ReleasePercent
	}
	return release
}

func parseIndexerAllowlistWindows(raw interface{}) ([]IndexerAllowlistWindow, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("indexer_allowlist_windows must be a list")
	}

	windows := make([]IndexerAllowlistWindow, 0, len(items))
	for i, item := range items {
		m, err := mapFromConfigObject(item, i, "indexer_allowlist_windows")
		if err != nil {
			return nil, err
		}
		startHeight, err := parseUint32Value(m["start_height"])
		if err != nil {
			return nil, fmt.Errorf("indexer_allowlist_windows[%d].start_height invalid: %w", i, err)
		}
		endHeight, err := parseUint32Value(m["end_height"])
		if err != nil {
			return nil, fmt.Errorf("indexer_allowlist_windows[%d].end_height invalid: %w", i, err)
		}
		if endHeight <= startHeight {
			return nil, fmt.Errorf("indexer_allowlist_windows[%d].end_height must be greater than start_height", i)
		}
		indexerIDs := normalizeStringSlice(interfaceStringSlice(m["indexer_ids"]))
		for _, indexerID := range indexerIDs {
			if !protocolparser.IsIndexerIDHeightTxIdx(indexerID) {
				return nil, fmt.Errorf("indexer_allowlist_windows[%d].indexer_ids contains invalid indexer_id %q", i, indexerID)
			}
		}
		windows = append(windows, IndexerAllowlistWindow{
			StartHeight: startHeight,
			EndHeight:   endHeight,
			IndexerIDs:  indexerIDs,
			indexerSet:  stringSet(indexerIDs),
		})
	}
	return windows, nil
}

func parseRewardReleaseTiers(raw interface{}) ([]RewardReleaseTier, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("reward_release_tiers must be a list")
	}
	tiers := make([]RewardReleaseTier, 0, len(items))
	for i, item := range items {
		m, err := mapFromConfigObject(item, i, "reward_release_tiers")
		if err != nil {
			return nil, err
		}
		startHeight, err := parseUint64Value(m["start_height"])
		if err != nil {
			return nil, fmt.Errorf("reward_release_tiers[%d].start_height invalid: %w", i, err)
		}
		releasePercent, err := parsePercentValue(m["release_percent"])
		if err != nil {
			return nil, fmt.Errorf("reward_release_tiers[%d].release_percent invalid: %w", i, err)
		}
		tiers = append(tiers, RewardReleaseTier{StartHeight: startHeight, ReleasePercent: releasePercent})
	}
	return tiers, nil
}

func mapFromConfigObject(item interface{}, index int, field string) (map[string]interface{}, error) {
	switch typed := item.(type) {
	case map[string]interface{}:
		return typed, nil
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			key, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] key must be a string", field, index)
			}
			m[key] = v
		}
		return m, nil
	default:
		return nil, fmt.Errorf("%s[%d] must be an object", field, index)
	}
}

func parseUint64Value(v interface{}) (uint64, error) {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || strings.EqualFold(s, "<nil>") {
		return 0, fmt.Errorf("empty value")
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func parseUint32Value(v interface{}) (uint32, error) {
	n, err := parseUint64Value(v)
	if err != nil {
		return 0, err
	}
	if n > uint64(^uint32(0)) {
		return 0, fmt.Errorf("value out of uint32 range")
	}
	return uint32(n), nil
}

func parsePercentValue(v interface{}) (float64, error) {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || strings.EqualFold(s, "<nil>") {
		return 0, fmt.Errorf("empty value")
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid float value")
	}
	if n < 0 || n > 100 {
		return 0, fmt.Errorf("value out of range [0,100]")
	}
	return n, nil
}

func interfaceStringSlice(raw interface{}) []string {
	switch typed := raw.(type) {
	case []string:
		return typed
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, fmt.Sprint(item))
		}
		return values
	default:
		return nil
	}
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
