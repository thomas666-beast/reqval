package reqval

import (
	"strings"
	"sync"
)

var (
	bundleMu        sync.RWMutex
	defaultMessages map[string]string
)

// SetDefaultMessages installs the application-wide message bundle.
// Keys are "<field>.<rule>" or "*.<rule>". Call once at startup,
// usually with a translation bundle such as reqval.ZH().
func SetDefaultMessages(m map[string]string) {
	bundleMu.Lock()
	defer bundleMu.Unlock()
	defaultMessages = m
}

// messageFor resolves the message for a field/rule pair.
// Priority: request's Messages() > application default bundle > fallback.
func messageFor(r Request, field, rule, param string) string {
	if r != nil {
		if m := r.Messages(); m != nil {
			if msg, ok := m[field+"."+rule]; ok {
				return renderMessage(msg, field, param)
			}
			if msg, ok := m["*."+rule]; ok {
				return renderMessage(msg, field, param)
			}
		}
	}

	bundleMu.RLock()
	if defaultMessages != nil {
		if msg, ok := defaultMessages[field+"."+rule]; ok {
			bundleMu.RUnlock()
			return renderMessage(msg, field, param)
		}
		if msg, ok := defaultMessages["*."+rule]; ok {
			bundleMu.RUnlock()
			return renderMessage(msg, field, param)
		}
	}
	bundleMu.RUnlock()

	return "The " + field + " field is invalid."
}

func renderMessage(tpl, field, param string) string {
	r := strings.NewReplacer("{{field}}", field, "{{param}}", param)
	return r.Replace(tpl)
}
