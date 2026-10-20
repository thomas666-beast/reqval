package reqval

import (
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	msgMu       sync.RWMutex
	tagMessages map[string]string            // tag -> template
	fieldMsgs   map[string]map[string]string // structName -> field -> template
)

func init() {
	tagMessages = map[string]string{
		"required":  "The {{field}} field is required.",
		"max":       "The {{field}} field must not exceed {{param}} characters.",
		"min":       "The {{field}} field must be at least {{param}} characters.",
		"unique":    "The {{field}} has already been taken.",
		"exists":    "The selected {{field}} is invalid.",
		"email":     "The {{field}} must be a valid email address.",
		"url":       "The {{field}} must be a valid URL.",
		"uuid":      "The {{field}} must be a valid UUID.",
		"numeric":   "The {{field}} must be a number.",
		"in":        "The selected {{field}} is invalid.",
		"regex":     "The {{field}} format is invalid.",
		"between":   "The {{field}} must be between {{param}}.",
		"confirmed": "The {{field}} confirmation does not match.",
		"alpha":     "The {{field}} may only contain letters.",
		"alphanum":  "The {{field}} may only contain letters and numbers.",
		"gte":       "The {{field}} must be greater than or equal to {{param}}.",
		"lte":       "The {{field}} must be less than or equal to {{param}}.",
	}
	fieldMsgs = map[string]map[string]string{}
}

// RegisterMessage overrides the message for a validation tag globally.
// Template supports {{field}} and {{param}} placeholders.
func RegisterMessage(tag, template string) {
	msgMu.Lock()
	defer msgMu.Unlock()
	tagMessages[tag] = template
}

// RegisterFieldMessage overrides the message for a specific field.
// Highest priority. Template supports {{field}} and {{param}}.
func RegisterFieldMessage(structName, field, template string) {
	msgMu.Lock()
	defer msgMu.Unlock()
	if fieldMsgs[structName] == nil {
		fieldMsgs[structName] = map[string]string{}
	}
	fieldMsgs[structName][field] = template
}

// resolve builds the final message for a field error.
func resolve(structName string, fe validator.FieldError) string {
	msgMu.RLock()
	defer msgMu.RUnlock()

	if m, ok := fieldMsgs[structName][fe.Field()]; ok {
		return render(m, fe)
	}
	if tpl, ok := tagMessages[fe.Tag()]; ok {
		return render(tpl, fe)
	}
	return "The " + fe.Field() + " field is invalid."
}

func render(tpl string, fe validator.FieldError) string {
	r := strings.NewReplacer(
		"{{field}}", fe.Field(),
		"{{param}}", fe.Param(),
	)
	return r.Replace(tpl)
}
