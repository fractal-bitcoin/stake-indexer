package fast

import "stake_indexer/model"

type BlockHandler interface {
	HandleFastBlock(block *model.Block) error
}

func HandleBlock(h BlockHandler, block *model.Block) error {
	if h == nil {
		return nil
	}
	return h.HandleFastBlock(block)
}
