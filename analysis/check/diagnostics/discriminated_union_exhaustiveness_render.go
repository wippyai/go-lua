package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func dispatchKeyName(table, key string) string {
	if identifierName(key) {
		return table + "." + key
	}
	return table + "[" + formatType(typ.LiteralString(key)) + "]"
}

func registrationCaseName(registry, key string) string {
	return dispatchKeyName(registry, key)
}

func identifierName(s string) bool {
	if s == "" {
		return false
	}
	if !((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z') || s[0] == '_') {
		return false
	}
	for i := 1; i < len(s); i++ {
		ch := s[i]
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	return true
}

func discriminantCaseList(cases []string) string {
	return strings.Join(codeNames(cases), ", ")
}

func dispatchKeyList(keys []string) string {
	if len(keys) == 0 {
		return "none"
	}
	return strings.Join(codeNames(keys), ", ")
}

func dispatchMissingKeyCases(keys []string, cases []string) string {
	if len(keys) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(keys))
	for i, key := range keys {
		if i < len(cases) && cases[i] != "" {
			parts = append(parts, codeName(key)+" for "+codeName(cases[i]))
		} else {
			parts = append(parts, codeName(key))
		}
	}
	return strings.Join(parts, ", ")
}
