package reqval

// LoadZH registers Chinese default messages for the built-in tags.
// Call once at startup.
func LoadZH() {
	RegisterMessage("required", "{{field}} 字段是必填的。")
	RegisterMessage("max", "{{field}} 字段不能超过 {{param}} 个字符。")
	RegisterMessage("min", "{{field}} 字段至少需要 {{param}} 个字符。")
}
