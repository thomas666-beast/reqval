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
	dbMu    sync.RWMutex
	querier DBQuerier
	errNoDB = errors.New("reqval: no DB registered; call reqval.RegisterDB first")
)

// RegisterDB wires a database for unique/exists rules.
// Safe to call more than once.
func RegisterDB(db DBQuerier) {
	dbMu.Lock()
	defer dbMu.Unlock()
	querier = db
}

func getDB() (DBQuerier, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	if querier == nil {
		return nil, errNoDB
	}
	return querier, nil
}

// checkUnique handles "unique:table,column" and "unique:table,column,idCol,ignoreID".
func checkUnique(ctx context.Context, param string, val any) bool {
	db, err := getDB()
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
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ? AND %s <> ?", table, column, idColumn)
		if err := db.QueryRowContext(ctx, q, s, ignoreID).Scan(&count); err != nil {
			return false
		}
	} else {
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column)
		if err := db.QueryRowContext(ctx, q, s).Scan(&count); err != nil {
			return false
		}
	}
	return count == 0
}

// checkExists handles "exists:table,column".
func checkExists(ctx context.Context, param string, val any) bool {
	db, err := getDB()
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

	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column)
	var count int
	if err := db.QueryRowContext(ctx, q, val).Scan(&count); err != nil {
		return false
	}
	return count > 0
}
