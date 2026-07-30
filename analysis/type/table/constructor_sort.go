package table

import (
	"cmp"
	"slices"
)

func sortedConstructorStringKeys(values map[string]*constructorNode) []string {
	return sortedConstructorKeys(values)
}

func sortedConstructorIntKeys(values map[int64]*constructorNode) []int64 {
	return sortedConstructorKeys(values)
}

func sortedConstructorKeys[K cmp.Ordered](values map[K]*constructorNode) []K {
	if len(values) == 0 {
		return nil
	}
	keys := make([]K, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
