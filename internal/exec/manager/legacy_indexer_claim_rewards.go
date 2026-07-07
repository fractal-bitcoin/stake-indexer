package indexer

import "strings"

// Legacy indexer rewards claimed before height 1919900 were paid to the
// indexer's owner address instead of the configured reward address, and the
// claim records were not attributed back to the corresponding indexer. This
// map records the affected claim transactions so reindexing can classify them
// correctly and the startup backfill can repair data written by older builds.
var legacyIndexerClaimRewardFixes = map[string]string{
	"a56daf10ee0ba7f121292806ff39907a281f8c6c2da9f970c0530180ad88d451": "1842346:1",
	"3177448b0aa35555ea8ded4b05e523ef95d6ff2513917702c3dae2b1e28b66cb": "1851552:554",
	"9daecd5dd863b60fc4d43ed89dc6832d7d883dbd3634a0172d0ad89004bf3384": "1818297:10",
	"7870256b744efef7df0fe926e059d7b0fa29e65916998607bca147f210ce93d0": "1818297:10",
	"2519eaa8966eec692055c8e9396261cc0bc1c671750d51f6e2f5e4a0972cb5fb": "1851552:554",
	"8e5cc8711dae66f89c9b74ccdd33536f26614284eae554c0c19b75d83a99ef14": "1850357:1",
	"9392a0029d0d665fdb939e28ede851878b17cee8d74f75ef8bfe696917a0887f": "1761438:1",
	"8e382333fde5ffdb5748c55f7e40e88d2cb1f4b845a6d398ead736eddd77a007": "1761438:1",
	"be4a92e7cad3b6c84ecc8aad2fec4d362d80b750e25b0d0feaa307420d28a872": "1761438:1",
}

func ResolveLegacyIndexerClaimReward(txid string) (string, bool) {
	indexerID, ok := legacyIndexerClaimRewardFixes[strings.TrimSpace(txid)]
	return indexerID, ok
}

func LegacyIndexerClaimRewardFixes() map[string]string {
	fixes := make(map[string]string, len(legacyIndexerClaimRewardFixes))
	for txid, indexerID := range legacyIndexerClaimRewardFixes {
		fixes[txid] = indexerID
	}
	return fixes
}
