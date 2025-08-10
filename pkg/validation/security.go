package validation

import (
	"regexp"
	"strings"
)

// containsXSSPatterns checks for common XSS attack patterns
func (v *UnifiedValidator) containsXSSPatterns(s string) bool {
	if !v.config.EnableXSSProtection {
		return false
	}
	
	xssPatterns := []string{
		`<script[^>]*>.*?</script>`,
		`javascript:`,
		`vbscript:`,
		`onload\s*=`,
		`onerror\s*=`,
		`onclick\s*=`,
		`onmouseover\s*=`,
		`onfocus\s*=`,
		`onblur\s*=`,
		`onchange\s*=`,
		`onsubmit\s*=`,
		`<iframe[^>]*>`,
		`<object[^>]*>`,
		`<embed[^>]*>`,
		`<link[^>]*>`,
		`<meta[^>]*>`,
		`<style[^>]*>.*?</style>`,
		`expression\s*\(`,
		`url\s*\(.*javascript:`,
		`@import`,
		`</?\s*script[^>]*>`,
	}
	
	lowerS := strings.ToLower(s)
	
	for _, pattern := range xssPatterns {
		matched, err := regexp.MatchString(pattern, lowerS)
		if err == nil && matched {
			v.logger.WithField("pattern", pattern).WithField("input", s).Warn("XSS pattern detected")
			return true
		}
	}
	
	return false
}

// containsSQLInjectionPatterns checks for common SQL injection patterns
func (v *UnifiedValidator) containsSQLInjectionPatterns(s string) bool {
	if !v.config.EnableSQLInjection {
		return false
	}
	
	sqlPatterns := []string{
		`(?i)(\s|^)(union|select|insert|update|delete|drop|create|alter|exec|execute)\s`,
		`(?i)(\s|^)(or|and)\s+[\w\s]*\s*[=<>!]+\s*[\w\s]*(\s|$)`,
		`(?i)(\s|^)(or|and)\s+\d+\s*[=<>!]+\s*\d+(\s|$)`,
		`'[\s]*;[\s]*--`,
		`'[\s]*;[\s]*#`,
		`'[\s]*;[\s]*/\*`,
		`(?i)(\s|^)1\s*=\s*1(\s|$)`,
		`(?i)(\s|^)1\s*=\s*1\s+(or|and)(\s|$)`,
		`'[\s]*or[\s]*'[\w\s]*'[\s]*=[\s]*'[\w\s]*'`,
		`"[\s]*or[\s]*"[\w\s]*"[\s]*=[\s]*"[\w\s]*"`,
		`(?i)(\s|^)sleep\s*\(\s*\d+\s*\)(\s|$)`,
		`(?i)(\s|^)benchmark\s*\(`,
		`(?i)(\s|^)waitfor\s+delay(\s|$)`,
		`\|\|[\s]*concat`,
		`(?i)(\s|^)information_schema(\s|$)`,
		`(?i)(\s|^)sys\.[\w_]+(\s|$)`,
		`(?i)(\s|^)pg_[\w_]+(\s|$)`,
		`(?i)(\s|^)mysql\.[\w_]+(\s|$)`,
	}
	
	for _, pattern := range sqlPatterns {
		matched, err := regexp.MatchString(pattern, s)
		if err == nil && matched {
			v.logger.WithField("pattern", pattern).WithField("input", s).Warn("SQL injection pattern detected")
			return true
		}
	}
	
	return false
}

// containsScriptPatterns checks for script execution patterns
func (v *UnifiedValidator) containsScriptPatterns(s string) bool {
	if !v.config.EnableScriptCheck {
		return false
	}
	
	scriptPatterns := []string{
		`(?i)(\s|^)(eval|exec|system|shell_exec|passthru|popen|proc_open)\s*\(`,
		`(?i)(\s|^)(cmd|command|powershell|bash|sh|zsh)\s`,
		`(?i)(\s|^)(python|perl|ruby|node|java)\s+(-c|--command)`,
		`\$\{.*\}`,  // Template injection
		`#\{.*\}`,   // Ruby template injection
		`\{\{.*\}\}`, // Handlebars/Mustache injection
		`\<%.*%\>`,  // JSP/ASP injection
		`(?i)(\s|^)import\s+(os|sys|subprocess|commands)(\s|$)`,
		`(?i)(\s|^)require\s*\(\s*['"]child_process['"]`,
		`(?i)(\s|^)Runtime\.getRuntime\(\)\.exec`,
		`(?i)(\s|^)ProcessBuilder`,
		`(?i)(\s|^)__import__\s*\(`,
		`(?i)(\s|^)getattr\s*\(`,
		`(?i)(\s|^)setattr\s*\(`,
		`(?i)(\s|^)hasattr\s*\(`,
		`(?i)(\s|^)compile\s*\(`,
	}
	
	for _, pattern := range scriptPatterns {
		matched, err := regexp.MatchString(pattern, s)
		if err == nil && matched {
			v.logger.WithField("pattern", pattern).WithField("input", s).Warn("Script execution pattern detected")
			return true
		}
	}
	
	return false
}

// containsPathTraversalPatterns checks for path traversal attack patterns
func (v *UnifiedValidator) containsPathTraversalPatterns(s string) bool {
	if !v.config.EnablePathTraversal {
		return false
	}
	
	pathPatterns := []string{
		`\.\.[\\/]`,
		`[\\/]\.\.`,
		`%2e%2e[\\/]`,
		`[\\/]%2e%2e`,
		`%2e%2e%2f`,
		`%2e%2e%5c`,
		`%c0%ae%c0%ae[\\/]`,
		`%uff0e%uff0e[\\/]`,
		`(?i)(\s|^)(\/etc\/passwd|\/etc\/shadow|\/etc\/hosts)(\s|$)`,
		`(?i)(\s|^)(c:\\windows\\system32|c:\\windows\\system)(\s|$)`,
		`(?i)(\s|^)(\/proc\/self\/environ|\/proc\/version)(\s|$)`,
	}
	
	for _, pattern := range pathPatterns {
		matched, err := regexp.MatchString(pattern, s)
		if err == nil && matched {
			v.logger.WithField("pattern", pattern).WithField("input", s).Warn("Path traversal pattern detected")
			return true
		}
	}
	
	return false
}

// containsSuspiciousPatterns checks for other suspicious patterns
func (v *UnifiedValidator) containsSuspiciousPatterns(s string) bool {
	return v.containsXSSPatterns(s) ||
		   v.containsSQLInjectionPatterns(s) ||
		   v.containsScriptPatterns(s) ||
		   v.containsPathTraversalPatterns(s)
}

// escapeHTML escapes HTML special characters
func (v *UnifiedValidator) escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
		"/", "&#x2F;",
		"`", "&#x60;",
		"=", "&#x3D;",
	)
	return replacer.Replace(s)
}

// validateSecurity performs security validation on a struct
func (v *UnifiedValidator) validateSecurity(s interface{}) *ValidationResult {
	// This would implement struct-level security validation
	// For now, return valid since individual field validation handles security
	return &ValidationResult{
		IsValid: true,
		Errors:  []ValidationError{},
	}
}