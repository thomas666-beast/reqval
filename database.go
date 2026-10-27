package reqval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

type DBQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var (
	dbMu             sync.RWMutex
	querier          DBQuerier
	placeholderStyle = "?" // "?" for SQLite/MySQL, "$" for PostgreSQL
	errNoDB          = errors.New("reqval: no DB registered; call reqval.RegisterDB first")
)

// RegisterDB wires a database for unique/exists rules.
// Safe to call more than once.
//
// Placeholder style defaults to "?" (SQLite, MySQL, MariaDB).
// For PostgreSQL call RegisterDBWithPlaceholder(db, "$") instead.
func RegisterDB(db DBQuerier) {
	dbMu.Lock()
	defer dbMu.Unlock()
	querier = db
	placeholderStyle = "?"
}

// RegisterDBWithPlaceholder wires a database and sets the SQL placeholder
// style used by unique/exists rules.
//
// style must be "?" (SQLite, MySQL, MariaDB) or "$" (PostgreSQL).
// Any other value falls back to "?".
func RegisterDBWithPlaceholder(db DBQuerier, style string) {
	dbMu.Lock()
	defer dbMu.Unlock()
	querier = db
	switch style {
	case "$":
		placeholderStyle = "$"
	default:
		placeholderStyle = "?"
	}
}

func getDB() (DBQuerier, string, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	if querier == nil {
		return nil, "", errNoDB
	}
	return querier, placeholderStyle, nil
}

// ph renders a placeholder for the given 1-based index.
func ph(style string, n int) string {
	if style == "$" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// checkUnique handles "unique:table,column" and "unique:table,column,idCol,ignoreID".
func checkUnique(ctx context.Context, param string, val any) bool {
	db, style, err := getDB()
	if err != nil {
		return false
	}
	parts := parseParam(param)
	if len(parts) < 2 {
		return false
	}
	table, column := parts[0], parts[1]
	if !identRe.MatchString(table) || !identRe.MatchString(column) {
		return false
	}

	var ignoreID string
	var idColumn string
	if len(parts) >= 4 {
		idColumn, ignoreID = parts[2], parts[3]
		if !identRe.MatchString(idColumn) {
			return false
		}
	}

	s := fmt.Sprint(val)
	if s == "" || s == "0" {
		return true
	}

	var count int
	if ignoreID != "" {
		q := fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE %s = %s AND %s <> %s",
			table, column, ph(style, 1), idColumn, ph(style, 2),
		)
		if err := db.QueryRowContext(ctx, q, s, ignoreID).Scan(&count); err != nil {
			return false
		}
	} else {
		q := fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE %s = %s",
			table, column, ph(style, 1),
		)
		if err := db.QueryRowContext(ctx, q, s).Scan(&count); err != nil {
			return false
		}
	}
	return count == 0
}

// checkExists handles "exists:table,column".
func checkExists(ctx context.Context, param string, val any) bool {
	db, style, err := getDB()
	if err != nil {
		return false
	}
	parts := parseParam(param)
	if len(parts) != 2 {
		return false
	}
	table, column := parts[0], parts[1]
	if !identRe.MatchString(table) || !identRe.MatchString(column) {
		return false
	}

	if isEmpty(val) {
		return true
	}

	q := fmt.Sprintf(
		"SELECT COUNT(*) FROM %s WHERE %s = %s",
		table, column, ph(style, 1),
	)
	var count int
	if err := db.QueryRowContext(ctx, q, val).Scan(&count); err != nil {
		return false
	}
	return count > 0
}
