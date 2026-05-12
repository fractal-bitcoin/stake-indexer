package pgdb

import (
	"context"
	"fmt"
)

func RollbackStakeIndexerFromHeight(ctx context.Context, fromHeight uint32) error {
	if StakeDB == nil {
		return nil
	}

	tx, err := StakeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rollback tx failed: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`DELETE FROM stake_claimed_rewards WHERE height >= $1`,
		`DELETE FROM stake_allocated_rewards WHERE height >= $1`,
		`DELETE FROM stake_bindings WHERE height >= $1`,
		`DELETE FROM stake_indexer_registers WHERE height >= $1`,
		`DELETE FROM stake_proofs WHERE height >= $1`,
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt, fromHeight); err != nil {
			return fmt.Errorf("rollback execute failed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rollback tx failed: %w", err)
	}
	return nil
}
