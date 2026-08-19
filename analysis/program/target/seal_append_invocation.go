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
func (c *Contract) querySubedgeInputs(draft *operationDraft, values map[string]vocabulary.Values, keys exactkey.Table, effectCount int) ([]operationvalue.SubedgeInput, *operationvalue.SubedgeRelationInput, error) {
	if draft == nil {
		return nil, nil, errors.New("target: nil subedge draft")
	}
	rows := make([]operationvalue.SubedgeInput, len(draft.subedges))
	for index, edge := range draft.subedges {
		row := operationvalue.SubedgeInput{
			Source: uint32(index), Role: edge.Role, Family: edge.Family, Callee: edge.Callee.Kind,
			Admission: edge.Admission, RuleEntry: edge.RuleEntry,
		}
		if edge.Callee.Kind == vocabulary.SubedgeCalleeCallback {
			if edge.Callee.Callback == 0 {
				return nil, nil, errors.New("target: zero callback subedge source")
			}
			row.CallbackSource = uint32(edge.Callee.Callback - 1)
		} else {
			arguments, err := lookupSpecValues(draft, values, edge.Arguments)
			if err != nil {
				return nil, nil, err
			}
			row.Arguments = arguments
			row.CallbackSource = ^uint32(0)
			row.Terminals = make([]operationvalue.SubedgeTerminalInput, len(edge.Outcomes))
			for terminal, endpoint := range edge.Outcomes {
				value, valueErr := lookupSpecValues(draft, values, endpoint.Values)
				if valueErr != nil {
					return nil, nil, valueErr
				}
				row.Terminals[terminal] = operationvalue.SubedgeTerminalInput{Kind: endpoint.Kind, Values: value}
			}
		}
		failure, failureErr := lookupSpecValues(draft, values, edge.AdmissionFailure.Values)
		if failureErr != nil {
			return nil, nil, failureErr
		}
		row.AdmissionFailure = failure
		row.ArgumentOrigins = make([]operationvalue.SubedgeArgumentOriginInput, len(edge.ArgumentOrigins))
		for originIndex, origin := range edge.ArgumentOrigins {
			row.ArgumentOrigins[originIndex] = operationvalue.SubedgeArgumentOriginInput{
				Segment: origin.Segment, Index: origin.Index, Kind: origin.Kind, Source: origin.Source,
			}
		}
		if edge.Callee.Kind == vocabulary.SubedgeCalleeCapturedInitialRead {
			if index >= len(draft.subedgeReadRoots) || draft.subedgeReadRoots[index] == 0 {
				return nil, nil, errors.New("target: unresolved captured initial read root")
			}
			key, keyErr := exactKeyHandle(keys, edge.Callee.Read.Key)
			if keyErr != nil {
				return nil, nil, keyErr
			}
			row.ReadRoot, row.ReadKey = draft.subedgeReadRoots[index], key
		}
		if edge.Callee.Kind == vocabulary.SubedgeCalleeMetaKey {
			key, keyErr := exactKeyHandle(keys, edge.Callee.MetaKey)
			if keyErr != nil {
				return nil, nil, keyErr
			}
			row.MetaKey = key
		}
		var routeErr error
		failureRoute := edge.AdmissionFailure.Route
		row.AdmissionRoute, routeErr = querySubedgeRouteInput(draft, failureRoute.Route, failureRoute.Adjustment, failureRoute.Result, failureRoute.Placement, failureRoute.Offset, failureRoute.Outcome, failureRoute.Subedge, values)
		if routeErr != nil {
			return nil, nil, routeErr
		}
		if len(edge.Routes) != len(row.Routes) {
			return nil, nil, errors.New("target: incomplete subedge route table")
		}
		for terminal, route := range edge.Routes {
			row.Routes[terminal], routeErr = querySubedgeRouteInput(draft, route.Route, route.Adjustment, route.Result, route.Placement, route.Offset, route.Outcome, route.Subedge, values)
			if routeErr != nil {
				return nil, nil, routeErr
			}
		}
		rows[index] = row
	}
	var relation *operationvalue.SubedgeRelationInput
	if draft.subedgeRelation != nil {
		r := draft.subedgeRelation
		if r.Subedge == 0 {
			return nil, nil, errors.New("target: zero subedge relation source")
		}
		effects := append([]uint32(nil), r.EffectAliases...)
		sort.Slice(effects, func(left, right int) bool { return effects[left] < effects[right] })
		if len(effects) > effectCount {
			return nil, nil, errors.New("target: subedge relation effect alias outside operation")
		}
		relation = &operationvalue.SubedgeRelationInput{
			Operand: r.Operand, Selector: r.Selector, SubedgeRank: uint32(r.Subedge - 1),
			ResultOutcome: r.ResultOutcome, Result: r.Result, EffectAliases: effects,
		}
	}
	return rows, relation, nil
}

func lookupSpecValues(draft *operationDraft, values map[string]vocabulary.Values, spec vocabulary.ValuesSpec) (vocabulary.Values, error) {
	row, err := draft.freezeValues(spec, false)
	if err != nil {
		return 0, err
	}
	return lookupDraftValues(values, row)
}

func querySubedgeRouteInput(draft *operationDraft, route vocabulary.SubedgeRoute, adjustment vocabulary.Adjustment, resultSpec vocabulary.ValuesSpec, placement vocabulary.Placement, offset, outcome uint32, sibling vocabulary.SubedgeRef, values map[string]vocabulary.Values) (operationvalue.SubedgeRouteInput, error) {
	result, resultErr := lookupSpecValues(draft, values, resultSpec)
	if resultErr != nil {
		return operationvalue.SubedgeRouteInput{}, resultErr
	}
	siblingRank := uint32(0)
	if sibling != 0 {
		siblingRank = uint32(sibling - 1)
	}
	if route == vocabulary.RouteSubedge || route == vocabulary.RouteContinue || route == vocabulary.RoutePropagateYield || (route == vocabulary.RouteRejectYield && sibling != 0) {
		outcome = ^uint32(0)
	}
	return operationvalue.SubedgeRouteInput{
		Route: route, Adjustment: adjustment, Result: result,
		Placement: placement, Offset: offset, Outcome: outcome,
		HasSibling: sibling != 0, SiblingRank: siblingRank,
	}, nil
}

func appendCallbacks(builder *operationvalue.QueryBuilder, owner vocabulary.Operation, input []callbackDraft, values map[string]vocabulary.Values, issued []vocabulary.CallbackID) ([]vocabulary.CallbackID, []operationvalue.CallbackQueryInput, error) {
	if builder == nil || owner == 0 {
		return nil, nil, errors.New("target: unavailable callback owner")
	}
	if len(issued) != len(input) {
		return nil, nil, errors.New("target: operation callback geometry mismatch")
	}
	ids := append([]vocabulary.CallbackID(nil), issued...)
	query := make([]operationvalue.CallbackQueryInput, len(input))
	for index := range input {
		callback := &input[index]
		id := issued[index]
		if id == 0 {
			return nil, nil, errors.New("target: operation callback geometry has zero handle")
		}
		ids[callback.source] = id
		callback.sealed = id
		effects := make([]operationvalue.EffectInput, len(callback.effects.effects))
		for effectIndex, effect := range callback.effects.effects {
			effects[effectIndex] = effectInput(effect)
		}
		if err := builder.AppendCallbackEffects(id, callback.effects.tail, callback.effects.variable, effects); err != nil {
			return nil, nil, err
		}
		arguments, valuesErr := lookupDraftValues(values, callback.arguments)
		if valuesErr != nil {
			return nil, nil, valuesErr
		}
		var outcomes [5]vocabulary.Values
		for terminal := range callback.outcomes {
			value, terminalErr := lookupDraftValues(values, callback.outcomes[terminal])
			if terminalErr != nil {
				return nil, nil, terminalErr
			}
			outcomes[terminal] = value
		}
		query[index] = operationvalue.CallbackQueryInput{
			Source: uint32(callback.source), Admission: callback.admission,
			Arguments: arguments, Outcomes: outcomes,
		}
	}
	return ids, query, nil
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

func callbackReleaseInputs(drafts []operationDraft) ([]operationvalue.CallbackReleaseInput, error) {
	if len(drafts) == 0 {
		return nil, nil
	}
	inputs := make([]operationvalue.CallbackReleaseInput, 0)
	for draftIndex := range drafts {
		for callbackIndex := range drafts[draftIndex].callbacks {
			callback := drafts[draftIndex].callbacks[callbackIndex]
			if callback.release == nil {
				continue
			}
			if callback.sealed == 0 || callback.release.operation == 0 ||
				uint64(callback.release.operation) > uint64(len(drafts)) {
				return nil, errors.New("target: unresolved callback release")
			}
			inputs = append(inputs, operationvalue.CallbackReleaseInput{
				Callback: callback.sealed, Operation: callback.release.operation,
				Input: callback.release.input, Outcome: callback.release.outcome,
				Mode: callback.release.mode, ZeroBehavior: callback.release.zeroBehavior,
				ZeroOutcome: callback.release.zeroOutcome,
			})
		}
	}
	return inputs, nil
}
