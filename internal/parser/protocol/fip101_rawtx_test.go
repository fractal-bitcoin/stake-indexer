package protocolparser

import (
	"encoding/hex"
	"testing"

	parser "stake_indexer/internal/parser/node"
	"stake_indexer/utils"
)

const registerRawTxHex = "020000000001012777491d8bed660904bd37735335e509cd6b8d21be16024a4cac14b39553d6c30000000000ffffffff0122020000000000001600149fd82bb291a704052df25a6c9926f0ac97d2607f0340cb37f43fe33315439c3b07ed00e3f2a2a81a638b16124c092824d06d54239f0ac50f17bdbde2c07a92c9b4946a7db22f057b5fafd80b3adbee9bd880f5a345168020fe94274b1fbb5f25ff4d2730774d18ef897f0442234a74646c658f639a1d9410ac0063036f7264510474657874004c4e6669703130312c312c72656769737465722c313030302c626331716e6c767a6876353335757a713274306a74666b666a666873346a74617963726c6e327465746e2c7465746e2d696e64657865726821c150929b74c1a04954b78b4b6035e97a5e078a5a0f28ec96d547bfee9ace803ac000000000"

func TestParseProtocolTxFromModelTx_RegisterRawTx_LegacyHeaderRejected(t *testing.T) {
	rawTx, err := hex.DecodeString(registerRawTxHex)
	if err != nil {
		t.Fatalf("decode raw tx failed: %v", err)
	}

	tx, offset := parser.NewTx(rawTx)
	if tx == nil {
		t.Fatalf("parsed tx is nil")
	}
	if int(offset) != len(rawTx) {
		t.Fatalf("tx parse consumed unexpected bytes: offset=%d len=%d", offset, len(rawTx))
	}

	tx.Raw = rawTx
	if tx.WitOffset > 0 {
		tx.TxId = utils.GetWitnessHash256(tx.Raw, tx.WitOffset)
	} else {
		tx.TxId = utils.GetHash256(tx.Raw)
	}
	tx.TxIdHex = utils.HashString(tx.TxId)

	parsed, err := ParseProtocolTxFromModelTx(tx, 0, 100)
	if err != nil {
		t.Fatalf("parse protocol tx failed: %v", err)
	}
	if parsed != nil {
		t.Fatalf("expected legacy register tx to be rejected by strict header rule")
	}
}
