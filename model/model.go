package model

import (
	"encoding/binary"
	scriptDecoder "stake_indexer/lib/script"

	"github.com/golang/snappy"
	"go.uber.org/zap/zapcore"
)

type Tx struct {
	Raw       []byte
	TxIdHex   string // 64
	TxId      []byte // 32
	Size      uint32
	WitOffset uint32
	LockTime  uint32
	Version   uint32
	TxInCnt   uint32
	TxOutCnt  uint32
	TxIns     []*TxIn
	TxOuts    []*TxOut
}

type TxIn struct {
	InputHashHex string // 32
	InputHash    []byte // 32
	InputVout    uint32
	ScriptSig    []byte
	Sequence     uint32

	ScriptWitness []byte

	InputOutpointKey string // 32 + 4
	InputOutpoint    []byte // 32 + 4
	InputPoint       []byte // 32 + 4
}

func (t *TxIn) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("t", t.InputHashHex)
	enc.AddUint32("i", t.InputVout)
	return nil
}

type TxOut struct {
	Satoshi  uint64
	PkScript []byte

	Outpoint    []byte // 32 + 4
	OutpointKey string // 32 + 4
	ScriptType  []byte

	AddressData *scriptDecoder.AddressData

	LockingScriptUnspendable bool
}

type TxWit struct {
	Script []byte
}

type Block struct {
	Raw        []byte
	Hash       []byte // 32 bytes
	HashHex    string
	Height     uint32
	Txs        []*Tx
	Version    uint32
	MerkleRoot []byte // 32 bytes
	BlockTime  uint32
	Bits       uint32
	Nonce      uint32
	Size       uint32
	TxCnt      uint32
	Parent     []byte // 32 bytes
	ParentHex  string
	ParseData  *ProcessBlock
}

type BlockIndex struct {
	Height    uint32
	HashHex   string
	ParentHex string
}

type BlockIndexInfo struct {
	Height     uint32
	HashHex    string
	TxCnt      uint32
	FileIdx    int
	FileOffset int64
}

// //////////////
type ProcessBlock struct {
	Height uint32

	SpentUtxoKeysMap map[string]struct{}
	SpentUtxoDataMap map[string]*TxoData
	NewUtxoDataMap   map[string]*TxoData
	// key: address string, value: net delta satoshi in this block
	AddressBalanceDeltaMap map[string]int64
}

type TxData struct {
	Raw  []byte
	TxId []byte // 32
}

type TxIdxPkScriptData struct {
	TxIdx    uint32
	PkScript []byte
}

type TxoData struct {
	BlockHeight uint32
	TxIdx       uint32
	Satoshi     uint64
	PkScript    []byte
}

func (d *TxoData) MakeMarshalBuf() (buf []byte) {
	size := 24 + len(d.PkScript)
	buf = make([]byte, size)
	return buf
}

func (d *TxoData) Marshal(buf []byte) (zbuf []byte, size int) {
	offset := 4
	buf[0] = 0
	buf[1] = 0
	buf[2] = 0
	buf[3] = 0

	binary.LittleEndian.PutUint32(buf[offset:], d.BlockHeight) // 4
	offset += 4

	binary.LittleEndian.PutUint32(buf[offset:], d.TxIdx) // 4
	offset += 4

	binary.LittleEndian.PutUint64(buf[offset:], d.Satoshi) // 8
	offset += 8

	scriptSize := uint32(len(d.PkScript))
	binary.LittleEndian.PutUint32(buf[offset:], scriptSize) // 4
	offset += 4

	copy(buf[offset:], d.PkScript)
	offset += int(scriptSize)

	return buf[:offset], offset
}

func (d *TxoData) Unmarshal(zbuf []byte) bool {
	if len(zbuf) < 4 {
		return false
	}

	zipflag := binary.LittleEndian.Uint32(zbuf[:4]) // 4
	offset := 4

	buf := zbuf
	if zipflag != 0 {
		var err error
		buf, err = snappy.Decode(nil, zbuf)
		if err != nil {
			return false
		}
	}

	if len(buf) < 4+20 {
		return false
	}

	d.BlockHeight = binary.LittleEndian.Uint32(buf[offset:]) // 4
	offset += 4

	d.TxIdx = binary.LittleEndian.Uint32(buf[offset:]) // 4
	offset += 4

	d.Satoshi = binary.LittleEndian.Uint64(buf[offset:]) // 8
	offset += 8

	scriptSize := binary.LittleEndian.Uint32(buf[offset:]) // 4
	offset += 4

	if len(buf) < 4+20+int(scriptSize) {
		return false
	}

	d.PkScript = make([]byte, scriptSize)
	copy(d.PkScript, buf[offset:offset+int(scriptSize)])
	offset += int(scriptSize)

	return true
}
