package rollback

const indexerUndoRetentionDistance uint32 = 100

func MinIndexerUndoKeepHeight(latestHeight uint32) uint32 {
	if latestHeight <= indexerUndoRetentionDistance {
		return 0
	}
	return latestHeight - indexerUndoRetentionDistance
}

func ShouldPersistIndexerUndo(latestHeight, itemHeight uint32) bool {
	if itemHeight > latestHeight {
		return false
	}
	return latestHeight-itemHeight <= indexerUndoRetentionDistance
}
