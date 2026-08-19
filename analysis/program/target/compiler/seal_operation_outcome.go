package compiler

import (
	"errors"
	"fmt"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func (d *operationDraft) freezeOutcomes(input []vocabulary.OutcomeSpec) ([]outcomeDraft, error) {
	if len(input) == 0 {
		return nil, errors.New("target: operation has no outcomes")
	}
	if _, err := vocabulary.CheckedStoredLength("outcome table", len(input)); err != nil {
		return nil, err
	}
	out := make([]outcomeDraft, len(input))
	for index, item := range input {
		if !validOperationOutcome(item.Kind) {
			return nil, fmt.Errorf("target: invalid outcome kind %d", item.Kind)
		}
		values, err := d.freezeValues(item.Values, false)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		produced, err := d.freezeProduced(item.Produced, values)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		callbackResults, err := d.freezeCallbackResults(item.CallbackResults, values, produced)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		resultAliases, err := d.freezeResultAliases(item.ResultAliases, values)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		fresh, err := d.freezeFreshResults(item.FreshResults, values, resultAliases)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		if err := validateProducedTypeValueFreshResults(produced, fresh); err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		anchor, anchorErr := outcomeAnchor(item.Kind, values, fresh)
		if anchorErr != nil {
			return nil, fmt.Errorf("target: outcome %d anchor: %w", index, anchorErr)
		}
		out[index] = outcomeDraft{
			source: index, kind: item.Kind, values: values,
			anchor:   anchor,
			produced: produced, fresh: fresh, callbackResults: callbackResults, resultAliases: resultAliases,
		}
	}
	sort.Slice(out, func(left, right int) bool { return compareOutcome(out[left], out[right]) < 0 })
	for index := 1; index < len(out); index++ {
		if compareOutcome(out[index-1], out[index]) == 0 {
			// The frozen outcome discriminator is Kind, Values, and the
			// nominal FreshResult relation. Produced, callback-result, and
			// alias rows are conjunctive annotations, not alternate-case keys.
			return nil, errors.New("target: duplicate outcome case")
		}
	}
	return out, nil
}

func outcomeAnchor(kind flowkind.OutcomeKind, values valuesDraft, fresh []freshResultDraft) (string, error) {
	valuesKey, err := values.key()
	if err != nil {
		return "", err
	}
	if _, err := vocabulary.CheckedStoredLength("outcome anchor Values key", len(valuesKey)); err != nil {
		return "", err
	}
	if _, err := vocabulary.CheckedStoredLength("outcome anchor fresh result table", len(fresh)); err != nil {
		return "", err
	}
	// Kind plus framed Values key and fixed-width canonical Fresh rows.  The
	// frame matters because Values keys end in an uncounted suffix sequence.
	parts := []int{1, 4, len(valuesKey), 4}
	for range fresh {
		parts = append(parts, 4, 1)
	}
	total, err := vocabulary.CheckedStoredTotal("outcome anchor", parts...)
	if err != nil {
		return "", err
	}
	out := make([]byte, 0, total)
	out = append(out, byte(kind))
	out = appendUint32(out, uint32(len(valuesKey)))
	out = append(out, valuesKey...)
	out = appendUint32(out, uint32(len(fresh)))
	for _, row := range fresh {
		out = appendUint32(out, row.result)
		out = append(out, byte(row.kind))
	}
	return string(out), nil
}

func (d operationDraft) freezeCallbackResults(input []vocabulary.CallbackResultSpec, outcome valuesDraft, produced []producedDraft) ([]callbackResultDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("callback result table", len(input)); err != nil {
		return nil, err
	}
	out := make([]callbackResultDraft, len(input))
	for index, result := range input {
		if uint64(result.Result) >= uint64(len(outcome.types)) {
			return nil, fmt.Errorf("callback result %d is not a fixed outcome slot", index)
		}
		if result.Callback == 0 || uint64(result.Callback) > uint64(len(d.callbacks)) {
			return nil, fmt.Errorf("callback result %d callback outside scope", index)
		}
		callback, found := d.callbackBySource(int(result.Callback - 1))
		if !found || callback.function.Kind != vocabulary.InputSourceValueFormal || uint64(callback.function.Ordinal) >= uint64(len(d.input.types)) {
			return nil, fmt.Errorf("callback result %d callback source is malformed", index)
		}
		sourceType := d.input.types[callback.function.Ordinal]
		resultType := outcome.types[result.Result]
		sourceDeclaration, sourceOK := d.declarations[sourceType]
		resultDeclaration, resultOK := d.declarations[resultType]
		if !sourceOK || !resultOK {
			return nil, fmt.Errorf("callback result %d type relation: type declaration is not admitted", index)
		}
		assignable, relationErr := d.semantics.Assignable(sourceDeclaration, resultDeclaration, d.formalConstraints)
		if relationErr != nil {
			return nil, fmt.Errorf("callback result %d type relation: %w", index, relationErr)
		}
		if !assignable {
			return nil, fmt.Errorf("callback result %d is type-incompatible with its callback", index)
		}
		sourceCallable, sourceErr := d.semantics.Callable(sourceDeclaration, callback.admission, d.formalConstraints)
		if sourceErr != nil {
			return nil, fmt.Errorf("callback result %d source callable relation: %w", index, sourceErr)
		}
		resultCallable, resultErr := d.semantics.Callable(resultDeclaration, callback.admission, d.formalConstraints)
		if resultErr != nil {
			return nil, fmt.Errorf("callback result %d result callable relation: %w", index, resultErr)
		}
		if !sourceCallable || !resultCallable {
			return nil, fmt.Errorf("callback result %d is not callable under its admission", index)
		}
		out[index] = callbackResultDraft{result: result.Result, callback: result.Callback}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].result < out[right].result })
	for index := 1; index < len(out); index++ {
		if out[index-1].result == out[index].result {
			return nil, errors.New("target: duplicate callback outcome result")
		}
	}
	producedIndex, callbackIndex := 0, 0
	for producedIndex < len(produced) && callbackIndex < len(out) {
		if produced[producedIndex].result < out[callbackIndex].result {
			producedIndex++
			continue
		}
		if produced[producedIndex].result > out[callbackIndex].result {
			callbackIndex++
			continue
		}
		return nil, errors.New("target: callback result overlaps produced result")
	}
	return out, nil
}

func (d operationDraft) callbackBySource(source int) (callbackDraft, bool) {
	for _, callback := range d.callbacks {
		if callback.source == source {
			return callback, true
		}
	}
	return callbackDraft{}, false
}

func (d operationDraft) freezeResultAliases(input []vocabulary.ResultAliasSpec, outcome valuesDraft) ([]resultAliasDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("result alias table", len(input)); err != nil {
		return nil, err
	}
	out := make([]resultAliasDraft, len(input))
	for index, alias := range input {
		if uint64(alias.Result) >= uint64(len(outcome.types)) {
			return nil, fmt.Errorf("result alias %d is not a fixed outcome slot", index)
		}
		if alias.Source.Kind != vocabulary.InputSourceValueFormal || uint64(alias.Source.Ordinal) >= uint64(d.valueFormalCount()) {
			return nil, fmt.Errorf("result alias %d source is not a ValueFormal in scope", index)
		}
		sourceDeclaration, sourceOK := d.declarations[d.input.types[alias.Source.Ordinal]]
		resultDeclaration, resultOK := d.declarations[outcome.types[alias.Result]]
		if !sourceOK || !resultOK {
			return nil, fmt.Errorf("result alias %d type relation: type declaration is not admitted", index)
		}
		assignable, relationErr := d.semantics.Assignable(sourceDeclaration, resultDeclaration, d.formalConstraints)
		if relationErr != nil {
			return nil, fmt.Errorf("result alias %d type relation: %w", index, relationErr)
		}
		if !assignable {
			return nil, fmt.Errorf("result alias %d is type-incompatible with its input", index)
		}
		out[index] = resultAliasDraft{result: alias.Result, source: alias.Source}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].result < out[right].result })
	for index := 1; index < len(out); index++ {
		if out[index-1].result == out[index].result {
			return nil, errors.New("target: duplicate result alias outcome result")
		}
	}
	return out, nil
}

func (d operationDraft) freezeFreshResults(input []vocabulary.FreshResultSpec, outcome valuesDraft, aliases []resultAliasDraft) ([]freshResultDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("fresh result table", len(input)); err != nil {
		return nil, err
	}
	out := make([]freshResultDraft, len(input))
	for index, fresh := range input {
		if uint64(fresh.Result) >= uint64(len(outcome.types)) {
			return nil, fmt.Errorf("fresh result %d is not a fixed outcome slot", index)
		}
		if !fresh.Kind.Available() {
			return nil, fmt.Errorf("fresh result %d has invalid kind", index)
		}
		declaration, declarationOK := d.declarations[outcome.types[fresh.Result]]
		if !declarationOK {
			return nil, fmt.Errorf("fresh result %d type relation: type declaration is not admitted", index)
		}
		compatible, relationErr := d.semantics.Fresh(declaration, fresh.Kind, d.formalConstraints)
		if relationErr != nil {
			return nil, fmt.Errorf("fresh result %d type relation: %w", index, relationErr)
		}
		if !compatible {
			return nil, fmt.Errorf("fresh result %d contradicts its runtime kind", index)
		}
		out[index] = freshResultDraft{result: fresh.Result, kind: fresh.Kind}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].result < out[right].result })
	for index := range out {
		if index != 0 && out[index-1].result == out[index].result {
			return nil, errors.New("target: duplicate fresh outcome result")
		}
		out[index].ordinal = uint32(index)
	}
	aliasIndex, freshIndex := 0, 0
	for aliasIndex < len(aliases) && freshIndex < len(out) {
		if aliases[aliasIndex].result < out[freshIndex].result {
			aliasIndex++
			continue
		}
		if aliases[aliasIndex].result > out[freshIndex].result {
			freshIndex++
			continue
		}
		return nil, errors.New("target: fresh result overlaps result alias")
	}
	return out, nil
}
