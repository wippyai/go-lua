package callpayload

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// CallOutcomeSiteShape is the complete freeze-time declaration of one
// provider's site-specialized surface. Correlations contain structural
// addresses only; abstract target values remain execution-time payload.
type CallOutcomeSiteShape struct {
	FieldNames               []string
	InputLanes               state.LaneSet
	TypestateResourceQueries []state.TypestateResourceQuery
	Correlations             []CallOutcomeCorrelationShape
	// InputObservation is the leaf-owned authority for CallOutcomeInput value
	// roles.  It is deliberately separate from output fields and proof seeds:
	// neither can prove which source values an evaluator may observe.  A zero
	// observation inherits the program's sealed default; use
	// ObserveNoCallOutcomeOperands for an explicit empty declaration.
	InputObservation CallOutcomeInputObservation
	// ProofSeeds are the provider leaf's finite declarations for normal-return
	// proof facts which may require sibling return coordinates. They are not a
	// capability field: composition retains them at the declaring leaf so an
	// occurrence can bind the declaration without borrowing another provider's
	// authority.
	ProofSeeds []CallOutcomeProofSeed
}

// CallOutcomeInputObservation is an exhaustive, immutable declaration of the
// value roles an evaluator may inspect.  Lanes and keyed queries remain in the
// capability because their identities are supplied by the State domain; this
// declaration controls the source-term/read-point authority which cannot be
// inferred from an evaluator callback.
type CallOutcomeInputObservation struct {
	declared     bool
	callee       bool
	receiver     bool
	arguments    []int
	allArguments bool
}

// ObserveCallOutcomeOperands declares exactly the selected source roles.  A
// negative argument ordinal is rejected while sealing the site program.
func ObserveCallOutcomeOperands(callee, receiver bool, arguments ...int) CallOutcomeInputObservation {
	return CallOutcomeInputObservation{
		declared: true, callee: callee, receiver: receiver,
		arguments: append([]int(nil), arguments...),
	}
}

// ObserveAllCallOutcomeOperands is the compatibility-safe declaration for a
// provider that genuinely observes the complete call operand tuple.
func ObserveAllCallOutcomeOperands() CallOutcomeInputObservation {
	return CallOutcomeInputObservation{declared: true, callee: true, receiver: true, allArguments: true}
}

// ObserveNoCallOutcomeOperands explicitly declares a provider which reads no
// value role.  It differs from the zero value, which inherits the program
// default so existing providers remain source compatible.
func ObserveNoCallOutcomeOperands() CallOutcomeInputObservation {
	return CallOutcomeInputObservation{declared: true}
}

func (o CallOutcomeInputObservation) canonical() (CallOutcomeInputObservation, error) {
	if !o.declared {
		return o, nil
	}
	out := o
	out.arguments = append([]int(nil), o.arguments...)
	sort.Ints(out.arguments)
	write := 0
	for _, index := range out.arguments {
		if index < 0 {
			return CallOutcomeInputObservation{}, fmt.Errorf("callpayload: negative call-outcome argument observation")
		}
		if write == 0 || out.arguments[write-1] != index {
			out.arguments[write], write = index, write+1
		}
	}
	out.arguments = out.arguments[:write]
	if out.allArguments {
		out.arguments = nil
	}
	return out, nil
}

// ObservesCallee, ObservesReceiver, and ObservesArgument expose the frozen
// certificate without lending its backing storage to a provider.
func (o CallOutcomeInputObservation) ObservesCallee() bool   { return o.callee }
func (o CallOutcomeInputObservation) ObservesReceiver() bool { return o.receiver }
func (o CallOutcomeInputObservation) ObservesArgument(index int) bool {
	if index < 0 {
		return false
	}
	if o.allArguments {
		return true
	}
	position := sort.SearchInts(o.arguments, index)
	return position < len(o.arguments) && o.arguments[position] == index
}
func (o CallOutcomeInputObservation) AllArguments() bool { return o.allArguments }
func (o CallOutcomeInputObservation) Declared() bool     { return o.declared }

func canonicalTypestateResourceQueries(in []state.TypestateResourceQuery) ([]state.TypestateResourceQuery, error) {
	out := append([]state.TypestateResourceQuery(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Less(out[j]) })
	write := 0
	for _, query := range out {
		if write != 0 && !out[write-1].Less(query) && !query.Less(out[write-1]) {
			if out[write-1].Equal(query) {
				continue
			}
			return nil, fmt.Errorf("callpayload: typestate resource queries have comparator-equal foreign owners")
		}
		out[write], write = query, write+1
	}
	return out[:write], nil
}

type CallOutcomeCorrelationKind uint8

const (
	CallOutcomeCorrelationInvalid CallOutcomeCorrelationKind = iota
	CallOutcomeReturnConditionPath
	CallOutcomeReturnConditionSlot
	CallOutcomeReturnPresence
)

// CallOutcomeCorrelationShape is one fixed correlation address. Only Kind's
// fields participate; Value is intentionally absent.
type CallOutcomeCorrelationShape struct {
	Kind            CallOutcomeCorrelationKind
	ReturnIndex     int
	ReturnValue     bool
	Target          pathdom.Path
	TargetIndex     int
	TriggerPresence presence.Value
	TargetPresence  presence.Value
}

func (s CallOutcomeCorrelationShape) Equal(other CallOutcomeCorrelationShape) bool {
	return correlationShapeEqual(s, other)
}

func ReturnConditionPathShape(index int, truthy bool, target pathdom.Path) CallOutcomeCorrelationShape {
	return CallOutcomeCorrelationShape{Kind: CallOutcomeReturnConditionPath, ReturnIndex: index, ReturnValue: truthy, Target: target.Clone()}
}

func ReturnConditionSlotShape(index int, truthy bool, target int) CallOutcomeCorrelationShape {
	return CallOutcomeCorrelationShape{Kind: CallOutcomeReturnConditionSlot, ReturnIndex: index, ReturnValue: truthy, TargetIndex: target}
}

func ReturnPresenceShape(trigger int, triggerPresence presence.Value, target int, targetPresence presence.Value) CallOutcomeCorrelationShape {
	return CallOutcomeCorrelationShape{Kind: CallOutcomeReturnPresence, ReturnIndex: trigger, TriggerPresence: triggerPresence, TargetIndex: target, TargetPresence: targetPresence}
}

func (s CallOutcomeCorrelationShape) fieldName() string {
	switch s.Kind {
	case CallOutcomeReturnConditionPath:
		return "ReturnConditionRefinements"
	case CallOutcomeReturnConditionSlot:
		return "ReturnConditionSlots"
	case CallOutcomeReturnPresence:
		return "ReturnPresenceRelations"
	default:
		return ""
	}
}

func (s CallOutcomeCorrelationShape) valid() bool {
	if s.ReturnIndex < 0 {
		return false
	}
	switch s.Kind {
	case CallOutcomeReturnConditionPath:
		return !s.Target.IsEmpty() && s.TargetIndex == 0
	case CallOutcomeReturnConditionSlot:
		return s.Target.IsEmpty() && s.TargetIndex >= 0
	case CallOutcomeReturnPresence:
		return s.Target.IsEmpty() && s.TargetIndex >= 0 &&
			(presence.Equal(s.TriggerPresence, presence.Present()) || presence.Equal(s.TriggerPresence, presence.Absent())) &&
			(presence.Equal(s.TargetPresence, presence.Present()) || presence.Equal(s.TargetPresence, presence.Absent()))
	default:
		return false
	}
}

func correlationShapeLess(a, b CallOutcomeCorrelationShape) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.ReturnIndex != b.ReturnIndex {
		return a.ReturnIndex < b.ReturnIndex
	}
	if a.ReturnValue != b.ReturnValue {
		return !a.ReturnValue
	}
	if !a.Target.Equal(b.Target) {
		return a.Target.Less(b.Target)
	}
	if a.TargetIndex != b.TargetIndex {
		return a.TargetIndex < b.TargetIndex
	}
	if a.TriggerPresence != b.TriggerPresence {
		return a.TriggerPresence < b.TriggerPresence
	}
	return a.TargetPresence < b.TargetPresence
}

func correlationShapeEqual(a, b CallOutcomeCorrelationShape) bool {
	return a.Kind == b.Kind && a.ReturnIndex == b.ReturnIndex && a.ReturnValue == b.ReturnValue &&
		a.Target.Equal(b.Target) && a.TargetIndex == b.TargetIndex &&
		presence.Equal(a.TriggerPresence, b.TriggerPresence) && presence.Equal(a.TargetPresence, b.TargetPresence)
}

func canonicalCorrelationShapes(roles []CallOutcomeFieldRole, in []CallOutcomeCorrelationShape) ([]CallOutcomeCorrelationShape, error) {
	out := append([]CallOutcomeCorrelationShape(nil), in...)
	for index := range out {
		out[index].Target = out[index].Target.Clone()
		if !out[index].valid() || !hasCallOutcomeRole(roles, out[index].fieldName()) {
			return nil, fmt.Errorf("callpayload: invalid or undeclared correlation shape %d", index)
		}
	}
	sort.Slice(out, func(i, j int) bool { return correlationShapeLess(out[i], out[j]) })
	write := 0
	for _, shape := range out {
		if write != 0 && correlationShapeEqual(out[write-1], shape) {
			continue
		}
		out[write], write = shape, write+1
	}
	return out[:write], nil
}

func correlationShapeForPath(value CallReturnConditionRefinement) CallOutcomeCorrelationShape {
	return ReturnConditionPathShape(value.ReturnIndex, value.ReturnValue, value.Target)
}
func correlationShapeForSlot(value CallReturnConditionSlotRefinement) CallOutcomeCorrelationShape {
	return ReturnConditionSlotShape(value.ReturnIndex, value.ReturnValue, value.TargetIndex)
}
func correlationShapeForPresence(value CallReturnPresenceRelation) CallOutcomeCorrelationShape {
	return ReturnPresenceShape(value.TriggerIndex, value.TriggerPresence, value.TargetIndex, value.TargetPresence)
}

func validateOutcomeCorrelations(capability CallOutcomeCapability, outcome CallOutcome) error {
	declared := capability.correlations
	contains := func(shape CallOutcomeCorrelationShape) bool {
		index := sort.Search(len(declared), func(index int) bool { return !correlationShapeLess(declared[index], shape) })
		return index < len(declared) && correlationShapeEqual(declared[index], shape)
	}
	for _, value := range outcome.ReturnConditionRefinements {
		if !contains(correlationShapeForPath(value)) {
			return fmt.Errorf("callpayload: evaluator emitted undeclared ReturnConditionRefinements shape")
		}
	}
	for _, value := range outcome.ReturnConditionSlots {
		if !contains(correlationShapeForSlot(value)) {
			return fmt.Errorf("callpayload: evaluator emitted undeclared ReturnConditionSlots shape")
		}
	}
	for _, value := range outcome.ReturnPresenceRelations {
		if !contains(correlationShapeForPresence(value)) {
			return fmt.Errorf("callpayload: evaluator emitted undeclared ReturnPresenceRelations shape")
		}
	}
	return nil
}
