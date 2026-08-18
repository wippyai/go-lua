package target

import (
	"errors"
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"sort"
)

// freezeSubedges seals the one Target-owned internal-application relation.
// It resolves local route references by dense table coordinates only: a local
// cycle is finite structural recurrence, later compiled to Mu, and is never
// followed by Go recursion here.
func (d *operationDraft) freezeSubedges(input []vocabulary.SubedgeSpec) ([]subedgeDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("subedge table", len(input)); err != nil {
		return nil, err
	}
	if len(input) == 0 {
		if err := validateSubedgeEntries(nil, d.callbacks); err != nil {
			return nil, err
		}
		return nil, nil
	}
	callbackRanks, err := callbackRanks(d.callbacks)
	if err != nil {
		return nil, err
	}
	outcomeOrdinals, err := outcomeOrdinals(d.outcomes)
	if err != nil {
		return nil, err
	}
	out := make([]subedgeDraft, len(input))
	for index, item := range input {
		edge, freezeErr := d.freezeSubedge(index, item, callbackRanks, outcomeOrdinals)
		if freezeErr != nil {
			return nil, freezeErr
		}
		out[index] = edge
	}
	sort.Slice(out, func(left, right int) bool {
		return compareSubedgeIdentity(out[left], out[right]) < 0
	})
	for index := 1; index < len(out); index++ {
		if out[index-1].role == out[index].role {
			return nil, errors.New("target: duplicate subedge role")
		}
	}
	ranks := make([]uint32, len(out))
	for rank := range out {
		if out[rank].source < 0 || out[rank].source >= len(ranks) {
			return nil, errors.New("target: malformed subedge source")
		}
		ranks[out[rank].source] = uint32(rank)
	}
	for index := range out {
		for terminal := range out[index].routes {
			route := &out[index].routes[terminal]
			if err := resolveSubedgeRouteSibling(route, ranks, out); err != nil {
				return nil, fmt.Errorf("target: subedge %d route %d: %w", index, terminal, err)
			}
		}
		if err := resolveSubedgeRouteSibling(&out[index].admissionRoute, ranks, out); err != nil {
			return nil, fmt.Errorf("target: subedge %d admission failure: %w", index, err)
		}
	}
	for index := range out {
		if err := d.validateSubedge(&out[index], out); err != nil {
			return nil, fmt.Errorf("target: subedge %d: %w", index, err)
		}
	}
	if err := validateSubedgeEntries(out, d.callbacks); err != nil {
		return nil, err
	}
	if err := validateSubedgeRecurrence(out, d.callbacks); err != nil {
		return nil, err
	}
	return out, nil
}

func callbackRanks(callbacks []callbackDraft) ([]uint32, error) {
	ranks := make([]uint32, len(callbacks))
	for rank, callback := range callbacks {
		if callback.source < 0 || callback.source >= len(ranks) {
			return nil, errors.New("target: malformed callback source")
		}
		ranks[callback.source] = uint32(rank)
	}
	return ranks, nil
}

func outcomeOrdinals(outcomes []outcomeDraft) ([]uint32, error) {
	ordinals := make([]uint32, len(outcomes))
	for ordinal, outcome := range outcomes {
		if outcome.source < 0 || outcome.source >= len(ordinals) {
			return nil, errors.New("target: malformed outcome source")
		}
		ordinals[outcome.source] = uint32(ordinal)
	}
	return ordinals, nil
}

func (d *operationDraft) freezeSubedge(source int, item vocabulary.SubedgeSpec, callbackRanks, outcomeOrdinals []uint32) (subedgeDraft, error) {
	if !vocabulary.ValidSubedgeFamily(item.Family) {
		return subedgeDraft{}, errors.New("invalid family")
	}
	if item.Role == 0 {
		return subedgeDraft{}, errors.New("zero semantic role")
	}
	edge := subedgeDraft{source: source, role: item.Role, family: item.Family}
	if item.Family == vocabulary.SubedgeFamilyCall {
		if err := d.freezeCallCallee(&edge, item, callbackRanks); err != nil {
			return subedgeDraft{}, err
		}
	} else {
		if !emptySubedgeCallee(item.Callee) {
			return subedgeDraft{}, errors.New("non-Call family carries callee")
		}
		if item.Admission != schematype.CallableAdmissionOrdinary {
			return subedgeDraft{}, errors.New("non-Call family requires ordinary callable admission")
		}
		edge.admission = item.Admission
		arguments, outcomes, endpointsErr := d.freezeInlineEndpoints(item.Arguments, item.Outcomes)
		if endpointsErr != nil {
			return subedgeDraft{}, endpointsErr
		}
		if err := validateClosedFamilyABI(item.Family, arguments); err != nil {
			return subedgeDraft{}, err
		}
		edge.arguments, edge.outcomes = arguments, outcomes
	}
	origins, originsErr := d.freezeSubedgeArgumentOrigins(item.ArgumentOrigins, edge.arguments)
	if originsErr != nil {
		return subedgeDraft{}, originsErr
	}
	if item.RuleEntry && (len(origins) != 0 || argumentSegmentCount(edge.arguments) != 0) {
		return subedgeDraft{}, errors.New("RuleEntry requires an empty argument product")
	}
	edge.ruleEntry = item.RuleEntry
	edge.argumentOrigins = origins
	routes, routesErr := d.freezeSubedgeRoutes(item.Routes, outcomeOrdinals)
	if routesErr != nil {
		return subedgeDraft{}, routesErr
	}
	edge.routes = routes
	failure, failureRoute, failureErr := d.freezeSubedgeAdmissionFailure(item.AdmissionFailure, outcomeOrdinals)
	if failureErr != nil {
		return subedgeDraft{}, failureErr
	}
	edge.admissionFailure, edge.admissionRoute = failure, failureRoute
	return edge, nil
}

func (d *operationDraft) freezeCallCallee(edge *subedgeDraft, item vocabulary.SubedgeSpec, callbackRanks []uint32) error {
	switch item.Callee.Kind {
	case vocabulary.SubedgeCalleeCallback:
		if item.Callee.Callback == 0 || uint64(item.Callee.Callback) > uint64(len(callbackRanks)) ||
			!emptyCapturedInitialRead(item.Callee.Read) || !zeroLiteral(item.Callee.MetaKey) ||
			item.Admission != schematype.CallableAdmissionInvalid || !emptyValuesSpec(item.Arguments) || len(item.Outcomes) != 0 {
			return errors.New("malformed callback callee union")
		}
		source := int(item.Callee.Callback - 1)
		rank := callbackRanks[source]
		if uint64(rank) >= uint64(len(d.callbacks)) || d.callbacks[rank].source != source {
			return errors.New("callback callee outside scope")
		}
		callback := d.callbacks[rank]
		edge.callee, edge.callback, edge.callbackRank = vocabulary.SubedgeCalleeCallback, item.Callee.Callback, rank
		edge.arguments, edge.outcomes, edge.admission = callback.arguments, callback.outcomes, callback.admission
		return nil
	case vocabulary.SubedgeCalleeCapturedInitialRead:
		if item.Callee.Callback != 0 || !zeroLiteral(item.Callee.MetaKey) || item.Callee.Read.Root == "" ||
			!item.Admission.Available() {
			return errors.New("malformed captured initial read callee union")
		}
		key, err := normalizeRequiredExactKey(item.Callee.Read.Key)
		if err != nil {
			return fmt.Errorf("captured initial read key: %w", err)
		}
		arguments, outcomes, endpointsErr := d.freezeInlineEndpoints(item.Arguments, item.Outcomes)
		if endpointsErr != nil {
			return endpointsErr
		}
		edge.callee, edge.readRoot, edge.readKey = vocabulary.SubedgeCalleeCapturedInitialRead, item.Callee.Read.Root, key
		edge.arguments, edge.outcomes, edge.admission = arguments, outcomes, item.Admission
		return nil
	case vocabulary.SubedgeCalleeMetaKey:
		if item.Callee.Callback != 0 || !emptyCapturedInitialRead(item.Callee.Read) || !item.Admission.Available() {
			return errors.New("malformed meta-key callee union")
		}
		key, err := normalizeRequiredExactKey(item.Callee.MetaKey)
		if err != nil {
			return fmt.Errorf("meta key: %w", err)
		}
		arguments, outcomes, endpointsErr := d.freezeInlineEndpoints(item.Arguments, item.Outcomes)
		if endpointsErr != nil {
			return endpointsErr
		}
		edge.callee, edge.metaKey = vocabulary.SubedgeCalleeMetaKey, key
		edge.arguments, edge.outcomes, edge.admission = arguments, outcomes, item.Admission
		return nil
	default:
		return errors.New("call family has invalid callee")
	}
}

func (d *operationDraft) freezeInlineEndpoints(arguments vocabulary.ValuesSpec, terminals []vocabulary.TerminalSpec) (valuesDraft, [5]valuesDraft, error) {
	result, err := d.freezeValues(arguments, false)
	if err != nil {
		return valuesDraft{}, [5]valuesDraft{}, fmt.Errorf("arguments: %w", err)
	}
	if len(terminals) != 5 {
		return valuesDraft{}, [5]valuesDraft{}, errors.New("inline callee has incomplete terminals")
	}
	var outcomes [5]valuesDraft
	seen := [5]bool{}
	for index, terminal := range terminals {
		kind, valid := vocabulary.CrossActivationOutcomeIndex(terminal.Kind)
		if !valid || seen[kind] {
			return valuesDraft{}, [5]valuesDraft{}, fmt.Errorf("inline terminal %d is invalid or duplicate", index)
		}
		values, freezeErr := d.freezeValues(terminal.Values, false)
		if freezeErr != nil {
			return valuesDraft{}, [5]valuesDraft{}, fmt.Errorf("inline terminal %d: %w", index, freezeErr)
		}
		seen[kind], outcomes[kind] = true, values
	}
	return result, outcomes, nil
}

func validateClosedFamilyABI(family vocabulary.SubedgeFamily, arguments valuesDraft) error {
	var want int
	switch family {
	case vocabulary.SubedgeFamilyLength:
		want = 1
	case vocabulary.SubedgeFamilyIndexGet, vocabulary.SubedgeFamilyEqual, vocabulary.SubedgeFamilyLess:
		want = 2
	case vocabulary.SubedgeFamilyIndexSet:
		want = 3
	default:
		return errors.New("invalid non-Call family")
	}
	if arguments.tail != vocabulary.ValuesClosed || len(arguments.suffix) != 0 || len(arguments.types) != want {
		return fmt.Errorf("family %d requires exactly %d closed arguments", family, want)
	}
	return nil
}

func (d *operationDraft) freezeSubedgeRelation(input vocabulary.SubedgeRelationSpec) (subedgeRelationDraft, error) {
	if uint64(input.Operand) >= uint64(d.valueFormalCount()) {
		return subedgeRelationDraft{}, errors.New("target: subedge relation operand outside operation scope")
	}
	if input.Subedge == 0 || uint64(input.Subedge) > uint64(len(d.subedges)) {
		return subedgeRelationDraft{}, errors.New("target: subedge relation subedge outside operation scope")
	}
	if uint64(input.ResultOutcome) >= uint64(len(d.outcomes)) {
		return subedgeRelationDraft{}, errors.New("target: subedge relation outcome outside operation scope")
	}
	resultOutcome := d.outcomes[int(input.ResultOutcome)]
	if uint64(input.Result) >= uint64(len(resultOutcome.values.types)) {
		return subedgeRelationDraft{}, errors.New("target: subedge relation result outside outcome prefix")
	}
	effects := append([]uint32(nil), input.EffectAliases...)
	sort.Slice(effects, func(left, right int) bool { return effects[left] < effects[right] })
	for index, effect := range effects {
		if uint64(effect) >= uint64(len(d.effects)) || (index != 0 && effects[index-1] == effect) {
			return subedgeRelationDraft{}, fmt.Errorf("target: subedge relation effect alias %d invalid", index)
		}
	}
	rank := indexOfSubedgeSource(d.subedges, int(input.Subedge-1))
	if rank < 0 {
		return subedgeRelationDraft{}, errors.New("target: subedge relation subedge outside operation scope")
	}
	outcome := indexOfOutcomeSource(d.outcomes, int(input.ResultOutcome))
	if outcome < 0 {
		return subedgeRelationDraft{}, errors.New("target: subedge relation outcome outside operation scope")
	}
	return subedgeRelationDraft{
		operand: input.Operand, selector: input.Selector, subedgeSource: input.Subedge,
		subedgeRank: uint32(rank), resultOutcome: uint32(outcome), result: input.Result,
		effects: effects,
	}, nil
}

func indexOfSubedgeSource(rows []subedgeDraft, source int) int {
	for index := range rows {
		if rows[index].source == source {
			return index
		}
	}
	return -1
}

func indexOfOutcomeSource(rows []outcomeDraft, source int) int {
	for index := range rows {
		if rows[index].source == source {
			return index
		}
	}
	return -1
}
