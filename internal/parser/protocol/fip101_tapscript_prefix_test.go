package protocolparser

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/txscript"
)

func TestExtractActorPubKeyAndAddressTypeFromTapScript_ValidPrefix(t *testing.T) {
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i + 1)
	}

	script, err := txscript.NewScriptBuilder().
		AddData(pub).
		AddOp(txscript.OP_CHECKSIGVERIFY).
		AddOp(txscript.OP_6).
		AddOp(txscript.OP_FALSE).
		AddOp(txscript.OP_IF).
		AddData([]byte("ord")).
		AddOp(txscript.OP_ENDIF).
		Script()
	if err != nil {
		t.Fatalf("build script failed: %v", err)
	}

	gotPub, gotType, ok := extractActorPubKeyAndAddressTypeFromTapScript(script)
	if !ok {
		t.Fatalf("expected valid script prefix")
	}
	if !bytes.Equal(gotPub, pub) {
		t.Fatalf("unexpected pubkey")
	}
	if gotType != "6" {
		t.Fatalf("unexpected address type: %s", gotType)
	}
}

func TestExtractActorPubKeyAndAddressTypeFromTapScript_RejectLegacyChecksig(t *testing.T) {
	pub := make([]byte, 32)
	script, err := txscript.NewScriptBuilder().
		AddData(pub).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_1).
		Script()
	if err != nil {
		t.Fatalf("build script failed: %v", err)
	}

	if _, _, ok := extractActorPubKeyAndAddressTypeFromTapScript(script); ok {
		t.Fatalf("expected legacy CHECKSIG prefix to be rejected")
	}
}

func TestExtractActorPubKeyAndAddressTypeFromTapScript_RejectPushData1(t *testing.T) {
	pub := make([]byte, 32)
	script := make([]byte, 0, 36)
	script = append(script, txscript.OP_DATA_32)
	script = append(script, pub...)
	script = append(script, txscript.OP_CHECKSIGVERIFY)
	script = append(script, txscript.OP_DATA_1, 0x01)

	if _, _, ok := extractActorPubKeyAndAddressTypeFromTapScript(script); ok {
		t.Fatalf("expected PUSHDATA(0x01) prefix to be rejected")
	}
}

func TestExtractActorPubKeyAndAddressTypeFromTapScript_RejectOutOfRangeType(t *testing.T) {
	pub := make([]byte, 32)
	script, err := txscript.NewScriptBuilder().
		AddData(pub).
		AddOp(txscript.OP_CHECKSIGVERIFY).
		AddOp(txscript.OP_9).
		Script()
	if err != nil {
		t.Fatalf("build script failed: %v", err)
	}

	if _, _, ok := extractActorPubKeyAndAddressTypeFromTapScript(script); ok {
		t.Fatalf("expected OP_9 prefix to be rejected")
	}
}
