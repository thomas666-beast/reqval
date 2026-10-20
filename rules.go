package reqval

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

// registerCustomRules is a no-op in the method-based design.
// Rules are dispatched through applyRule, not through validator's tag engine.
func registerCustomRules(v *validator.Validate) {
	_ = v
}

// applyRule dispatches a single rule check.
func applyRule(name, param string, val any) bool {
	switch name {
	// --- built-in ---
	case "required":
		return !isEmpty(val)
	case "min":
		return cmpLen(val) >= int(paramFloat(param, 0))
	case "max":
		return cmpLen(val) <= int(paramFloat(param, 0))
	case "email":
		s, _ := val.(string)
		return emailRe.MatchString(s)
	case "url":
		s, _ := val.(string)
		return urlRe.MatchString(s)
	case "uuid":
		s, _ := val.(string)
		return uuidRe.MatchString(s)
	case "numeric":
		s, _ := val.(string)
		return numericRe.MatchString(s)
	case "alpha":
		s, _ := val.(string)
		return alphaRe.MatchString(s)
	case "alphanum":
		s, _ := val.(string)
		return alphanumRe.MatchString(s)
	case "gte":
		return toFloat(val) >= paramFloat(param, 0)
	case "lte":
		return toFloat(val) <= paramFloat(param, 0)
	case "in":
		return inList(val, param)
	case "oneof":
		return inList(val, strings.ReplaceAll(param, ",", " "))

	// --- reqval custom ---
	case "between":
		parts := parseParam(param)
		if len(parts) != 2 {
			return false
		}
		lo := paramFloat(parts[0], 0)
		hi := paramFloat(parts[1], 0)
		if s, ok := val.(string); ok {
			n := float64(len(s))
			return n >= lo && n <= hi
		}
		n := toFloat(val)
		return n >= lo && n <= hi

	case "regexp":
		s, ok := val.(string)
		if !ok {
			return false
		}
		re, err := regexp.Compile(param)
		if err != nil {
			return false
		}
		return re.MatchString(s)

	case "confirmed":
		return true // handled at struct level in ValidateCtx

	case "unique":
		return checkUnique(context.Background(), param, val)

	case "exists":
		return checkExists(context.Background(), param, val)

	default:
		return true
	}
}

// --- string pattern regexps ---

var (
	emailRe    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	urlRe      = regexp.MustCompile(`^https?://[^\s]+$`)
	uuidRe     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	numericRe  = regexp.MustCompile(`^[0-9]+$`)
	alphaRe    = regexp.MustCompile(`^[a-zA-Z]+$`)
	alphanumRe = regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	identRe    = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// --- helpers ---

func isEmpty(v any) bool {
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
		return reflect.ValueOf(v).IsZero()
	}
}

func cmpLen(v any) int {
	if s, ok := v.(string); ok {
		return len(s)
	}
	return int(toFloat(v))
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}

func inList(val any, param string) bool {
	items := strings.FieldsFunc(param, func(r rune) bool {
		return r == ',' || r == ' ' || r == ':'
	})
	for _, it := range items {
		if it == fmt.Sprint(val) {
			return true
		}
	}
	return false
}
