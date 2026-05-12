package protocolparser

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

func TestStakePubKeyMatchesInputAddressTaproot(t *testing.T) {
	pub := findPubKeyWithPrefix(t, 0x02)
	pubKeyHex := hex.EncodeToString(pub.SerializeCompressed())
	inputAddress := mustTaprootAddress(t, pub)

	if !StakePubKeyMatchesInputAddress(pubKeyHex, inputAddress, &chaincfg.MainNetParams) {
		t.Fatalf("expected taproot address to match")
	}
}

func TestStakePubKeyMatchesInputAddressP2WPKHCompressedAndUncompressed(t *testing.T) {
	pub := findPubKeyWithPrefix(t, 0x02)
	inputAddress := mustP2WPKHAddress(t, pub)

	compressedHex := hex.EncodeToString(pub.SerializeCompressed())
	if !StakePubKeyMatchesInputAddress(compressedHex, inputAddress, &chaincfg.MainNetParams) {
		t.Fatalf("expected compressed pubkey to match p2wpkh address")
	}

	uncompressedHex := hex.EncodeToString(pub.SerializeUncompressed())
	if !StakePubKeyMatchesInputAddress(uncompressedHex, inputAddress, &chaincfg.MainNetParams) {
		t.Fatalf("expected uncompressed pubkey to match p2wpkh address")
	}
}

func TestStakePubKeyMatchesInputAddressP2WPKHXOnlyOddParity(t *testing.T) {
	pub := findPubKeyWithPrefix(t, 0x03)
	inputAddress := mustP2WPKHAddress(t, pub)
	xOnlyHex := hex.EncodeToString(schnorr.SerializePubKey(pub))

	if !StakePubKeyMatchesInputAddress(xOnlyHex, inputAddress, &chaincfg.MainNetParams) {
		t.Fatalf("expected x-only pubkey to match odd-parity p2wpkh address")
	}
}

func TestStakePubKeyMatchesInputAddressRejectMismatch(t *testing.T) {
	pubA := findPubKeyWithPrefix(t, 0x02)
	pubB := findPubKeyWithPrefix(t, 0x03)
	inputAddress := mustP2WPKHAddress(t, pubA)
	pubKeyHexB := hex.EncodeToString(pubB.SerializeCompressed())

	if StakePubKeyMatchesInputAddress(pubKeyHexB, inputAddress, &chaincfg.MainNetParams) {
		t.Fatalf("expected mismatched pubkey and address to fail")
	}
}

func TestDeriveAddressFromPubKeyAndType(t *testing.T) {
	pub := findPubKeyWithPrefix(t, 0x02)
	pubKeyHex := hex.EncodeToString(pub.SerializeCompressed())

	gotTaproot, err := DeriveAddressFromPubKeyAndType(pubKeyHex, AddressTypeP2TR, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("derive taproot address failed: %v", err)
	}
	if gotTaproot != mustTaprootAddress(t, pub) {
		t.Fatalf("unexpected taproot address: got %s", gotTaproot)
	}

	gotP2WPKH, err := DeriveAddressFromPubKeyAndType(pubKeyHex, AddressTypeP2WPKH, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("derive p2wpkh address failed: %v", err)
	}
	if gotP2WPKH != mustP2WPKHAddress(t, pub) {
		t.Fatalf("unexpected p2wpkh address: got %s", gotP2WPKH)
	}
}

func TestDeriveAddressFromPubKeyAndType_CodeVariants(t *testing.T) {
	evenPub := findPubKeyWithPrefix(t, 0x02)
	oddPub := findPubKeyWithPrefix(t, 0x03)
	evenXOnlyHex := hex.EncodeToString(schnorr.SerializePubKey(evenPub))
	oddXOnlyHex := hex.EncodeToString(schnorr.SerializePubKey(oddPub))

	cases := []struct {
		name        string
		pubKeyHex   string
		addressType string
		expected    string
	}{
		{name: "p2tr-script-path", pubKeyHex: evenXOnlyHex, addressType: "1", expected: mustTaprootAddress(t, evenPub)},
		{name: "p2wpkh-even", pubKeyHex: evenXOnlyHex, addressType: "2", expected: mustP2WPKHAddress(t, evenPub)},
		{name: "p2wpkh-odd", pubKeyHex: oddXOnlyHex, addressType: "3", expected: mustP2WPKHAddress(t, oddPub)},
		{name: "p2pkh-even", pubKeyHex: evenXOnlyHex, addressType: "4", expected: mustP2PKHAddress(t, evenPub)},
		{name: "p2pkh-odd", pubKeyHex: oddXOnlyHex, addressType: "5", expected: mustP2PKHAddress(t, oddPub)},
		{name: "p2sh-p2wpkh-even", pubKeyHex: evenXOnlyHex, addressType: "6", expected: mustP2SHP2WPKHAddress(t, evenPub)},
		{name: "p2sh-p2wpkh-odd", pubKeyHex: oddXOnlyHex, addressType: "7", expected: mustP2SHP2WPKHAddress(t, oddPub)},
		{name: "p2tr-key-path", pubKeyHex: evenXOnlyHex, addressType: "8", expected: mustTaprootAddress(t, evenPub)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := DeriveAddressFromPubKeyAndType(tc.pubKeyHex, tc.addressType, &chaincfg.MainNetParams)
			if err != nil {
				t.Fatalf("derive address failed: %v", err)
			}
			if got != tc.expected {
				t.Fatalf("unexpected address: got %s want %s", got, tc.expected)
			}
		})
	}
}

func TestDeriveStakeAddress_AddressTypeCodeAffectsStakeAddress(t *testing.T) {
	pub := findPubKeyWithPrefix(t, 0x02)
	pubKeyHex := hex.EncodeToString(schnorr.SerializePubKey(pub))

	addr2, err := DeriveStakeAddress("1:2", pubKeyHex, "2", &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("derive stake address(2) failed: %v", err)
	}
	addr3, err := DeriveStakeAddress("1:2", pubKeyHex, "3", &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("derive stake address(3) failed: %v", err)
	}
	if addr2 == addr3 {
		t.Fatalf("stake address should differ across address type code")
	}
}

func findPubKeyWithPrefix(t *testing.T, prefix byte) *btcec.PublicKey {
	t.Helper()

	for i := uint32(1); i < 1<<16; i++ {
		var raw [32]byte
		raw[28] = byte(i >> 24)
		raw[29] = byte(i >> 16)
		raw[30] = byte(i >> 8)
		raw[31] = byte(i)
		_, pub := btcec.PrivKeyFromBytes(raw[:])
		if pub == nil {
			continue
		}
		if pub.SerializeCompressed()[0] == prefix {
			return pub
		}
	}

	t.Fatalf("failed to find pubkey with prefix %x", prefix)
	return nil
}

func mustP2WPKHAddress(t *testing.T, pub *btcec.PublicKey) string {
	t.Helper()

	hash := btcutil.Hash160(pub.SerializeCompressed())
	addr, err := btcutil.NewAddressWitnessPubKeyHash(hash, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("build p2wpkh address failed: %v", err)
	}
	return addr.EncodeAddress()
}

func mustP2PKHAddress(t *testing.T, pub *btcec.PublicKey) string {
	t.Helper()

	hash := btcutil.Hash160(pub.SerializeCompressed())
	addr, err := btcutil.NewAddressPubKeyHash(hash, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("build p2pkh address failed: %v", err)
	}
	return addr.EncodeAddress()
}

func mustP2SHP2WPKHAddress(t *testing.T, pub *btcec.PublicKey) string {
	t.Helper()

	redeemScript := make([]byte, 0, 22)
	redeemScript = append(redeemScript, txscript.OP_0, txscript.OP_DATA_20)
	redeemScript = append(redeemScript, btcutil.Hash160(pub.SerializeCompressed())...)
	addr, err := btcutil.NewAddressScriptHash(redeemScript, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("build p2sh-p2wpkh address failed: %v", err)
	}
	return addr.EncodeAddress()
}

func mustTaprootAddress(t *testing.T, pub *btcec.PublicKey) string {
	t.Helper()

	taprootKey := txscript.ComputeTaprootKeyNoScript(pub)
	addr, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(taprootKey), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("build taproot address failed: %v", err)
	}
	return addr.EncodeAddress()
}
