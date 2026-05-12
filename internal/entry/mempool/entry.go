package mempool

type Syncer interface {
	SyncMempoolProtocolTxs(tipHeight uint32) (ProtocolSyncStats, error)
}

func Sync(s Syncer, tipHeight uint32) (ProtocolSyncStats, error) {
	if s == nil {
		return ProtocolSyncStats{}, nil
	}
	return s.SyncMempoolProtocolTxs(tipHeight)
}
