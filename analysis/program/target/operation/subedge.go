package operation

import (
	"errors"
	"fmt"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

const noSubedgeSource = ^uint32(0)

// appendQuerySubedges is the sole construction path for the immutable
// operation-owned Subedge table. Target contributes authored neutral Values,
// key, and boot handles; Core canonicalizes source coordinates, constructs
// every route/callee row, and discharges the complete subedge relation laws.
func (core *Core) appendQuerySubedges(op vocabulary.Operation, operation *queryOperationRow, input QueryOperationInput) error {
	if core == nil || operation == nil {
		return invalidQuery("nil operation subedge row")
	}
	if len(input.Subedges) == 0 {
		if input.SubedgeRelation != nil {
			return invalidQuery("subedge relation has no subedge table")
		}
		return core.validateSubedgeEntries(op, operation, nil)
	}
	if len(input.Subedges) >= int(^uint32(0)) {
		return invalidQuery("subedge table overflow")
	}
	items := append([]SubedgeInput(nil), input.Subedges...)
	for index := range items {
		items[index].ArgumentOrigins = append([]SubedgeArgumentOriginInput(nil), items[index].ArgumentOrigins...)
		sort.Slice(items[index].ArgumentOrigins, func(left, right int) bool {
			l, r := items[index].ArgumentOrigins[left], items[index].ArgumentOrigins[right]
			if l.Segment != r.Segment {
				return l.Segment < r.Segment
			}
			return l.Index < r.Index
		})
	}
	sort.Slice(items, func(left, right int) bool {
		return core.compareSubedgeInput(op, items[left], items[right]) < 0
	})
	for index := 1; index < len(items); index++ {
		if items[index-1].Role == items[index].Role {
			return invalidQuery("duplicate subedge role")
		}
	}

	start := len(core.query.subedges)
	sourceIDs := make([]vocabulary.SubedgeID, len(items))
	for index, item := range items {
		if item.Source >= uint32(len(items)) || sourceIDs[item.Source] != 0 {
			return invalidQuery("subedge source coordinate is malformed")
		}
		if item.Role == 0 || !vocabulary.ValidSubedgeFamily(item.Family) {
			return invalidQuery("subedge has invalid role or family")
		}
		handle, err := checkedSubedgeHandle(start + index)
		if err != nil {
			return err
		}
		sourceIDs[item.Source] = handle
	}

	pending := make([]querySubedgeRow, len(items))
	for index, item := range items {
		row, err := core.prepareSubedgeInput(op, item, operation, input.Semantics)
		if err != nil {
			return fmt.Errorf("target/operation: subedge %d: %w", index, err)
		}
		pending[index] = row
	}

	for index, item := range items {
		var err error
		pending[index].admissionRoute, err = core.appendQuerySubedgeRoute(
			item.AdmissionRoute, sourceIDs, pending, start, op, operation,
			true,
		)
		if err != nil {
			return fmt.Errorf("target/operation: subedge %d admission failure: %w", index, err)
		}
		for terminal, route := range item.Routes {
			pending[index].routes[terminal], err = core.appendQuerySubedgeRoute(
				route, sourceIDs, pending, start, op, operation,
				false,
			)
			if err != nil {
				return fmt.Errorf("target/operation: subedge %d route %d: %w", index, terminal, err)
			}
		}
		if err := core.validateTransportRows(op, pending[index], input.Semantics); err != nil {
			return fmt.Errorf("target/operation: subedge %d transport: %w", index, err)
		}
	}

	for index := range pending {
		originStart := len(core.query.subedgeOrigins)
		for _, origin := range items[index].ArgumentOrigins {
			core.query.subedgeOrigins = append(core.query.subedgeOrigins, querySubedgeArgumentOriginRow{
				segment: origin.Segment, index: origin.Index, kind: origin.Kind, source: origin.Source,
			})
		}
		pending[index].argumentOrigins = queryRange{start: originStart, end: len(core.query.subedgeOrigins)}
		core.query.subedges = append(core.query.subedges, pending[index])
	}
	operation.subedges = queryRange{start: start, end: len(core.query.subedges)}

	for index, row := range pending {
		if row.callee != vocabulary.SubedgeCalleeCallback {
			continue
		}
		callback := &core.query.callbacks[int(row.callback)-1]
		if callback.subedge != 0 {
			return invalidQuery("callback has multiple direct subedges")
		}
		callback.subedge = sourceIDs[items[index].Source]
	}
	if input.SubedgeRelation != nil {
		relation, err := core.appendQuerySubedgeRelation(op, operation, *input.SubedgeRelation, sourceIDs, len(input.EffectIndices))
		if err != nil {
			return err
		}
		operation.subedgeRelation = relation
	}
	if err := core.validateSubedgeEntries(op, operation, pending); err != nil {
		return err
	}
	if err := core.validateSubedgeRecurrence(op, operation, pending); err != nil {
		return err
	}
	return nil
}

func checkedSubedgeHandle(index int) (vocabulary.SubedgeID, error) {
	if index < 0 || uint64(index) >= uint64(^uint32(0)) {
		return 0, errors.New("target/operation: subedge table overflow")
	}
	return vocabulary.SubedgeID(index + 1), nil
}

func (core Core) compareSubedgeInput(op vocabulary.Operation, left, right SubedgeInput) int {
	if left.Role != right.Role {
		if left.Role < right.Role {
			return -1
		}
		return 1
	}
	if left.Family != right.Family {
		if left.Family < right.Family {
			return -1
		}
		return 1
	}
	if left.Callee != right.Callee {
		if left.Callee < right.Callee {
			return -1
		}
		return 1
	}
	if left.Callee == vocabulary.SubedgeCalleeCallback {
		leftID, _, _ := core.callbackBySource(op, left.CallbackSource)
		rightID, _, _ := core.callbackBySource(op, right.CallbackSource)
		if leftID != rightID {
			if leftID < rightID {
				return -1
			}
			return 1
		}
	}
	if left.ReadRoot != right.ReadRoot {
		if left.ReadRoot < right.ReadRoot {
			return -1
		}
		return 1
	}
	if left.ReadKey != right.ReadKey {
		if left.ReadKey < right.ReadKey {
			return -1
		}
		return 1
	}
	if left.MetaKey != right.MetaKey {
		if left.MetaKey < right.MetaKey {
			return -1
		}
		return 1
	}
	if left.Admission != right.Admission {
		if left.Admission < right.Admission {
			return -1
		}
		return 1
	}
	if order := core.compareValues(left.Arguments, right.Arguments); order != 0 {
		return order
	}
	for index := 0; index < len(left.Terminals) && index < len(right.Terminals); index++ {
		if left.Terminals[index].Kind != right.Terminals[index].Kind {
			if left.Terminals[index].Kind < right.Terminals[index].Kind {
				return -1
			}
			return 1
		}
		if order := core.compareValues(left.Terminals[index].Values, right.Terminals[index].Values); order != 0 {
			return order
		}
	}
	if len(left.Terminals) != len(right.Terminals) {
		if len(left.Terminals) < len(right.Terminals) {
			return -1
		}
		return 1
	}
	return 0
}

func (core Core) compareValues(left, right vocabulary.Values) int {
	if left == right {
		return 0
	}
	l, lok := core.queryValues(left)
	r, rok := core.queryValues(right)
	if !lok || !rok {
		if !lok && !rok {
			return 0
		}
		if !lok {
			return -1
		}
		return 1
	}
	limit := len(l.types)
	if len(r.types) < limit {
		limit = len(r.types)
	}
	for index := 0; index < limit; index++ {
		if l.types[index] != r.types[index] {
			if l.types[index] < r.types[index] {
				return -1
			}
			return 1
		}
	}
	if len(l.types) != len(r.types) {
		if len(l.types) < len(r.types) {
			return -1
		}
		return 1
	}
	if l.tail != r.tail {
		if l.tail < r.tail {
			return -1
		}
		return 1
	}
	if l.varID != r.varID {
		if l.varID < r.varID {
			return -1
		}
		return 1
	}
	return compareTypes(l.suffix, r.suffix)
}

func compareTypes(left, right []vocabulary.Type) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] != right[index] {
			if left[index] < right[index] {
				return -1
			}
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func (core Core) appendQueryCallbackValues(op vocabulary.Operation, input []CallbackQueryInput) error {
	row, ok := core.operation(op)
	if !ok {
		return invalidQuery("callback values owner is outside operation table")
	}
	want := row.callbacks.Len()
	if len(input) != want {
		if want == 0 && len(input) == 0 {
			return nil
		}
		return invalidQuery("callback values table does not match operation geometry")
	}
	seen := make([]bool, want)
	for _, item := range input {
		id, callback, found := core.callbackBySource(op, item.Source)
		if !found || callback.valuesSet || uint64(item.Source) >= uint64(want) {
			return invalidQuery("callback values source coordinate is malformed")
		}
		local := int(id) - int(row.callbacks.start) - 1
		if local < 0 || local >= want || seen[local] {
			return invalidQuery("duplicate callback values source coordinate")
		}
		if !item.Admission.Available() || !core.validOwnedSubedgeValues(op, item.Arguments) {
			return invalidQuery("callback values endpoint is outside owner")
		}
		for _, value := range item.Outcomes {
			if !core.validOwnedSubedgeValues(op, value) {
				return invalidQuery("callback outcome Values endpoint is outside owner")
			}
		}
		callback.admission, callback.arguments, callback.outcomes, callback.valuesSet = item.Admission, item.Arguments, item.Outcomes, true
		seen[local] = true
	}
	for _, value := range seen {
		if !value {
			return invalidQuery("callback values source coordinates are incomplete")
		}
	}
	return nil
}

func (core Core) callbackBySource(op vocabulary.Operation, source uint32) (vocabulary.CallbackID, *queryCallbackRow, bool) {
	row, ok := core.operation(op)
	if !ok {
		return 0, nil, false
	}
	for index := 0; index < row.callbacks.Len(); index++ {
		id, callbackOK := core.CallbackAt(op, index)
		if !callbackOK {
			return 0, nil, false
		}
		callback := &core.query.callbacks[int(id)-1]
		if callback.source == source && callback.owner == op {
			return id, callback, true
		}
	}
	return 0, nil, false
}

func (core Core) prepareSubedgeInput(op vocabulary.Operation, item SubedgeInput, operation *queryOperationRow, semantics schematype.Semantics) (querySubedgeRow, error) {
	if !item.Admission.Available() && item.Callee != vocabulary.SubedgeCalleeCallback {
		return querySubedgeRow{}, errors.New("invalid effective admission")
	}
	row := querySubedgeRow{
		owner: op, role: item.Role, family: item.Family, callee: item.Callee,
		readRoot: item.ReadRoot, readKey: item.ReadKey, metaKey: item.MetaKey,
		ruleEntry: item.RuleEntry, admissionFailure: item.AdmissionFailure,
	}
	arguments := item.Arguments
	outcomes := [5]vocabulary.Values{}
	admission := item.Admission
	if item.Callee == vocabulary.SubedgeCalleeCallback {
		if item.Admission != schematype.CallableAdmissionInvalid || item.ReadRoot != 0 || item.ReadKey != 0 || item.MetaKey != 0 || item.Arguments != 0 || len(item.Terminals) != 0 {
			return querySubedgeRow{}, errors.New("malformed callback callee union")
		}
		callbackID, callback, found := core.callbackBySource(op, item.CallbackSource)
		if !found || !callback.valuesSet {
			return querySubedgeRow{}, errors.New("callback callee is outside owner")
		}
		row.callback = callbackID
		arguments, outcomes, admission = callback.arguments, callback.outcomes, callback.admission
	} else {
		if item.CallbackSource != noSubedgeSource {
			return querySubedgeRow{}, errors.New("non-callback callee carries callback source")
		}
		if err := core.validateSubedgeCallee(op, item); err != nil {
			return querySubedgeRow{}, err
		}
		if len(item.Terminals) != 5 {
			return querySubedgeRow{}, errors.New("inline callee has incomplete terminals")
		}
		seen := [5]bool{}
		for index, terminal := range item.Terminals {
			kind, valid := vocabulary.CrossActivationOutcomeIndex(terminal.Kind)
			if !valid || seen[kind] {
				return querySubedgeRow{}, fmt.Errorf("inline terminal %d is invalid or duplicate", index)
			}
			if !core.validOwnedSubedgeValues(op, terminal.Values) {
				return querySubedgeRow{}, fmt.Errorf("inline terminal %d Values is outside owner", index)
			}
			seen[kind], outcomes[kind] = true, terminal.Values
		}
	}
	if !core.validOwnedSubedgeValues(op, arguments) || !core.validOwnedSubedgeValues(op, item.AdmissionFailure) {
		return querySubedgeRow{}, errors.New("subedge Values endpoint is outside owner")
	}
	for _, value := range outcomes {
		if !core.validOwnedSubedgeValues(op, value) {
			return querySubedgeRow{}, errors.New("subedge outcome Values endpoint is outside owner")
		}
	}
	row.arguments, row.outcomes, row.admission = arguments, outcomes, admission
	if item.RuleEntry && (argumentSegmentCount(core, arguments) != 0 || len(item.ArgumentOrigins) != 0) {
		return querySubedgeRow{}, errors.New("RuleEntry requires an empty argument product")
	}
	if err := core.validateArgumentOrigins(op, arguments, item.ArgumentOrigins, semantics); err != nil {
		return querySubedgeRow{}, err
	}
	if err := core.validateClosedFamily(item.Family, arguments); err != nil {
		return querySubedgeRow{}, err
	}
	return row, nil
}

func (core Core) validateSubedgeCallee(op vocabulary.Operation, item SubedgeInput) error {
	if item.Family == vocabulary.SubedgeFamilyCall {
		switch item.Callee {
		case vocabulary.SubedgeCalleeCapturedInitialRead:
			if item.ReadRoot == 0 || !core.validExactKey(item.ReadKey) || item.MetaKey != 0 {
				return errors.New("captured initial read callee is malformed")
			}
		case vocabulary.SubedgeCalleeMetaKey:
			if item.ReadRoot != 0 || item.ReadKey != 0 || !core.validExactKey(item.MetaKey) {
				return errors.New("meta-key callee is malformed")
			}
		default:
			return errors.New("Call subedge has invalid callee")
		}
		return nil
	}
	if item.Callee != vocabulary.SubedgeCalleeInvalid || item.ReadRoot != 0 || item.ReadKey != 0 || item.MetaKey != 0 || item.Admission != schematype.CallableAdmissionOrdinary {
		return errors.New("non-Call subedge callee is malformed")
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

func argumentSegmentCount(core Core, values vocabulary.Values) int {
	row, ok := core.queryValues(values)
	if !ok {
		return 0
	}
	count := len(row.types) + len(row.suffix)
	if row.tail == vocabulary.ValuesVariable {
		count++
	}
	return count
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

func (core Core) validateArgumentOrigins(op vocabulary.Operation, arguments vocabulary.Values, origins []SubedgeArgumentOriginInput, semantics schematype.Semantics) error {
	segments := argumentSegmentCount(core, arguments)
	if len(origins) != 0 && len(origins) != segments {
		return errors.New("argument origins are incomplete")
	}
	seen := make(map[[2]uint32]struct{}, len(origins))
	for _, origin := range origins {
		key := [2]uint32{uint32(origin.Segment), origin.Index}
		if _, exists := seen[key]; exists {
			return errors.New("duplicate subedge argument origin")
		}
		seen[key] = struct{}{}
		if !validSubedgeArgumentCoordinate(core, arguments, origin.Segment, origin.Index) {
			return errors.New("argument origin does not name a Values segment")
		}
		switch origin.Kind {
		case vocabulary.ArgumentSourceRule:
			if origin.Source != (vocabulary.InputSource{}) {
				return errors.New("Rule argument origin carries input")
			}
		case vocabulary.ArgumentSourceInput:
			if err := core.validateDirectArgumentOrigin(op, arguments, origin, semantics); err != nil {
				return err
			}
		default:
			return errors.New("invalid argument source")
		}
	}
	return nil
}

func (core Core) validateDirectArgumentOrigin(op vocabulary.Operation, arguments vocabulary.Values, origin SubedgeArgumentOriginInput, semantics schematype.Semantics) error {
	inputHandle, inputAvailable := core.Input(op)
	input, ok := core.queryValues(inputHandle)
	if !inputAvailable || !ok {
		return errors.New("argument origin owner input is unavailable")
	}
	var sourceType vocabulary.Type
	var sourceOK bool
	switch origin.Source.Kind {
	case vocabulary.InputSourceValueFormal:
		if uint64(origin.Source.Ordinal) >= uint64(len(input.types)) {
			return errors.New("fixed argument origin is not an owner ValueFormal")
		}
		sourceType, sourceOK = input.types[origin.Source.Ordinal], true
	case vocabulary.InputSourceValuesVar:
		if input.tail != vocabulary.ValuesVariable || origin.Source.Ordinal != uint32(input.varID) {
			return errors.New("tail argument origin is not the owner input tail")
		}
		sourceType, sourceOK = core.ValuesVarType(op, input.varID)
	default:
		return errors.New("argument origin input is outside owner ABI")
	}
	destination, destinationOK := argumentSegmentType(core, arguments, origin.Segment, origin.Index)
	if !sourceOK || !destinationOK {
		return errors.New("argument origin is type-incompatible")
	}
	return core.assignableTypeWithSemantics(op, sourceType, destination, semantics)
}

func argumentSegmentType(core Core, values vocabulary.Values, segment vocabulary.ArgumentSegment, index uint32) (vocabulary.Type, bool) {
	row, ok := core.queryValues(values)
	if !ok {
		return 0, false
	}
	switch segment {
	case vocabulary.ArgumentFixed:
		if uint64(index) < uint64(len(row.types)) {
			return row.types[index], true
		}
	case vocabulary.ArgumentSuffix:
		if uint64(index) < uint64(len(row.suffix)) {
			return row.suffix[index], true
		}
	case vocabulary.ArgumentTail:
		if row.tail == vocabulary.ValuesVariable && index == 0 {
			return core.ValuesVarType(row.owner, row.varID)
		}
	}
	return 0, false
}

func (core Core) validateSubedgeRouteInput(op vocabulary.Operation, route SubedgeRouteInput, operation *queryOperationRow, allowFailure bool) (uint32, error) {
	if !core.validOwnedSubedgeValues(op, route.Result) {
		return 0, errors.New("subedge route result is outside owner")
	}
	if route.Route == vocabulary.RouteInvalid {
		return 0, errors.New("subedge route has invalid disposition")
	}
	outcome, err := core.canonicalOutcome(operation, route.Outcome)
	if err != nil {
		return 0, err
	}
	switch route.Route {
	case vocabulary.RouteOutcome:
		if allowFailure && route.HasSibling {
			return 0, errors.New("admission failure outcome route mixes sibling")
		}
		if !allowFailure && route.HasSibling {
			return 0, errors.New("subedge outcome route mixes sibling")
		}
		if route.HasSibling || route.SiblingRank != 0 && route.Route == vocabulary.RouteOutcome {
			return 0, errors.New("subedge outcome route is malformed")
		}
	case vocabulary.RouteSubedge:
		if !route.HasSibling || outcome != noSubedgeSource {
			return 0, errors.New("subedge sibling route is malformed")
		}
	case vocabulary.RouteRejectYield:
		if allowFailure {
			return 0, errors.New("admission failure has invalid disposition")
		}
		if route.HasSibling {
			if outcome != noSubedgeSource {
				return 0, errors.New("C-boundary sibling route carries owner outcome")
			}
		} else if outcome == noSubedgeSource {
			return 0, errors.New("C-boundary owner route lacks outcome")
		}
	case vocabulary.RouteContinue, vocabulary.RoutePropagateYield:
		if allowFailure || route.HasSibling || outcome != noSubedgeSource {
			return 0, errors.New("subedge continuation route is malformed")
		}
	default:
		return 0, errors.New("subedge route has invalid disposition")
	}
	return outcome, nil
}

func (core Core) appendQuerySubedgeRoute(input SubedgeRouteInput, sourceIDs []vocabulary.SubedgeID, pending []querySubedgeRow, start int, op vocabulary.Operation, operation *queryOperationRow, allowFailure bool) (querySubedgeRouteRow, error) {
	outcome, err := core.validateSubedgeRouteInput(op, input, operation, allowFailure)
	if err != nil {
		return querySubedgeRouteRow{}, err
	}
	if input.HasSibling && (input.SiblingRank >= uint32(len(sourceIDs)) || sourceIDs[input.SiblingRank] == 0) {
		return querySubedgeRouteRow{}, errors.New("subedge sibling rank is unresolved")
	}
	row := querySubedgeRouteRow{
		route: input.Route, adjustment: input.Adjustment, result: input.Result,
		placement: input.Placement, offset: input.Offset,
	}
	if outcome != noSubedgeSource {
		row.outcome = outcome
	}
	if input.HasSibling {
		row.subedge = sourceIDs[input.SiblingRank]
		local := int(row.subedge) - start - 1
		if local < 0 || local >= len(pending) {
			return querySubedgeRouteRow{}, errors.New("subedge sibling rank is unresolved")
		}
		row.destination = pending[local].arguments
	} else if input.Route == vocabulary.RouteOutcome || input.Route == vocabulary.RouteRejectYield {
		if outcome == noSubedgeSource || int(outcome) >= operation.outcomes.len() {
			return querySubedgeRouteRow{}, errors.New("subedge owner outcome is outside owner")
		}
		row.destination = core.query.outcomeRows[operation.outcomes.start+int(outcome)].values
	}
	if row.destination == 0 && input.Route != vocabulary.RouteContinue && input.Route != vocabulary.RoutePropagateYield {
		return querySubedgeRouteRow{}, errors.New("subedge route lacks destination")
	}
	return row, nil
}

func (core Core) canonicalOutcome(operation *queryOperationRow, source uint32) (uint32, error) {
	if source == noSubedgeSource {
		return noSubedgeSource, nil
	}
	if uint64(source) >= uint64(len(operation.outcomeSources)) {
		return 0, errors.New("owner outcome is outside operation")
	}
	outcome := operation.outcomeSources[source]
	if uint64(outcome) >= uint64(operation.outcomes.len()) {
		return 0, errors.New("owner outcome is outside operation")
	}
	return outcome, nil
}

func (core *Core) appendQuerySubedgeRelation(op vocabulary.Operation, operation *queryOperationRow, input SubedgeRelationInput, sourceIDs []vocabulary.SubedgeID, effectCount int) (uint32, error) {
	if uint64(input.Operand) >= uint64(core.ValueFormalCount(op)) || input.SubedgeRank >= uint32(len(sourceIDs)) || sourceIDs[input.SubedgeRank] == 0 {
		return 0, invalidQuery("subedge relation coordinate is outside owner")
	}
	subedge := sourceIDs[input.SubedgeRank]
	resultOutcome, err := core.canonicalOutcome(operation, input.ResultOutcome)
	if err != nil || resultOutcome == noSubedgeSource {
		return 0, invalidQuery("subedge relation outcome is outside owner")
	}
	if input.Result >= core.outcomeSlots(op, resultOutcome) {
		return 0, invalidQuery("subedge relation result is outside owner outcome")
	}
	for index, effect := range input.EffectAliases {
		if uint64(effect) >= uint64(effectCount) || (index > 0 && input.EffectAliases[index-1] >= effect) {
			return 0, invalidQuery("subedge relation effect alias is malformed")
		}
	}
	effectsStart := len(core.query.subedgeRelationEffects)
	core.query.subedgeRelationEffects = append(core.query.subedgeRelationEffects, input.EffectAliases...)
	core.query.subedgeRelations = append(core.query.subedgeRelations, querySubedgeRelationRow{
		operand: input.Operand, selector: input.Selector, subedge: subedge,
		resultOutcome: resultOutcome, result: input.Result,
		effects: queryRange{start: effectsStart, end: len(core.query.subedgeRelationEffects)},
	})
	return uint32(len(core.query.subedgeRelations)), nil
}

func (core Core) validateSubedgeEntries(op vocabulary.Operation, owner *queryOperationRow, edges []querySubedgeRow) error {
	inbound := make([][]*querySubedgeRouteRow, len(edges))
	ids := make(map[vocabulary.SubedgeID]int, len(edges))
	if owner == nil {
		return errors.New("subedge owner is outside operation table")
	}
	for index := range edges {
		ids[vocabulary.SubedgeID(owner.subedges.start+index+1)] = index
	}
	callbackEdges := make(map[vocabulary.CallbackID]int)
	collect := func(route *querySubedgeRouteRow) {
		if route.route != vocabulary.RouteSubedge && (route.route != vocabulary.RouteRejectYield || route.subedge == 0) {
			return
		}
		if index, ok := ids[route.subedge]; ok {
			inbound[index] = append(inbound[index], route)
		}
	}
	for index := range edges {
		edge := &edges[index]
		if edge.callee == vocabulary.SubedgeCalleeCallback {
			callbackEdges[edge.callback]++
		}
		for terminal := range edge.routes {
			collect(&edge.routes[terminal])
		}
		collect(&edge.admissionRoute)
	}
	for index := range edges {
		edge := &edges[index]
		if edge.callee == vocabulary.SubedgeCalleeCallback && callbackEdges[edge.callback] != 1 {
			return errors.New("callback has multiple direct subedges")
		}
		if edge.argumentOrigins.len() != 0 {
			if edge.ruleEntry {
				return errors.New("argument origins carry redundant RuleEntry")
			}
			if len(inbound[index]) != 0 {
				return errors.New("route-fed arguments also carry direct origins")
			}
			continue
		}
		if edge.ruleEntry {
			continue
		}
		if len(inbound[index]) == 0 {
			return fmt.Errorf("subedge role %d has no entry authority", edge.role)
		}
		for _, route := range inbound[index] {
			if !core.routeCompletelyFeedsArguments(*route, edge.arguments) {
				return errors.New("route-fed arguments are partial")
			}
		}
	}
	for index := 0; index < core.CallbackCount(op); index++ {
		callback, ok := core.CallbackAt(op, index)
		if !ok {
			return errors.New("callback geometry is malformed")
		}
		count := callbackEdges[callback]
		lifecycle, lifecycleOK := core.CallbackLifecycle(callback)
		if !lifecycleOK {
			return errors.New("callback lifecycle is malformed")
		}
		if count > 1 || (!retainedCallbackLifecycle(lifecycle) && count != 1) {
			if count > 1 {
				return errors.New("callback has multiple direct subedges")
			}
			return errors.New("Sync callback lacks direct Subedge")
		}
	}
	return nil
}

func (core Core) routeCompletelyFeedsArguments(route querySubedgeRouteRow, destination vocabulary.Values) bool {
	result, resultOK := core.queryValues(route.result)
	dest, destinationOK := core.queryValues(destination)
	if !resultOK || !destinationOK {
		return false
	}
	switch route.placement {
	case vocabulary.PlacementFixed:
		return route.offset == 0 && result.tail == vocabulary.ValuesClosed && dest.tail == vocabulary.ValuesClosed && len(result.types) == len(dest.types) && len(result.suffix) == 0 && len(dest.suffix) == 0
	case vocabulary.PlacementTail:
		return route.adjustment == vocabulary.AdjustmentPreserve && pureValuesTail(result) && pureValuesTail(dest) && result.varID == dest.varID
	default:
		return false
	}
}

func (core Core) validateSubedgeRecurrence(op vocabulary.Operation, owner *queryOperationRow, edges []querySubedgeRow) error {
	if len(edges) == 0 {
		return nil
	}
	if owner == nil {
		return errors.New("subedge owner is outside operation table")
	}
	idToLocal := make(map[vocabulary.SubedgeID]int, len(edges))
	for index := range edges {
		idToLocal[vocabulary.SubedgeID(owner.subedges.start+index+1)] = index
	}
	outgoing := make([][]int, len(edges))
	incoming := make([][]int, len(edges))
	add := func(from int, route querySubedgeRouteRow) error {
		if route.route != vocabulary.RouteSubedge && (route.route != vocabulary.RouteRejectYield || route.subedge == 0) {
			return nil
		}
		to, ok := idToLocal[route.subedge]
		if !ok {
			return errors.New("malformed recurrence sibling")
		}
		outgoing[from] = append(outgoing[from], to)
		incoming[to] = append(incoming[to], from)
		return nil
	}
	for index, edge := range edges {
		if err := add(index, edge.admissionRoute); err != nil {
			return err
		}
		for _, route := range edge.routes {
			if err := add(index, route); err != nil {
				return err
			}
		}
	}
	reachable := make([]bool, len(edges))
	work := make([]int, 0, len(edges))
	for index, edge := range edges {
		if edge.argumentOrigins.len() == 0 && !edge.ruleEntry {
			continue
		}
		reachable[index] = true
		work = append(work, index)
	}
	for len(work) != 0 {
		index := work[len(work)-1]
		work = work[:len(work)-1]
		for _, successor := range outgoing[index] {
			if !reachable[successor] {
				reachable[successor] = true
				work = append(work, successor)
			}
		}
	}
	for index := range edges {
		if !reachable[index] {
			return fmt.Errorf("Subedge role %d has no executable entry authority", edges[index].role)
		}
	}
	seen := make([]bool, len(edges))
	order := make([]int, 0, len(edges))
	type frame struct{ edge, next int }
	for start := range edges {
		if !reachable[start] || seen[start] {
			continue
		}
		seen[start] = true
		stack := []frame{{edge: start}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next < len(outgoing[top.edge]) {
				successor := outgoing[top.edge][top.next]
				top.next++
				if reachable[successor] && !seen[successor] {
					seen[successor] = true
					stack = append(stack, frame{edge: successor})
				}
				continue
			}
			order = append(order, top.edge)
			stack = stack[:len(stack)-1]
		}
	}
	assigned := make([]bool, len(edges))
	for orderIndex := len(order) - 1; orderIndex >= 0; orderIndex-- {
		start := order[orderIndex]
		if assigned[start] {
			continue
		}
		component := make([]int, 0, 1)
		stack := []int{start}
		assigned[start] = true
		for len(stack) != 0 {
			index := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			component = append(component, index)
			for _, predecessor := range incoming[index] {
				if reachable[predecessor] && !assigned[predecessor] {
					assigned[predecessor] = true
					stack = append(stack, predecessor)
				}
			}
		}
		if len(component) == 1 && !subedgeSelfReenters(component[0], outgoing) {
			continue
		}
		for _, index := range component {
			edge := edges[index]
			if edge.callee != vocabulary.SubedgeCalleeCallback {
				continue
			}
			lifecycle, ok := core.CallbackLifecycle(edge.callback)
			if ok && onceCallbackLifecycle(lifecycle) {
				return fmt.Errorf("Once callback direct Subedge role %d re-enters through a reachable cycle", edge.role)
			}
		}
	}
	return nil
}

func subedgeSelfReenters(index int, outgoing [][]int) bool {
	for _, successor := range outgoing[index] {
		if successor == index {
			return true
		}
	}
	return false
}

func (core Core) validateClosedFamily(family vocabulary.SubedgeFamily, arguments vocabulary.Values) error {
	want := 0
	switch family {
	case vocabulary.SubedgeFamilyCall:
		return nil
	case vocabulary.SubedgeFamilyLength:
		want = 1
	case vocabulary.SubedgeFamilyIndexGet, vocabulary.SubedgeFamilyEqual, vocabulary.SubedgeFamilyLess:
		want = 2
	case vocabulary.SubedgeFamilyIndexSet:
		want = 3
	default:
		return errors.New("invalid non-Call family")
	}
	row, ok := core.queryValues(arguments)
	if !ok || row.tail != vocabulary.ValuesClosed || len(row.suffix) != 0 || len(row.types) != want {
		return fmt.Errorf("family %d requires exactly %d closed arguments", family, want)
	}
	return nil
}

func (core Core) validateSubedgeTransport(op vocabulary.Operation, source, result, destination vocabulary.Values, adjustment vocabulary.Adjustment, placement vocabulary.Placement, offset uint32, semantics schematype.Semantics) error {
	if err := core.validateProjectedResult(op, source, result, adjustment, semantics); err != nil {
		return err
	}
	return core.validateResultPlacement(op, result, destination, adjustment, placement, offset, semantics)
}

func (core Core) validateProjectedResult(op vocabulary.Operation, source, result vocabulary.Values, adjustment vocabulary.Adjustment, semantics schematype.Semantics) error {
	sourceRow, sourceOK := core.queryValues(source)
	resultRow, resultOK := core.queryValues(result)
	if !sourceOK || !resultOK {
		return errors.New("Values endpoint is unavailable")
	}
	switch adjustment {
	case vocabulary.AdjustmentPreserve:
		if core.compareValues(source, result) != 0 {
			return errors.New("preserve adjustment changes Values")
		}
		return nil
	case vocabulary.AdjustmentExact:
		return core.validateExactProjection(op, sourceRow, resultRow, semantics)
	default:
		return errors.New("invalid adjustment")
	}
}

func (core Core) validateExactProjection(op vocabulary.Operation, source, result queryValuesRow, semantics schematype.Semantics) error {
	if result.tail != vocabulary.ValuesClosed {
		return errors.New("exact adjustment result is not closed")
	}
	for index, destination := range result.types {
		if index < len(source.types) {
			if err := core.assignableTypeWithSemantics(op, source.types[index], destination, semantics); err != nil {
				return fmt.Errorf("exact adjustment fixed source type relation: %w", err)
			}
			continue
		}
		position := index - len(source.types)
		switch source.tail {
		case vocabulary.ValuesClosed:
			destinationType, ok := core.typeDeclaration(destination)
			if !ok {
				return errors.New("exact adjustment destination type is unavailable")
			}
			if err := core.assignableType(op, primitiveType(schematype.PrimitiveNil), destinationType, semantics); err != nil {
				return fmt.Errorf("exact adjustment nil fill type relation: %w", err)
			}
		case vocabulary.ValuesVariable:
			tail, ok := core.ValuesVarType(op, source.varID)
			if !ok {
				return errors.New("exact adjustment tail source type relation: type declaration is not admitted")
			}
			if err := core.assignableTypeWithSemantics(op, tail, destination, semantics); err != nil {
				return fmt.Errorf("exact adjustment tail source type relation: %w", err)
			}
			for suffix := 0; suffix <= position && suffix < len(source.suffix); suffix++ {
				if err := core.assignableTypeWithSemantics(op, source.suffix[suffix], destination, semantics); err != nil {
					return fmt.Errorf("exact adjustment suffix type relation: %w", err)
				}
			}
			if position >= len(source.suffix) {
				destinationType, ok := core.typeDeclaration(destination)
				if !ok {
					return errors.New("exact adjustment destination type is unavailable")
				}
				if err := core.assignableType(op, primitiveType(schematype.PrimitiveNil), destinationType, semantics); err != nil {
					return fmt.Errorf("exact adjustment tail nil fill type relation: %w", err)
				}
			}
		case vocabulary.ValuesUnknown:
			destinationType, ok := core.typeDeclaration(destination)
			if !ok {
				return errors.New("exact adjustment destination type is unavailable")
			}
			if err := core.assignableType(op, primitiveType(schematype.PrimitiveAny), destinationType, semantics); err != nil {
				return fmt.Errorf("exact adjustment unknown source type relation: %w", err)
			}
		default:
			return errors.New("exact adjustment source has invalid tail")
		}
	}
	return nil
}

func (core Core) validateResultPlacement(op vocabulary.Operation, result, destination vocabulary.Values, adjustment vocabulary.Adjustment, placement vocabulary.Placement, offset uint32, semantics schematype.Semantics) error {
	resultRow, resultOK := core.queryValues(result)
	destinationRow, destinationOK := core.queryValues(destination)
	if !resultOK || !destinationOK {
		return errors.New("route Values endpoint is unavailable")
	}
	switch placement {
	case vocabulary.PlacementTail:
		if adjustment != vocabulary.AdjustmentPreserve || offset != 0 || !pureValuesTail(resultRow) || destinationRow.tail != vocabulary.ValuesVariable || destinationRow.varID != resultRow.varID || len(destinationRow.suffix) != 0 {
			return errors.New("invalid tail placement")
		}
	case vocabulary.PlacementFixed:
		if resultRow.tail != vocabulary.ValuesClosed || destinationRow.tail != vocabulary.ValuesClosed || uint64(offset) > uint64(len(destinationRow.types)) || uint64(len(resultRow.types)) > uint64(len(destinationRow.types))-uint64(offset) {
			return errors.New("invalid fixed placement")
		}
		for index := range resultRow.types {
			if err := core.assignableTypeWithSemantics(op, resultRow.types[index], destinationRow.types[uint32(index)+offset], semantics); err != nil {
				return fmt.Errorf("fixed placement type relation: %w", err)
			}
		}
	default:
		return errors.New("invalid placement")
	}
	return nil
}

func primitiveType(primitive schematype.Primitive) schematype.Type {
	value, _ := schematype.NewPrimitive(primitive)
	return value
}

func (core Core) assignableType(op vocabulary.Operation, source, destination schematype.Type, semantics schematype.Semantics) error {
	if semantics == nil {
		return errors.New("type semantics are unavailable")
	}
	row, ok := core.queryOperation(op)
	if !ok {
		return errors.New("operation is unavailable")
	}
	formals := make([]schematype.Type, len(row.typeFormals))
	for index, handle := range row.typeFormals {
		if handle != 0 {
			declaration, found := core.typeDeclaration(handle)
			if !found {
				return errors.New("type formal declaration is unavailable")
			}
			formals[index] = declaration
		}
	}
	assignable, err := semantics.Assignable(source, destination, formals)
	if err != nil {
		return err
	}
	if !assignable {
		return errors.New("type-incompatible Values")
	}
	return nil
}

func (core Core) typeDeclaration(handle vocabulary.Type) (schematype.Type, bool) {
	if handle == 0 || int(handle) > len(core.query.types) {
		return schematype.Type{}, false
	}
	return core.query.types[int(handle)-1].declaration, true
}

func (core Core) assignableTypeWithSemantics(op vocabulary.Operation, source, destination vocabulary.Type, semantics schematype.Semantics) error {
	sourceType, sourceOK := core.typeDeclaration(source)
	destinationType, destinationOK := core.typeDeclaration(destination)
	if !sourceOK || !destinationOK {
		return errors.New("type declaration is not admitted")
	}
	return core.assignableType(op, sourceType, destinationType, semantics)
}

func (core Core) validateTransportRows(op vocabulary.Operation, row querySubedgeRow, semantics schematype.Semantics) error {
	if err := core.validateSubedgeTransport(op, row.admissionFailure, row.admissionRoute.result, row.admissionRoute.destination, row.admissionRoute.adjustment, row.admissionRoute.placement, row.admissionRoute.offset, semantics); err != nil {
		return fmt.Errorf("admission failure: %w", err)
	}
	for index := range row.routes {
		route := row.routes[index]
		source := row.outcomes[index]
		switch route.route {
		case vocabulary.RouteOutcome, vocabulary.RouteSubedge:
			if err := core.validateSubedgeTransport(op, source, route.result, route.destination, route.adjustment, route.placement, route.offset, semantics); err != nil {
				return fmt.Errorf("terminal %d: %w", index, err)
			}
		case vocabulary.RouteContinue:
			if route.outcome != 0 || route.subedge != 0 || route.placement != vocabulary.PlacementInvalid || route.offset != 0 {
				return fmt.Errorf("terminal %d malformed continuation route", index)
			}
			if err := core.validateProjectedResult(op, source, route.result, route.adjustment, semantics); err != nil {
				return fmt.Errorf("terminal %d: %w", index, err)
			}
		case vocabulary.RoutePropagateYield:
			if index != 3 || route.outcome != 0 || route.subedge != 0 || route.placement != vocabulary.PlacementInvalid || route.offset != 0 || route.adjustment != vocabulary.AdjustmentPreserve || core.compareValues(source, route.result) != 0 {
				return fmt.Errorf("terminal %d malformed PropagateYield route", index)
			}
		case vocabulary.RouteRejectYield:
			if index != 3 || route.adjustment != vocabulary.AdjustmentExact || route.placement != vocabulary.PlacementFixed || route.offset != 0 || !core.canonicalRejectedYield(route.result) {
				return fmt.Errorf("terminal %d malformed RejectYield route", index)
			}
			if err := core.validateResultPlacement(op, route.result, route.destination, route.adjustment, route.placement, route.offset, semantics); err != nil {
				return fmt.Errorf("terminal %d C-boundary: %w", index, err)
			}
		default:
			return fmt.Errorf("terminal %d has invalid route", index)
		}
	}
	return nil
}

func (core Core) canonicalRejectedYield(values vocabulary.Values) bool {
	row, ok := core.queryValues(values)
	return ok && row.tail == vocabulary.ValuesClosed && len(row.types) == 1 && len(row.suffix) == 0
}

func pureValuesTail(values queryValuesRow) bool {
	return len(values.types) == 0 && len(values.suffix) == 0 && values.tail == vocabulary.ValuesVariable
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

func retainedCallbackLifecycle(lifecycle vocabulary.CallbackLifecycle) bool {
	return lifecycle >= vocabulary.CallbackRetainedOptionalOnce && lifecycle <= vocabulary.CallbackRetainedRequiredMany
}

func onceCallbackLifecycle(lifecycle vocabulary.CallbackLifecycle) bool {
	switch lifecycle {
	case vocabulary.CallbackSyncOptionalOnce, vocabulary.CallbackSyncRequiredOnce,
		vocabulary.CallbackRetainedOptionalOnce, vocabulary.CallbackRetainedRequiredOnce:
		return true
	default:
		return false
	}
}
