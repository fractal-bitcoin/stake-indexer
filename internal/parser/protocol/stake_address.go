package protocolparser

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

const taprootOnlyScriptPubKeyHex = "50929b74c1a04954b78b4b6035e97a5e078a5a0f28ec96d547bfee9ace803ac0"

type normalizedStakePubKey struct {
	XOnly      []byte
	Compressed []byte
	Raw        []byte
	Parsed     *btcec.PublicKey
}

func compressedPubKeyByCode(key *normalizedStakePubKey, code uint64) ([]byte, error) {
	if key == nil || len(key.XOnly) != 32 {
		return nil, fmt.Errorf("invalid pubkey")
	}
	prefix := byte(0x02)
	switch code {
	case addressTypeCodeP2WPKHEven, addressTypeCodeP2PKHEven, addressTypeCodeP2SHP2WPKHEven:
		prefix = 0x02
	case addressTypeCodeP2WPKHOdd, addressTypeCodeP2PKHOdd, addressTypeCodeP2SHP2WPKHOdd:
		prefix = 0x03
	default:
		return nil, fmt.Errorf("invalid parity code")
	}
	out := make([]byte, 33)
	out[0] = prefix
	copy(out[1:], key.XOnly)
	return out, nil
}

func DeriveAddressFromPubKeyAndType(pubKeyHex, addressType string, params *chaincfg.Params) (string, error) {
	if params == nil {
		params = &chaincfg.MainNetParams
	}
	code, ok := ParseAddressTypeCode(addressType)
	if !ok {
		return "", fmt.Errorf("unsupported address type %q", addressType)
	}

	key, err := normalizeStakePubKey(pubKeyHex)
	if err != nil {
		return "", err
	}

	switch code {
	case addressTypeCodeP2TRScriptPath, addressTypeCodeP2TRKeyPath:
		taprootOutputKey := txscript.ComputeTaprootKeyNoScript(key.Parsed)
		if taprootOutputKey == nil {
			return "", fmt.Errorf("derive taproot key failed")
		}
		addr, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(taprootOutputKey), params)
		if err != nil {
			return "", fmt.Errorf("build taproot address failed: %w", err)
		}
		return addr.EncodeAddress(), nil
	case addressTypeCodeP2WPKHEven, addressTypeCodeP2WPKHOdd:
		compressed, err := compressedPubKeyByCode(key, code)
		if err != nil {
			return "", err
		}
		addr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(compressed), params)
		if err != nil {
			return "", fmt.Errorf("build p2wpkh address failed: %w", err)
		}
		return addr.EncodeAddress(), nil
	case addressTypeCodeP2PKHEven, addressTypeCodeP2PKHOdd:
		compressed, err := compressedPubKeyByCode(key, code)
		if err != nil {
			return "", err
		}
		addr, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(compressed), params)
		if err != nil {
			return "", fmt.Errorf("build p2pkh address failed: %w", err)
		}
		return addr.EncodeAddress(), nil
	case addressTypeCodeP2SHP2WPKHEven, addressTypeCodeP2SHP2WPKHOdd:
		compressed, err := compressedPubKeyByCode(key, code)
		if err != nil {
			return "", err
		}
		redeemScript := make([]byte, 0, 22)
		redeemScript = append(redeemScript, txscript.OP_0, txscript.OP_DATA_20)
		redeemScript = append(redeemScript, btcutil.Hash160(compressed)...)
		addr, err := btcutil.NewAddressScriptHash(redeemScript, params)
		if err != nil {
			return "", fmt.Errorf("build p2sh-p2wpkh address failed: %w", err)
		}
		return addr.EncodeAddress(), nil
	default:
		return "", fmt.Errorf("unsupported address type code %d", code)
	}
}

func DeriveStakeAddress(indexerID, pubKeyHex, addressType string, params *chaincfg.Params) (string, error) {
	indexerID = strings.TrimSpace(indexerID)
	if indexerID == "" {
		return "", fmt.Errorf("empty indexer id")
	}
	if params == nil {
		params = &chaincfg.MainNetParams
	}

	code, ok := ParseAddressTypeCode(addressType)
	if !ok {
		return "", fmt.Errorf("unsupported address type %q", addressType)
	}

	key, err := normalizeStakePubKey(pubKeyHex)
	if err != nil {
		return "", err
	}

	internalKey, err := parseTaprootOnlyInternalKey()
	if err != nil {
		return "", err
	}

	leafScript, err := txscript.NewScriptBuilder().
		AddData([]byte(indexerID)).
		AddOp(txscript.OP_DROP).
		AddData([]byte(strconv.FormatUint(code, 10))).
		AddOp(txscript.OP_DROP).
		AddData(key.XOnly).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	if err != nil {
		return "", fmt.Errorf("build stake leaf script failed: %w", err)
	}

	leaf := txscript.NewBaseTapLeaf(leafScript)
	leafHash := leaf.TapHash()
	taprootOutputKey := txscript.ComputeTaprootOutputKey(internalKey, leafHash[:])

	addr, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(taprootOutputKey), params)
	if err != nil {
		return "", fmt.Errorf("build stake address failed: %w", err)
	}
	return addr.EncodeAddress(), nil
}

func StakePubKeyMatchesInputAddress(pubKeyHex, inputAddress string, params *chaincfg.Params) bool {
	inputAddress = strings.TrimSpace(inputAddress)
	if inputAddress == "" {
		return false
	}
	if params == nil {
		params = &chaincfg.MainNetParams
	}

	decodedAddr, err := btcutil.DecodeAddress(inputAddress, params)
	if err != nil {
		return false
	}

	key, err := normalizeStakePubKey(pubKeyHex)
	if err != nil {
		return false
	}

	switch addr := decodedAddr.(type) {
	case *btcutil.AddressTaproot:
		taprootKeyNoScript := txscript.ComputeTaprootKeyNoScript(key.Parsed)
		if taprootKeyNoScript == nil {
			return false
		}
		expectedAddr, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(taprootKeyNoScript), params)
		if err != nil {
			return false
		}
		return strings.EqualFold(expectedAddr.EncodeAddress(), inputAddress)
	case *btcutil.AddressWitnessPubKeyHash:
		target := addr.Hash160()
		for _, candidate := range witnessPubKeyHashCandidates(key) {
			if bytes.Equal(candidate, target[:]) {
				return true
			}
		}
		return false
	case *btcutil.AddressPubKeyHash:
		target := addr.Hash160()
		for _, candidate := range witnessPubKeyHashCandidates(key) {
			if bytes.Equal(candidate, target[:]) {
				return true
			}
		}
		return false
	case *btcutil.AddressScriptHash:
		target := addr.ScriptAddress()
		for _, candidate := range witnessPubKeyHashCandidates(key) {
			redeemScript := make([]byte, 0, 22)
			redeemScript = append(redeemScript, txscript.OP_0, txscript.OP_DATA_20)
			redeemScript = append(redeemScript, candidate...)
			if bytes.Equal(target, btcutil.Hash160(redeemScript)) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func parseTaprootOnlyInternalKey() (*btcec.PublicKey, error) {
	raw, err := hex.DecodeString(taprootOnlyScriptPubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decode internal pubkey failed: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("invalid internal pubkey length %d", len(raw))
	}
	key, err := schnorr.ParsePubKey(raw)
	if err != nil {
		return nil, fmt.Errorf("parse internal pubkey failed: %w", err)
	}
	return key, nil
}

func normalizeStakePubKey(pubKeyHex string) (*normalizedStakePubKey, error) {
	pubKeyHex = strings.TrimSpace(pubKeyHex)
	pubKeyHex = strings.TrimPrefix(strings.TrimPrefix(pubKeyHex, "0x"), "0X")
	if pubKeyHex == "" {
		return nil, fmt.Errorf("empty pubkey")
	}

	raw, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey hex: %w", err)
	}

	switch len(raw) {
	case 32:
		parsed, err := schnorr.ParsePubKey(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid x-only pubkey: %w", err)
		}
		return &normalizedStakePubKey{XOnly: append([]byte(nil), raw...), Compressed: parsed.SerializeCompressed(), Raw: append([]byte(nil), raw...), Parsed: parsed}, nil
	case 33, 65:
		parsed, err := btcec.ParsePubKey(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid pubkey: %w", err)
		}
		return &normalizedStakePubKey{XOnly: schnorr.SerializePubKey(parsed), Compressed: parsed.SerializeCompressed(), Raw: append([]byte(nil), raw...), Parsed: parsed}, nil
	default:
		return nil, fmt.Errorf("unsupported pubkey length %d", len(raw))
	}
}

func witnessPubKeyHashCandidates(key *normalizedStakePubKey) [][]byte {
	if key == nil || len(key.Compressed) == 0 {
		return nil
	}

	candidates := [][]byte{btcutil.Hash160(key.Compressed)}
	if len(key.Raw) == 32 {
		oddCompressed := make([]byte, 33)
		oddCompressed[0] = 0x03
		copy(oddCompressed[1:], key.Raw)
		if oddParsed, err := btcec.ParsePubKey(oddCompressed); err == nil {
			oddHash := btcutil.Hash160(oddParsed.SerializeCompressed())
			if !bytes.Equal(oddHash, candidates[0]) {
				candidates = append(candidates, oddHash)
			}
		}
	}

	return candidates
}
