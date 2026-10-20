package reqval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

type DBQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var (
	dbMu    sync.RWMutex
	querier DBQuerier
	dbOnce  sync.Once
	identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	errNoDB = errors.New("reqval: no DB registered; call reqval.RegisterDB first")
)

func RegisterDB(db DBQuerier) {
	dbMu.Lock()
	defer dbMu.Unlock()
	querier = db
	registerDBRules()
}

func getDB() (DBQuerier, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	if querier == nil {
		return nil, errNoDB
	}
	return querier, nil
}

func registerDBRules() {
	dbOnce.Do(func() {
		// Ctx variants so validators can read ignoreID out of the context.
		_ = engine().RegisterValidationCtx("unique", validateUniqueCtx)
		_ = engine().RegisterValidationCtx("exists", validateExistsCtx)
	})
}

// ---- unique ----------------------------------------------------------

type uniqueParams struct {
	table    string
	column   string
	idColumn string // "" if not provided
	ignoreID string // "" if not provided (from tag)
}

// parseUniqueParams accepts:
//   "table:column"
//   "table:column:idColumn"
//   "table:column:idColumn:ignoreID"
func parseUniqueParams(param string) (uniqueParams, bool) {
	parts := strings.Split(param, ":")
	switch len(parts) {
	case 2:
		// ok, nothing extra
	case 3, 4:
		// ok, handled below
	default:
		return uniqueParams{}, false
	}

	p := uniqueParams{table: parts[0], column: parts[1]}
	if !identRe.MatchString(p.table) || !identRe.MatchString(p.column) {
		return uniqueParams{}, false
	}
	if len(parts) >= 3 {
		p.idColumn = parts[2]
		if !identRe.MatchString(p.idColumn) {
			return uniqueParams{}, false
		}
	}
	if len(parts) == 4 {
		p.ignoreID = parts[3]
	}
	return p, true
}

func validateUniqueCtx(ctx context.Context, fl validator.FieldLevel) bool {
	db, err := getDB()
	if err != nil {
		return false
	}
	p, ok := parseUniqueParams(fl.Param())
	if !ok {
		return false
	}

	val := fl.Field().String()
	if val == "" {
		return true
	}

	// Resolve ignore ID: tag wins if set, else context.
	ignoreID := p.ignoreID
	if ignoreID == "" && p.idColumn != "" {
		if v, ok := ignoreIDFrom(ctx); ok {
			ignoreID = fmt.Sprint(v)
		}
	}

	var count int
	if ignoreID != "" && p.idColumn != "" {
		q := fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE %s = ? AND %s <> ?",
			p.table, p.column, p.idColumn,
		)
		if err := db.QueryRowContext(ctx, q, val, ignoreID).Scan(&count); err != nil {
			return false
		}
	} else {
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", p.table, p.column)
		if err := db.QueryRowContext(ctx, q, val).Scan(&count); err != nil {
			return false
		}
	}
	return count == 0
}

// ---- exists ----------------------------------------------------------

func parseExistsParams(param string) (table, column string, ok bool) {
	parts := strings.Split(param, ":")
	if len(parts) != 2 {
		return "", "", false
	}
	table, column = parts[0], parts[1]
	if !identRe.MatchString(table) || !identRe.MatchString(column) {
		return "", "", false
	}
	return table, column, true
}

func validateExistsCtx(ctx context.Context, fl validator.FieldLevel) bool {
	db, err := getDB()
	if err != nil {
		return false
	}
	table, column, ok := parseExistsParams(fl.Param())
	if !ok {
		return false
	}
	val := fl.Field().Interface()
	if isEmptyValue(val) {
		return true
	}
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column)
	var count int
	if err := db.QueryRowContext(ctx, q, val).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

// ---- helpers ---------------------------------------------------------

func isEmptyValue(v any) bool {
	switch x := v.(type) {
	case string:
		return x == ""
	case int:
		return x == 0
	case int8:
		return x == 0
	case int16:
		return x == 0
	case int32:
		return x == 0
	case int64:
		return x == 0
	case uint:
		return x == 0
	case uint8:
		return x == 0
	case uint16:
		return x == 0
	case uint32:
		return x == 0
	case uint64:
		return x == 0
	case float32:
		return x == 0
	case float64:
		return x == 0
	case bool:
		return !x
	default:
		return false
	}
}
