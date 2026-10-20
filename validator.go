package reqval

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	once     sync.Once
	validate *validator.Validate
)

func engine() *validator.Validate {
	once.Do(func() {
		validate = validator.New()
		registerCustomRules(validate)
	})
	return validate
}

// ValidateCtx runs Rules() through the engine and builds a ValidationError
// from Messages() overrides and the application default bundle.
func ValidateCtx(ctx context.Context, req any) error {
	r, ok := req.(Request)
	if !ok {
		return fmt.Errorf("reqval: type %T does not implement Request", req)
	}

	rules := r.Rules()
	if len(rules) == 0 {
		return nil
	}

	v := validator.New()
	registerCustomRules(v)

	fields := map[string]string{}

	rv := reflect.ValueOf(req)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("reqval: request must be a struct or pointer to struct, got %T", req)
	}

	for field, fieldRules := range rules {
		fv := rv.FieldByName(field)
		if !fv.IsValid() {
			fields[field] = fmt.Sprintf("The %s field does not exist.", field)
			continue
		}
		for _, rule := range fieldRules {
			if !checkRule(v, field, fv, rule) {
				msg := messageFor(r, field, ruleName(rule), ruleParam(rule))
				fields[field] = msg
				break
			}
		}
	}

	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

// checkRule applies a single rule string ("required", "max:255", "unique:users,email")
// to a reflect.Value and returns whether it passes.
func checkRule(v *validator.Validate, field string, fv reflect.Value, rule string) bool {
	name, param := splitRule(rule)

	var val any
	switch fv.Kind() {
	case reflect.String:
		val = fv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val = fv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val = fv.Uint()
	case reflect.Float32, reflect.Float64:
		val = fv.Float()
	case reflect.Bool:
		val = fv.Bool()
	default:
		val = fv.Interface()
	}

	return applyRule(name, param, val)
}

// splitRule parses "name" or "name:param" into two pieces.
func splitRule(rule string) (name, param string) {
	if i := strings.Index(rule, ":"); i >= 0 {
		return rule[:i], rule[i+1:]
	}
	return rule, ""
}

func ruleName(rule string) string  { n, _ := splitRule(rule); return n }
func ruleParam(rule string) string { _, p := splitRule(rule); return p }

// Validate is ValidateCtx with a background context.
func Validate(req any) error {
	return ValidateCtx(context.Background(), req)
}

// parseParam splits comma-separated params ("users,email,id" -> ["users","email","id"]).
func parseParam(p string) []string {
	if p == "" {
		return nil
	}
	parts := strings.Split(p, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// paramFloat parses a numeric rule param.
func paramFloat(p string, fallback float64) float64 {
	f, err := strconv.ParseFloat(p, 64)
	if err != nil {
		return fallback
	}
	return f
}
