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
//
// Rules keys may be:
//   - "Title"            — a struct field
//   - "items.*.name"     — every element's Name field
//   - "items.0.name"     — a specific element (0-indexed)
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

	for path, fieldRules := range rules {
		targets := resolvePath(rv, path)
		if len(targets) == 0 {
			fields[path] = fmt.Sprintf("The %s field does not exist.", path)
			continue
		}

		for _, t := range targets {
			for _, rule := range fieldRules {
				name := ruleName(rule)
				param := ruleParam(rule)

				// confirmed is cross-field: it needs the parent struct.
				if name == "confirmed" {
					if !checkConfirmed(t.parent, t.fieldName) {
						fields[t.display] = messageFor(r, t.fieldName, name, param)
						break
					}
					continue
				}

				if !checkRule(v, t.fieldName, t.value, rule) {
					fields[t.display] = messageFor(r, t.fieldName, name, param)
					break
				}
			}
		}
	}

	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

// target is one resolved value at a path.
type target struct {
	value     reflect.Value // the value to check
	parent    reflect.Value // the struct that contains the value
	fieldName string        // bare field name, e.g. "Name"
	display   string        // path with indexes, e.g. "items.0.name"
}

// resolvePath walks a dotted path and returns every concrete target.
// "*" iterates the current slice/array/map.
func resolvePath(rv reflect.Value, path string) []target {
	segments := strings.Split(path, ".")
	return walkPath(rv, segments, "")
}

func walkPath(rv reflect.Value, segments []string, prefix string) []target {
	if len(segments) == 0 {
		return nil
	}
	seg := segments[0]
	rest := segments[1:]

	if seg == "*" {
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			out := []target{}
			for i := 0; i < rv.Len(); i++ {
				idx := fmt.Sprintf("%s.%d", strings.TrimPrefix(prefix, "."), i)
				out = append(out, walkPath(rv.Index(i), rest, idx)...)
			}
			return out
		case reflect.Map:
			out := []target{}
			iter := rv.MapRange()
			for iter.Next() {
				key := fmt.Sprint(iter.Key().Interface())
				idx := fmt.Sprintf("%s.%s", strings.TrimPrefix(prefix, "."), key)
				out = append(out, walkPath(iter.Value(), rest, idx)...)
			}
			return out
		default:
			return nil
		}
	}

	if rv.Kind() != reflect.Struct && rv.Kind() != reflect.Ptr {
		return nil
	}
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	fv := rv.FieldByName(seg)
	if !fv.IsValid() {
		return nil
	}

	newPrefix := prefix + "." + seg
	if len(rest) == 0 {
		return []target{{
			value:     fv,
			parent:    rv,
			fieldName: seg,
			display:   strings.TrimPrefix(newPrefix, "."),
		}}
	}

	return walkPath(fv, rest, newPrefix)
}

// checkConfirmed compares field with <Field>Confirmation, falling back to
// <field>_confirmation. Returns false if neither exists.
func checkConfirmed(rv reflect.Value, field string) bool {
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return false
	}

	conf := rv.FieldByName(field + "Confirmation")
	if !conf.IsValid() {
		conf = rv.FieldByName(field + "_confirmation")
	}
	if !conf.IsValid() {
		return false
	}

	cur := rv.FieldByName(field)
	if !cur.IsValid() {
		return false
	}
	if cur.Kind() != conf.Kind() {
		return false
	}
	return reflect.DeepEqual(cur.Interface(), conf.Interface())
}

// checkRule applies a single rule string to a reflect.Value.
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

func splitRule(rule string) (name, param string) {
	if i := strings.Index(rule, ":"); i >= 0 {
		return rule[:i], rule[i+1:]
	}
	return rule, ""
}

func ruleName(rule string) string  { n, _ := splitRule(rule); return n }
func ruleParam(rule string) string { _, p := splitRule(rule); return p }

func Validate(req any) error {
	return ValidateCtx(context.Background(), req)
}

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

func paramFloat(p string, fallback float64) float64 {
	f, err := strconv.ParseFloat(p, 64)
	if err != nil {
		return fallback
	}
	return f
}
