package protocolparser

import (
	"strconv"
	"strings"
)

const (
	AddressTypeP2TR       = "P2TR"
	AddressTypeP2WPKH     = "P2WPKH"
	AddressTypeP2PKH      = "P2PKH"
	AddressTypeP2SHP2WPKH = "P2SH-P2WPKH"
)

const (
	addressTypeCodeP2TRScriptPath = uint64(1)
	addressTypeCodeP2WPKHEven     = uint64(2)
	addressTypeCodeP2WPKHOdd      = uint64(3)
	addressTypeCodeP2PKHEven      = uint64(4)
	addressTypeCodeP2PKHOdd       = uint64(5)
	addressTypeCodeP2SHP2WPKHEven = uint64(6)
	addressTypeCodeP2SHP2WPKHOdd  = uint64(7)
	addressTypeCodeP2TRKeyPath    = uint64(8)
)

func ParseAddressTypeCode(raw string) (uint64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if code, err := strconv.ParseUint(raw, 10, 64); err == nil {
		if code >= 1 && code <= 8 {
			return code, true
		}
		return 0, false
	}

	switch strings.ToUpper(raw) {
	case AddressTypeP2TR:
		return addressTypeCodeP2TRScriptPath, true
	case AddressTypeP2WPKH:
		return addressTypeCodeP2WPKHEven, true
	case AddressTypeP2PKH:
		return addressTypeCodeP2PKHEven, true
	case AddressTypeP2SHP2WPKH:
		return addressTypeCodeP2SHP2WPKHEven, true
	default:
		return 0, false
	}
}

func AddressTypeFromCode(code uint64) (string, bool) {
	switch code {
	case addressTypeCodeP2TRScriptPath, addressTypeCodeP2TRKeyPath:
		return AddressTypeP2TR, true
	case addressTypeCodeP2WPKHEven, addressTypeCodeP2WPKHOdd:
		return AddressTypeP2WPKH, true
	case addressTypeCodeP2PKHEven, addressTypeCodeP2PKHOdd:
		return AddressTypeP2PKH, true
	case addressTypeCodeP2SHP2WPKHEven, addressTypeCodeP2SHP2WPKHOdd:
		return AddressTypeP2SHP2WPKH, true
	default:
		return "", false
	}
}

func ParseAddressType(raw string) (string, bool) {
	code, ok := ParseAddressTypeCode(raw)
	if !ok {
		return "", false
	}
	return AddressTypeFromCode(code)
}
