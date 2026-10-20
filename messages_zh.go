package reqval

func ZH() map[string]string {
	return map[string]string{
		"*.required":  "{{field}} 字段是必填的。",
		"*.min":       "{{field}} 字段至少需要 {{param}} 个字符。",
		"*.max":       "{{field}} 字段不能超过 {{param}} 个字符。",
		"*.email":     "{{field}} 必须是有效的邮箱地址。",
		"*.url":       "{{field}} 必须是有效的 URL。",
		"*.uuid":      "{{field}} 必须是有效的 UUID。",
		"*.numeric":   "{{field}} 必须是数字。",
		"*.alpha":     "{{field}} 只能包含字母。",
		"*.alphanum":  "{{field}} 只能包含字母和数字。",
		"*.gte":       "{{field}} 不能小于 {{param}}。",
		"*.lte":       "{{field}} 不能大于 {{param}}。",
		"*.in":        "所选 {{field}} 无效。",
		"*.oneof":     "所选 {{field}} 无效。",
		"*.between":   "{{field}} 必须在 {{param}} 之间。",
		"*.regexp":    "{{field}} 格式无效。",
		"*.date":      "{{field}} 必须是有效的日期。",
		"*.confirmed": "{{field}} 确认不匹配。",
		"*.unique":    "{{field}} 已被占用。",
		"*.exists":    "所选 {{field}} 无效。",
	}
}
