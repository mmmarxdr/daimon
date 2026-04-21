package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"daimon/internal/config"
	"daimon/internal/store"
)

// transcriptHeuristic is the rule that flags an entry as a transcript-style
// dump rather than a real fact. Two passes:
//   - long entries (> 800 chars) — facts are atomic, this length is excessive.
//   - structured-markdown entries (> 200 chars AND contains ##, code fences,
//     tables, or bullet lists) — those are tutorial/explanation pastes.
//
// Both signals are deliberately loose: the dry-run reports candidates so the
// user reviews before confirming. The threshold values can drift; this is a
// one-shot cleanup utility, not a continuous policy.
func transcriptHeuristic(content string) (matched bool, reason string) {
	if len(content) > 800 {
		return true, "long (>800 chars)"
	}
	if len(content) > 200 {
		hasHeader := strings.Contains(content, "\n## ") || strings.HasPrefix(content, "## ") ||
			strings.Contains(content, "\n### ") || strings.HasPrefix(content, "### ")
		hasCode := strings.Contains(content, "```")
		hasTable := strings.Contains(content, "\n|") || strings.HasPrefix(content, "|")
		hasBullets := strings.Contains(content, "\n- ") || strings.HasPrefix(content, "- ")
		if hasHeader || hasCode || hasTable || hasBullets {
			return true, "markdown structure (headers / code / tables / lists)"
		}
	}
	return false, ""
}

// runPruneMemories scans the memory table for entries matching the transcript
// heuristic. Without confirm, prints a summary + first-line preview of each
// candidate so the user can decide. With confirm, archives them in a single
// transaction (sets archived_at, doesn't hard-delete — recoverable).
func runPruneMemories(cfgPath string, confirm bool) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.Store.Type != "sqlite" {
		return fmt.Errorf("prune-memories requires store.type = sqlite (got %q)", cfg.Store.Type)
	}
	st, err := store.NewSQLiteStore(cfg.Store)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	candidates, err := scanTranscriptCandidates(context.Background(), st.DB())
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		fmt.Println("No transcript-style memory entries found. Nothing to prune.")
		return nil
	}

	fmt.Printf("Found %d transcript-style memory entries:\n\n", len(candidates))
	for i, c := range candidates {
		preview := strings.SplitN(strings.TrimSpace(c.Content), "\n", 2)[0]
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		fmt.Printf("  %3d. [%s, %d chars] %s\n", i+1, c.Reason, len(c.Content), preview)
	}
	fmt.Println()

	if !confirm {
		fmt.Println("Dry-run only. Re-run with --confirm to archive these entries.")
		fmt.Println("(Archived entries are hidden from the dashboard but kept in the DB; restore by clearing archived_at.)")
		return nil
	}

	n, err := archiveCandidates(context.Background(), st.DB(), candidates)
	if err != nil {
		return err
	}
	fmt.Printf("Archived %d entries.\n", n)
	return nil
}

type transcriptCandidate struct {
	ID      string
	Content string
	Reason  string
}

func scanTranscriptCandidates(ctx context.Context, db *sql.DB) ([]transcriptCandidate, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, content FROM memory WHERE archived_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("scanning memory: %w", err)
	}
	defer rows.Close()

	var out []transcriptCandidate
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		if matched, reason := transcriptHeuristic(content); matched {
			out = append(out, transcriptCandidate{ID: id, Content: content, Reason: reason})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating: %w", err)
	}
	return out, nil
}

func archiveCandidates(ctx context.Context, db *sql.DB, candidates []transcriptCandidate) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `UPDATE memory SET archived_at = datetime('now') WHERE id = ?`)
	if err != nil {
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	n := 0
	for _, c := range candidates {
		if _, err := stmt.ExecContext(ctx, c.ID); err != nil {
			return 0, fmt.Errorf("archiving %s: %w", c.ID, err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}
