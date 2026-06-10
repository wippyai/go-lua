package path

import "sort"

// SortedPathKeys returns PathKeys in deterministic order for map iteration.
func SortedPathKeys[T any](m map[PathKey]T) []PathKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]PathKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}

// ForEachSortedPathKey visits map keys in deterministic order without forcing
// callers that only need visitation to materialize a returned key slice.
func ForEachSortedPathKey[T any](m map[PathKey]T, visit func(PathKey)) {
	switch len(m) {
	case 0:
		return
	case 1:
		for key := range m {
			visit(key)
		}
		return
	default:
		keys := make([]PathKey, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			return keys[i] < keys[j]
		})
		for _, key := range keys {
			visit(key)
		}
	}
}
