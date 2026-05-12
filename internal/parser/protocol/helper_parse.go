package protocolparser

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseRatio(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}

	percent := false
	if strings.HasSuffix(raw, "%") {
		percent = true
		raw = strings.TrimSuffix(raw, "%")
	}

	ratio, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, false
	}
	if percent || ratio > 1 {
		ratio = ratio / 100
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return ratio, true
}

func FormatRatio(ratio float64) string {
	return strconv.FormatFloat(ratio, 'f', 8, 64)
}

func ParseUint32(raw string) uint32 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(value)
}

func BuildIndexerID(height, txIdx uint32) string {
	return fmt.Sprintf("%d:%d", height, txIdx)
}

func IsIndexerIDHeightTxIdx(raw string) bool {
	parts := strings.SplitN(strings.TrimSpace(raw), ":", 2)
	if len(parts) != 2 {
		return false
	}
	if _, err := strconv.ParseUint(parts[0], 10, 32); err != nil {
		return false
	}
	if _, err := strconv.ParseUint(parts[1], 10, 32); err != nil {
		return false
	}
	return true
}
