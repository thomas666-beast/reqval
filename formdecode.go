package reqval

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// decodeForm assigns form values to struct fields using the `form` tag
// (falling back to `json` tag if `form` is absent).
//
// Supported field kinds: string, int*, uint*, float*, bool, []string.
func decodeForm(values url.Values, dst any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("reqval: dst must be a non-nil pointer")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("reqval: dst must point to a struct")
	}
	rt := rv.Type()

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("form")
		if tag == "" {
			tag = field.Tag.Get("json")
		}
		if tag == "" || tag == "-" {
			continue
		}
		tag = splitTag(tag)
		vals, ok := values[tag]
		if !ok || len(vals) == 0 {
			continue
		}
		fv := rv.Field(i)

		// []string is special: assign all values.
		if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String {
			fv.Set(reflect.ValueOf(vals))
			continue
		}

		if err := setFieldFromString(fv, vals[0]); err != nil {
			return fmt.Errorf("reqval: field %s: %w", field.Name, err)
		}
	}
	return nil
}

func splitTag(tag string) string {
	for i := 0; i < len(tag); i++ {
		if tag[i] == ',' {
			return tag[:i]
		}
	}
	return tag
}

func setFieldFromString(fv reflect.Value, s string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	default:
		return fmt.Errorf("unsupported kind %s", fv.Kind())
	}
	return nil
}

// splitComma splits a comma-separated string, trimming whitespace.
func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}