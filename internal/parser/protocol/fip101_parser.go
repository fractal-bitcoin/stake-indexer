package protocolparser

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	pgdb "stake_indexer/internal/component/pg"
	scriptDecoder "stake_indexer/lib/script"
	"stake_indexer/model"
	"stake_indexer/utils"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

const (
	fip101ContentType = "text/plain;charset=utf-8"
	fip101Protocol    = "fip101"
	fip101Version     = "1"
	fip101ClaimReward = "FIP-101:claim_reward"
	MaxIndexerNameLen = 64
)

var indexerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)

type ParsedProtocolTx struct {
	Snapshot TxSnapshot
	Payload  *OpReturnPayload
	Event    *pgdb.FIP101InscriptionEvent
}

func ParseProtocolTxFromModelTx(tx *model.Tx, txIdx uint32, blockHeight int64) (*ParsedProtocolTx, error) {
	if tx == nil || tx.TxIdHex == "" {
		return nil, nil
	}

	txSnapshot := TxSnapshot{
		TxID:    tx.TxIdHex,
		TxIdx:   txIdx,
		Inputs:  make([]InputSnapshot, 0, len(tx.TxIns)),
		Outputs: make([]OutputSnapshot, 0, len(tx.TxOuts)),
	}
	for inIdx, input := range tx.TxIns {
		if input == nil {
			continue
		}
		txSnapshot.Inputs = append(txSnapshot.Inputs, InputSnapshot{
			InputIdx:    uint32(inIdx),
			OutpointKey: input.InputOutpointKey,
		})
	}
	for outIdx, output := range tx.TxOuts {
		if output == nil {
			continue
		}
		address := AddressFromPkScript(output.PkScript)
		txSnapshot.Outputs = append(txSnapshot.Outputs, OutputSnapshot{
			OutputIdx:  uint32(outIdx),
			Satoshi:    output.Satoshi,
			AddressKey: strings.TrimSpace(address),
			PkScript:   append([]byte(nil), output.PkScript...),
		})
	}

	if payload, content, ok := parseFIP101ClaimRewardFromOutputs(&txSnapshot); ok {
		event := &pgdb.FIP101InscriptionEvent{
			TxID:               tx.TxIdHex,
			Op:                 payloadTagToOperation(payload.Tag),
			Height:             blockHeight,
			InscriptionContent: content,
			BizInvalidFlags:    0,
			TxIdx:              txIdx,
		}
		populateInscriptionEvent(event, payload, "", &txSnapshot)
		return &ParsedProtocolTx{
			Snapshot: txSnapshot,
			Payload:  payload,
			Event:    event,
		}, nil
	}

	payload, actorAddress, bodyCBOR, err := parseStrictFIP101InscriptionFromTx(tx)
	if err != nil || payload == nil {
		return nil, err
	}

	event := &pgdb.FIP101InscriptionEvent{
		TxID:               tx.TxIdHex,
		Op:                 payloadTagToOperation(payload.Tag),
		Height:             blockHeight,
		InscriptionContent: string(bodyCBOR),
		BizInvalidFlags:    0,
		TxIdx:              txIdx,
	}
	populateInscriptionEvent(event, payload, actorAddress, &txSnapshot)

	return &ParsedProtocolTx{
		Snapshot: txSnapshot,
		Payload:  payload,
		Event:    event,
	}, nil
}

func populateInscriptionEvent(event *pgdb.FIP101InscriptionEvent, payload *OpReturnPayload, actorAddress string, tx *TxSnapshot) {
	if event == nil || payload == nil {
		return
	}

	event.IndexerID = strings.TrimSpace(payload.Get(OpFieldIndexerID))
	event.UserAddress = strings.TrimSpace(actorAddress)
	event.RewardAddress = strings.TrimSpace(payload.Get(OpFieldRewardAddr))
	event.IndexerName = strings.TrimSpace(payload.Get(OpFieldIndexerName))
	event.ProveBlockHeight = ParseUint32(payload.Get(OpFieldBlockHeight))
	event.ProveDataHash = strings.TrimSpace(payload.Get(OpFieldBlockHash))
	if ratio, ok := ParseRatio(payload.Get(OpFieldIndexRatio)); ok {
		event.IndexRatio = ratio
	}

	switch payload.Tag {
	case TagStake:
		out, ok := firstNonOpReturnOutputFromSnapshot(tx)
		if ok {
			event.StakeAddress = strings.TrimSpace(out.AddressKey)
			event.Amount = out.Satoshi
		}
	case TagPledgedReward:
		out, ok := firstSpendableOutputFromSnapshot(tx)
		if ok {
			event.UserAddress = strings.TrimSpace(out.AddressKey)
			event.Amount = out.Satoshi
		}
	}
}

func firstSpendableOutputFromSnapshot(tx *TxSnapshot) (*OutputSnapshot, bool) {
	if tx == nil {
		return nil, false
	}
	for i := range tx.Outputs {
		if strings.TrimSpace(tx.Outputs[i].AddressKey) == "" {
			continue
		}
		return &tx.Outputs[i], true
	}
	return nil, false
}

func firstNonOpReturnOutputFromSnapshot(tx *TxSnapshot) (*OutputSnapshot, bool) {
	if tx == nil {
		return nil, false
	}
	for i := range tx.Outputs {
		out := &tx.Outputs[i]
		if len(out.PkScript) > 0 && out.PkScript[0] == txscript.OP_RETURN {
			continue
		}
		if strings.TrimSpace(out.AddressKey) == "" {
			continue
		}
		return out, true
	}
	return nil, false
}

func parseFIP101ClaimRewardFromOutputs(tx *TxSnapshot) (*OpReturnPayload, string, bool) {
	if tx == nil {
		return nil, "", false
	}
	for i := range tx.Outputs {
		content, ok := opReturnSinglePushText(tx.Outputs[i].PkScript)
		if !ok {
			continue
		}
		payload, ok := parseClaimRewardOpReturnContent(content)
		if !ok {
			continue
		}
		return payload, content, true
	}
	return nil, "", false
}

func parseClaimRewardOpReturnContent(content string) (*OpReturnPayload, bool) {
	content = strings.TrimSpace(content)
	if content == fip101ClaimReward {
		return &OpReturnPayload{Tag: TagPledgedReward, Fields: make(map[string]string)}, true
	}
	if !strings.HasPrefix(content, fip101ClaimReward) {
		return nil, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(content, fip101ClaimReward))
	if rest == "" {
		return &OpReturnPayload{Tag: TagPledgedReward, Fields: make(map[string]string)}, true
	}
	if strings.HasPrefix(rest, ",") || strings.HasPrefix(rest, ":") {
		rest = strings.TrimSpace(rest[1:])
	}
	if rest == "" {
		return &OpReturnPayload{Tag: TagPledgedReward, Fields: make(map[string]string)}, true
	}
	if rest == OpValueEarlySupporterReward {
		return &OpReturnPayload{
			Tag:    TagPledgedReward,
			Fields: map[string]string{OpFieldRewardClaimType: OpValueEarlySupporterReward},
		}, true
	}
	if strings.HasPrefix(rest, OpValueEarlySupporterReward+":") {
		indexerID := strings.TrimSpace(strings.TrimPrefix(rest, OpValueEarlySupporterReward+":"))
		if !IsIndexerIDHeightTxIdx(indexerID) {
			return nil, false
		}
		return &OpReturnPayload{
			Tag: TagPledgedReward,
			Fields: map[string]string{
				OpFieldRewardClaimType: OpValueEarlySupporterReward,
				OpFieldIndexerID:       indexerID,
			},
		}, true
	}
	fields := strings.FieldsFunc(rest, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) == 0 {
		return &OpReturnPayload{Tag: TagPledgedReward, Fields: make(map[string]string)}, true
	}
	indexerID := strings.TrimSpace(fields[0])
	if !IsIndexerIDHeightTxIdx(indexerID) {
		return nil, false
	}
	return &OpReturnPayload{
		Tag:    TagPledgedReward,
		Fields: map[string]string{OpFieldIndexerID: indexerID},
	}, true
}

func opReturnSinglePushText(pkScript []byte) (string, bool) {
	offset := 0
	if len(pkScript) > 1 && pkScript[0] == txscript.OP_FALSE && pkScript[1] == txscript.OP_RETURN {
		offset = 2
	} else if len(pkScript) > 0 && pkScript[0] == txscript.OP_RETURN {
		offset = 1
	} else {
		return "", false
	}
	if offset >= len(pkScript) {
		return "", false
	}

	data, next, ok := readScriptPushData(pkScript, offset)
	if !ok || next != len(pkScript) {
		return "", false
	}
	return string(data), true
}

func readScriptPushData(script []byte, offset int) ([]byte, int, bool) {
	if offset < 0 || offset >= len(script) {
		return nil, offset, false
	}
	op := script[offset]
	offset++

	var size uint64
	switch {
	case op >= 0x01 && op <= 0x4b:
		size = uint64(op)
	case op == txscript.OP_PUSHDATA1:
		if offset >= len(script) {
			return nil, offset, false
		}
		size = uint64(script[offset])
		offset++
	case op == txscript.OP_PUSHDATA2:
		if offset+2 > len(script) {
			return nil, offset, false
		}
		size = uint64(script[offset]) | uint64(script[offset+1])<<8
		offset += 2
	case op == txscript.OP_PUSHDATA4:
		if offset+4 > len(script) {
			return nil, offset, false
		}
		size = uint64(script[offset]) | uint64(script[offset+1])<<8 | uint64(script[offset+2])<<16 | uint64(script[offset+3])<<24
		offset += 4
	default:
		return nil, offset, false
	}

	if size > uint64(len(script)-offset) {
		return nil, offset, false
	}
	end := offset + int(size)
	return script[offset:end], end, true
}

func parseStrictFIP101InscriptionFromTx(tx *model.Tx) (*OpReturnPayload, string, []byte, error) {
	if tx == nil || len(tx.TxIns) == 0 {
		return nil, "", nil, nil
	}

	type inscriptionMatch struct {
		payload      *OpReturnPayload
		actorAddress string
		actorPubKey  []byte
		bodyCBOR     []byte
	}

	var hit *inscriptionMatch
	for _, in := range tx.TxIns {
		if in == nil || len(in.ScriptWitness) == 0 {
			continue
		}

		wits, _ := utils.NewTxWit(in.ScriptWitness)
		if len(wits) < 2 {
			continue
		}
		tapScript := wits[len(wits)-2].Script
		if len(tapScript) == 0 {
			continue
		}

		nfts := scriptDecoder.ExtractPkScriptForNFTJubilee(tapScript)
		if len(nfts) != 1 {
			continue
		}
		nft := nfts[0]
		if nft == nil {
			continue
		}
		if nft.IsUnrecognizedEven || nft.IsDuplicateField || nft.IsIncompleteField {
			continue
		}
		if len(nft.ContentBody) == 0 {
			continue
		}
		if !bytes.Equal(bytes.TrimSpace(nft.ContentType), []byte(fip101ContentType)) {
			continue
		}

		actorPubKey, actorAddressType, ok := extractActorPubKeyAndAddressTypeFromTapScript(tapScript)
		if !ok {
			continue
		}
		actorAddress, err := DeriveAddressFromPubKeyAndType(hex.EncodeToString(actorPubKey), actorAddressType, nil)
		if err != nil || actorAddress == "" {
			continue
		}

		payload, _, err := ParseFIP101PayloadFromCSV(nft.ContentBody, actorPubKey, actorAddress, actorAddressType)
		if err != nil || payload == nil {
			continue
		}

		if hit != nil {
			return nil, "", nil, nil
		}
		hit = &inscriptionMatch{
			payload:      payload,
			actorAddress: actorAddress,
			actorPubKey:  append([]byte(nil), actorPubKey...),
			bodyCBOR:     append([]byte(nil), nft.ContentBody...),
		}
	}

	if hit == nil {
		return nil, "", nil, nil
	}
	return hit.payload, hit.actorAddress, hit.bodyCBOR, nil
}

func extractActorPubKeyAndAddressTypeFromTapScript(tapScript []byte) ([]byte, string, bool) {
	if len(tapScript) == 0 {
		return nil, "", false
	}

	size, firstData, isPush, isOpcode := scriptDecoder.GetOpcodeFormScript(tapScript)
	if !isPush || isOpcode || size == 0 || len(firstData) != 32 {
		return nil, "", false
	}
	offset := int(size)
	if offset >= len(tapScript) {
		return nil, "", false
	}

	size, opData, isPush, isOpcode := scriptDecoder.GetOpcodeFormScript(tapScript[offset:])
	if isPush || !isOpcode || size == 0 || len(opData) != 1 || opData[0] != scriptDecoder.OP_CHECKSIGVERIFY {
		return nil, "", false
	}
	offset += int(size)
	if offset >= len(tapScript) {
		return nil, "", false
	}

	size, typeData, isPush, isOpcode := scriptDecoder.GetOpcodeFormScript(tapScript[offset:])
	if !isPush || !isOpcode || size != 1 || len(typeData) != 1 || typeData[0] < 1 || typeData[0] > 8 {
		return nil, "", false
	}

	out := make([]byte, 32)
	copy(out, firstData)
	return out, strconv.FormatUint(uint64(typeData[0]), 10), true
}

func ParseFIP101PayloadFromCSV(raw []byte, actorPubKey []byte, actorAddress string, actorAddressType string) (*OpReturnPayload, json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, nil, fmt.Errorf("invalid utf8 csv body")
	}
	record, err := parseSingleCSVRecord(string(raw))
	if err != nil {
		return nil, nil, err
	}
	if len(record) < 3 {
		return nil, nil, fmt.Errorf("invalid csv columns")
	}
	if !strings.EqualFold(strings.TrimSpace(record[0]), fip101Protocol) {
		return nil, nil, fmt.Errorf("invalid protocol")
	}
	if strings.TrimSpace(record[1]) != fip101Version {
		return nil, nil, fmt.Errorf("invalid protocol version")
	}
	opRaw := strings.ToLower(strings.TrimSpace(record[2]))
	tag := mapOperationToTag(opRaw)

	payload := &OpReturnPayload{
		Tag:    tag,
		Fields: make(map[string]string, 10),
	}
	if payload.Tag == "" {
		return nil, nil, fmt.Errorf("unsupported op")
	}

	actorHex := hex.EncodeToString(actorPubKey)
	payload.Fields[OpFieldActorPubKey] = actorHex
	payload.Fields[OpFieldActorAddr] = strings.TrimSpace(actorAddress)
	payload.Fields[OpFieldPubKey] = actorHex
	payload.Fields[OpFieldAddressType] = strings.TrimSpace(actorAddressType)

	data := make(map[string]interface{})
	switch payload.Tag {
	case TagRegister:
		if len(record) != 6 {
			return nil, nil, fmt.Errorf("invalid register schema")
		}
		ratioBP, ok := parseCSVUint64(record[3])
		if !ok || ratioBP > 10000 {
			return nil, nil, fmt.Errorf("invalid register ratio")
		}
		rewardAddr := strings.TrimSpace(record[4])
		if rewardAddr == "" {
			return nil, nil, fmt.Errorf("invalid reward addr")
		}
		if err := validateRewardAddressAndNetwork(rewardAddr); err != nil {
			return nil, nil, err
		}
		name := TruncateRunes(strings.TrimSpace(record[5]), MaxIndexerNameLen)
		if !IsValidIndexerName(name) {
			return nil, nil, fmt.Errorf("invalid register name")
		}
		payload.Fields[OpFieldIndexerName] = name
		payload.Fields[OpFieldIndexRatio] = ratioBPToString(ratioBP)
		payload.Fields[OpFieldRewardAddr] = rewardAddr
		data["name"] = name
		data["index_ratio_bp"] = ratioBP
		data["reward_addr"] = rewardAddr
	case TagAllocatRatio:
		if len(record) != 5 {
			return nil, nil, fmt.Errorf("invalid ratio schema")
		}
		indexerID := strings.TrimSpace(record[3])
		if indexerID == "" {
			return nil, nil, fmt.Errorf("missing indexer_id")
		}
		ratioBP, ok := parseCSVUint64(record[4])
		if !ok || ratioBP > 10000 {
			return nil, nil, fmt.Errorf("invalid ratio")
		}
		payload.Fields[OpFieldIndexerID] = indexerID
		payload.Fields[OpFieldIndexRatio] = ratioBPToString(ratioBP)
		data["indexer_id"] = indexerID
		data["index_ratio_bp"] = ratioBP
	case TagProveStake:
		if len(record) != 6 {
			return nil, nil, fmt.Errorf("invalid prove schema")
		}
		indexerID := strings.TrimSpace(record[3])
		proveHeight, ok := parseCSVUint64(record[4])
		proveHash := strings.TrimSpace(record[5])
		if indexerID == "" || !ok || proveHash == "" {
			return nil, nil, fmt.Errorf("invalid prove payload")
		}
		proveHashRaw, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(proveHash), "0x"))
		if err != nil || len(proveHashRaw) != 32 {
			return nil, nil, fmt.Errorf("invalid prove hash")
		}
		payload.Fields[OpFieldIndexerID] = indexerID
		payload.Fields[OpFieldBlockHeight] = strconv.FormatUint(proveHeight, 10)
		payload.Fields[OpFieldBlockHash] = hex.EncodeToString(proveHashRaw)
		data["indexer_id"] = indexerID
		data["prove_height"] = proveHeight
		data["prove_hash"] = hex.EncodeToString(proveHashRaw)
	case TagStake:
		if len(record) != 4 {
			return nil, nil, fmt.Errorf("invalid stake schema")
		}
		indexerID := strings.TrimSpace(record[3])
		if indexerID == "" {
			return nil, nil, fmt.Errorf("missing indexer_id")
		}
		payload.Fields[OpFieldIndexerID] = indexerID
		data["indexer_id"] = indexerID
	case TagPledgedReward:
		return nil, nil, fmt.Errorf("claim must use opreturn %s", fip101ClaimReward)
	}

	bodyJSON, err := json.Marshal(map[string]interface{}{
		"op":   opRaw,
		"data": data,
	})
	if err != nil {
		return nil, nil, err
	}
	return payload, bodyJSON, nil
}

func parseSingleCSVRecord(raw string) ([]string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return nil, fmt.Errorf("empty csv body")
	}
	if strings.Contains(normalized, "\n") || strings.Contains(normalized, "\r") {
		return nil, fmt.Errorf("invalid csv rows")
	}
	rec := strings.Split(normalized, ",")
	for i := range rec {
		rec[i] = strings.TrimSpace(rec[i])
	}
	return rec, nil
}

func parseCSVUint64(raw string) (uint64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func TruncateRunes(raw string, limit int) string {
	if limit <= 0 {
		return ""
	}
	rs := []rune(raw)
	if len(rs) <= limit {
		return raw
	}
	return string(rs[:limit])
}

func IsValidIndexerName(name string) bool {
	return indexerNamePattern.MatchString(name)
}

func mapOperationToTag(op string) string {
	switch op {
	case "register_indexer":
		return TagRegister
	case "stake":
		return TagStake
	case "submit_proof":
		return TagProveStake
	case "commission_rate":
		return TagAllocatRatio
	default:
		return ""
	}
}

func payloadTagToOperation(tag string) string {
	switch tag {
	case TagRegister:
		return "register_indexer"
	case TagStake:
		return "stake"
	case TagProveStake:
		return "submit_proof"
	case TagAllocatRatio:
		return "commission_rate"
	case TagPledgedReward:
		return "claim_reward"
	default:
		return strings.ToLower(strings.TrimSpace(tag))
	}
}

func ratioBPToString(bp uint64) string {
	if bp > 10000 {
		bp = 10000
	}
	return strconv.FormatFloat(float64(bp)/10000, 'f', 8, 64)
}

func validateRewardAddressAndNetwork(rewardAddr string) error {
	rewardAddr = strings.TrimSpace(rewardAddr)
	if rewardAddr == "" {
		return fmt.Errorf("invalid reward addr")
	}

	addr, err := btcutil.DecodeAddress(rewardAddr, &chaincfg.MainNetParams)
	if err != nil {
		return fmt.Errorf("invalid reward addr format")
	}
	script, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return fmt.Errorf("invalid reward addr script")
	}
	if len(script) == 0 {
		return fmt.Errorf("invalid reward addr script")
	}

	switch addr.(type) {
	case *btcutil.AddressWitnessPubKeyHash,
		*btcutil.AddressTaproot,
		*btcutil.AddressPubKeyHash,
		*btcutil.AddressScriptHash,
		*btcutil.AddressWitnessScriptHash:
		return nil
	default:
		return fmt.Errorf("unsupported reward addr type")
	}
}
