package transformer

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalRootInputTemplate is the immutable parametric input declaration for
// one lexical relation. It is syntax metadata, not a formalRelationTuple: no
// caller value, State, registered factor, or physical Bottom is retained.
// The sole WTO executor will later bind these references for one application.
type formalRootInputTemplate struct {
	program  *RelationProgram
	variable relationVar
	care     bool
	bindings []formalRootInputBinding
	groups   []formalInputGroupRef
	// entrySeeds is the canonical missing-only Values law rebound once from
	// concrete lexical slots into this body's FormalSlot vocabulary.  The
	// members are its exact sparse physical selection in catalog order.
	entrySeeds        state.EntrySeedFactorPlan[FormalSlot]
	entrySeedOrdinals []formalFiberOrdinal
	entrySeedSlots    []FormalSlot
}

// formalRootInputBinding is one atomic IN -> MID binding. Value and optional
// path syntax travel together so an imprecise choice can never cross-pair a
// value from one alternative with the address of another.
type formalRootInputBinding struct {
	arena       *Arena
	middle      Root
	input       Root
	middleValue ValueTerm
	inputValue  ValueTerm
	// hasPath means this binding carries one optional path variable. The term
	// is present in syntax even when a later application binds it to no path.
	hasPath    bool
	middlePath PathTerm
	inputPath  PathTerm
}

func (b formalRootInputBinding) valid(arena *Arena, shape Shape) bool {
	if arena == nil || b.arena != arena || !arena.Sealed() ||
		!arena.middle.validRoot(b.middle) || !shape.validateInput(b.input) ||
		b.middleValue == 0 || b.inputValue == 0 || !b.hasPath || b.middlePath == 0 || b.inputPath == 0 {
		return false
	}
	register, ok := arena.middle.register(b.middle)
	if !ok || register.kind != relationMiddleRegisterSymbol {
		return false
	}
	return int(b.middleValue) < len(arena.values) && arena.values[b.middleValue].op == valueRoot && arena.values[b.middleValue].root == b.middle &&
		int(b.inputValue) < len(arena.values) && arena.values[b.inputValue].op == valueRoot && arena.values[b.inputValue].root == b.input &&
		int(b.middlePath) < len(arena.paths) && arena.paths[b.middlePath].root == b.middle && len(arena.paths[b.middlePath].segments) == 0 &&
		int(b.inputPath) < len(arena.paths) && arena.paths[b.inputPath].root == b.input && len(arena.paths[b.inputPath].segments) == 0
}

// existingRootValue and existingRootPath are read-only sealed-syntax lookups.
// Root input freeze is forbidden from calling Arena constructors, even when a
// hash-cons cache hit would normally avoid growth: absence must fail closed,
// never be repaired after the constructor fence.
func existingRootValue(arena *Arena, root Root) (ValueTerm, bool) {
	if arena == nil || !arena.Sealed() {
		return 0, false
	}
	var found ValueTerm
	for term := ValueTerm(1); int(term) < len(arena.values); term++ {
		node := arena.values[term]
		if node.op != valueRoot || node.root != root {
			continue
		}
		if found != 0 {
			return 0, false
		}
		found = term
	}
	return found, found != 0
}

func existingRootPath(arena *Arena, root Root) (PathTerm, bool) {
	if arena == nil || !arena.Sealed() {
		return 0, false
	}
	var found PathTerm
	for term := PathTerm(1); int(term) < len(arena.paths); term++ {
		node := arena.paths[term]
		if node.root != root || node.environment != 0 || len(node.segments) != 0 {
			continue
		}
		if found != 0 {
			return 0, false
		}
		found = term
	}
	return found, found != 0
}

type formalInputGroupKind uint8

const (
	formalInputGroupInvalid formalInputGroupKind = iota
	formalInputGroupOrdinaryLane
	formalInputGroupCoordinateLane
	formalInputGroupValues
)

// formalInputGroupRef names one complete registered carrier in the frozen
// formal inventory. Every ProductLane, including a one-member ordinary lane,
// is addressed through the same dependent-group identity. It contains no
// factor value.
type formalInputGroupRef struct {
	forest      *formalFiberInventory
	variable    relationVar
	kind        formalInputGroupKind
	lane        state.LaneOrdinal
	groupGlobal int
}

func (r formalInputGroupRef) valid() bool {
	if r.forest == nil || r.variable == 0 {
		return false
	}
	span, ok := r.forest.span(r.variable)
	if !ok {
		return false
	}
	switch r.kind {
	case formalInputGroupOrdinaryLane, formalInputGroupCoordinateLane, formalInputGroupValues:
		if r.groupGlobal < span.groupFirst || r.groupGlobal >= span.groupFirst+span.groupCount {
			return false
		}
		group := r.forest.groups[r.groupGlobal]
		if !group.valid() || group.variable != r.variable || group.lane.Ordinal() != r.lane {
			return false
		}
		return r.kind == formalInputGroupOrdinaryLane && group.kind == formalFiberGroupOrdinaryLane ||
			r.kind == formalInputGroupCoordinateLane && group.kind == formalFiberGroupCoordinateLane ||
			r.kind == formalInputGroupValues && group.kind == formalFiberGroupValues
	default:
		return false
	}
}

func formalRootInputBody(program *RelationProgram, variable relationVar) (*relationProgramBody, bool) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) {
		return nil, false
	}
	body := &program.bodies[variable-1]
	return body, body.variable == variable
}

func freezeFormalRootInputTemplates(program *RelationProgram) ([]formalRootInputTemplate, error) {
	if program == nil || program.formalFibers == nil || len(program.bodies) == 0 {
		return nil, fmt.Errorf("transformer: formal root inputs have no frozen forest")
	}
	out := make([]formalRootInputTemplate, len(program.bodies))
	for index := range program.bodies {
		variable := relationVar(index + 1)
		root, err := freezeFormalRootInputTemplate(program, variable)
		if err != nil {
			return nil, fmt.Errorf("transformer: freeze formal root input %d: %w", variable, err)
		}
		out[index] = root
	}
	return out, nil
}

func freezeFormalRootInputTemplate(program *RelationProgram, variable relationVar) (formalRootInputTemplate, error) {
	body, ok := formalRootInputBody(program, variable)
	if !ok || body.relation.arena == nil || !body.relation.arena.Sealed() ||
		body.relation.code == nil || !body.relation.code.sealed || body.productDomain.Registry() != program.registry {
		return formalRootInputTemplate{}, fmt.Errorf("root input has no sealed lexical owner")
	}
	arena := body.relation.arena
	shape := body.relation.shape
	entries := arena.middle.entries
	if len(entries) != shape.InputCount() {
		return formalRootInputTemplate{}, fmt.Errorf("root input bindings cover %d/%d boundary roots", len(entries), shape.InputCount())
	}
	bindings := make([]formalRootInputBinding, len(entries))
	seenMiddle := make(map[Root]struct{}, len(entries))
	seenInput := make(map[Root]struct{}, len(entries))
	for index, entry := range entries {
		if _, duplicate := seenMiddle[entry.middle]; duplicate {
			return formalRootInputTemplate{}, fmt.Errorf("root input has duplicate Middle binding")
		}
		if _, duplicate := seenInput[entry.input]; duplicate {
			return formalRootInputTemplate{}, fmt.Errorf("root input has duplicate boundary binding")
		}
		middleValue, middleValueOK := existingRootValue(arena, entry.middle)
		inputValue, inputValueOK := existingRootValue(arena, entry.input)
		middlePath, middlePathOK := existingRootPath(arena, entry.middle)
		inputPath, inputPathOK := existingRootPath(arena, entry.input)
		if !middleValueOK || !inputValueOK || !middlePathOK || !inputPathOK {
			return formalRootInputTemplate{}, fmt.Errorf("root input binding %d has no pre-interned value/path identity", index)
		}
		binding := formalRootInputBinding{
			arena: arena, middle: entry.middle, input: entry.input,
			middleValue: middleValue, inputValue: inputValue,
			hasPath: true, middlePath: middlePath, inputPath: inputPath,
		}
		if !binding.valid(arena, shape) {
			return formalRootInputTemplate{}, fmt.Errorf("root input binding %d is malformed", index)
		}
		seenMiddle[entry.middle], seenInput[entry.input] = struct{}{}, struct{}{}
		bindings[index] = binding
	}
	for _, kind := range []struct {
		kind  RootKind
		count uint32
	}{{RootParam, shape.Params}, {RootCapture, shape.Captures}, {RootGlobal, shape.Globals}, {RootAmbient, shape.Ambients}} {
		for ordinal := uint32(0); ordinal < kind.count; ordinal++ {
			if _, present := seenInput[Root{Kind: kind.kind, Index: ordinal}]; !present {
				return formalRootInputTemplate{}, fmt.Errorf("root input omits boundary root %d/%d", kind.kind, ordinal)
			}
		}
	}

	groups, err := freezeFormalInputGroupRefs(program.formalFibers, body)
	if err != nil {
		return formalRootInputTemplate{}, err
	}
	entrySeeds, err := state.BindEntrySeedFactorPlan(program.registry, body.entrySeedPlan, func(slot statekey.Value) (FormalSlot, bool) {
		return formalInitialValueSlot(program.formalFibers.slots, body, slot)
	})
	if err != nil {
		return formalRootInputTemplate{}, fmt.Errorf("root input EntrySeed rekey: %w", err)
	}
	span, present := program.formalFibers.span(variable)
	if !present {
		return formalRootInputTemplate{}, fmt.Errorf("root input EntrySeed has no formal span")
	}
	values, present := span.valuesGroup()
	if !present {
		return formalRootInputTemplate{}, fmt.Errorf("root input EntrySeed has no Values carrier")
	}
	seedSet := make(map[FormalSlot]struct{}, entrySeeds.Len())
	for _, slot := range entrySeeds.Slots() {
		seedSet[slot] = struct{}{}
	}
	seedOrdinals := make([]formalFiberOrdinal, 0, len(seedSet)+1)
	seedSlots := make([]FormalSlot, 0, len(seedSet)+1)
	top, present := values.top()
	if !present {
		return formalRootInputTemplate{}, fmt.Errorf("root input EntrySeed has no Values Top")
	}
	topOrdinal, present := top.address(values.descriptor)
	if !present {
		return formalRootInputTemplate{}, fmt.Errorf("root input EntrySeed Values Top is malformed")
	}
	seedOrdinals = append(seedOrdinals, topOrdinal)
	seedSlots = append(seedSlots, FormalSlot{})
	for _, slot := range values.descriptor.valueSlots {
		if _, selected := seedSet[slot.slot]; !selected {
			continue
		}
		member, memberOK := values.slot(slot.slot)
		if !memberOK {
			return formalRootInputTemplate{}, fmt.Errorf("root input EntrySeed slot is outside Values")
		}
		ordinal, ordinalOK := member.address(values.descriptor)
		if !ordinalOK {
			return formalRootInputTemplate{}, fmt.Errorf("root input EntrySeed slot has no physical address")
		}
		seedOrdinals = append(seedOrdinals, ordinal)
		seedSlots = append(seedSlots, slot.slot)
		delete(seedSet, slot.slot)
	}
	if len(seedSet) != 0 {
		return formalRootInputTemplate{}, fmt.Errorf("root input EntrySeed leaves %d slots outside Values", len(seedSet))
	}
	root := formalRootInputTemplate{
		program: program, variable: variable, care: true,
		bindings: bindings, groups: groups, entrySeeds: entrySeeds,
		entrySeedOrdinals: seedOrdinals, entrySeedSlots: seedSlots,
	}
	if !root.valid() {
		return formalRootInputTemplate{}, fmt.Errorf("root input template is incomplete")
	}
	return root, nil
}

func freezeFormalInputGroupRefs(forest *formalFiberInventory, body *relationProgramBody) ([]formalInputGroupRef, error) {
	if forest == nil || body == nil || !body.productDomain.Valid() {
		return nil, fmt.Errorf("root input groups have no registered product owner")
	}
	span, ok := forest.span(body.variable)
	if !ok {
		return nil, fmt.Errorf("root input groups have no formal span")
	}
	if err := validateFormalFiberProductGroups(span, body.productDomain); err != nil {
		return nil, err
	}
	lanes := body.productDomain.LaneInventory()
	out := make([]formalInputGroupRef, 0, len(lanes))
	seenLanes := make(map[state.LaneOrdinal]struct{}, len(lanes))
	seenGroups := make(map[int]struct{}, span.groupCount)
	for _, lane := range lanes {
		if _, duplicate := seenLanes[lane.Ordinal()]; duplicate {
			return nil, fmt.Errorf("root input has duplicate registered lane %q", lane.ID())
		}
		seenLanes[lane.Ordinal()] = struct{}{}
		if values, enabled := body.productDomain.SlotFactoredCarrier(); enabled && lane == values {
			group, present := span.valuesGroup()
			if !present || !group.valid() || group.descriptor.lane != lane {
				return nil, fmt.Errorf("root input omits Values lane %q", lane.ID())
			}
			ref := formalInputGroupRef{forest: forest, variable: body.variable, kind: formalInputGroupValues, lane: lane.Ordinal(), groupGlobal: group.descriptor.global}
			if !ref.valid() {
				return nil, fmt.Errorf("root input Values lane %q is foreign", lane.ID())
			}
			seenGroups[ref.groupGlobal] = struct{}{}
			out = append(out, ref)
			continue
		}
		families, err := body.productDomain.CoordinateFamilies(lane)
		if err != nil {
			return nil, err
		}
		if len(families) != 0 {
			group, present := span.coordinateLaneGroup(lane)
			if !present || !group.valid() {
				return nil, fmt.Errorf("root input omits coordinate lane %q", lane.ID())
			}
			ref := formalInputGroupRef{forest: forest, variable: body.variable, kind: formalInputGroupCoordinateLane, lane: lane.Ordinal(), groupGlobal: group.descriptor.global}
			if !ref.valid() {
				return nil, fmt.Errorf("root input coordinate lane %q is foreign", lane.ID())
			}
			if _, duplicate := seenGroups[ref.groupGlobal]; duplicate {
				return nil, fmt.Errorf("root input duplicates coordinate lane %q", lane.ID())
			}
			seenGroups[ref.groupGlobal] = struct{}{}
			out = append(out, ref)
			continue
		}
		group, present := span.ordinaryLaneGroup(lane)
		if !present || !group.valid() {
			return nil, fmt.Errorf("root input omits ordinary lane %q", lane.ID())
		}
		ref := formalInputGroupRef{forest: forest, variable: body.variable, kind: formalInputGroupOrdinaryLane, lane: lane.Ordinal(), groupGlobal: group.descriptor.global}
		if !ref.valid() {
			return nil, fmt.Errorf("root input ordinary lane %q is foreign", lane.ID())
		}
		seenGroups[ref.groupGlobal] = struct{}{}
		out = append(out, ref)
	}
	for groupIndex := span.groupFirst; groupIndex < span.groupFirst+span.groupCount; groupIndex++ {
		if _, present := seenGroups[groupIndex]; !present {
			return nil, fmt.Errorf("root input leaves product group %d unowned", groupIndex-span.groupFirst)
		}
	}
	return out, nil
}

func (t *formalRootInputTemplate) valid() bool {
	if t == nil || !t.care || t.program == nil {
		return false
	}
	body, ok := formalRootInputBody(t.program, t.variable)
	if !ok || len(t.bindings) != body.relation.shape.InputCount() {
		return false
	}
	if !t.entrySeeds.Valid() || len(t.entrySeedOrdinals) == 0 || len(t.entrySeedSlots) != len(t.entrySeedOrdinals) {
		return false
	}
	seenInputs := make(map[Root]struct{}, len(t.bindings))
	seenMiddle := make(map[Root]struct{}, len(t.bindings))
	for _, binding := range t.bindings {
		if !binding.valid(body.relation.arena, body.relation.shape) {
			return false
		}
		if _, duplicate := seenInputs[binding.input]; duplicate {
			return false
		}
		if _, duplicate := seenMiddle[binding.middle]; duplicate {
			return false
		}
		seenInputs[binding.input], seenMiddle[binding.middle] = struct{}{}, struct{}{}
	}
	expectedGroups, err := freezeFormalInputGroupRefs(t.program.formalFibers, body)
	if err != nil || len(t.groups) != len(expectedGroups) {
		return false
	}
	seenGroups := make(map[[2]int]struct{}, len(t.groups))
	for index, group := range t.groups {
		if !group.valid() || group.forest != t.program.formalFibers || group.variable != t.variable {
			return false
		}
		expected := expectedGroups[index]
		if group.kind != expected.kind || group.lane != expected.lane || group.groupGlobal != expected.groupGlobal {
			return false
		}
		identity := [2]int{group.groupGlobal, int(group.lane)}
		if _, duplicate := seenGroups[identity]; duplicate {
			return false
		}
		seenGroups[identity] = struct{}{}
	}
	span, present := t.program.formalFibers.span(t.variable)
	values, valuesPresent := span.valuesGroup()
	if !present || !valuesPresent {
		return false
	}
	wantSlots := make(map[FormalSlot]struct{}, t.entrySeeds.Len())
	for _, slot := range t.entrySeeds.Slots() {
		wantSlots[slot] = struct{}{}
	}
	var prior formalFiberOrdinal
	for index, ordinal := range t.entrySeedOrdinals {
		if index > 0 && ordinal <= prior {
			return false
		}
		prior = ordinal
		if index == 0 {
			top, topOK := values.top()
			topOrdinal, addressOK := top.address(values.descriptor)
			if !topOK || !addressOK || ordinal != topOrdinal || t.entrySeedSlots[index].Valid() {
				return false
			}
			continue
		}
		slot := t.entrySeedSlots[index]
		member, memberOK := values.slot(slot)
		memberOrdinal, addressOK := member.address(values.descriptor)
		if !memberOK || !addressOK || memberOrdinal != ordinal {
			return false
		}
		if _, selected := wantSlots[slot]; !selected {
			return false
		}
		delete(wantSlots, slot)
	}
	if len(wantSlots) != 0 {
		return false
	}
	return true
}
