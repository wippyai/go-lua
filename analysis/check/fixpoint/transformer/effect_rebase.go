package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

// EffectRebaseOutput preserves the source program order exactly.
type EffectRebaseOutput struct {
	Effects []EffectTerm
}

type effectRebaseLayout struct {
	target, table                    effectTargetRebaseLayout
	preciseTable, keyPath, valuePath int
	preciseKey, key, value           int
	pathStoreAssignment              pathStoreWriteRebaseLayout
	pathStoreStatic                  pathStoreWriteRebaseLayout
	pathStoreObject                  pathStoreObjectRebaseLayout
}

type pathStoreWriteRebaseLayout struct{ target, value, source int }

type pathStoreObjectRebaseLayout struct {
	heaps   []pathStoreHeapRebaseLayout
	entries []pathStoreEntryRebaseLayout
}

type pathStoreHeapRebaseLayout struct {
	root    int
	members []pathStoreMemberRebaseLayout
}

type pathStoreMemberRebaseLayout struct {
	value, expected int
}

type pathStoreEntryRebaseLayout struct {
	target, value, source, expected int
}

type effectTargetRebaseLayout struct {
	kind       effectTargetKind
	path       int
	allocation AllocationTemplateTerm
}

func emptyEffectRebaseLayout() effectRebaseLayout {
	return effectRebaseLayout{
		target: effectTargetRebaseLayout{path: -1}, table: effectTargetRebaseLayout{path: -1},
		preciseTable: -1, keyPath: -1, valuePath: -1,
		preciseKey: -1, key: -1, value: -1,
		pathStoreAssignment: pathStoreWriteRebaseLayout{target: -1, value: -1, source: -1},
		pathStoreStatic:     pathStoreWriteRebaseLayout{target: -1, value: -1, source: -1},
	}
}

// RebaseEffectDAGs transactionally imports an ordered effect sequence. All
// scalar/path dependencies are validated and rebased in one TermRebaseDAGs
// transaction before any destination EffectTerm is interned.
func RebaseEffectDAGs(caller, callee *EffectArena, bindings TermRootBindings, effects []EffectTerm) (EffectRebaseOutput, error) {
	return rebaseEffectDAGs(caller, callee, bindings, effects, false)
}

func rebaseDirectCallEffectDAGs(caller, callee *EffectArena, bindings TermRootBindings, effects []EffectTerm) (EffectRebaseOutput, error) {
	return rebaseEffectDAGs(caller, callee, bindings, effects, true)
}

func rebaseEffectDAGs(caller, callee *EffectArena, bindings TermRootBindings, effects []EffectTerm, directCall bool) (EffectRebaseOutput, error) {
	if caller == nil || callee == nil || caller.terms == nil || callee.terms == nil {
		return EffectRebaseOutput{}, fmt.Errorf("transformer: effect rebasing requires two effect arenas")
	}
	input := TermRebaseInput{}
	layouts := make([]effectRebaseLayout, len(effects))
	for i, term := range effects {
		if !validEffectNodeForRebase(callee, term, bindings.callee) {
			return EffectRebaseOutput{}, fmt.Errorf("transformer: invalid source effect term %d at sequence index %d", term, i)
		}
		node := callee.nodes[term]
		layout := emptyEffectRebaseLayout()
		if node.kind == EffectAllocationTemplate {
			layouts[i] = layout
			continue
		}
		if node.kind == EffectPathStore || node.kind == EffectObjectMaterialization {
			if node.pathStoreHasAssignment {
				layout.pathStoreAssignment = appendPathStoreWriteRebase(&input, node.pathStoreAssignment)
			}
			if node.pathStoreHasStatic {
				layout.pathStoreStatic = appendPathStoreWriteRebase(&input, node.pathStoreStatic)
			}
			layout.pathStoreObject = appendPathStoreObjectRebase(&input, node.pathStoreObject)
			layouts[i] = layout
			continue
		}
		layout.target = appendEffectTarget(&input, node.invalidation.Target)
		if node.invalidation.Precise != nil {
			layout.preciseTable = appendEffectPath(&input, node.invalidation.Precise.Table)
			layout.preciseKey = appendEffectValue(&input, node.invalidation.Precise.Key)
		}
		if node.kind == EffectIndexMutation {
			layout.table = appendEffectTarget(&input, node.table)
			layout.key = appendEffectValue(&input, node.key)
			layout.value = appendEffectValue(&input, node.value)
			if node.keyPath != 0 {
				layout.keyPath = appendEffectPath(&input, node.keyPath)
			}
			if node.valuePath != 0 {
				layout.valuePath = appendEffectPath(&input, node.valuePath)
			}
		}
		layouts[i] = layout
	}
	var rebased TermRebaseOutput
	var err error
	if directCall {
		rebased, err = rebaseDirectCallTermDAGs(caller.terms, callee.terms, bindings, input)
	} else {
		rebased, err = RebaseTermDAGs(caller.terms, callee.terms, bindings, input)
	}
	if err != nil {
		return EffectRebaseOutput{}, err
	}
	out := EffectRebaseOutput{Effects: make([]EffectTerm, len(effects))}
	for i, term := range effects {
		node := callee.nodes[term]
		layout := layouts[i]
		if node.kind == EffectAllocationTemplate {
			allocation := caller.terms.AllocationTemplate(callee.terms.allocations[node.allocation].op)
			out.Effects[i], err = caller.AllocationTemplate(allocation)
			if err != nil {
				return EffectRebaseOutput{}, fmt.Errorf("transformer: allocation effect %d failed to rebase: %w", i, err)
			}
			continue
		}
		if node.kind == EffectObjectMaterialization {
			object := rebasePathStoreObject(rebased, layout.pathStoreObject, node.pathStoreObject)
			out.Effects[i], err = caller.ObjectMaterialization(object, node.site)
			if err != nil {
				return EffectRebaseOutput{}, fmt.Errorf("transformer: object-materialization effect %d failed to rebase: %w", i, err)
			}
			continue
		}
		if node.kind == EffectPathStore {
			object := rebasePathStoreObject(rebased, layout.pathStoreObject, node.pathStoreObject)
			config := PathStoreConfig{
				HasAssignment: node.pathStoreHasAssignment, HasStatic: node.pathStoreHasStatic,
				StaticHasAnnotation: node.pathStoreStaticHasAnnotation, Object: object, Site: node.site,
			}
			if node.pathStoreHasAssignment {
				config.Assignment = rebasePathStoreWrite(rebased, layout.pathStoreAssignment, node.pathStoreAssignment)
			}
			if node.pathStoreHasStatic {
				config.Static = rebasePathStoreWrite(rebased, layout.pathStoreStatic, node.pathStoreStatic)
			}
			out.Effects[i], err = caller.PathStore(config)
			if err != nil {
				return EffectRebaseOutput{}, fmt.Errorf("transformer: path-store effect %d failed to rebase: %w", i, err)
			}
			continue
		}
		invalidation := InvalidatePathConfig{
			Target: rebaseEffectTarget(caller.terms, callee.terms, rebased, layout.target), Scope: node.invalidation.Scope,
			PreserveStructuralWitness:       node.invalidation.PreserveStructuralWitness,
			PreserveDynamicValueMemberships: node.invalidation.PreserveDynamicValueMemberships,
		}
		if node.invalidation.Precise != nil {
			invalidation.Precise = &PreciseDynamicTarget{
				Table: rebased.Paths[layout.preciseTable], Key: rebased.Values[layout.preciseKey],
				Suffix: append([]segment.Segment(nil), node.invalidation.Precise.Suffix...),
			}
		}
		if node.kind == EffectInvalidatePath {
			out.Effects[i], err = caller.InvalidatePath(invalidation)
		} else {
			config := IndexMutationConfig{
				Invalidation: invalidation, Table: rebaseEffectTarget(caller.terms, callee.terms, rebased, layout.table),
				Key: rebased.Values[layout.key], Value: rebased.Values[layout.value],
				Admission: node.admission, Readback: node.readback,
				Append: node.appendMode, Site: node.site,
			}
			if layout.keyPath >= 0 {
				config.KeyPath = rebased.Paths[layout.keyPath]
			}
			if layout.valuePath >= 0 {
				config.ValuePath = rebased.Paths[layout.valuePath]
			}
			out.Effects[i], err = caller.IndexMutation(config)
		}
		if err != nil {
			return EffectRebaseOutput{}, fmt.Errorf("transformer: validated effect %d failed to rebase: %w", i, err)
		}
	}
	return out, nil
}

func appendPathStoreWriteRebase(input *TermRebaseInput, write PathStoreWriteConfig) pathStoreWriteRebaseLayout {
	layout := pathStoreWriteRebaseLayout{target: appendEffectPath(input, write.Target), value: appendEffectValue(input, write.Value), source: -1}
	if write.HasSourcePath {
		layout.source = appendEffectPath(input, write.SourcePath)
	}
	return layout
}

func rebasePathStoreWrite(rebased TermRebaseOutput, layout pathStoreWriteRebaseLayout, source PathStoreWriteConfig) PathStoreWriteConfig {
	out := PathStoreWriteConfig{Target: rebased.Paths[layout.target], Value: rebased.Values[layout.value], HasSourcePath: source.HasSourcePath, SuppressProof: source.SuppressProof}
	if source.HasSourcePath {
		out.SourcePath = rebased.Paths[layout.source]
	}
	return out
}

func appendPathStoreObjectRebase(input *TermRebaseInput, object PathStoreObjectConfig) pathStoreObjectRebaseLayout {
	layout := pathStoreObjectRebaseLayout{heaps: make([]pathStoreHeapRebaseLayout, len(object.Heaps)), entries: make([]pathStoreEntryRebaseLayout, len(object.Entries))}
	for heapIndex, heap := range object.Heaps {
		heapLayout := pathStoreHeapRebaseLayout{root: appendEffectValue(input, heap.Root), members: make([]pathStoreMemberRebaseLayout, len(heap.Members))}
		for memberIndex, member := range heap.Members {
			memberLayout := pathStoreMemberRebaseLayout{value: appendEffectValue(input, member.Value), expected: -1}
			if member.HasExpected {
				memberLayout.expected = appendEffectValue(input, member.Expected)
			}
			heapLayout.members[memberIndex] = memberLayout
		}
		layout.heaps[heapIndex] = heapLayout
	}
	for entryIndex, entry := range object.Entries {
		entryLayout := pathStoreEntryRebaseLayout{target: appendEffectPath(input, entry.Target), value: appendEffectValue(input, entry.Value), source: -1, expected: -1}
		if entry.HasSourcePath {
			entryLayout.source = appendEffectPath(input, entry.SourcePath)
		}
		if entry.HasExpected {
			entryLayout.expected = appendEffectValue(input, entry.Expected)
		}
		layout.entries[entryIndex] = entryLayout
	}
	return layout
}

func rebasePathStoreObject(rebased TermRebaseOutput, layout pathStoreObjectRebaseLayout, source PathStoreObjectConfig) PathStoreObjectConfig {
	object := PathStoreObjectConfig{Heaps: make([]PathStoreHeapObjectConfig, len(source.Heaps)), Entries: make([]PathStoreObjectEntryConfig, len(source.Entries)), ListFloor: source.ListFloor}
	for heapIndex, heapSource := range source.Heaps {
		heapLayout := layout.heaps[heapIndex]
		heap := PathStoreHeapObjectConfig{Root: rebased.Values[heapLayout.root], Members: make([]PathStoreHeapMemberConfig, len(heapSource.Members)), StableShape: heapSource.StableShape}
		for memberIndex, memberSource := range heapSource.Members {
			memberLayout := heapLayout.members[memberIndex]
			member := PathStoreHeapMemberConfig{Suffix: append([]segment.Segment(nil), memberSource.Suffix...), Value: rebased.Values[memberLayout.value], HasExpected: memberSource.HasExpected}
			if memberSource.HasExpected {
				member.Expected = rebased.Values[memberLayout.expected]
			}
			heap.Members[memberIndex] = member
		}
		object.Heaps[heapIndex] = heap
	}
	for entryIndex, entrySource := range source.Entries {
		entryLayout := layout.entries[entryIndex]
		entry := PathStoreObjectEntryConfig{Target: rebased.Paths[entryLayout.target], Value: rebased.Values[entryLayout.value], HasSourcePath: entrySource.HasSourcePath, SuppressProof: entrySource.SuppressProof, HasExpected: entrySource.HasExpected}
		if entrySource.HasSourcePath {
			entry.SourcePath = rebased.Paths[entryLayout.source]
		}
		if entrySource.HasExpected {
			entry.Expected = rebased.Values[entryLayout.expected]
		}
		object.Entries[entryIndex] = entry
	}
	return object
}

func appendEffectTarget(input *TermRebaseInput, target EffectTargetTerm) effectTargetRebaseLayout {
	layout := effectTargetRebaseLayout{kind: target.kind, path: -1, allocation: target.allocation}
	if target.kind == effectTargetPath {
		layout.path = appendEffectPath(input, target.path)
	}
	return layout
}

func rebaseEffectTarget(caller, callee *Arena, rebased TermRebaseOutput, layout effectTargetRebaseLayout) EffectTargetTerm {
	if layout.kind == effectTargetPath {
		return PathEffectTarget(rebased.Paths[layout.path])
	}
	if layout.kind == effectTargetAllocation && callee.validAllocation(layout.allocation) {
		return AllocationEffectTarget(caller.AllocationTemplate(callee.allocations[layout.allocation].op))
	}
	return EffectTargetTerm{}
}

func appendEffectPath(input *TermRebaseInput, term PathTerm) int {
	index := len(input.Paths)
	input.Paths = append(input.Paths, term)
	return index
}

func appendEffectValue(input *TermRebaseInput, term ValueTerm) int {
	index := len(input.Values)
	input.Values = append(input.Values, term)
	return index
}

func validEffectNodeForRebase(arena *EffectArena, term EffectTerm, shape Shape) bool {
	if !arena.Valid(term, shape) {
		return false
	}
	node := arena.nodes[term]
	if node.kind == EffectAllocationTemplate {
		return arena.terms.validAllocation(node.allocation)
	}
	if node.kind == EffectObjectMaterialization {
		return node.site.Owner != 0
	}
	if node.kind == EffectPathStore {
		return node.site.Owner != 0
	}
	if err := validInvalidationConfig(node.invalidation); err != nil {
		return false
	}
	if node.kind == EffectInvalidatePath {
		return true
	}
	return node.kind == EffectIndexMutation && node.site.Owner != 0 &&
		node.admission != dynamicindex.AdmissionBottom &&
		node.readback >= factflow.DynamicIndexReadbackNone &&
		node.readback <= factflow.DynamicIndexReadbackKeyAndValue
}
