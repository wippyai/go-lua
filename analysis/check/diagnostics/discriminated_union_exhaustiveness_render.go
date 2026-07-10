package diagnostics

import (
	"strings"
)

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
