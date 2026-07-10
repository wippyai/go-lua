package bind

import (
	"sort"
)

func normalizeNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
