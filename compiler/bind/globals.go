package bind

import "sort"

// PredeclaredGlobalNames returns the deterministic non-empty names from a global
// value namespace. The binder consumes names, not types; callers that hold
// map-backed global namespaces should normalize them here before Bind.
func PredeclaredGlobalNames[T any](globals map[string]T) []string {
	if len(globals) == 0 {
		return nil
	}
	names := make([]string, 0, len(globals))
	for name := range globals {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
