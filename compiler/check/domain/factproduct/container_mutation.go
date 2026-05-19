package factproduct

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
)

// ContainerMutationMerger merges an incoming mutation with an existing mutation
// on the same canonical path key. When prev is nil, next is a new key.
type ContainerMutationMerger func(prev *api.ContainerMutation, next api.ContainerMutation) api.ContainerMutation

// MergeContainerMutationSlices merges mutation slices by canonical path key.
// Output ordering is deterministic by key.
func MergeContainerMutationSlices(
	existing []api.ContainerMutation,
	next []api.ContainerMutation,
	merge ContainerMutationMerger,
) []api.ContainerMutation {
	if len(existing) == 0 && len(next) == 0 {
		return nil
	}

	mergeFn := merge
	if mergeFn == nil {
		mergeFn = func(_ *api.ContainerMutation, n api.ContainerMutation) api.ContainerMutation { return n }
	}

	byKey := make(map[string]api.ContainerMutation, len(existing)+len(next))
	add := func(m api.ContainerMutation) {
		key := api.ContainerMutationKey(m)
		if prev, ok := byKey[key]; ok {
			merged := mergeFn(&prev, m)
			byKey[key] = merged
			return
		}
		byKey[key] = mergeFn(nil, m)
	}
	for _, m := range existing {
		add(m)
	}
	for _, m := range next {
		add(m)
	}

	out := make([]api.ContainerMutation, 0, len(byKey))
	for _, key := range cfg.SortedFieldNames(byKey) {
		out = append(out, byKey[key])
	}
	return out
}

// MergeCapturedContainerMutationMaps merges per-symbol captured mutation maps.
func MergeCapturedContainerMutationMaps(
	existing map[cfg.SymbolID][]api.ContainerMutation,
	next map[cfg.SymbolID][]api.ContainerMutation,
	merge ContainerMutationMerger,
) map[cfg.SymbolID][]api.ContainerMutation {
	if len(existing) == 0 && len(next) == 0 {
		return nil
	}
	merged := make(map[cfg.SymbolID][]api.ContainerMutation, len(existing)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(existing) {
		merged[sym] = MergeContainerMutationSlices(nil, existing[sym], merge)
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		merged[sym] = MergeContainerMutationSlices(merged[sym], next[sym], merge)
	}
	return merged
}
