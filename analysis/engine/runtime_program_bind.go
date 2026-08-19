// runtime_program_bind.go is the program binder. It folds the cold per-member
// answers of a Group's sealed drafts into one binder result and mints the hot
// rows of the sealed program from the same drafts. The binder result is
// consumed by the caller that seals the contribution plan and the demand
// family, and is then dropped: nothing sealed points back at it.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// programExecutor is the closed harvest seam from a binder draft to the sealed
// row's execution closure. It is declared here, not on runtimeMember: the draft
// contract does not grow for the substrate that replaces it, and this seam dies
// with this file once rows are minted at the bind sites themselves.
type programExecutor interface {
	programExec() memberExec
}

// programExec mints the row closure over the concrete bound rule. The closure
// captures the rule, never the draft, so a sealed row keeps no attachment
// alive. A draft that cannot execute mints nothing.
func (bound *boundRuleMember[V, O]) programExec() memberExec {
	if bound == nil || bound.rule == nil {
		return nil
	}
	rule := bound.rule
	return func(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) memberResult {
		patch, reads, wrote, ok, boundary := rule.execute(work, base, inputs, within)
		return memberResult{patch: patch, wrote: wrote, reads: reads, boundary: boundary, valid: ok}
	}
}

func (bound *boundActivationMember) programExec() memberExec {
	if bound == nil || bound.rule == nil {
		return nil
	}
	rule := bound.rule
	return func(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) memberResult {
		selected, reads, ok, phase := rule.execute(work, base, inputs, within)
		return memberResult{activations: selected, reads: reads, boundary: phase, valid: ok}
	}
}

// programMemberExec harvests one already-constructed draft. The generic Rule
// draft cannot be named in a type switch, so the closed set is reached through
// the private single-method seam every concrete draft implements.
func programMemberExec(draft runtimeMember) memberExec {
	executor, harvestable := draft.(programExecutor)
	if !harvestable || executor == nil {
		return nil
	}
	return executor.programExec()
}

// memberFold is one Group's binder result: the ten cold per-member answers
// aggregated once, in graph member order. It is the sole authority for the
// contribution writes/carries, the demand family's read sets, and the
// occurrence recurrence footprint.
type memberFold struct {
	writes       []shape.Slot
	sources      []carrier.ContributionSource
	initialReads []demand.Observation
	dynamicReads []demand.DynamicRead
	carries      []demand.Carry
	footprint    []recurrenceFootprint
	supportPrune bool
}

// foldMemberDrafts aggregates one Group's attached drafts. Targets are
// deduplicated per Factor, and a route write contributes only the Factor owner
// identity, so the footprint never grows as Group x route universe.
func foldMemberDrafts(inputCount int, drafts []runtimeMember) (memberFold, bool) {
	if inputCount < 0 {
		return memberFold{}, false
	}
	fold := memberFold{
		writes:       make([]shape.Slot, 0, len(drafts)),
		sources:      make([]carrier.ContributionSource, 0),
		initialReads: make([]demand.Observation, 0),
		dynamicReads: make([]demand.DynamicRead, 0),
		carries:      make([]demand.Carry, 0),
		footprint:    make([]recurrenceFootprint, 0, len(drafts)),
	}
	// These sets are fold-local deduplication scratch. The published
	// recurrenceFootprint retains authored occurrence targets only; the
	// Factor-owned O(R) route universe remains an owner identity until the
	// active Region binds its one recurrence scope.
	footprintIndexByFactor := make(map[composition.Key]int, len(drafts))
	footprintTargets := make(map[composition.Key]map[carrier.Target]struct{}, len(drafts))
	footprintNarrowTargets := make(map[composition.Key]map[carrier.Target]struct{}, len(drafts))
	appendFootprintTarget := func(index int, target carrier.Target, narrow bool) bool {
		if index < 0 || index >= len(fold.footprint) || !fold.footprint[index].key.Available() {
			return false
		}
		key := fold.footprint[index].key
		seenByFactor := footprintTargets
		if narrow {
			seenByFactor = footprintNarrowTargets
		}
		seen := seenByFactor[key]
		if seen == nil {
			seen = make(map[carrier.Target]struct{})
			seenByFactor[key] = seen
		}
		if _, duplicate := seen[target]; duplicate {
			return true
		}
		seen[target] = struct{}{}
		if narrow {
			fold.footprint[index].narrowTargets = append(fold.footprint[index].narrowTargets, target)
		} else {
			fold.footprint[index].targets = append(fold.footprint[index].targets, target)
		}
		return true
	}
	for _, draft := range drafts {
		if draft == nil {
			return memberFold{}, false
		}
		slot, hasSlot := draft.outputSlot()
		fold.supportPrune = fold.supportPrune || !hasSlot
		fold.initialReads = append(fold.initialReads, draft.initialReads()...)
		fold.dynamicReads = append(fold.dynamicReads, draft.dynamicReads()...)
		if !hasSlot {
			continue
		}
		factor, factorOK := draft.factorKey()
		if !factorOK {
			return memberFold{}, false
		}
		memberCarries := draft.carries()
		// A carrying member publishes both surfaces: its own exact write target
		// and every target its carry closure reaches. The occurrence footprint is
		// their union, so the recurrence scope the active Region seals from it
		// always contains the member's own writes.
		occurrenceTargets := draft.targets()
		if len(memberCarries) != 0 {
			occurrenceTargets = unionRuntimeTargets(occurrenceTargets, draft.carryTargets())
		}
		narrowTargets := draft.narrowTargets()
		for _, target := range narrowTargets {
			if !runtimeContainsTarget(occurrenceTargets, target) {
				return memberFold{}, false
			}
		}
		footprintIndex, present := footprintIndexByFactor[factor]
		if !present {
			fold.footprint = append(fold.footprint, recurrenceFootprint{key: factor})
			footprintIndex = len(fold.footprint) - 1
			footprintIndexByFactor[factor] = footprintIndex
		}
		if routeFactor := draft.routeScope(); routeFactor != nil {
			if compositionKeyOf(routeFactor.semantic()) != factor {
				return memberFold{}, false
			}
			if fold.footprint[footprintIndex].routeFactor != nil && fold.footprint[footprintIndex].routeFactor != routeFactor {
				return memberFold{}, false
			}
			fold.footprint[footprintIndex].routeFactor = routeFactor
			fold.footprint[footprintIndex].route = true
			fold.footprint[footprintIndex].narrowRoute = fold.footprint[footprintIndex].narrowRoute || draft.routeNarrow()
		}
		for _, target := range occurrenceTargets {
			if !appendFootprintTarget(footprintIndex, target, false) {
				return memberFold{}, false
			}
		}
		for _, target := range narrowTargets {
			if !appendFootprintTarget(footprintIndex, target, true) {
				return memberFold{}, false
			}
		}
		if draft.writesOutput() {
			fold.writes = append(fold.writes, slot)
		}
		for _, input := range memberCarries {
			if input < 0 || input >= inputCount {
				return memberFold{}, false
			}
			fold.sources = append(fold.sources, carrier.ContributionSource{Slot: slot, Input: input})
			fold.carries = append(fold.carries, demand.Carry{Input: uint64(input), Slot: slot})
		}
	}
	sort.Slice(fold.footprint, func(left, right int) bool {
		return lessRuntimeKey(fold.footprint[left].key, fold.footprint[right].key)
	})
	return fold, true
}

// bindRuntimeProgram lowers the sealed drafts of one graph into the row-model
// program. It is total over the graph: every Group's members become rows, so
// the sealed program describes the whole compiled program and a later demand
// revision selects from it rather than rebuilding it.
func bindRuntimeProgram(bindingState *schemaBindingState, bindingAuthority *schemaBindingAuthority, graph *equation.Graph, runtime *carrier.Composition, factors map[composition.Key]runtimeFactor, drafts []runtimeMember, queries []runtimeQuery, observations []runtimeObservation) (*runtimeProgram, []memberFold, bool) {
	if bindingState == nil || bindingAuthority == nil || bindingState.phase != schemaBindingSealed || bindingState.authority != bindingAuthority || bindingState.schema == nil || !bindingState.schema.Available() || graph == nil || runtime == nil || runtime.Guards() == nil || factors == nil {
		return nil, nil, false
	}
	if graph.CompositionID() != bindingState.schema.coldID() {
		return nil, nil, false
	}
	records, owners, factorsOK := bindProgramFactorTable(runtime, factors)
	if !factorsOK {
		return nil, nil, false
	}
	byMember := make(map[composition.Key]runtimeMember, len(drafts))
	for _, draft := range drafts {
		if draft == nil {
			return nil, nil, false
		}
		key := draft.member().Key()
		if !key.Available() || byMember[key] != nil {
			return nil, nil, false
		}
		byMember[key] = draft
	}
	rows := make([]memberRow, 0, len(drafts))
	spans := make([]memberSpan, graph.GroupCount())
	folds := make([]memberFold, graph.GroupCount())
	attached := make([]runtimeMember, 0, len(drafts))
	positions := make([]int, 0, len(drafts))
	for index := 0; index < graph.GroupCount(); index++ {
		group, groupOK := graph.HyperedgeAt(index)
		groupIndex, indexed := graph.GroupIndex(group)
		if !groupOK || !indexed || groupIndex != index || !graph.OwnsGroup(group) || !group.Key().Available() {
			return nil, nil, false
		}
		attached, positions = attached[:0], positions[:0]
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK || !member.Key().Available() {
				return nil, nil, false
			}
			draft := byMember[member.Key()]
			if draft == nil || draft.member().Key() != member.Key() || !draft.member().Rule().Available() {
				return nil, nil, false
			}
			// A Rule instance belongs to exactly one compiled Group. Consuming the
			// lookup here proves every supplied draft was attached without a later
			// member x group verification sweep.
			delete(byMember, member.Key())
			attached = append(attached, draft)
			positions = append(positions, memberIndex)
		}
		fold, foldOK := foldMemberDrafts(group.InputCount(), attached)
		if !foldOK {
			return nil, nil, false
		}
		folds[index] = fold
		// Rows are ordered by canonical member key, exactly as the assembled
		// producer orders its members, while each row keeps the graph position it
		// came from so its identity stays recoverable from the Group.
		sort.Slice(positions, func(left, right int) bool {
			return lessRuntimeKey(attached[positions[left]].member().Key(), attached[positions[right]].member().Key())
		})
		start := int32(len(rows))
		for _, position := range positions {
			draft := attached[position]
			exec := programMemberExec(draft)
			if exec == nil {
				return nil, nil, false
			}
			slot, hasSlot := draft.outputSlot()
			rows = append(rows, memberRow{exec: exec, outputSlot: slot, memberIndex: int32(position), hasSlot: hasSlot})
		}
		spans[index] = memberSpan{start: start, end: int32(len(rows))}
	}
	if len(byMember) != 0 {
		return nil, nil, false
	}
	program, sealed := sealRuntimeProgram(rows, spans, records, owners, append([]runtimeQuery(nil), queries...), append([]runtimeObservation(nil), observations...))
	if !sealed {
		return nil, nil, false
	}
	return program, folds, true
}

// assembleProgramRuntime is the one entry from attached drafts to an
// executable runtime. It seals the program first and assembles from it, so the
// drafts are unreachable the moment this call returns.
func assembleProgramRuntime(state *schemaBindingState, authority *schemaBindingAuthority, graph *equation.Graph, runtime *carrier.Composition, factors map[composition.Key]runtimeFactor, drafts []runtimeMember, queries []runtimeQuery, observations []runtimeObservation) (*solverRuntime, bool) {
	program, folds, bound := bindRuntimeProgram(state, authority, graph, runtime, factors, drafts, queries, observations)
	if !bound {
		return nil, false
	}
	return assembleRuntimeOwned(state, authority, graph, runtime, program, folds)
}

// bindProgramFactorTable orders the bound Factors by canonical key so the dense
// owner index is a property of the program rather than of a map walk.
func bindProgramFactorTable(runtime *carrier.Composition, factors map[composition.Key]runtimeFactor) ([]factorRecord, []runtimeFactor, bool) {
	keys := make([]composition.Key, 0, len(factors))
	for key := range factors {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return lessRuntimeKey(keys[left], keys[right]) })
	records := make([]factorRecord, 0, len(keys))
	owners := make([]runtimeFactor, 0, len(keys))
	for _, key := range keys {
		factor := factors[key]
		slot, slotOK := shape.Slot(0), false
		if factor != nil {
			slot, slotOK = factor.runtimeSlot()
		}
		if !key.Available() || factor == nil || compositionKeyOf(factor.semantic()) != key || !slotOK || slot < 0 || int(slot) >= runtime.Count() {
			return nil, nil, false
		}
		records = append(records, factorRecord{key: key, slot: slot, owner: int32(len(owners))})
		owners = append(owners, factor)
	}
	return records, owners, true
}
