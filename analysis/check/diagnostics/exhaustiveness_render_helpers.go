package diagnostics

import "strings"

func channelCaseList(cases []string) string {
	return strings.Join(codeNames(cases), ", ")
}

func codeNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, codeName(name))
	}
	return out
}
