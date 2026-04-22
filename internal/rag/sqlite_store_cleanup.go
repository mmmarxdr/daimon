package rag

import (
	"context"
	"fmt"
)

// CleanupJunkChunks detects and removes trailing suffix-junk chunks produced
// by the pre-fix chunker bug.  After the final real chunk was emitted, the old
// loop re-entered and emitted one chunk per overlap rune — each chunk being the
// previous content with the first rune stripped (one rune shorter each time).
//
// Detection heuristic (conservative — no false positives on real data):
//   - Walk each document's chunks in reverse idx order.
//   - A chunk[i] is junk iff runeLen(chunk[i]) == runeLen(chunk[i-1]) - 1 AND
//     chunk[i].content == chunk[i-1].content[first-rune-stripped:].
//   - Stop at the first pair that breaks the pattern.
//
// Idempotent — safe to call on every startup.
// Returns (docsScanned, chunksDeleted, error).
func (s *SQLiteDocumentStore) CleanupJunkChunks(ctx context.Context) (int, int, error) {
	// Fetch all document IDs.
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM documents ORDER BY id`)
	if err != nil {
		return 0, 0, fmt.Errorf("rag: cleanup list docs: %w", err)
	}
	var docIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("rag: cleanup scan doc id: %w", err)
		}
		docIDs = append(docIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("rag: cleanup iter docs: %w", err)
	}

	docsScanned := 0
	totalDeleted := 0

	for _, docID := range docIDs {
		docsScanned++
		deleted, err := s.cleanupDocJunk(ctx, docID)
		if err != nil {
			return docsScanned, totalDeleted, err
		}
		totalDeleted += deleted
	}

	return docsScanned, totalDeleted, nil
}

// cleanupDocJunk removes trailing junk chunks for a single document.
// Returns the number of chunks deleted.
func (s *SQLiteDocumentStore) cleanupDocJunk(ctx context.Context, docID string) (int, error) {
	// Fetch all chunks for this doc ordered by idx DESC so we walk tail-first.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content FROM document_chunks WHERE doc_id = ? ORDER BY idx DESC`,
		docID)
	if err != nil {
		return 0, fmt.Errorf("rag: cleanup fetch chunks for %s: %w", docID, err)
	}

	type row struct {
		id      string
		content string
	}
	var chunkRows []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.content); err != nil {
			rows.Close()
			return 0, fmt.Errorf("rag: cleanup scan chunk for %s: %w", docID, err)
		}
		chunkRows = append(chunkRows, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("rag: cleanup iter chunks for %s: %w", docID, err)
	}

	if len(chunkRows) < 2 {
		return 0, nil
	}

	// Walk pairs (tail-first): chunkRows[0] is the last chunk by idx.
	// Collect IDs of junk chunks to delete.
	var junkIDs []string
	for i := 0; i < len(chunkRows)-1; i++ {
		curr := []rune(chunkRows[i].content)
		prev := []rune(chunkRows[i+1].content)

		// Junk pattern: curr is exactly 1 rune shorter than prev, and
		// curr == prev with the first rune stripped.
		if len(curr) == len(prev)-1 && string(curr) == string(prev[1:]) {
			junkIDs = append(junkIDs, chunkRows[i].id)
		} else {
			// Pattern broken — stop here.
			break
		}
	}

	if len(junkIDs) == 0 {
		return 0, nil
	}

	// Delete junk chunks in a single statement using IN clause.
	placeholders := buildPlaceholders(len(junkIDs))
	args := make([]any, len(junkIDs))
	for i, id := range junkIDs {
		args[i] = id
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM document_chunks WHERE id IN (`+placeholders+`)`,
		args...)
	if err != nil {
		return 0, fmt.Errorf("rag: cleanup delete junk chunks for %s: %w", docID, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// buildPlaceholders returns n comma-separated "?" placeholders.
func buildPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, 0, n*2-1)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
	}
	return string(b)
}
