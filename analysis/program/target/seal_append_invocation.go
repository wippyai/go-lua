package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"
)

// querySubedgeInputs resolves the Target draft's Values and exact-key
// declarations into the neutral operation-owner boundary. No Target row or
// draft escapes into operation.Core.
func (c *Contract) querySubedgeInputs(draft *operationDraft, callbacks []vocabulary.CallbackID, values map[string]vocabulary.Values, keys exactkey.Table, effectCount int) ([]operationvalue.SubedgeInput, *operationvalue.SubedgeRelationInput, error) {
	if draft == nil {
		return nil, nil, errors.New("target: nil subedge draft")
	}
	rows := make([]operationvalue.SubedgeInput, len(draft.subedges))
	for index, edge := range draft.subedges {
		arguments, err := lookupDraftValues(values, edge.arguments)
		if err != nil {
			return nil, nil, err
		}
		row := operationvalue.SubedgeInput{
			Role: edge.role, Family: edge.family, Callee: edge.callee,
			Admission: edge.admission, Arguments: arguments, RuleEntry: edge.ruleEntry,
		}
		for terminal := range edge.outcomes {
			value, valueErr := lookupDraftValues(values, edge.outcomes[terminal])
			if valueErr != nil {
				return nil, nil, valueErr
			}
			row.Outcomes[terminal] = value
		}
		failure, failureErr := lookupDraftValues(values, edge.admissionFailure)
		if failureErr != nil {
			return nil, nil, failureErr
		}
		row.AdmissionFailure = failure
		row.ArgumentOrigins = make([]operationvalue.SubedgeArgumentOriginInput, len(edge.argumentOrigins))
		for originIndex, origin := range edge.argumentOrigins {
			row.ArgumentOrigins[originIndex] = operationvalue.SubedgeArgumentOriginInput{
				Segment: origin.segment, Index: origin.index, Kind: origin.kind, Source: origin.source,
			}
		}
		if edge.callee == vocabulary.SubedgeCalleeCallback {
			if edge.callback == 0 || int(edge.callback) > len(callbacks) || callbacks[edge.callback-1] == 0 {
				return nil, nil, errors.New("target: unresolved callback subedge callee")
			}
			row.Callback = callbacks[edge.callback-1]
		}
		if edge.callee == vocabulary.SubedgeCalleeCapturedInitialRead {
			if edge.readRootID == 0 {
				return nil, nil, errors.New("target: unresolved captured initial read root")
			}
			key, keyErr := exactKeyHandle(keys, edge.readKey)
			if keyErr != nil {
				return nil, nil, keyErr
			}
			row.ReadRoot, row.ReadKey = edge.readRootID, key
		}
		if edge.callee == vocabulary.SubedgeCalleeMetaKey {
			key, keyErr := exactKeyHandle(keys, edge.metaKey)
			if keyErr != nil {
				return nil, nil, keyErr
			}
			row.MetaKey = key
		}
		var routeErr error
		row.AdmissionRoute, routeErr = c.querySubedgeRouteInput(edge.admissionRoute, values)
		if routeErr != nil {
			return nil, nil, routeErr
		}
		for terminal, route := range edge.routes {
			row.Routes[terminal], routeErr = c.querySubedgeRouteInput(route, values)
			if routeErr != nil {
				return nil, nil, routeErr
			}
		}
		rows[index] = row
	}
	var relation *operationvalue.SubedgeRelationInput
	if draft.subedgeRelation != nil {
		r := draft.subedgeRelation
		effects := append([]uint32(nil), r.effects...)
		if len(effects) > effectCount {
			return nil, nil, errors.New("target: subedge relation effect alias outside operation")
		}
		relation = &operationvalue.SubedgeRelationInput{
			Operand: r.operand, Selector: r.selector, SubedgeRank: r.subedgeRank,
			ResultOutcome: r.resultOutcome, Result: r.result, EffectAliases: effects,
		}
	}
	return rows, relation, nil
}

func (c *Contract) querySubedgeRouteInput(route subedgeRouteDraft, values map[string]vocabulary.Values) (operationvalue.SubedgeRouteInput, error) {
	result, resultErr := lookupDraftValues(values, route.result)
	if resultErr != nil {
		return operationvalue.SubedgeRouteInput{}, resultErr
	}
	destination := vocabulary.Values(0)
	if route.destination.tail != 0 || route.destination.varID != 0 || route.destination.tailType != "" || len(route.destination.types) != 0 || len(route.destination.suffix) != 0 {
		var destinationErr error
		destination, destinationErr = lookupDraftValues(values, route.destination)
		if destinationErr != nil {
			return operationvalue.SubedgeRouteInput{}, destinationErr
		}
	}
	return operationvalue.SubedgeRouteInput{
		Route: route.route, Adjustment: route.adjustment, Result: result,
		Placement: route.placement, Offset: route.offset, Outcome: route.outcome,
		HasSibling: route.subedge != 0, SiblingRank: route.subedgeRank, Destination: destination,
	}, nil
}

func (c *Contract) appendCallbacks(builder *operationvalue.QueryBuilder, owner vocabulary.Operation, input []callbackDraft, values map[string]vocabulary.Values, issued []vocabulary.CallbackID) ([]vocabulary.CallbackID, error) {
	if _, err := checkedStoredRange("callback table", len(c.callbacks), len(input)); err != nil {
		return nil, err
	}
	if len(issued) != len(input) {
		return nil, errors.New("target: operation callback geometry mismatch")
	}
	ids := append([]vocabulary.CallbackID(nil), issued...)
	for index := range input {
		callback := &input[index]
		id := issued[index]
		if id == 0 {
			return nil, errors.New("target: operation callback geometry has zero handle")
		}
		ids[callback.source] = id
		callback.sealed = id
		effects := make([]operationvalue.EffectInput, len(callback.effects.effects))
		for effectIndex, effect := range callback.effects.effects {
			effects[effectIndex] = effectInput(effect)
		}
		if err := builder.AppendCallbackEffects(id, callback.effects.tail, callback.effects.variable, effects); err != nil {
			return nil, err
		}
		arguments, valuesErr := lookupDraftValues(values, callback.arguments)
		if valuesErr != nil {
			return nil, valuesErr
		}
		var outcomes [5]vocabulary.Values
		for terminal := range callback.outcomes {
			value, terminalErr := lookupDraftValues(values, callback.outcomes[terminal])
			if terminalErr != nil {
				return nil, terminalErr
			}
			outcomes[terminal] = value
		}
		c.callbacks = append(c.callbacks, callbackRow{
			function: callback.function, admission: callback.admission,
			arguments: arguments, outcomes: outcomes,
		})
	}
	return ids, nil
}

func effectInput(effect effectDraft) operationvalue.EffectInput {
	input := operationvalue.EffectInput{
		Target:         effect.target,
		Values:         append([]vocabulary.ValueFormal(nil), effect.values...),
		Types:          append([]vocabulary.TypeFormal(nil), effect.types...),
		ValuesVar:      append([]vocabulary.ValuesVar(nil), effect.valuesVar...),
		Rows:           append([]vocabulary.RowVar(nil), effect.rows...),
		HasPublication: effect.hasPublication,
	}
	if effect.hasPublication {
		input.Publication = effect.publication
	}
	return input
}

func (c *Contract) appendCallbackReleases(drafts []operationDraft) error {
	pending := make([][]callbackReleaseRow, len(c.operations))
	for draftIndex := range drafts {
		for callbackIndex := range drafts[draftIndex].callbacks {
			callback := drafts[draftIndex].callbacks[callbackIndex]
			if callback.release == nil {
				continue
			}
			if callback.sealed == 0 || callback.release.operation == 0 ||
				uint64(callback.release.operation) > uint64(len(pending)) {
				return errors.New("target: unresolved callback release")
			}
			pending[uint32(callback.release.operation)-1] = append(
				pending[uint32(callback.release.operation)-1],
				callbackReleaseRow{
					callback: callback.sealed, operation: callback.release.operation,
					input: callback.release.input, outcome: callback.release.outcome,
					mode: callback.release.mode, zeroBehavior: callback.release.zeroBehavior,
					zeroOutcome: callback.release.zeroOutcome,
				},
			)
		}
	}
	for operation := range pending {
		releases := pending[operation]
		if len(releases) == 0 {
			continue
		}
		sort.Slice(releases, func(left, right int) bool {
			return compareCallbackRelease(releases[left], releases[right]) < 0
		})
		for index := 1; index < len(releases); index++ {
			if releases[index-1].callback == releases[index].callback {
				return errors.New("target: callback has duplicate release")
			}
		}
		rangeOut, err := checkedStoredRange("callback release table", len(c.callbackReleases), len(releases))
		if err != nil {
			return err
		}
		c.operations[operation].releases = rangeOut
		for _, release := range releases {
			handle, handleErr := checkedStoredHandle("callback release table", len(c.callbackReleases))
			if handleErr != nil {
				return handleErr
			}
			c.callbackReleases = append(c.callbackReleases, release)
			c.callbacks[uint32(release.callback)-1].release = handle
		}
	}
	return nil
}

func compareCallbackRelease(left, right callbackReleaseRow) int {
	if left.callback < right.callback {
		return -1
	}
	if left.callback > right.callback {
		return 1
	}
	if left.input < right.input {
		return -1
	}
	if left.input > right.input {
		return 1
	}
	if left.outcome < right.outcome {
		return -1
	}
	if left.outcome > right.outcome {
		return 1
	}
	if left.mode < right.mode {
		return -1
	}
	if left.mode > right.mode {
		return 1
	}
	if left.zeroBehavior < right.zeroBehavior {
		return -1
	}
	if left.zeroBehavior > right.zeroBehavior {
		return 1
	}
	if left.zeroOutcome < right.zeroOutcome {
		return -1
	}
	if left.zeroOutcome > right.zeroOutcome {
		return 1
	}
	return 0
}
