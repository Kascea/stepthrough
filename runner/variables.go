package runner

import (
	"regexp"
	"strings"
)

var azureVarRef = regexp.MustCompile(`\$\(([A-Za-z_][A-Za-z0-9_.-]*)\)`)

func expandAzureVariables(script string) string {
	return azureVarRef.ReplaceAllStringFunc(script, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "$("), ")")
		return "${" + azureEnvName(name) + "}"
	})
}

func azureEnvName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r - ('a' - 'A'))
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}

func shellValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "`", "\\`")
	return `"` + value + `"`
}
