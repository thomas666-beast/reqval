package reqval

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

// registerRules wires library-owned custom rules onto v.
// It must NOT call engine(); the caller owns the Once and passes v in.
//
// IMPORTANT: tag params can never contain commas — the tag parser splits
// on ',' before the rule is dispatched, so no custom validator ever sees a
// comma in fl.Param(). Multi-value rules therefore accept space or colon
// as the internal separator.
func registerRules(v *validator.Validate) {
	_ = v.RegisterValidation("confirmed", validateConfirmed)
	_ = v.RegisterValidation("in", validateIn)
	_ = v.RegisterValidation("between", validateBetween)
	_ = v.RegisterValidation("regexp", validateRegexp)
}

// validateConfirmed compares the field to a confirmation field.
// It looks for "<Field>Confirmation" first (Go-idiomatic camelCase),
// then "<field>_confirmation" (Laravel snake_case convention).
func validateConfirmed(fl validator.FieldLevel) bool {
	parent := fl.Parent()
	fieldName := fl.StructFieldName()

	conf := parent.FieldByName(fieldName + "Confirmation") // PasswordConfirmation
	if !conf.IsValid() {
		conf = parent.FieldByName(fieldName + "_confirmation") // password_confirmation
	}
	if !conf.IsValid() {
		return false
	}
	if fl.Field().Kind() != conf.Kind() {
		return false
	}
	return reflect.DeepEqual(fl.Field().Interface(), conf.Interface())
}

// validateIn accepts space- or colon-separated values.
//
//	validate:"in=admin editor"
//	validate:"in=admin:editor"
func validateIn(fl validator.FieldLevel) bool {
	values := splitList(fl.Param())
	field := fl.Field()
	for _, v := range values {
		if valuesEqual(field, v) {
			return true
		}
	}
	return false
}

// validateBetween handles ints, uints, floats, and strings (length).
//
//	validate:"between=1 100"      // numeric range
//	validate:"between=1:100"      // numeric range (colon form)
//	validate:"between=3 10"       // string length range
func validateBetween(fl validator.FieldLevel) bool {
	params := splitList(fl.Param())
	if len(params) != 2 {
		return false
	}
	lo, err1 := strconv.ParseFloat(params[0], 64)
	hi, err2 := strconv.ParseFloat(params[1], 64)
	if err1 != nil || err2 != nil {
		return false
	}

	field := fl.Field()
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v := float64(field.Int())
		return v >= lo && v <= hi
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v := float64(field.Uint())
		return v >= lo && v <= hi
	case reflect.Float32, reflect.Float64:
		v := field.Float()
		return v >= lo && v <= hi
	case reflect.String:
		n := float64(len(field.String()))
		return n >= lo && n <= hi
	default:
		return false
	}
}

// validateRegexp compiles the param as a Go regexp and matches the field.
// Only valid for string fields.
func validateRegexp(fl validator.FieldLevel) bool {
	if fl.Field().Kind() != reflect.String {
		return false
	}
	pattern := fl.Param()
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(fl.Field().String())
}

// splitList splits a param on spaces or colons (never commas).
func splitList(param string) []string {
	fields := strings.FieldsFunc(param, func(r rune) bool {
		return r == ':' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// valuesEqual compares a reflect.Value against a string literal.
func valuesEqual(v reflect.Value, want string) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == want
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(want, 10, 64)
		return err == nil && v.Int() == n
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(want, 10, 64)
		return err == nil && v.Uint() == n
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(want, 64)
		return err == nil && v.Float() == n
	default:
		return false
	}
}
