package protocolparser

import (
	scriptDecoder "stake_indexer/lib/script"
	"stake_indexer/utils"

	"github.com/btcsuite/btcd/chaincfg"
)

const (
	TagRegister      = "FIP_101_REGISTER"
	TagAllocatRatio  = "FIP_101_ALLOCAT_RATIO"
	TagStake         = "FIP_101_STAKE"
	TagProveStake    = "FIP_101_PROVE_STAKE"
	TagPledgedReward = "FIP_101_PLEDEGED_REWARD"
)

type BlockSnapshot struct {
	Height         uint32
	HashHex        string
	Version        uint32
	CoinbaseReward uint64
	Txs            []TxSnapshot
}

type TxSnapshot struct {
	TxID    string
	TxIdx   uint32
	Inputs  []InputSnapshot
	Outputs []OutputSnapshot
}

type InputSnapshot struct {
	InputIdx    uint32
	OutpointKey string
	AddressKey  string
	Satoshi     uint64
}

type OutputSnapshot struct {
	OutputIdx  uint32
	Satoshi    uint64
	AddressKey string
	PkScript   []byte
}

func AddressFromPkScript(pkScript []byte) string {
	if len(pkScript) == 0 {
		return ""
	}
	addr, err := utils.GetAddressFromScript(pkScript, &chaincfg.MainNetParams)
	if err != nil {
		return ""
	}
	return addr
}

func AddressFromCodeTypeHash(codeType uint32, hash160 []byte) string {
	if len(hash160) != 20 {
		return ""
	}

	switch codeType {
	case scriptDecoder.CodeType_P2PKH:
		pkScript := make([]byte, 0, 25)
		pkScript = append(pkScript, scriptDecoder.OP_DUP, scriptDecoder.OP_HASH160, scriptDecoder.OP_DATA_20)
		pkScript = append(pkScript, hash160...)
		pkScript = append(pkScript, scriptDecoder.OP_EQUALVERIFY, scriptDecoder.OP_CHECKSIG)
		return AddressFromPkScript(pkScript)
	case scriptDecoder.CodeType_P2WPKH:
		pkScript := make([]byte, 0, 22)
		pkScript = append(pkScript, scriptDecoder.OP_0, scriptDecoder.OP_DATA_20)
		pkScript = append(pkScript, hash160...)
		return AddressFromPkScript(pkScript)
	}
	return ""
}
