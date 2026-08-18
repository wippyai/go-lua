package target

import (
	"errors"
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"sort"
)

func freezeBindings(input []vocabulary.BindingSpec) ([]vocabulary.BindingSpec, error) {
	if _, err := vocabulary.CheckedStoredLength("operation binding table", len(input)); err != nil {
		return nil, err
	}
	out := make([]vocabulary.BindingSpec, len(input))
	for index, binding := range input {
		if !vocabulary.ValidBinding(binding) {
			return nil, fmt.Errorf("target: invalid binding %d", index)
		}
		out[index] = cloneBinding(binding)
	}
	sort.Slice(out, func(left, right int) bool { return compareBinding(out[left], out[right]) < 0 })
	for index := 1; index < len(out); index++ {
		if compareBinding(out[index-1], out[index]) == 0 {
			return nil, errors.New("target: duplicate operation binding")
		}
	}
	return out, nil
}

func (d operationDraft) freezeCallbacks(input []vocabulary.CallbackSpec) ([]callbackDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("callback table", len(input)); err != nil {
		return nil, err
	}
	out := make([]callbackDraft, len(input))
	for index, callback := range input {
		if callback.Function.Kind != vocabulary.InputSourceValueFormal || uint64(callback.Function.Ordinal) >= uint64(d.valueFormalCount()) {
			return nil, fmt.Errorf("target: callback %d has invalid function source", index)
		}
		if !callback.Admission.Available() {
			return nil, fmt.Errorf("target: callback %d has invalid admission", index)
		}
		arguments, argumentsErr := d.freezeValues(callback.Arguments, false)
		if argumentsErr != nil {
			return nil, fmt.Errorf("target: callback %d arguments: %w", index, argumentsErr)
		}
		if !validCallbackLifecycle(callback.Lifecycle) {
			return nil, fmt.Errorf("target: callback %d has invalid lifecycle", index)
		}
		effects, effectErr := d.freezeRow(callback.Effects, "callback expected")
		if effectErr != nil {
			return nil, fmt.Errorf("target: callback %d effects: %w", index, effectErr)
		}
		var release *callbackReleaseDraft
		if callback.Release != nil {
			if !retainedCallbackLifecycle(callback.Lifecycle) {
				return nil, fmt.Errorf("target: callback %d release requires retained lifecycle", index)
			}
			if callback.Release.Operation == 0 {
				return nil, fmt.Errorf("target: callback %d release has invalid operation", index)
			}
			if !validCallbackReleaseMode(callback.Release.Mode) {
				return nil, fmt.Errorf("target: callback %d release has invalid mode", index)
			}
			if !vocabulary.ValidCallbackReleaseZeroBehavior(callback.Release.Zero.Behavior) {
				return nil, fmt.Errorf("target: callback %d release has invalid zero behavior", index)
			}
			if callback.Release.Zero.Behavior == vocabulary.CallbackReleaseZeroSuppress && callback.Release.Zero.Outcome != 0 {
				return nil, fmt.Errorf("target: callback %d suppressed zero release has an outcome", index)
			}
			release = &callbackReleaseDraft{
				operationSource: callback.Release.Operation,
				input:           callback.Release.Input,
				outcome:         callback.Release.Outcome,
				mode:            callback.Release.Mode,
				zeroBehavior:    callback.Release.Zero.Behavior,
				zeroOutcome:     callback.Release.Zero.Outcome,
			}
		}
		if len(callback.Outcomes) != 5 {
			return nil, fmt.Errorf("target: callback %d has incomplete activation outcomes", index)
		}
		var outcomes [5]valuesDraft
		seen := [5]bool{}
		for outcomeIndex, outcome := range callback.Outcomes {
			kind, ok := vocabulary.CrossActivationOutcomeIndex(outcome.Kind)
			if !ok {
				return nil, fmt.Errorf("target: callback %d outcome %d has invalid activation kind", index, outcomeIndex)
			}
			if seen[kind] {
				return nil, fmt.Errorf("target: callback %d has duplicate activation kind", index)
			}
			values, valuesErr := d.freezeValues(outcome.Values, false)
			if valuesErr != nil {
				return nil, fmt.Errorf("target: callback %d outcome %d: %w", index, outcomeIndex, valuesErr)
			}
			seen[kind] = true
			outcomes[kind] = values
		}
		out[index] = callbackDraft{
			source: index, function: callback.Function, admission: callback.Admission,
			arguments: arguments, outcomes: outcomes, lifecycle: callback.Lifecycle,
			effects: effects, release: release,
		}
	}
	sort.Slice(out, func(left, right int) bool { return compareCallback(out[left], out[right]) < 0 })
	for index := 1; index < len(out); index++ {
		if compareCallbackIdentity(out[index-1], out[index]) == 0 {
			return nil, errors.New("target: duplicate callback")
		}
	}
	return out, nil
}

func (d operationDraft) freezeProduced(input []vocabulary.ProducedSpec, outcome valuesDraft) ([]producedDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("produced operation table", len(input)); err != nil {
		return nil, err
	}
	out := make([]producedDraft, len(input))
	for index, produced := range input {
		if produced.Operation == 0 {
			return nil, fmt.Errorf("produced operation %d has invalid target", index)
		}
		if uint64(produced.Result) >= uint64(len(outcome.types)) {
			return nil, fmt.Errorf("produced operation %d result is not a fixed outcome slot", index)
		}
		declaration, declarationOK := d.declarations[outcome.types[produced.Result]]
		if !declarationOK {
			return nil, fmt.Errorf("produced operation %d callable relation: type declaration is not admitted", index)
		}
		callable, relationErr := d.semantics.Callable(declaration, schematype.CallableAdmissionDirectFunction, d.formalConstraints)
		if relationErr != nil {
			return nil, fmt.Errorf("produced operation %d callable relation: %w", index, relationErr)
		}
		if !callable {
			return nil, fmt.Errorf("produced operation %d result is not a direct function", index)
		}
		if _, err := vocabulary.CheckedStoredLength("produced capture table", len(produced.Captures)); err != nil {
			return nil, err
		}
		captures := append([]vocabulary.CaptureSpec(nil), produced.Captures...)
		typeValueCaptures := 0
		for captureIndex, capture := range captures {
			switch capture.Kind {
			case vocabulary.CaptureValueFormal:
				if uint64(capture.Ordinal) >= uint64(d.valueFormalCount()) {
					return nil, fmt.Errorf("produced operation %d capture %d ValueFormal outside scope", index, captureIndex)
				}
			case vocabulary.CaptureTypeValueFormal:
				if uint64(capture.Ordinal) >= uint64(d.valueFormalCount()) {
					return nil, fmt.Errorf("produced operation %d capture %d TypeValueFormal outside scope", index, captureIndex)
				}
				typeValueCaptures++
				if typeValueCaptures > 1 {
					return nil, fmt.Errorf("produced operation %d has more than one TypeValueFormal capture", index)
				}
			case vocabulary.CaptureValuesVar:
				if uint64(capture.Ordinal) >= uint64(d.valuesVars) {
					return nil, fmt.Errorf("produced operation %d capture %d ValuesVar outside scope", index, captureIndex)
				}
			case vocabulary.CaptureCallback:
				if capture.Ordinal == 0 || uint64(capture.Ordinal) > uint64(len(d.callbacks)) {
					return nil, fmt.Errorf("produced operation %d capture %d callback outside scope", index, captureIndex)
				}
			default:
				return nil, fmt.Errorf("produced operation %d capture %d invalid source", index, captureIndex)
			}
		}
		out[index] = producedDraft{result: produced.Result, targetSource: produced.Operation, captures: captures}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].result < out[right].result })
	for index := 1; index < len(out); index++ {
		if out[index-1].result == out[index].result {
			return nil, errors.New("target: duplicate produced outcome result")
		}
	}
	return out, nil
}

// validateProducedTypeValueFreshResults closes the structural identity law
// for retained runtime TypeValues. A TypeValue capture describes the identity
// of its Produced callable result, so that exact fixed result must also be the
// outcome's nominal FreshFunction. Both inputs are already canonical and
// unique by result; a linear merge proves the correspondence without names,
// inferred ordinals, or a Produced×FreshResult product.
func validateProducedTypeValueFreshResults(produced []producedDraft, fresh []freshResultDraft) error {
	freshIndex := 0
	for producedIndex, row := range produced {
		typeValue := false
		for _, capture := range row.captures {
			if capture.Kind == vocabulary.CaptureTypeValueFormal {
				typeValue = true
				break
			}
		}
		if !typeValue {
			continue
		}
		for freshIndex < len(fresh) && fresh[freshIndex].result < row.result {
			freshIndex++
		}
		if freshIndex >= len(fresh) || fresh[freshIndex].result != row.result {
			return fmt.Errorf("produced operation %d TypeValue capture result %d lacks FreshFunction", producedIndex, row.result)
		}
		if fresh[freshIndex].kind != schematype.FreshClassFunction {
			return fmt.Errorf("produced operation %d TypeValue capture result %d has fresh kind %d, want FreshFunction", producedIndex, row.result, fresh[freshIndex].kind)
		}
	}
	return nil
}

func (d *operationDraft) freezeEffects(input vocabulary.RowSpec) error {
	row, err := d.freezeRow(input, "ordinary")
	if err != nil {
		return err
	}
	d.effects, d.effectTail, d.effectVar = row.effects, row.tail, row.variable
	return nil
}

func (d operationDraft) freezeRow(input vocabulary.RowSpec, owner string) (rowDraft, error) {
	if input.Tail != vocabulary.RowClosed && input.Tail != vocabulary.RowVariable {
		return rowDraft{}, fmt.Errorf("target: %s row has invalid tail", owner)
	}
	if input.Tail == vocabulary.RowVariable {
		if uint64(input.Var) >= uint64(d.rowFormals) {
			return rowDraft{}, errors.New("target: row variable outside operation scope")
		}
	} else if input.Var != 0 {
		return rowDraft{}, errors.New("target: closed row carries variable")
	}
	out := rowDraft{tail: input.Tail, variable: input.Var}
	if _, err := vocabulary.CheckedStoredLength("effect table", len(input.Occurrences)); err != nil {
		return rowDraft{}, err
	}
	if len(input.Occurrences) == 0 {
		return out, nil
	}
	out.effects = make([]effectDraft, len(input.Occurrences))
	valueCount := uint64(len(d.input.types))
	for index, item := range input.Occurrences {
		if item.Target == 0 {
			return rowDraft{}, fmt.Errorf("target: effect %d has invalid target", index)
		}
		draft := effectDraft{targetSource: item.Target}
		if item.Publication != nil {
			draft.publication, draft.hasPublication = *item.Publication, true
		}
		if _, err := vocabulary.CheckedStoredLength("effect value argument pool", len(item.ValueArgs)); err != nil {
			return rowDraft{}, err
		}
		if _, err := vocabulary.CheckedStoredLength("effect type argument pool", len(item.TypeArgs)); err != nil {
			return rowDraft{}, err
		}
		if _, err := vocabulary.CheckedStoredLength("effect Values argument pool", len(item.ValuesArgs)); err != nil {
			return rowDraft{}, err
		}
		if _, err := vocabulary.CheckedStoredLength("effect row argument pool", len(item.RowArgs)); err != nil {
			return rowDraft{}, err
		}
		draft.values = append([]vocabulary.ValueFormal(nil), item.ValueArgs...)
		draft.types = append([]vocabulary.TypeFormal(nil), item.TypeArgs...)
		draft.valuesVar = append([]vocabulary.ValuesVar(nil), item.ValuesArgs...)
		draft.rows = append([]vocabulary.RowVar(nil), item.RowArgs...)
		for _, value := range draft.values {
			if uint64(value) >= valueCount {
				return rowDraft{}, fmt.Errorf("target: effect %d has value argument outside scope", index)
			}
		}
		for _, formal := range draft.types {
			if uint64(formal) >= uint64(len(d.formals)) {
				return rowDraft{}, fmt.Errorf("target: effect %d has type argument outside scope", index)
			}
		}
		for _, variable := range draft.valuesVar {
			if uint64(variable) >= uint64(d.valuesVars) {
				return rowDraft{}, fmt.Errorf("target: effect %d has Values argument outside scope", index)
			}
		}
		for _, variable := range draft.rows {
			if uint64(variable) >= uint64(d.rowFormals) {
				return rowDraft{}, fmt.Errorf("target: effect %d has row argument outside scope", index)
			}
		}
		out.effects[index] = draft
	}
	return out, nil
}

func cloneBinding(input vocabulary.BindingSpec) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{
		Namespace: input.Namespace,
		Owner:     append([]string(nil), input.Owner...), Member: append([]string(nil), input.Member...),
	}
}
