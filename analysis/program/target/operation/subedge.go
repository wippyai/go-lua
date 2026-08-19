package operation

import (
	"errors"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// appendQuerySubedges is the sole construction path for the immutable
// operation-owned Subedge table. The input is already neutral: Target has
// resolved authored callbacks, sibling ranks, InitialRoots, and ExactKeys.
// Core still repeats all coordinate/owner fences before publishing rows.
func (core *Core) appendQuerySubedges(op vocabulary.Operation, operation *queryOperationRow, input QueryOperationInput) error {
	if core == nil || operation == nil {
		return invalidQuery("nil operation subedge row")
	}
	if len(input.Subedges) == 0 {
		if input.SubedgeRelation != nil {
			return invalidQuery("subedge relation has no subedge table")
		}
		return nil
	}
	if len(input.Subedges) >= int(^uint32(0)) {
		return invalidQuery("subedge table overflow")
	}
	start := len(core.query.subedges)
	ids := make([]vocabulary.SubedgeID, len(input.Subedges))
	sourceIDs := make([]vocabulary.SubedgeID, len(input.Subedges))
	sourceSeen := make([]bool, len(input.Subedges))
	for index, item := range input.Subedges {
		if item.Role == 0 || !vocabulary.ValidSubedgeFamily(item.Family) {
			return invalidQuery("subedge has invalid role or family")
		}
		if item.Source >= uint32(len(input.Subedges)) || sourceSeen[item.Source] {
			return invalidQuery("subedge source coordinate is malformed")
		}
		sourceSeen[item.Source] = true
		if index > 0 && input.Subedges[index-1].Role >= item.Role {
			return invalidQuery("subedge rows are not strictly ordered")
		}
		handle, err := checkedSubedgeHandle(len(core.query.subedges) + index)
		if err != nil {
			return err
		}
		ids[index] = handle
		sourceIDs[item.Source] = handle
	}
	for index, item := range input.Subedges {
		if index > 0 && input.Subedges[index-1].Role == item.Role {
			return invalidQuery("duplicate subedge role")
		}
		if err := core.validateSubedgeInput(op, item, len(input.Subedges), len(input.EffectIndices)); err != nil {
			return err
		}
	}
	// Validate sibling ranks and relation coordinates before appending any
	// reverse callback or relation row. A failed builder is consumed, but this
	// ordering keeps the one-shot mutation easy to audit.
	for index, item := range input.Subedges {
		row := querySubedgeRow{
			owner: op, role: item.Role, family: item.Family, callee: item.Callee,
			callback: item.Callback, readRoot: item.ReadRoot, readKey: item.ReadKey,
			metaKey: item.MetaKey, admission: item.Admission, arguments: item.Arguments,
			ruleEntry: item.RuleEntry, outcomes: item.Outcomes,
			admissionFailure: item.AdmissionFailure,
		}
		originStart := len(core.query.subedgeOrigins)
		for _, origin := range item.ArgumentOrigins {
			core.query.subedgeOrigins = append(core.query.subedgeOrigins, querySubedgeArgumentOriginRow{
				segment: origin.Segment, index: origin.Index, kind: origin.Kind, source: origin.Source,
			})
		}
		row.argumentOrigins = queryRange{start: originStart, end: len(core.query.subedgeOrigins)}
		var err error
		row.admissionRoute, err = core.appendQuerySubedgeRoute(item.AdmissionRoute, sourceIDs, op, len(input.Subedges), operation.outcomes.len())
		if err != nil {
			return err
		}
		for terminal, route := range item.Routes {
			row.routes[terminal], err = core.appendQuerySubedgeRoute(route, sourceIDs, op, len(input.Subedges), operation.outcomes.len())
			if err != nil {
				return err
			}
		}
		core.query.subedges = append(core.query.subedges, row)
		if item.Callee == vocabulary.SubedgeCalleeCallback {
			callback := &core.query.callbacks[int(item.Callback)-1]
			if callback.subedge != 0 {
				return invalidQuery("callback has multiple direct subedges")
			}
			callback.subedge = ids[index]
		}
	}
	operation.subedges = queryRange{start: start, end: len(core.query.subedges)}
	if input.SubedgeRelation != nil {
		relation, err := core.appendQuerySubedgeRelation(op, operation, *input.SubedgeRelation, ids, len(input.EffectIndices))
		if err != nil {
			return err
		}
		operation.subedgeRelation = relation
	}
	return nil
}

func checkedSubedgeHandle(index int) (vocabulary.SubedgeID, error) {
	if index < 0 || uint64(index) >= uint64(^uint32(0)) {
		return 0, errors.New("target/operation: subedge table overflow")
	}
	return vocabulary.SubedgeID(index + 1), nil
}

func (core Core) validateSubedgeInput(op vocabulary.Operation, item SubedgeInput, count, effectCount int) error {
	if !item.Admission.Available() {
		return invalidQuery("subedge has invalid admission")
	}
	if !core.validOwnedSubedgeValues(op, item.Arguments) || !core.validOwnedSubedgeValues(op, item.AdmissionFailure) {
		return invalidQuery("subedge Values endpoint is outside owner")
	}
	for _, value := range item.Outcomes {
		if !core.validOwnedSubedgeValues(op, value) {
			return invalidQuery("subedge outcome Values endpoint is outside owner")
		}
	}
	if err := core.validateSubedgeCallee(op, item); err != nil {
		return err
	}
	seenOrigins := make(map[[2]uint32]struct{}, len(item.ArgumentOrigins))
	for _, origin := range item.ArgumentOrigins {
		key := [2]uint32{uint32(origin.Segment), origin.Index}
		if _, exists := seenOrigins[key]; exists {
			return invalidQuery("duplicate subedge argument origin")
		}
		seenOrigins[key] = struct{}{}
		if !validSubedgeArgumentCoordinate(core, item.Arguments, origin.Segment, origin.Index) {
			return invalidQuery("subedge argument origin is outside Values")
		}
		switch origin.Kind {
		case vocabulary.ArgumentSourceRule:
			if origin.Source != (vocabulary.InputSource{}) {
				return invalidQuery("Rule argument origin carries input")
			}
		case vocabulary.ArgumentSourceInput:
			if !validSubedgeInputSource(core, op, origin.Source) {
				return invalidQuery("subedge argument origin input is outside owner ABI")
			}
		default:
			return invalidQuery("subedge argument origin has invalid source")
		}
	}
	for _, route := range item.Routes {
		if err := core.validateSubedgeRouteInput(op, route, count, core.OutcomeCount(op)); err != nil {
			return err
		}
	}
	return core.validateSubedgeRouteInput(op, item.AdmissionRoute, count, core.OutcomeCount(op))
}

func (core Core) validateSubedgeCallee(op vocabulary.Operation, item SubedgeInput) error {
	switch item.Family {
	case vocabulary.SubedgeFamilyCall:
		switch item.Callee {
		case vocabulary.SubedgeCalleeCallback:
			owner, ok := core.CallbackOwner(item.Callback)
			if !ok || owner != op {
				return invalidQuery("callback subedge callee is outside owner")
			}
			if item.ReadRoot != 0 || item.ReadKey != 0 || item.MetaKey != 0 {
				return invalidQuery("callback subedge callee carries foreign source")
			}
		case vocabulary.SubedgeCalleeCapturedInitialRead:
			if item.Callback != 0 || item.ReadRoot == 0 || !core.validExactKey(item.ReadKey) || item.MetaKey != 0 {
				return invalidQuery("captured initial read callee is malformed")
			}
		case vocabulary.SubedgeCalleeMetaKey:
			if item.Callback != 0 || item.ReadRoot != 0 || item.ReadKey != 0 || !core.validExactKey(item.MetaKey) {
				return invalidQuery("meta-key callee is malformed")
			}
		default:
			return invalidQuery("Call subedge has invalid callee")
		}
	default:
		if item.Callee != vocabulary.SubedgeCalleeInvalid || item.Callback != 0 || item.ReadRoot != 0 || item.ReadKey != 0 || item.MetaKey != 0 || item.Admission != schematype.CallableAdmissionOrdinary {
			return invalidQuery("non-Call subedge callee is malformed")
		}
	}
	return nil
}

func (core Core) validExactKey(key vocabulary.ExactKey) bool {
	if key == 0 {
		return false
	}
	_, ok := core.keys.Value(key)
	return ok
}

func (core Core) validOwnedSubedgeValues(op vocabulary.Operation, values vocabulary.Values) bool {
	if values == 0 {
		return false
	}
	row, ok := core.queryValues(values)
	return ok && row.owner == op
}

func validSubedgeArgumentCoordinate(core Core, values vocabulary.Values, segment vocabulary.ArgumentSegment, index uint32) bool {
	row, ok := core.queryValues(values)
	if !ok {
		return false
	}
	switch segment {
	case vocabulary.ArgumentFixed:
		return uint64(index) < uint64(len(row.types))
	case vocabulary.ArgumentSuffix:
		return uint64(index) < uint64(len(row.suffix))
	case vocabulary.ArgumentTail:
		return row.tail == vocabulary.ValuesVariable && index == 0
	default:
		return false
	}
}

func validSubedgeInputSource(core Core, op vocabulary.Operation, source vocabulary.InputSource) bool {
	switch source.Kind {
	case vocabulary.InputSourceValueFormal:
		return uint64(source.Ordinal) < uint64(core.ValueFormalCount(op))
	case vocabulary.InputSourceValuesVar:
		return uint64(source.Ordinal) < uint64(core.ValuesVarCount(op))
	default:
		return false
	}
}

func (core Core) validateSubedgeRouteInput(op vocabulary.Operation, route SubedgeRouteInput, count, outcomes int) error {
	if !core.validOwnedSubedgeValues(op, route.Result) {
		return invalidQuery("subedge route result is outside owner")
	}
	switch route.Route {
	case vocabulary.RouteOutcome:
		if route.HasSibling || route.Outcome >= uint32(outcomes) || !core.validOwnedSubedgeValues(op, route.Destination) {
			return invalidQuery("subedge outcome route is malformed")
		}
	case vocabulary.RouteSubedge:
		if !route.HasSibling || route.Outcome != 0 || route.SiblingRank >= uint32(count) || !core.validOwnedSubedgeValues(op, route.Destination) {
			return invalidQuery("subedge sibling route is malformed")
		}
	case vocabulary.RouteRejectYield:
		if route.HasSibling {
			if route.Outcome != 0 || route.SiblingRank >= uint32(count) || !core.validOwnedSubedgeValues(op, route.Destination) {
				return invalidQuery("subedge C-boundary sibling route is malformed")
			}
		} else if route.Outcome >= uint32(outcomes) || !core.validOwnedSubedgeValues(op, route.Destination) {
			return invalidQuery("subedge C-boundary outcome route is malformed")
		}
	case vocabulary.RouteContinue, vocabulary.RoutePropagateYield:
		if route.HasSibling || route.Outcome != 0 || route.Destination != 0 {
			return invalidQuery("subedge continuation route is malformed")
		}
	default:
		return invalidQuery("subedge route has invalid disposition")
	}
	return nil
}

func (core Core) appendQuerySubedgeRoute(input SubedgeRouteInput, ids []vocabulary.SubedgeID, op vocabulary.Operation, count, outcomes int) (querySubedgeRouteRow, error) {
	if err := core.validateSubedgeRouteInput(op, input, count, outcomes); err != nil {
		return querySubedgeRouteRow{}, err
	}
	row := querySubedgeRouteRow{
		route: input.Route, adjustment: input.Adjustment, result: input.Result,
		placement: input.Placement, offset: input.Offset, outcome: input.Outcome,
		destination: input.Destination,
	}
	if input.HasSibling {
		if input.SiblingRank >= uint32(len(ids)) || ids[input.SiblingRank] == 0 {
			return querySubedgeRouteRow{}, invalidQuery("subedge sibling rank is unresolved")
		}
		row.subedge = ids[input.SiblingRank]
	}
	return row, nil
}

func (core *Core) appendQuerySubedgeRelation(op vocabulary.Operation, operation *queryOperationRow, input SubedgeRelationInput, ids []vocabulary.SubedgeID, effectCount int) (uint32, error) {
	if uint64(input.Operand) >= uint64(core.ValueFormalCount(op)) || input.SubedgeRank >= uint32(len(ids)) || input.ResultOutcome >= uint32(operation.outcomes.len()) {
		return 0, invalidQuery("subedge relation coordinate is outside owner")
	}
	if input.Result >= core.outcomeSlots(op, input.ResultOutcome) {
		return 0, invalidQuery("subedge relation result is outside owner outcome")
	}
	for index, effect := range input.EffectAliases {
		if uint64(effect) >= uint64(effectCount) || (index > 0 && input.EffectAliases[index-1] >= effect) {
			return 0, invalidQuery("subedge relation effect alias is malformed")
		}
	}
	start := len(core.query.subedgeRelationEffects)
	core.query.subedgeRelationEffects = append(core.query.subedgeRelationEffects, input.EffectAliases...)
	core.query.subedgeRelations = append(core.query.subedgeRelations, querySubedgeRelationRow{
		operand: input.Operand, selector: input.Selector, subedge: ids[input.SubedgeRank],
		resultOutcome: input.ResultOutcome, result: input.Result,
		effects: queryRange{start: start, end: len(core.query.subedgeRelationEffects)},
	})
	return uint32(len(core.query.subedgeRelations)), nil
}

func (core Core) querySubedge(id vocabulary.SubedgeID) (querySubedgeRow, bool) {
	if id == 0 || uint64(id) > uint64(len(core.query.subedges)) {
		return querySubedgeRow{}, false
	}
	return core.query.subedges[int(id)-1], true
}

func (core Core) CallbackSubedge(id vocabulary.CallbackID) (vocabulary.SubedgeID, bool) {
	row, ok := core.callbackQuery(id)
	if !ok || row.subedge == 0 {
		return 0, false
	}
	edge, edgeOK := core.querySubedge(row.subedge)
	return row.subedge, edgeOK && edge.callee == vocabulary.SubedgeCalleeCallback && edge.callback == id
}

func (core Core) SubedgeCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok {
		return 0
	}
	return row.subedges.len()
}

func (core Core) SubedgeAt(op vocabulary.Operation, index int) (vocabulary.SubedgeID, bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 || index >= row.subedges.len() {
		return 0, false
	}
	return vocabulary.SubedgeID(row.subedges.start + index + 1), true
}

func (core Core) SubedgeOwner(id vocabulary.SubedgeID) (vocabulary.Operation, bool) {
	row, ok := core.querySubedge(id)
	if !ok || row.owner == 0 {
		return 0, false
	}
	owner, ownerOK := core.queryOperation(row.owner)
	index := int(id) - 1
	return row.owner, ownerOK && index >= owner.subedges.start && index < owner.subedges.end
}

func (core Core) SubedgeRole(id vocabulary.SubedgeID) (uint32, bool) {
	row, ok := core.querySubedge(id)
	return row.role, ok
}

func (core Core) SubedgeFamily(id vocabulary.SubedgeID) (vocabulary.SubedgeFamily, bool) {
	row, ok := core.querySubedge(id)
	return row.family, ok
}

func (core Core) SubedgeCallee(id vocabulary.SubedgeID) (vocabulary.SubedgeCalleeKind, bool) {
	row, ok := core.querySubedge(id)
	return row.callee, ok
}

func (core Core) SubedgeAdmission(id vocabulary.SubedgeID) (schematype.CallableAdmission, bool) {
	row, ok := core.querySubedge(id)
	return row.admission, ok
}

func (core Core) SubedgeCallback(id vocabulary.SubedgeID) (vocabulary.CallbackID, bool) {
	row, ok := core.querySubedge(id)
	if !ok || row.callee != vocabulary.SubedgeCalleeCallback || row.callback == 0 {
		return 0, false
	}
	owner, ownerOK := core.SubedgeOwner(id)
	callbackOwner, callbackOK := core.CallbackOwner(row.callback)
	return row.callback, ownerOK && callbackOK && owner == callbackOwner
}

func (core Core) SubedgeCapturedInitialRead(id vocabulary.SubedgeID) (vocabulary.InitialRoot, vocabulary.ExactKey, bool) {
	row, ok := core.querySubedge(id)
	if !ok || row.callee != vocabulary.SubedgeCalleeCapturedInitialRead || row.readRoot == 0 || !core.validExactKey(row.readKey) {
		return 0, 0, false
	}
	return row.readRoot, row.readKey, true
}

func (core Core) SubedgeMetaKey(id vocabulary.SubedgeID) (vocabulary.ExactKey, bool) {
	row, ok := core.querySubedge(id)
	if !ok || row.callee != vocabulary.SubedgeCalleeMetaKey || !core.validExactKey(row.metaKey) {
		return 0, false
	}
	return row.metaKey, true
}

func (core Core) SubedgeArguments(id vocabulary.SubedgeID) (vocabulary.Values, bool) {
	row, ok := core.querySubedge(id)
	return row.arguments, ok
}

func (core Core) SubedgeRuleEntry(id vocabulary.SubedgeID) (bool, bool) {
	row, ok := core.querySubedge(id)
	return row.ruleEntry, ok
}

func (core Core) SubedgeArgumentOriginCount(id vocabulary.SubedgeID) int {
	row, ok := core.querySubedge(id)
	if !ok {
		return 0
	}
	return row.argumentOrigins.len()
}

func (core Core) SubedgeArgumentOriginAt(id vocabulary.SubedgeID, index int) (segment vocabulary.ArgumentSegment, ordinal uint32, source vocabulary.ArgumentSource, input vocabulary.InputSource, ok bool) {
	row, found := core.querySubedge(id)
	if !found || index < 0 || index >= row.argumentOrigins.len() {
		return vocabulary.ArgumentSegmentInvalid, 0, vocabulary.ArgumentSourceInvalid, vocabulary.InputSource{}, false
	}
	item := core.query.subedgeOrigins[row.argumentOrigins.start+index]
	return item.segment, item.index, item.kind, item.source, true
}

func (core Core) SubedgeTerminal(id vocabulary.SubedgeID, kind flowkind.OutcomeKind) (vocabulary.Values, bool) {
	index, valid := vocabulary.CrossActivationOutcomeIndex(kind)
	if !valid {
		return 0, false
	}
	row, ok := core.querySubedge(id)
	return row.outcomes[index], ok
}

func (core Core) SubedgeAdmissionFailure(id vocabulary.SubedgeID) (vocabulary.Values, bool) {
	row, ok := core.querySubedge(id)
	return row.admissionFailure, ok && row.admissionFailure != 0
}

func (core Core) SubedgeAdmissionRoute(id vocabulary.SubedgeID) (route vocabulary.SubedgeRoute, adjustment vocabulary.Adjustment, result vocabulary.Values, placement vocabulary.Placement, offset uint32, outcome uint32, sibling vocabulary.SubedgeID, destination vocabulary.Values, ok bool) {
	row, found := core.querySubedge(id)
	if !found || row.admissionRoute.route == vocabulary.RouteInvalid {
		return vocabulary.RouteInvalid, vocabulary.AdjustmentInvalid, 0, vocabulary.PlacementInvalid, 0, 0, 0, 0, false
	}
	item := row.admissionRoute
	return item.route, item.adjustment, item.result, item.placement, item.offset, item.outcome, item.subedge, item.destination, true
}

func (core Core) SubedgeRouteAt(id vocabulary.SubedgeID, kind flowkind.OutcomeKind) (route vocabulary.SubedgeRoute, adjustment vocabulary.Adjustment, result vocabulary.Values, placement vocabulary.Placement, offset uint32, outcome uint32, sibling vocabulary.SubedgeID, destination vocabulary.Values, ok bool) {
	index, valid := vocabulary.CrossActivationOutcomeIndex(kind)
	if !valid {
		return vocabulary.RouteInvalid, vocabulary.AdjustmentInvalid, 0, vocabulary.PlacementInvalid, 0, 0, 0, 0, false
	}
	row, found := core.querySubedge(id)
	if !found {
		return vocabulary.RouteInvalid, vocabulary.AdjustmentInvalid, 0, vocabulary.PlacementInvalid, 0, 0, 0, 0, false
	}
	item := row.routes[index]
	if item.route == vocabulary.RouteInvalid {
		return vocabulary.RouteInvalid, vocabulary.AdjustmentInvalid, 0, vocabulary.PlacementInvalid, 0, 0, 0, 0, false
	}
	return item.route, item.adjustment, item.result, item.placement, item.offset, item.outcome, item.subedge, item.destination, true
}

func (core Core) OperationSubedgeRelation(op vocabulary.Operation) (operand vocabulary.ValueFormal, selector uint32, subedge vocabulary.SubedgeID, resultOutcome, result uint32, ok bool) {
	row, found := core.queryOperation(op)
	if !found || row.subedgeRelation == 0 || int(row.subedgeRelation) > len(core.query.subedgeRelations) {
		return 0, 0, 0, 0, 0, false
	}
	relation := core.query.subedgeRelations[row.subedgeRelation-1]
	owner, ownerOK := core.SubedgeOwner(relation.subedge)
	if !ownerOK || owner != op || relation.resultOutcome >= uint32(core.OutcomeCount(op)) || relation.result >= core.outcomeSlots(op, relation.resultOutcome) {
		return 0, 0, 0, 0, 0, false
	}
	return relation.operand, relation.selector, relation.subedge, relation.resultOutcome, relation.result, true
}

func (core Core) OperationSubedgeRelationOutcome(op vocabulary.Operation, kind flowkind.OutcomeKind) (uint32, bool) {
	_, _, subedge, resultOutcome, _, ok := core.OperationSubedgeRelation(op)
	if !ok {
		return 0, false
	}
	if kind == flowkind.OutcomeNormal || kind == flowkind.OutcomeReturn {
		return resultOutcome, true
	}
	route, _, _, _, _, outcome, _, _, found := core.SubedgeRouteAt(subedge, kind)
	if !found || (route != vocabulary.RouteOutcome && route != vocabulary.RouteRejectYield) {
		return 0, false
	}
	return outcome, true
}

func (core Core) OperationSubedgeRelationEffectAliasCount(op vocabulary.Operation) int {
	row, found := core.queryOperation(op)
	if !found || row.subedgeRelation == 0 || int(row.subedgeRelation) > len(core.query.subedgeRelations) {
		return 0
	}
	return core.query.subedgeRelations[row.subedgeRelation-1].effects.len()
}

func (core Core) OperationSubedgeRelationEffectAliasAt(op vocabulary.Operation, index int) (int, bool) {
	row, found := core.queryOperation(op)
	if !found || row.subedgeRelation == 0 || int(row.subedgeRelation) > len(core.query.subedgeRelations) || index < 0 {
		return 0, false
	}
	relation := core.query.subedgeRelations[row.subedgeRelation-1]
	if index >= relation.effects.len() {
		return 0, false
	}
	effect := core.query.subedgeRelationEffects[relation.effects.start+index]
	if effect >= uint32(core.EffectCount(op)) {
		return 0, false
	}
	return int(effect), true
}
