package target

import (
	"errors"
	"fmt"
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
	if input.SubedgeRelation != nil {
		branch, branchErr := draft.freezeSubedgeRelation(*input.SubedgeRelation)
		if branchErr != nil {
			return operationDraft{}, branchErr
		}
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

func (d *operationDraft) freezeType(value schematype.Type) (string, error) {
	if !value.Available() {
		return "", errors.New("unavailable type declaration")
	}
	if err := d.semantics.Validate(value, d.formalConstraints); err != nil {
		return "", fmt.Errorf("type declaration rejected: %w", err)
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
	if existing, exists := d.declarations[key]; exists && !existing.Equal(value) {
		return "", errors.New("distinct neutral type declarations share a storage key")
	}
	if _, exists := d.types[key]; !exists {
		d.types[key] = append([]byte(nil), encoded...)
		d.declarations[key] = value
	}
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
		if err := visitSubedgeValues(subedge, check); err != nil {
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

// visitSubedgeValues is the complete closure of Values endpoints owned by one
// Subedge relation. Keeping this enumeration singular makes the class-table
// and interning closures agree as the relation evolves.
func visitSubedgeValues(edge subedgeDraft, visit func(valuesDraft) error) error {
	if err := visit(edge.arguments); err != nil {
		return err
	}
	for _, terminal := range edge.outcomes {
		if err := visit(terminal); err != nil {
			return err
		}
	}
	// Admission failure is a distinct Values source, and its route owns a
	// separate projected Result. Neither is derived from a callee terminal.
	if err := visit(edge.admissionFailure); err != nil {
		return err
	}
	if err := visit(edge.admissionRoute.result); err != nil {
		return err
	}
	for _, route := range edge.routes {
		if err := visit(route.result); err != nil {
			return err
		}
	}
	return nil
}
