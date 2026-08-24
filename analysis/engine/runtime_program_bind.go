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
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// memberFold is one Group's binder result: the ten cold per-member answers
// aggregated once, in graph member order. It is the sole authority for the
// contribution writes/carries, the demand family's read sets, and the
// occurrence recurrence footprint.
type memberFold struct {
	writes  []shape.Slot
	sources []carrier.ContributionSource
	// carryExclusions is transient seal input. Each member pairs its own carry
	// source with only its direct StrongTarget output rows; recurrence footprint
	// is deliberately unrelated and is never reused for this mask.
	carryExclusions map[carrier.ContributionSource][]carrier.Target
	initialReads    []demand.Observation
	dynamicReads    []demand.DynamicRead
	carries         []demand.Carry
	footprint       []recurrenceFootprint
}

// foldMemberDrafts aggregates one Group's attached drafts. Targets are
// deduplicated per Factor, and a route write contributes only the Factor owner
// identity, so the footprint never grows as Group x route universe.
func foldMemberDrafts(inputCount int, drafts []memberRow) (memberFold, bool) {
	if inputCount < 0 {
		return memberFold{}, false
	}
	fold := memberFold{
		writes:          make([]shape.Slot, 0, len(drafts)),
		sources:         make([]carrier.ContributionSource, 0),
		carryExclusions: make(map[carrier.ContributionSource][]carrier.Target),
		initialReads:    make([]demand.Observation, 0),
		dynamicReads:    make([]demand.DynamicRead, 0),
		carries:         make([]demand.Carry, 0),
		footprint:       make([]recurrenceFootprint, 0, len(drafts)),
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
		geometry, geometryOK := draft.geometry()
		if !geometryOK || geometry == nil || !draft.valid() {
			return memberFold{}, false
		}
		slot, hasSlot := geometry.outputSlot()
		fold.initialReads = append(fold.initialReads, geometry.initialReads()...)
		fold.dynamicReads = append(fold.dynamicReads, geometry.dynamicReads()...)
		if !hasSlot {
			continue
		}
		factor, factorOK := geometry.factorKey()
		if !factorOK {
			return memberFold{}, false
		}
		memberCarries := geometry.carries()
		// A carrying member publishes both surfaces: its own exact write target
		// and every target its carry closure reaches. The occurrence footprint is
		// their union, so the recurrence scope the active Region seals from it
		// always contains the member's own writes.
		occurrenceTargets := geometry.targets()
		if len(memberCarries) != 0 {
			occurrenceTargets = unionRuntimeTargets(occurrenceTargets, geometry.carryTargets())
		}
		narrowTargets := geometry.narrowTargets()
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
		if routeFactor := geometry.routeScope(); routeFactor != nil {
			if compositionKeyOf(routeFactor.semantic()) != factor {
				return memberFold{}, false
			}
			if fold.footprint[footprintIndex].routeFactor != nil && fold.footprint[footprintIndex].routeFactor != routeFactor {
				return memberFold{}, false
			}
			fold.footprint[footprintIndex].routeFactor = routeFactor
			fold.footprint[footprintIndex].route = true
			fold.footprint[footprintIndex].narrowRoute = fold.footprint[footprintIndex].narrowRoute || geometry.routeNarrow()
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
		if geometry.writesOutput() {
			fold.writes = append(fold.writes, slot)
		}
		for _, input := range memberCarries {
			if input < 0 || input >= inputCount {
				return memberFold{}, false
			}
			source := carrier.ContributionSource{Slot: slot, Input: input}
			fold.sources = append(fold.sources, source)
			// A member's direct output targets are the only targets its carry
			// may mask. Route/carry closure targets are not strong direct
			// writes and therefore never enter this exclusion relation.
			for _, target := range geometry.targets() {
				if target.Mode() != carrier.StrongTarget {
					continue
				}
				seen := false
				for _, existing := range fold.carryExclusions[source] {
					if existing.Same(target) {
						seen = true
						break
					}
				}
				if !seen {
					fold.carryExclusions[source] = append(fold.carryExclusions[source], target)
				}
			}
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
func bindRuntimeProgram(schema *Schema, graph *equation.Graph, runtime *carrier.Composition, factors map[composition.Key]runtimeFactor, drafts []memberRow, queries []queryRow, observations []observationRow, contexts executioncontext.Directory, contextIndex contextfiber.Index, contextLayout contextfiber.Layout, pointOwners []contextfiber.PointOwner, artifactBacked bool) (*runtimeProgram, []memberFold, topologyConstructionRefusal, bool) {
	if schema == nil || !schema.Available() || graph == nil || runtime == nil || runtime.Guards() == nil || factors == nil {
		return nil, nil, refuseProgramSeal(topologyConstructionStepDeclarationShape), false
	}
	if graph.CompositionID() != schema.coldID() {
		return nil, nil, refuseProgramSeal(topologyConstructionStepDeclarationShape), false
	}
	records, owners, factorsOK := bindProgramFactorTable(schema, runtime, factors)
	if !factorsOK {
		return nil, nil, refuseProgramSeal(topologyConstructionStepBinding), false
	}
	byMember := make(map[composition.Key]memberRow, len(drafts))
	for _, draft := range drafts {
		geometry, geometryOK := draft.geometry()
		if !geometryOK || geometry == nil {
			return nil, nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		key := geometry.member().Key()
		if !key.Available() {
			return nil, nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		if _, duplicate := byMember[key]; duplicate {
			return nil, nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		byMember[key] = draft
	}
	rows := make([]memberRow, 0, len(drafts))
	spans := make([]memberSpan, graph.GroupCount())
	folds := make([]memberFold, graph.GroupCount())
	attached := make([]memberRow, 0, len(drafts))
	positions := make([]int, 0, len(drafts))
	for index := 0; index < graph.GroupCount(); index++ {
		group, groupOK := graph.HyperedgeAt(index)
		groupIndex, indexed := graph.GroupIndex(group)
		if !groupOK || !indexed || groupIndex != index || !graph.OwnsGroup(group) || !group.Key().Available() {
			return nil, nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		attached, positions = attached[:0], positions[:0]
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK || !member.Key().Available() {
				return nil, nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
			draft, draftPresent := byMember[member.Key()]
			geometry, geometryOK := draft.geometry()
			if !draftPresent || !geometryOK || geometry == nil || geometry.member().Key() != member.Key() || !geometry.member().Rule().Available() {
				return nil, nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
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
			return nil, nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		folds[index] = fold
		// Rows are ordered by canonical member key, exactly as the assembled
		// producer orders its members, while each row keeps the graph position it
		// came from so its identity stays recoverable from the Group.
		sort.Slice(positions, func(left, right int) bool {
			leftGeometry, _ := attached[positions[left]].geometry()
			rightGeometry, _ := attached[positions[right]].geometry()
			return lessRuntimeKey(leftGeometry.member().Key(), rightGeometry.member().Key())
		})
		start := int32(len(rows))
		for _, position := range positions {
			rows = append(rows, attached[position])
		}
		spans[index] = memberSpan{start: start, end: int32(len(rows))}
	}
	if len(byMember) != 0 {
		return nil, nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
	}
	program, refusal, sealed := sealRuntimeProgram(schema, graph, runtime, rows, spans, records, owners, append([]queryRow(nil), queries...), append([]observationRow(nil), observations...), contexts, contextIndex, contextLayout, pointOwners, artifactBacked)
	if !sealed {
		return nil, nil, refusal, false
	}
	return program, folds, topologyConstructionRefusal{}, true
}

// assembleProgramRuntime is the one entry from attached drafts to an
// executable runtime. It seals the program first and retains each canonical
// runtime member only through the row's direct execute method value.
func assembleProgramRuntime(schema *Schema, graph *equation.Graph, runtime *carrier.Composition, factors map[composition.Key]runtimeFactor, drafts []memberRow, queries []queryRow, observations []observationRow, contexts executioncontext.Directory, contextIndex contextfiber.Index, contextLayout contextfiber.Layout, pointOwners []contextfiber.PointOwner, pointTransitions []ProgramPointTransition, artifactBacked bool) (*solverRuntime, topologyConstructionRefusal, bool) {
	program, folds, refusal, bound := bindRuntimeProgram(schema, graph, runtime, factors, drafts, queries, observations, contexts, contextIndex, contextLayout, pointOwners, artifactBacked)
	if !bound {
		return nil, refusal, false
	}
	// Owning the assembled runtime is the schedule half of the seal: the
	// program's tables are already proved consistent, and what remains is the
	// solver runtime built over them.
	assembled, owned := assembleRuntimeOwned(graph, runtime, program, folds, contexts, contextIndex, contextLayout, pointOwners, pointTransitions, artifactBacked)
	if !owned {
		return nil, refuseProgramSeal(topologyConstructionStepSchedule), false
	}
	return assembled, topologyConstructionRefusal{}, true
}

// bindProgramFactorTable places every Factor at its Schema ordinal. This makes
// the program's factor ordinal a direct address shared by query rows, rule
// rows, and the sealed Schema instead of an independently sorted owner table.
func bindProgramFactorTable(schema *Schema, runtime *carrier.Composition, factors map[composition.Key]runtimeFactor) ([]factorRecord, []runtimeFactor, bool) {
	if schema == nil || !schema.Available() || runtime == nil || len(factors) != schemaFactorCount(schema) {
		return nil, nil, false
	}
	records := make([]factorRecord, schemaFactorCount(schema))
	owners := make([]runtimeFactor, schemaFactorCount(schema))
	for ordinal := range records {
		key := schema.factorSemanticAt(uint64(ordinal))
		factor := factors[key]
		slot, slotOK := shape.Slot(0), false
		if factor != nil {
			slot, slotOK = factor.runtimeSlot()
		}
		if !key.Available() || factor == nil || compositionKeyOf(factor.semantic()) != key || !slotOK || slot < 0 || int(slot) >= runtime.Count() {
			return nil, nil, false
		}
		if ordinal > 0 && !lessRuntimeKey(records[ordinal-1].key, key) {
			return nil, nil, false
		}
		records[ordinal] = factorRecord{key: key, slot: slot, owner: int32(ordinal)}
		owners[ordinal] = factor
	}
	return records, owners, true
}
