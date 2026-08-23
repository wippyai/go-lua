package compiler

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func freezeOperation(source int, input vocabulary.OperationSpec, semantics schematype.Semantics) (operationDraft, error) {
	if err := vocabulary.CheckedCoordinateCount("Values variable count", input.ValuesVars); err != nil {
		return operationDraft{}, err
	}
	if err := vocabulary.CheckedCoordinateCount("row formal count", input.RowFormals); err != nil {
		return operationDraft{}, err
	}
	formals, err := copyFormals(input.TypeFormals)
	if err != nil {
		return operationDraft{}, err
	}
	if _, err := vocabulary.CheckedStoredLength("type formal pool", len(formals)); err != nil {
		return operationDraft{}, err
	}
	draft := operationDraft{
		source:            source,
		semantics:         semantics,
		formals:           formals,
		valuesVars:        input.ValuesVars,
		rowFormals:        input.RowFormals,
		types:             make(map[string][]byte),
		declarations:      make(map[string]schematype.Type),
		formalConstraints: make([]schematype.Type, len(formals)),
		constraints:       make([]string, len(formals)),
	}
	for index, formal := range formals {
		draft.formalConstraints[index] = formal.Constraint
		if !formal.Constraint.Available() {
			continue
		}
		key, freezeErr := draft.freezeType(formal.Constraint)
		if freezeErr != nil {
			return operationDraft{}, fmt.Errorf("target: type formal %d constraint: %w", index, freezeErr)
		}
		draft.constraints[index] = key
	}
	draft.input, err = draft.freezeValues(input.Input, false)
	if err != nil {
		return operationDraft{}, fmt.Errorf("target: input: %w", err)
	}
	if len(input.Input.Suffix) != 0 {
		return operationDraft{}, errors.New("target: input Values cannot have a suffix")
	}
	draft.bindings, err = freezeBindings(input.Bindings)
	if err != nil {
		return operationDraft{}, err
	}
	draft.callbacks, err = draft.freezeCallbacks(input.Callbacks)
	if err != nil {
		return operationDraft{}, err
	}
	draft.outcomes, err = draft.freezeOutcomes(input.Outcomes)
	if err != nil {
		return operationDraft{}, err
	}
	draft.behavior, err = draft.freezeBehavior(input.Behavior)
	if err != nil {
		return operationDraft{}, err
	}
	draft.suspensions, err = draft.freezeSuspensions(input.Suspensions)
	if err != nil {
		return operationDraft{}, err
	}
	draft.subedges, err = draft.freezeSubedges(input.Subedges)
	if err != nil {
		return operationDraft{}, err
	}
	draft.resumes, err = draft.freezeResumes(input.Resumes)
	if err != nil {
		return operationDraft{}, err
	}
	if err := draft.sealValuesVarTypes(); err != nil {
		return operationDraft{}, err
	}
	draft.spawns, err = draft.freezeSpawns(input.Spawns)
	if err != nil {
		return operationDraft{}, err
	}
	draft.transfers, err = draft.freezeTransfers(input.Transfers)
	if err != nil {
		return operationDraft{}, err
	}
	if err := draft.freezeEffects(input.Effects); err != nil {
		return operationDraft{}, err
	}
	draft.formalEffects, draft.formalEffectTail, err = freezeFormalEffects(input.FormalEffects)
	if err != nil {
		return operationDraft{}, err
	}
	if input.SubedgeRelation != nil {
		branch := *input.SubedgeRelation
		branch.EffectAliases = append([]uint32(nil), input.SubedgeRelation.EffectAliases...)
		draft.subedgeRelation = &branch
	}
	return draft, nil
}

func emptyClosedValues(values valuesDraft) bool {
	return len(values.types) == 0 && len(values.suffix) == 0 && values.tail == vocabulary.ValuesClosed && values.varID == 0
}

func copyFormals(input []vocabulary.TypeFormalSpec) ([]vocabulary.TypeFormalSpec, error) {
	if len(input) == 0 {
		return nil, nil
	}
	return append([]vocabulary.TypeFormalSpec(nil), input...), nil
}

// freezeBehavior copies the provider's neutral behavior declaration into
// canonical Target-local rows. It resolves authored outcome ordinals after
// outcome canonicalization and performs no interpretation of the relation
// identity. A behavior row is an annotation of one fixed result slot; it is
// not an additional result or value axis.
func (d *operationDraft) freezeBehavior(input *vocabulary.OperationBehaviorSpec) (behaviorDraft, error) {
	if input == nil || (len(input.Results) == 0 && len(input.Predicates) == 0) {
		return behaviorDraft{}, nil
	}
	outcomes := make(map[uint32]uint32, len(d.outcomes))
	for index, outcome := range d.outcomes {
		outcomes[uint32(outcome.source)] = uint32(index)
	}
	resolve := func(outcome, result uint32) (uint32, error) {
		canonical, ok := outcomes[outcome]
		if !ok {
			return 0, fmt.Errorf("behavior outcome %d is outside the operation", outcome)
		}
		if uint64(result) >= uint64(len(d.outcomes[canonical].values.types)) {
			return 0, fmt.Errorf("behavior result %d is not a fixed outcome slot", result)
		}
		return canonical, nil
	}
	if _, err := vocabulary.CheckedStoredLength("behavior result table", len(input.Results)); err != nil {
		return behaviorDraft{}, err
	}
	if _, err := vocabulary.CheckedStoredLength("behavior predicate table", len(input.Predicates)); err != nil {
		return behaviorDraft{}, err
	}
	draft := behaviorDraft{
		results:    make([]behaviorResultDraft, len(input.Results)),
		predicates: make([]behaviorPredicateDraft, len(input.Predicates)),
	}
	for index, item := range input.Results {
		outcome, err := resolve(item.Outcome, item.Result)
		if err != nil {
			return behaviorDraft{}, fmt.Errorf("behavior result %d: %w", index, err)
		}
		if !item.Relation.Available() {
			return behaviorDraft{}, fmt.Errorf("behavior result %d has no relation identity", index)
		}
		if !validAuthoredInputSource(item.Source, d.valueFormalCount(), d.valuesVars) {
			return behaviorDraft{}, fmt.Errorf("behavior result %d has an invalid input source", index)
		}
		draft.results[index] = behaviorResultDraft{outcome: outcome, result: item.Result, source: item.Source, relation: item.Relation}
	}
	for index, item := range input.Predicates {
		outcome, err := resolve(item.Outcome, item.Result)
		if err != nil {
			return behaviorDraft{}, fmt.Errorf("behavior predicate %d: %w", index, err)
		}
		if !item.Relation.Available() {
			return behaviorDraft{}, fmt.Errorf("behavior predicate %d has no relation identity", index)
		}
		if !validAuthoredInputSource(item.Subject, d.valueFormalCount(), d.valuesVars) {
			return behaviorDraft{}, fmt.Errorf("behavior predicate %d has an invalid input subject", index)
		}
		draft.predicates[index] = behaviorPredicateDraft{outcome: outcome, result: item.Result, subject: item.Subject, relation: item.Relation}
	}
	sort.Slice(draft.results, func(left, right int) bool {
		return compareBehaviorResult(draft.results[left], draft.results[right]) < 0
	})
	for index := 1; index < len(draft.results); index++ {
		if draft.results[index-1].outcome == draft.results[index].outcome && draft.results[index-1].result == draft.results[index].result {
			return behaviorDraft{}, errors.New("target: duplicate behavior result row")
		}
	}
	sort.Slice(draft.predicates, func(left, right int) bool {
		return compareBehaviorPredicate(draft.predicates[left], draft.predicates[right]) < 0
	})
	for index := 1; index < len(draft.predicates); index++ {
		if draft.predicates[index-1].outcome == draft.predicates[index].outcome && draft.predicates[index-1].result == draft.predicates[index].result {
			return behaviorDraft{}, errors.New("target: duplicate behavior predicate row")
		}
	}
	return draft, nil
}

func compareBehaviorResult(left, right behaviorResultDraft) int {
	if left.outcome != right.outcome {
		if left.outcome < right.outcome {
			return -1
		}
		return 1
	}
	if left.result != right.result {
		if left.result < right.result {
			return -1
		}
		return 1
	}
	if order := compareInputSource(left.source, right.source); order != 0 {
		return order
	}
	return bytes.Compare(left.relation[:], right.relation[:])
}

func compareBehaviorPredicate(left, right behaviorPredicateDraft) int {
	if left.outcome != right.outcome {
		if left.outcome < right.outcome {
			return -1
		}
		return 1
	}
	if left.result != right.result {
		if left.result < right.result {
			return -1
		}
		return 1
	}
	if order := compareInputSource(left.subject, right.subject); order != 0 {
		return order
	}
	return bytes.Compare(left.relation[:], right.relation[:])
}

func (d *operationDraft) freezeType(value schematype.Type) (string, error) {
	if !value.Available() {
		return "", errors.New("unavailable type declaration")
	}
	encoded := value.Bytes()
	if _, err := vocabulary.CheckedStoredLength("type bytes", len(encoded)); err != nil {
		return "", err
	}
	key := string(encoded)
	if key == "" {
		// Primitive envelopes intentionally have no domain bytes. Their digest
		// remains a stable, collision-free local key while the declaration is
		// retained for the domain semantic adapter.
		digest := value.Digest()
		key = string(digest[:])
	}
	if existing, exists := d.declarations[key]; exists {
		if !existing.Equal(value) {
			return "", errors.New("distinct neutral type declarations share a storage key")
		}
		return key, nil
	}
	// Admission is a property of the declaration under this operation's formal
	// scope, and that scope is complete before the first declaration is
	// frozen. The draft's declaration table is therefore the admission
	// denominator: each distinct declaration is admitted once, however many
	// operation positions mention it.
	if err := d.semantics.Validate(value, d.formalConstraints); err != nil {
		return "", fmt.Errorf("type declaration rejected: %w", err)
	}
	d.types[key] = append([]byte(nil), encoded...)
	d.declarations[key] = value
	return key, nil
}

func (d *operationDraft) freezeValues(input vocabulary.ValuesSpec, opaque bool) (valuesDraft, error) {
	if !validValuesTail(input.Tail, input.Var, d.valuesVars, opaque) {
		return valuesDraft{}, errors.New("target: invalid Values tail")
	}
	out := valuesDraft{tail: input.Tail, varID: input.Var}
	if input.Tail != vocabulary.ValuesVariable {
		if input.TailType.Available() {
			return valuesDraft{}, errors.New("target: Values tail type requires a ValuesVariable tail")
		}
	} else {
		tailType := input.TailType
		if !tailType.Available() {
			any, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
			if !ok {
				return valuesDraft{}, errors.New("target: unavailable default Values tail type")
			}
			tailType = any
		}
		key, err := d.freezeType(tailType)
		if err != nil {
			return valuesDraft{}, fmt.Errorf("target: Values tail type: %w", err)
		}
		out.tailType = key
	}
	if _, err := vocabulary.CheckedStoredLength("Values fixed list", len(input.Fixed)); err != nil {
		return valuesDraft{}, err
	}
	if _, err := vocabulary.CheckedStoredLength("Values suffix list", len(input.Suffix)); err != nil {
		return valuesDraft{}, err
	}
	// A closed tail has no end-relative coordinate. Canonicalize its suffix
	// into the prefix so equivalent authored Values share one handle.
	fixed := input.Fixed
	suffix := input.Suffix
	if input.Tail == vocabulary.ValuesClosed && len(suffix) != 0 {
		fixed = make([]schematype.Type, 0, len(input.Fixed)+len(suffix))
		fixed = append(fixed, input.Fixed...)
		fixed = append(fixed, suffix...)
		suffix = nil
	}
	if len(fixed) != 0 {
		out.types = make([]string, len(fixed))
	}
	for index, value := range fixed {
		if !value.Available() {
			return valuesDraft{}, fmt.Errorf("target: nil Values element %d", index)
		}
		key, err := d.freezeType(value)
		if err != nil {
			return valuesDraft{}, fmt.Errorf("target: Values element %d: %w", index, err)
		}
		out.types[index] = key
	}
	if len(suffix) != 0 {
		out.suffix = make([]string, len(suffix))
	}
	for index, value := range suffix {
		if !value.Available() {
			return valuesDraft{}, fmt.Errorf("target: nil Values suffix element %d", index)
		}
		key, err := d.freezeType(value)
		if err != nil {
			return valuesDraft{}, fmt.Errorf("target: Values suffix element %d: %w", index, err)
		}
		out.suffix[index] = key
	}
	return out, nil
}

// sealValuesVarTypes makes the ValuesVar class table total. A variable which
// no ValuesSpec directly constrains still carries the ABI default neutral Any
// declaration; the domain adapter interprets that atom during later reads.
func (d *operationDraft) sealValuesVarTypes() error {
	if d.valuesVars == 0 {
		d.valuesTypes = nil
		return nil
	}
	any, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		return errors.New("target: unavailable default Values tail type")
	}
	anyKey, err := d.freezeType(any)
	if err != nil {
		return fmt.Errorf("target: default Values tail type: %w", err)
	}
	classes := make([]string, d.valuesVars)
	seen := make([]bool, d.valuesVars)
	check := func(values valuesDraft) error {
		if values.tail != vocabulary.ValuesVariable {
			return nil
		}
		variable := values.varID
		if !seen[variable] {
			classes[variable], seen[variable] = values.tailType, true
			return nil
		}
		if classes[variable] != values.tailType {
			return fmt.Errorf("target: Values variable %d has conflicting tail types", variable)
		}
		return nil
	}
	if err := check(d.input); err != nil {
		return err
	}
	for _, outcome := range d.outcomes {
		if err := check(outcome.values); err != nil {
			return err
		}
	}
	for _, callback := range d.callbacks {
		if err := check(callback.arguments); err != nil {
			return err
		}
		for _, terminal := range callback.outcomes {
			if err := check(terminal); err != nil {
				return err
			}
		}
	}
	for _, subedge := range d.subedges {
		if err := d.visitSubedgeValues(subedge, check); err != nil {
			return err
		}
	}
	for _, resume := range d.resumes {
		if err := check(resume.arguments); err != nil {
			return err
		}
	}
	for index := range classes {
		if !seen[index] {
			classes[index] = anyKey
		}
	}
	d.valuesTypes = classes
	return nil
}

// visitSubedgeValues is the complete closure of authored Values endpoints
// needed to issue operation-owned Values handles. It performs only the
// Target-specific type freeze; operation.Core validates the relation that
// connects these endpoints.
func (d *operationDraft) visitSubedgeValues(edge vocabulary.SubedgeSpec, visit func(valuesDraft) error) error {
	freeze := func(values vocabulary.ValuesSpec) (valuesDraft, error) {
		return d.freezeValues(values, false)
	}
	if edge.Callee.Kind != vocabulary.SubedgeCalleeCallback {
		arguments, err := freeze(edge.Arguments)
		if err != nil {
			return err
		}
		if err := visit(arguments); err != nil {
			return err
		}
	}
	for _, terminal := range edge.Outcomes {
		values, valuesErr := freeze(terminal.Values)
		if valuesErr != nil {
			return valuesErr
		}
		if err := visit(values); err != nil {
			return err
		}
	}
	// Admission failure is a distinct Values source, and its route owns a
	// separate projected Result. Neither is derived from a callee terminal.
	failure, failureErr := freeze(edge.AdmissionFailure.Values)
	if failureErr != nil {
		return failureErr
	}
	if err := visit(failure); err != nil {
		return err
	}
	failureResult, failureResultErr := freeze(edge.AdmissionFailure.Route.Result)
	if failureResultErr != nil {
		return failureResultErr
	}
	if err := visit(failureResult); err != nil {
		return err
	}
	for _, route := range edge.Routes {
		result, resultErr := freeze(route.Result)
		if resultErr != nil {
			return resultErr
		}
		if err := visit(result); err != nil {
			return err
		}
	}
	return nil
}
