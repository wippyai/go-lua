package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// valueTermFactorAccess derives the complete registered-factor dependency set
// from a frozen ValueTerm DAG. Values name exact slots; non-Values axes are
// selected only through ProductDomain registration. It is the sole authority
// for formal scalar evaluator reads; operations cannot add or omit axes.
func (b *relationProgramBody) valueTermFactorAccess(terms ...ValueTerm) (state.TransferInputAccess, error) {
	return b.valueTermFactorAccessMode(true, terms...)
}

// valueTermLaneFactorAccess is the formal-forest projection of the same
// dependency walk. Formal Values have forest-local FormalSlot identities, so
// this projection asks only for registered non-Values lanes; scalar roots are
// already present in the tuple leaf and must not be forced through a concrete
// state-key spelling.
func (b *relationProgramBody) valueTermLaneFactorAccess(terms ...ValueTerm) (state.TransferInputAccess, error) {
	return b.valueTermFactorAccessMode(false, terms...)
}

func (b *relationProgramBody) valueTermFactorAccessMode(includeValues bool, terms ...ValueTerm) (state.TransferInputAccess, error) {
	if b == nil || b.relation.arena == nil {
		return state.TransferInputAccess{}, fmt.Errorf("transformer: value access has no frozen term arena")
	}
	arena := b.relation.arena
	seen := make(map[ValueTerm]struct{})
	reads := make(map[statekey.Value]struct{})
	lanes := state.NewLaneSet()

	stack := append([]ValueTerm(nil), terms...)
	for len(stack) != 0 {
		term := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if term == 0 || int(term) >= len(arena.values) {
			return state.TransferInputAccess{}, fmt.Errorf("transformer: Values access contains a foreign term")
		}
		if _, done := seen[term]; done {
			continue
		}
		seen[term] = struct{}{}
		node := arena.values[term]
		direct, err := b.valueTermNodeFactorAccessMode(term, includeValues)
		if err != nil {
			return state.TransferInputAccess{}, err
		}
		for _, slot := range direct.Values {
			reads[slot] = struct{}{}
		}
		lanes = lanes.With(direct.Lanes.IDs()...)
		if node.op == valueSelect {
			atoms := make(map[ValueTerm]struct{})
			if err := collectRelationGuardAtoms(arena, node.guard, atoms, make(map[Guard]uint8)); err != nil {
				return state.TransferInputAccess{}, err
			}
			for atom := range atoms {
				stack = append(stack, atom)
			}
		}
		stack = append(stack, node.args...)
	}

	out := make([]statekey.Value, 0, len(reads))
	for slot := range reads {
		out = append(out, slot)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return state.TransferInputAccess{Values: out, Lanes: lanes}, nil
}

// valueTermNodeFactorAccess classifies one ValueTerm node without traversing
// children. This lets historical-wire consumers preserve the compiler's exact
// point assignment while sharing the same neutral axis vocabulary as ordinary
// DAG consumers.
func (b *relationProgramBody) valueTermNodeFactorAccess(term ValueTerm) (state.TransferInputAccess, error) {
	return b.valueTermNodeFactorAccessMode(term, true)
}

func (b *relationProgramBody) valueTermNodeFactorAccessMode(term ValueTerm, includeValues bool) (state.TransferInputAccess, error) {
	if b == nil || b.relation.arena == nil || term == 0 || int(term) >= len(b.relation.arena.values) {
		return state.TransferInputAccess{}, fmt.Errorf("transformer: value access contains a foreign term")
	}
	arena := b.relation.arena
	node := arena.values[term]
	values := make(map[statekey.Value]struct{})
	lanes := state.NewLaneSet()
	add := func(slot statekey.Value) {
		if includeValues && slot != 0 {
			values[slot] = struct{}{}
		}
	}
	addPath := func(term PathTerm) error {
		if term == 0 {
			return nil
		}
		if int(term) >= len(arena.paths) {
			return fmt.Errorf("transformer: value access has a foreign path term")
		}
		path := arena.paths[term]
		if path.environment != 0 {
			add(statekey.SymbolValue(path.environment))
			return nil
		}
		slot, exact := b.rootValueSlot(path.root)
		if !exact {
			if !includeValues {
				return nil
			}
			return fmt.Errorf("transformer: value access path has no sealed storage root")
		}
		add(slot)
		return nil
	}
	switch node.op {
	case valueRoot:
		slot, exact := b.rootValueSlot(node.root)
		if !exact {
			if !includeValues {
				break
			}
			return state.TransferInputAccess{}, fmt.Errorf("transformer: value access term has no sealed storage root")
		}
		add(slot)
	case valueEnvironment:
		if !arena.validEnvironmentSlot(node.slot) {
			return state.TransferInputAccess{}, fmt.Errorf("transformer: value access has an invalid environment slot")
		}
		add(node.slot)
	case valueFrameResult:
		if node.frame == 0 || int(node.frame) >= len(b.frames) || node.resultIndex < 0 {
			return state.TransferInputAccess{}, fmt.Errorf("transformer: value access has a foreign frame result")
		}
		frame := b.frames[node.frame]
		if !frame.valid() || node.resultIndex >= len(frame.resultSelectors) {
			return state.TransferInputAccess{}, fmt.Errorf("transformer: value access has an invalid frame result")
		}
		for _, target := range frame.resultSelectors[node.resultIndex].targets {
			if target.stateTarget && target.slot != 0 {
				add(target.slot)
			}
			if target.stateTarget && target.slot == 0 && target.path.Kind != keyspace.KindInvalid {
				family, ok := b.productDomain.PathValueFamily()
				if !ok {
					return state.TransferInputAccess{}, fmt.Errorf("transformer: value access has no registered path reader")
				}
				lanes = lanes.With(family.Lane().ID())
			}
		}
	case valueDynamicRead, valueDynamicTableRead:
		if err := addPath(node.path); err != nil {
			return state.TransferInputAccess{}, err
		}
		if err := addPath(node.keyPath); err != nil {
			return state.TransferInputAccess{}, err
		}
		if node.op == valueDynamicRead || node.path != 0 {
			family, ok := b.productDomain.PathValueFamily()
			if !ok {
				return state.TransferInputAccess{}, fmt.Errorf("transformer: dynamic value access has no registered path reader")
			}
			lanes = lanes.With(family.Lane().ID())
		}
		dynamic, err := b.productDomain.DynamicReadPotentialLanes()
		if err != nil {
			return state.TransferInputAccess{}, err
		}
		lanes = lanes.With(dynamic.IDs()...)
	case valueConstant, valueJoin, valueObjectLiteral, valueSelect,
		valueRefinement, valueFalsyAbsentRefinement, valueExpressionRefinement,
		valueCellResult, valueCallResult, valueStringConcat, valueUnaryOperation,
		valueBinaryOperation, valueIteratorProjection, valueGenericForResult,
		valueLoopContinuation, valuePredicateObservation, valueStaticIndex,
		valueAllocationResult, valueLuaTypeName:
	default:
		return state.TransferInputAccess{}, fmt.Errorf("transformer: value access has unclassified value op %d", node.op)
	}
	ordered := make([]statekey.Value, 0, len(values))
	for slot := range values {
		ordered = append(ordered, slot)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return state.TransferInputAccess{Values: ordered, Lanes: lanes}, nil
}

// valueTermReadSlots is the Values projection of the canonical factor access.
// Callers that evaluate a term must consume valueTermFactorAccess directly;
// this projection remains only for consumers whose contract is Values-only.
func (b *relationProgramBody) valueTermReadSlots(terms ...ValueTerm) ([]statekey.Value, error) {
	access, err := b.valueTermFactorAccess(terms...)
	if err != nil {
		return nil, err
	}
	return access.Values, nil
}

// freezeFormalValueFactorAccess binds the registered non-Values dependencies
// of a ValueTerm DAG to their formal fiber groups. The ProductDomain inventory
// owns the order; individual consumers neither name axes nor reconstruct a
// product State.
func freezeFormalValueFactorAccess(program *RelationProgram, owner relationVar, terms ...ValueTerm) (state.TransferInputAccess, []formalFiberGroupDescriptor, error) {
	if program == nil || program.formalFibers == nil || owner == 0 || int(owner) > len(program.bodies) {
		return state.TransferInputAccess{}, nil, fmt.Errorf("transformer: formal value factor access is unowned")
	}
	body := &program.bodies[owner-1]
	access, err := body.valueTermLaneFactorAccess(terms...)
	if err != nil {
		return state.TransferInputAccess{}, nil, err
	}
	span, ok := program.formalFibers.span(owner)
	if !ok {
		return state.TransferInputAccess{}, nil, errFormalComponentForeignOwner
	}
	groups := make([]formalFiberGroupDescriptor, 0, access.Lanes.Len())
	for _, lane := range body.productDomain.NonValuesLaneInventory() {
		if !access.Lanes.Has(lane.ID()) {
			continue
		}
		found := false
		for _, group := range span.groupDescriptors() {
			if group.kind != formalFiberGroupValues && group.lane == lane {
				groups = append(groups, group)
				found = true
				break
			}
		}
		if !found {
			return state.TransferInputAccess{}, nil, fmt.Errorf("transformer: formal value lane %q is outside formal fibers", lane.ID())
		}
	}
	if len(groups) != access.Lanes.Len() {
		return state.TransferInputAccess{}, nil, fmt.Errorf("transformer: formal value access has an incomplete registered factor image")
	}
	return access, groups, nil
}
