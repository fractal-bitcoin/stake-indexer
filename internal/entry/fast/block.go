package fast

import (
	pgdb "stake_indexer/internal/component/pg"
	protocolparser "stake_indexer/internal/parser/protocol"
	"stake_indexer/model"
)

type BlockDeps interface {
	ResolveBusinessInvalidFlags(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) uint64
	HandleRegisterTx(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error
	HandleProofTx(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error
	HandleClaimedRewardTx(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error
	HandleStakeBindTxWithPayload(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error
	HandleUpdateRatioTxLatest(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error
	AppendFIP101InscriptionEvent(event pgdb.FIP101InscriptionEvent)
}

func HandleBlockByDeps(deps BlockDeps, block *model.Block) error {
	if deps == nil || block == nil {
		return nil
	}

	for txIdx, tx := range block.Txs {
		parsed, err := protocolparser.ParseProtocolTxFromModelTx(tx, uint32(txIdx), int64(block.Height))
		if err != nil {
			return err
		}
		if parsed == nil || parsed.Payload == nil {
			continue
		}
		txSnapshot := parsed.Snapshot
		payload := parsed.Payload
		bizInvalidFlags := deps.ResolveBusinessInvalidFlags(block.Height, &txSnapshot, payload)

		if bizInvalidFlags == BizInvalidNone {
			switch payload.Tag {
			case protocolparser.TagRegister:
				if err := deps.HandleRegisterTx(block.Height, &txSnapshot, payload); err != nil {
					return err
				}
			case protocolparser.TagProveStake:
				if err := deps.HandleProofTx(block.Height, &txSnapshot, payload); err != nil {
					return err
				}
			case protocolparser.TagPledgedReward:
				if err := deps.HandleClaimedRewardTx(block.Height, &txSnapshot, payload); err != nil {
					return err
				}
			case protocolparser.TagStake:
				if err := deps.HandleStakeBindTxWithPayload(block.Height, &txSnapshot, payload); err != nil {
					return err
				}
			case protocolparser.TagAllocatRatio:
				if err := deps.HandleUpdateRatioTxLatest(block.Height, &txSnapshot, payload); err != nil {
					return err
				}
			}
		}
		if parsed.Event != nil {
			parsed.Event.BizInvalidFlags = bizInvalidFlags
			deps.AppendFIP101InscriptionEvent(*parsed.Event)
		}
	}
	return nil
}
