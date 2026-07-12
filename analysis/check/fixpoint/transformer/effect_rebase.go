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
	target, preciseTable, table, keyPath, valuePath int
	preciseKey, key, value                          int
}

func emptyEffectRebaseLayout() effectRebaseLayout {
	return effectRebaseLayout{
		target: -1, preciseTable: -1, table: -1, keyPath: -1, valuePath: -1,
		preciseKey: -1, key: -1, value: -1,
	}
}

// RebaseEffectDAGs transactionally imports an ordered effect sequence. All
// scalar/path dependencies are validated and rebased in one TermRebaseDAGs
// transaction before any destination EffectTerm is interned.
func RebaseEffectDAGs(caller, callee *EffectArena, bindings TermRootBindings, effects []EffectTerm) (EffectRebaseOutput, error) {
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
		layout.target = appendEffectPath(&input, node.invalidation.Target)
		if node.invalidation.Precise != nil {
			layout.preciseTable = appendEffectPath(&input, node.invalidation.Precise.Table)
			layout.preciseKey = appendEffectValue(&input, node.invalidation.Precise.Key)
		}
		if node.kind == EffectIndexMutation {
			layout.table = appendEffectPath(&input, node.table)
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
	rebased, err := RebaseTermDAGs(caller.terms, callee.terms, bindings, input)
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
		invalidation := InvalidatePathConfig{
			Target: rebased.Paths[layout.target], Scope: node.invalidation.Scope,
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
				Invalidation: invalidation, Table: rebased.Paths[layout.table],
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
