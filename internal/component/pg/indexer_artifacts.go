package pgdb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type IndexerAddrDelta struct {
	Address string
	Delta   int64
}

func ReplaceIndexerAddrDeltasTx(ctx context.Context, tx *sql.Tx, height uint32, deltas map[string]int64) error {
	if tx == nil {
		return fmt.Errorf("nil tx")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM indexer_addr_deltas WHERE height = $1`, height); err != nil {
		return fmt.Errorf("delete indexer addr deltas failed: %w", err)
	}
	if len(deltas) == 0 {
		return nil
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO indexer_addr_deltas (height, address, delta) VALUES ($1,$2,$3)`)
	if err != nil {
		return fmt.Errorf("prepare insert indexer addr deltas failed: %w", err)
	}
	defer stmt.Close()

	keys := make([]string, 0, len(deltas))
	for address, delta := range deltas {
		if address == "" || delta == 0 {
			continue
		}
		keys = append(keys, address)
	}
	sort.Strings(keys)
	for _, address := range keys {
		delta := deltas[address]
		if _, err := stmt.ExecContext(ctx, height, address, delta); err != nil {
			return fmt.Errorf("insert indexer addr delta failed: %w", err)
		}
	}
	return nil
}

func listIndexerUndoByHeight(ctx context.Context, table string, height uint32) (map[string][]byte, error) {
	if StakeDB == nil {
		return map[string][]byte{}, nil
	}
	sqlText := fmt.Sprintf(`SELECT outpoint, utxo_raw FROM %s WHERE height = $1`, table)
	rows, err := StakeDB.QueryContext(ctx, sqlText, height)
	if err != nil {
		return nil, fmt.Errorf("query %s by height failed: %w", table, err)
	}
	defer rows.Close()

	result := make(map[string][]byte, 1024)
	for rows.Next() {
		var outpointDB string
		var raw []byte
		if err := rows.Scan(&outpointDB, &raw); err != nil {
			return nil, fmt.Errorf("scan %s failed: %w", table, err)
		}
		outpoint := decodeOutpointFromDB(outpointDB)
		if outpoint == "" || len(raw) == 0 {
			continue
		}
		copied := make([]byte, len(raw))
		copy(copied, raw)
		result[outpoint] = copied
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s failed: %w", table, err)
	}
	return result, nil
}

func replaceIndexerUndoTx(ctx context.Context, tx *sql.Tx, table string, height uint32, rows map[string][]byte) error {
	if tx == nil {
		return fmt.Errorf("nil tx")
	}
	delSQL := fmt.Sprintf(`DELETE FROM %s WHERE height = $1`, table)
	if _, err := tx.ExecContext(ctx, delSQL, height); err != nil {
		return fmt.Errorf("delete %s failed: %w", table, err)
	}
	if len(rows) == 0 {
		return nil
	}

	insSQL := fmt.Sprintf(`INSERT INTO %s (height, outpoint, utxo_raw) VALUES ($1,$2,$3)`, table)
	stmt, err := tx.PrepareContext(ctx, insSQL)
	if err != nil {
		return fmt.Errorf("prepare insert %s failed: %w", table, err)
	}
	defer stmt.Close()

	keys := make([]string, 0, len(rows))
	for outpoint, raw := range rows {
		if outpoint == "" || len(raw) == 0 {
			continue
		}
		keys = append(keys, outpoint)
	}
	sort.Strings(keys)
	for _, outpoint := range keys {
		raw := rows[outpoint]
		outpointDB := encodeOutpointForDB(outpoint)
		if outpointDB == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, height, outpointDB, raw); err != nil {
			return fmt.Errorf("insert %s failed: %w", table, err)
		}
	}
	return nil
}

func encodeOutpointForDB(outpoint string) string {
	if outpoint == "" {
		return ""
	}
	return hex.EncodeToString([]byte(outpoint))
}

func decodeOutpointFromDB(outpointDB string) string {
	outpointDB = strings.TrimSpace(outpointDB)
	if outpointDB == "" {
		return ""
	}
	decoded, err := hex.DecodeString(outpointDB)
	if err != nil || len(decoded) == 0 {
		// Fallback for legacy rows that might have been written in plain-text format.
		return outpointDB
	}
	return string(decoded)
}

func ReplaceIndexerUndoNewTx(ctx context.Context, tx *sql.Tx, height uint32, rows map[string][]byte) error {
	return replaceIndexerUndoTx(ctx, tx, "indexer_undo_new", height, rows)
}

func ReplaceIndexerUndoSpentTx(ctx context.Context, tx *sql.Tx, height uint32, rows map[string][]byte) error {
	return replaceIndexerUndoTx(ctx, tx, "indexer_undo_spent", height, rows)
}

func DeleteIndexerUndoBeforeHeightTx(ctx context.Context, tx *sql.Tx, minKeepHeight uint32) error {
	if tx == nil {
		return fmt.Errorf("nil tx")
	}
	if minKeepHeight == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM indexer_undo_new WHERE height < $1`, minKeepHeight); err != nil {
		return fmt.Errorf("delete indexer_undo_new before height failed: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM indexer_undo_spent WHERE height < $1`, minKeepHeight); err != nil {
		return fmt.Errorf("delete indexer_undo_spent before height failed: %w", err)
	}
	return nil
}

func ListIndexerUndoNewByHeight(ctx context.Context, height uint32) (map[string][]byte, error) {
	return listIndexerUndoByHeight(ctx, "indexer_undo_new", height)
}

func ListIndexerUndoSpentByHeight(ctx context.Context, height uint32) (map[string][]byte, error) {
	return listIndexerUndoByHeight(ctx, "indexer_undo_spent", height)
}

func ListIndexerAddrDeltasByHeight(ctx context.Context, height uint32) ([]IndexerAddrDelta, error) {
	if StakeDB == nil {
		return []IndexerAddrDelta{}, nil
	}
	rows, err := StakeDB.QueryContext(ctx,
		`SELECT address, delta FROM indexer_addr_deltas WHERE height = $1 ORDER BY address`,
		height,
	)
	if err != nil {
		return nil, fmt.Errorf("query indexer addr deltas by height failed: %w", err)
	}
	defer rows.Close()

	result := make([]IndexerAddrDelta, 0, 1024)
	for rows.Next() {
		var item IndexerAddrDelta
		if err := rows.Scan(&item.Address, &item.Delta); err != nil {
			return nil, fmt.Errorf("scan indexer addr delta failed: %w", err)
		}
		if item.Address == "" || item.Delta == 0 {
			continue
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexer addr deltas failed: %w", err)
	}
	return result, nil
}

func DeleteIndexerArtifactsFromHeight(ctx context.Context, fromHeight uint32) error {
	if StakeDB == nil {
		return nil
	}
	tx, err := StakeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete indexer artifacts tx failed: %w", err)
	}
	defer tx.Rollback()

	stmts := []string{
		`DELETE FROM indexer_addr_deltas WHERE height >= $1`,
		`DELETE FROM indexer_undo_new WHERE height >= $1`,
		`DELETE FROM indexer_undo_spent WHERE height >= $1`,
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt, fromHeight); err != nil {
			return fmt.Errorf("delete indexer artifacts failed: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete indexer artifacts tx failed: %w", err)
	}
	return nil
}
