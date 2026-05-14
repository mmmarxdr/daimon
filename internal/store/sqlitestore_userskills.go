package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// isSQLiteUniqueError returns true if err is a SQLite UNIQUE constraint failure
// on the user_skills.name column. modernc.org/sqlite wraps this as a plain
// error whose message contains the standard SQLite text.
func isSQLiteUniqueError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") &&
		strings.Contains(msg, "user_skills.name")
}

// scanUserSkill scans a single user_skills row from rows or a single-row query
// into a UserSkill value. tools_allowlist and budget columns are NULL-able TEXT.
func scanUserSkill(
	id, name, description, prose *string,
	executable *int,
	model, provider *string,
	toolsAllowlistNS *sql.NullString,
	budgetNS *sql.NullString,
	version *int,
	source *string,
	createdAt, updatedAt *time.Time,
) UserSkill {
	return UserSkill{
		ID:             *id,
		Name:           *name,
		Description:    *description,
		Prose:          *prose,
		Executable:     *executable != 0,
		Model:          *model,
		Provider:       *provider,
		ToolsAllowlist: decodeAllowlist(*toolsAllowlistNS),
		Budget:         decodeBudget(*budgetNS),
		Version:        *version,
		Source:         *source,
		CreatedAt:      *createdAt,
		UpdatedAt:      *updatedAt,
	}
}

// ListUserSkills returns all user_skills rows ordered by name ASC.
// Returns an empty non-nil slice when no rows exist.
func (s *SQLiteStore) ListUserSkills(ctx context.Context) ([]UserSkill, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, prose, executable, model, provider,
		       tools_allowlist, budget, version, source, created_at, updated_at
		FROM user_skills
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list user_skills: %w", err)
	}
	defer rows.Close()

	var skills []UserSkill
	for rows.Next() {
		var (
			id, name, description, prose string
			executable                   int
			model, provider              string
			toolsAllowlistNS             sql.NullString
			budgetNS                     sql.NullString
			version                      int
			source                       string
			createdAt, updatedAt         time.Time
		)
		if err := rows.Scan(
			&id, &name, &description, &prose,
			&executable, &model, &provider,
			&toolsAllowlistNS, &budgetNS,
			&version, &source,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user_skill: %w", err)
		}
		skills = append(skills, scanUserSkill(
			&id, &name, &description, &prose,
			&executable, &model, &provider,
			&toolsAllowlistNS, &budgetNS,
			&version, &source,
			&createdAt, &updatedAt,
		))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user_skills: %w", err)
	}
	if skills == nil {
		skills = []UserSkill{}
	}
	return skills, nil
}

// GetUserSkill returns the user_skill row identified by name.
// Returns ErrNotFound (wrapped) when no row matches.
func (s *SQLiteStore) GetUserSkill(ctx context.Context, name string) (UserSkill, error) {
	var (
		id, nameCol, description, prose string
		executable                      int
		model, provider                 string
		toolsAllowlistNS                sql.NullString
		budgetNS                        sql.NullString
		version                         int
		source                          string
		createdAt, updatedAt            time.Time
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, prose, executable, model, provider,
		       tools_allowlist, budget, version, source, created_at, updated_at
		FROM user_skills
		WHERE name = ?
	`, name).Scan(
		&id, &nameCol, &description, &prose,
		&executable, &model, &provider,
		&toolsAllowlistNS, &budgetNS,
		&version, &source,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return UserSkill{}, fmt.Errorf("get user_skill %q: %w", name, ErrNotFound)
	}
	if err != nil {
		return UserSkill{}, fmt.Errorf("get user_skill %q: %w", name, err)
	}
	return scanUserSkill(
		&id, &nameCol, &description, &prose,
		&executable, &model, &provider,
		&toolsAllowlistNS, &budgetNS,
		&version, &source,
		&createdAt, &updatedAt,
	), nil
}

// CreateUserSkill inserts a new user_skill row and returns the created row.
// Returns ErrNameConflict (wrapped) when name already exists.
func (s *SQLiteStore) CreateUserSkill(ctx context.Context, skill UserSkill) (UserSkill, error) {
	now := time.Now().UTC()
	skill.CreatedAt = now
	skill.UpdatedAt = now
	if skill.Source == "" {
		skill.Source = "user"
	}
	if skill.Version == 0 {
		skill.Version = 1
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UserSkill{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	execFlag := 0
	if skill.Executable {
		execFlag = 1
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_skills
			(id, name, description, prose, executable, model, provider,
			 tools_allowlist, budget, version, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		skill.ID, skill.Name, skill.Description, skill.Prose,
		execFlag, skill.Model, skill.Provider,
		encodeAllowlist(skill.ToolsAllowlist),
		encodeBudget(skill.Budget),
		skill.Version, skill.Source,
		skill.CreatedAt, skill.UpdatedAt,
	)
	if err != nil {
		if isSQLiteUniqueError(err) {
			return UserSkill{}, fmt.Errorf("create user_skill %q: %w", skill.Name, ErrNameConflict)
		}
		return UserSkill{}, fmt.Errorf("insert user_skill: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return UserSkill{}, fmt.Errorf("commit user_skill insert: %w", err)
	}
	return skill, nil
}

// UpdateUserSkill replaces all mutable fields for skill.Name and advances
// updated_at to now(). Returns ErrNotFound (wrapped) when no row matches.
func (s *SQLiteStore) UpdateUserSkill(ctx context.Context, skill UserSkill) (UserSkill, error) {
	now := time.Now().UTC()
	skill.UpdatedAt = now

	execFlag := 0
	if skill.Executable {
		execFlag = 1
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UserSkill{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		UPDATE user_skills
		SET description = ?, prose = ?, executable = ?, model = ?, provider = ?,
		    tools_allowlist = ?, budget = ?, version = ?, updated_at = ?
		WHERE name = ?
	`,
		skill.Description, skill.Prose, execFlag, skill.Model, skill.Provider,
		encodeAllowlist(skill.ToolsAllowlist),
		encodeBudget(skill.Budget),
		skill.Version, skill.UpdatedAt,
		skill.Name,
	)
	if err != nil {
		return UserSkill{}, fmt.Errorf("update user_skill %q: %w", skill.Name, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return UserSkill{}, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return UserSkill{}, fmt.Errorf("update user_skill %q: %w", skill.Name, ErrNotFound)
	}

	if err := tx.Commit(); err != nil {
		return UserSkill{}, fmt.Errorf("commit user_skill update: %w", err)
	}
	return skill, nil
}

// DeleteUserSkill removes the user_skill row with the given name.
// Returns ErrNotFound (wrapped) when no row matches.
func (s *SQLiteStore) DeleteUserSkill(ctx context.Context, name string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `DELETE FROM user_skills WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete user_skill %q: %w", name, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("delete user_skill %q: %w", name, ErrNotFound)
	}

	return tx.Commit()
}
