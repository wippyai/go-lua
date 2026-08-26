package transfer

import (
	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// routeTag encodes Heap's dense allocation-root coordinate and the semantic
// escape kind.  It is transport evidence only; Placement's owner rechecks
// the exact Heap key before accepting the route.
type routeTag uint64

const (
	routeTagShift = uint(4)
	routeTagMask  = uint64(0x0f)
)

func routeTagForDense(schema placement.Schema, key heap.Key, dense int) (routeTag, bool) {
	if !schema.Valid() || !key.Valid() || key.Kind() != heap.RootAllocation || !schema.Heap().OwnsKey(key) ||
		dense < 0 || dense >= schema.DenseKeyCount() {
		return 0, false
	}
	canonical, canonicalOK := schema.KeyAt(dense)
	if !canonicalOK || canonical != key {
		return 0, false
	}
	code := uint64(placement.Send) + 1
	coordinate := uint64(dense) + 1
	if coordinate == 0 || coordinate > (^uint64(0)>>routeTagShift) || code == 0 || code > routeTagMask {
		return 0, false
	}
	return routeTag(coordinate<<routeTagShift | code), true
}

func routeTagFor(schema placement.Schema, key heap.Key) (routeTag, bool) {
	if !schema.Valid() || !key.Valid() || key.Kind() != heap.RootAllocation || !schema.Heap().OwnsKey(key) {
		return 0, false
	}
	dense, denseOK := schema.Heap().AllocationKeyIndex(key)
	if !denseOK {
		return 0, false
	}
	return routeTagForDense(schema, key, dense)
}

type route struct {
	key heap.Key
	tag routeTag
}

const routeInlineWidth = 8

type routePlan struct {
	inline  [routeInlineWidth]route
	extra   []route
	count   int
	allSend bool
	schema  placement.Schema
}

func (plan routePlan) routeCount() int {
	if !plan.schema.Valid() {
		return 0
	}
	if plan.allSend {
		return plan.schema.DenseKeyCount()
	}
	if plan.count < 0 {
		return 0
	}
	return plan.count
}

func (plan routePlan) routeAt(index int) (route, bool) {
	if index < 0 || index >= plan.routeCount() || !plan.schema.Valid() {
		return route{}, false
	}
	if plan.allSend {
		key, keyOK := plan.schema.KeyAt(index)
		if !keyOK || key.Kind() != heap.RootAllocation {
			return route{}, false
		}
		tag, tagOK := routeTagForDense(plan.schema, key, index)
		return route{key: key, tag: tag}, tagOK
	}
	if index < len(plan.inline) {
		return plan.inline[index], true
	}
	overflow := index - len(plan.inline)
	if overflow < 0 || overflow >= len(plan.extra) {
		return route{}, false
	}
	return plan.extra[overflow], true
}

func (plan *routePlan) appendRoute(candidate route) bool {
	if plan == nil || plan.allSend || plan.count < 0 {
		return false
	}
	if plan.count < len(plan.inline) {
		plan.inline[plan.count] = candidate
		plan.count++
		return true
	}
	plan.extra = append(plan.extra, candidate)
	plan.count++
	return true
}

// demand is kept sorted by Heap's canonical dense coordinate.  Transfer
// payloads are always Send demands; the only per-root distinction is whether
// identity became open before the Call dispatch boundary widened everything.
type demand struct {
	dense int
}

type demandScratch struct {
	inline [16]demand
	extra  []demand
	count  int
}

func (scratch demandScratch) at(index int) (demand, bool) {
	if index < 0 || index >= scratch.count {
		return demand{}, false
	}
	if index < len(scratch.inline) {
		return scratch.inline[index], true
	}
	overflow := index - len(scratch.inline)
	if overflow < 0 || overflow >= len(scratch.extra) {
		return demand{}, false
	}
	return scratch.extra[overflow], true
}

func (scratch *demandScratch) set(index int, value demand) bool {
	if scratch == nil || index < 0 || index >= scratch.count {
		return false
	}
	if index < len(scratch.inline) {
		scratch.inline[index] = value
		return true
	}
	overflow := index - len(scratch.inline)
	if overflow < 0 || overflow >= len(scratch.extra) {
		return false
	}
	scratch.extra[overflow] = value
	return true
}

func (scratch *demandScratch) add(candidate demand) bool {
	if scratch == nil || candidate.dense < 0 || scratch.count < 0 {
		return false
	}
	position := 0
	for position < scratch.count {
		current, ok := scratch.at(position)
		if !ok {
			return false
		}
		if current.dense == candidate.dense {
			return true
		}
		if current.dense > candidate.dense {
			break
		}
		position++
	}
	if scratch.count < len(scratch.inline) {
		for index := scratch.count; index > position; index-- {
			scratch.inline[index] = scratch.inline[index-1]
		}
		scratch.inline[position] = candidate
		scratch.count++
		return true
	}
	if position < len(scratch.inline) {
		carried := scratch.inline[len(scratch.inline)-1]
		for index := len(scratch.inline) - 1; index > position; index-- {
			scratch.inline[index] = scratch.inline[index-1]
		}
		scratch.inline[position] = candidate
		scratch.extra = append(scratch.extra, demand{})
		copy(scratch.extra[1:], scratch.extra[:len(scratch.extra)-1])
		scratch.extra[0] = carried
		scratch.count++
		return true
	}
	overflow := position - len(scratch.inline)
	if overflow < 0 || overflow > len(scratch.extra) {
		return false
	}
	scratch.extra = append(scratch.extra, demand{})
	copy(scratch.extra[overflow+1:], scratch.extra[overflow:len(scratch.extra)-1])
	scratch.extra[overflow] = candidate
	scratch.count++
	return true
}

func denseDemandKey(schema placement.Schema, key heap.Key) (int, bool) {
	if !schema.Valid() || !key.Valid() || key.Kind() != heap.RootAllocation || !schema.Heap().OwnsKey(key) {
		return 0, false
	}
	dense, denseOK := schema.Heap().AllocationKeyIndex(key)
	if !denseOK || dense < 0 || dense >= schema.DenseKeyCount() {
		return 0, false
	}
	canonical, canonicalOK := schema.KeyAt(dense)
	return dense, canonicalOK && canonical == key
}

func addExactDemand(schema placement.Schema, key heap.Key, scratch *demandScratch) bool {
	dense, ok := denseDemandKey(schema, key)
	return ok && scratch != nil && scratch.add(demand{dense: dense})
}

func (plan routePlan) seal(schema placement.Schema, scratch *demandScratch) (routePlan, bool) {
	if !schema.Valid() || scratch == nil || scratch.count < 0 {
		return routePlan{}, false
	}
	if scratch.count == 0 {
		return routePlan{schema: schema}, true
	}
	var sealed routePlan
	sealed.schema = schema
	if !sealed.prepare(scratch.count) {
		return routePlan{}, false
	}
	for index := 0; index < scratch.count; index++ {
		item, itemOK := scratch.at(index)
		if !itemOK {
			return routePlan{}, false
		}
		key, keyOK := schema.KeyAt(item.dense)
		if !keyOK || key.Kind() != heap.RootAllocation {
			return routePlan{}, false
		}
		tag, tagOK := routeTagForDense(schema, key, item.dense)
		if !tagOK || !sealed.appendRoute(route{key: key, tag: tag}) {
			return routePlan{}, false
		}
	}
	return sealed, true
}

func (plan *routePlan) prepare(count int) bool {
	if plan == nil || count < 0 || plan.count != 0 || plan.allSend {
		return false
	}
	if count > len(plan.inline) {
		plan.extra = make([]route, 0, count-len(plan.inline))
	}
	return true
}

func allSendPlan(schema placement.Schema) (routePlan, bool) {
	if !schema.Valid() {
		return routePlan{}, false
	}
	return routePlan{allSend: true, schema: schema}, true
}

func coordinateForActual(values *valuedomain.Schema, actual packdomain.MountedActualProjection, index int) (valuedomain.Coordinate, bool) {
	if values == nil || !values.Valid() || index < 0 || index >= actual.ActualCount() {
		return valuedomain.Coordinate{}, false
	}
	source, sourceOK := actual.ActualAt(index)
	if !sourceOK {
		return valuedomain.Coordinate{}, false
	}
	return values.CoordinateForMountedSemantic(source.Module(), source.ID())
}

func validTransferSource(target vocabulary.Operation, source vocabulary.InputSource, operations interface {
	ValueFormalCount(vocabulary.Operation) int
	Input(vocabulary.Operation) (vocabulary.Values, bool)
	ValuesTail(vocabulary.Values) (vocabulary.ValuesTail, vocabulary.ValuesVar, bool)
}) bool {
	if operations == nil || target == 0 {
		return false
	}
	switch source.Kind {
	case vocabulary.InputSourceValueFormal:
		return uint64(source.Ordinal) < uint64(operations.ValueFormalCount(target))
	case vocabulary.InputSourceValuesVar:
		input, inputOK := operations.Input(target)
		tail, variable, tailOK := operations.ValuesTail(input)
		return inputOK && tailOK && tail == vocabulary.ValuesVariable && variable == vocabulary.ValuesVar(source.Ordinal)
	default:
		return false
	}
}

func validTransferEndpoint(target vocabulary.Operation, endpoint vocabulary.TransferEndpoint, operations interface {
	ValueFormalCount(vocabulary.Operation) int
}) bool {
	if operations == nil || target == 0 {
		return false
	}
	switch endpoint.Kind {
	case vocabulary.TransferEndpointInput:
		return uint64(endpoint.Input) < uint64(operations.ValueFormalCount(target))
	case vocabulary.TransferEndpointExternal:
		return endpoint.Input == 0
	default:
		return false
	}
}

func validTransferDescription(identityValue vocabulary.TransferIdentity, capabilities vocabulary.TransferCapabilities) bool {
	return identityValue >= vocabulary.TransferIdentityUnspecified && identityValue <= vocabulary.TransferIdentityDistinct &&
		capabilities >= vocabulary.TransferCapabilitiesUnspecified && capabilities <= vocabulary.TransferCapabilitiesLoseAll
}

func validTransferPossibility(value vocabulary.TransferPossibility) bool {
	const valid = vocabulary.TransferMayDeliver | vocabulary.TransferMayReject
	return value != 0 && value&^valid == 0
}

func transferMayDeliver(operations interface {
	TransferOwner(vocabulary.TransferID) (vocabulary.Operation, bool)
	TransferOutcomeCount(vocabulary.Operation, int) int
	TransferOutcomeAt(vocabulary.Operation, int, int) (uint32, vocabulary.TransferPossibility, bool)
}, target vocabulary.Operation, transfer vocabulary.TransferID, transferIndex int) (bool, bool) {
	if operations == nil || transfer == 0 || target == 0 {
		return false, false
	}
	owner, ownerOK := operations.TransferOwner(transfer)
	count := operations.TransferOutcomeCount(target, transferIndex)
	if !ownerOK || owner != target || count <= 0 {
		return false, false
	}
	mayDeliver := false
	for outcome := 0; outcome < count; outcome++ {
		canonical, possibility, possibilityOK := operations.TransferOutcomeAt(target, transferIndex, outcome)
		if !possibilityOK || canonical != uint32(outcome) || !validTransferPossibility(possibility) {
			return false, false
		}
		if possibility&vocabulary.TransferMayDeliver != 0 {
			mayDeliver = true
		}
	}
	return mayDeliver, true
}

// actualDemand adds the exact Placement demand one delivered actual cell
// carries. The cell is a member of the whole vector Value published for this
// call, so its presence bit and the Factor default its declared sparsity
// substituted are the only absence policy; an authenticated opaque reference
// widens the identity to every Send root rather than naming one.
func actualDemand(schema placement.Schema, values *valuedomain.Schema, actuals operand.SummaryVector[valuedomain.Value], ordinal int, scratch *demandScratch) (allSend bool, ok bool) {
	if values == nil || !schema.Valid() || scratch == nil {
		return false, false
	}
	value, present, cell := actuals.At(ordinal)
	fact, factOK := values.AuthenticateFactorCell(value, present, cell)
	if !factOK {
		return false, false
	}
	projection, projectionOK := placement.ProjectValueAllocations(schema, values, fact)
	if !projectionOK || !projection.Valid() {
		return false, false
	}
	if projection.Widened() {
		return true, true
	}
	for index := 0; index < projection.ExactCount(); index++ {
		key, keyOK := projection.ExactAt(index)
		if !keyOK || !addExactDemand(schema, key, scratch) {
			return false, false
		}
	}
	return false, true
}

func addPayloadDemand(operations interface {
	Input(vocabulary.Operation) (vocabulary.Values, bool)
	ValuesTail(vocabulary.Values) (vocabulary.ValuesTail, vocabulary.ValuesVar, bool)
}, target vocabulary.Operation, payload vocabulary.InputSource, packs *packdomain.Schema, actual packdomain.MountedActualProjection, actuals operand.SummaryVector[valuedomain.Value], schema placement.Schema, values *valuedomain.Schema, runtimeTail bool, scratch *demandScratch) (allSend bool, ok bool) {
	if operations == nil || target == 0 || packs == nil || values == nil || scratch == nil {
		return false, false
	}
	selector, selectorOK := packs.InputSelector(target, payload)
	start, startOK := selector.Start()
	if !selectorOK || !packs.OwnsInputSelector(selector) || !startOK || start < 0 {
		return false, false
	}
	switch payload.Kind {
	case vocabulary.InputSourceValueFormal:
		if start >= actual.ActualCount() || start >= actuals.Count() {
			// A fixed Target formal beyond Pack's sealed prefix can be redeemed
			// only by Pack's authenticated open tail.  The identity is then
			// widened conservatively, but the authenticated MayDeliver still
			// supplies the Send/SharedHeap displacement.
			return runtimeTail, runtimeTail
		}
		return actualDemand(schema, values, actuals, start, scratch)
	case vocabulary.InputSourceValuesVar:
		input, inputOK := operations.Input(target)
		tail, variable, tailOK := operations.ValuesTail(input)
		if !inputOK || !tailOK || tail != vocabulary.ValuesVariable || variable != vocabulary.ValuesVar(payload.Ordinal) {
			return false, false
		}
		if start > actual.ActualCount() {
			// A closed under-applied tail is an authenticated empty source. An
			// open tail may still populate every position beyond the fixed Pack
			// prefix, so only that boundary can widen identity to all Send roots;
			// an empty closed suffix is a valid no-route result.
			return runtimeTail, true
		}
		for index := start; index < actual.ActualCount(); index++ {
			memberSend, memberOK := actualDemand(schema, values, actuals, index, scratch)
			if !memberOK {
				return false, false
			}
			allSend = allSend || memberSend
		}
		// TailID is Pack's sole open-boundary evidence; no synthetic Value is
		// created for it. Fixed suffix members remain exact above, while an
		// authenticated open tail widens the root identity without changing
		// the known Send/SharedHeap policy.
		return allSend || runtimeTail, true
	default:
		return false, false
	}
}

// planFor is the Target-transfer to Placement reduction. It reads only the
// already-authenticated Call/Value/Pack facts and the exact Target Contract;
// no runtime delivery strategy or Effect result is inferred here.
//
// The actuals arrive as the whole vector Value published for this call, in
// its own ordinal order: a cell's POSITION is the ordinal its owner declared
// it at, so the reduction addresses a member by that position and holds no
// per-invocation copy of the delivery.
func planFor(packs *packdomain.Schema, calls *calldomain.Algebra, schema placement.Schema, values *valuedomain.Schema, targetContract *contract.Contract, mounted calldomain.MountedCall, callFact calldomain.Value, actuals operand.SummaryVector[valuedomain.Value]) (routePlan, bool) {
	if packs == nil || calls == nil || !calls.Valid() || !schema.Valid() || values == nil || !values.Valid() || targetContract == nil ||
		!calls.OwnsTargetContract(targetContract) || !values.OwnsHeapSchema(schema.Heap()) || !values.LinkOwner().Matches(calls.LinkOwner()) ||
		!packs.LinkOwner().Available() || !packs.LinkOwner().Matches(calls.LinkOwner()) {
		return routePlan{}, false
	}
	_, callID, module, _, _, identityOK := calls.MountedCallIdentity(mounted)
	actual, actualOK := packs.MountedActualProjection(module, callID)
	key, keyOK := calls.KeyForMountedCall(mounted)
	if !identityOK || !actualOK || !actual.Valid() || !actual.OwnedBy(packs) || !keyOK || !calls.Admits(key, callFact) ||
		!actuals.Valid() || actuals.Count() != actual.ActualCount() {
		return routePlan{}, false
	}
	runtimeTail := false
	if _, open := actual.TailID(); open {
		runtimeTail = true
	}
	for index := 0; index < actuals.Count(); index++ {
		value, present, cell := actuals.At(index)
		coordinate, coordinateOK := coordinateForActual(values, actual, index)
		fact, factOK := values.AuthenticateFactorCell(value, present, cell)
		if !factOK || !coordinateOK || !values.AdmitsCoordinate(coordinate, fact) {
			return routePlan{}, false
		}
	}
	var demands demandScratch
	allSend := false
	// Pass the immutable owner by pointer through the small read interfaces.
	// Boxing Core itself copies its large sealed value into each interface and
	// allocates; a pointer preserves the single authority and the zero-allocation
	// invocation path without duplicating any admission logic.
	operations := &targetContract.Operations
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK || !calls.OwnsTarget(target) {
			return routePlan{}, false
		}
		operation, kind := calls.ClassifyTargetOperation(target)
		if !kind.Valid() {
			return routePlan{}, false
		}
		if kind == calldomain.TargetOperationNone {
			continue
		}
		declared, declaredOK := targetContract.Operations.OperationAt(int(operation) - 1)
		if !declaredOK || declared != operation {
			return routePlan{}, false
		}
		transferCount := targetContract.Operations.TransferCount(operation)
		for transferIndex := 0; transferIndex < transferCount; transferIndex++ {
			transferID, transferIDOK := targetContract.Operations.TransferIDAt(operation, transferIndex)
			endpoint, payload, alias, identityValue, capabilities, declarationOK := targetContract.Operations.TransferDeclaration(transferID)
			if !transferIDOK || !declarationOK || !validTransferEndpoint(operation, endpoint, operations) ||
				!validTransferSource(operation, payload, operations) || !validTransferSource(operation, alias, operations) ||
				!validTransferDescription(identityValue, capabilities) {
				return routePlan{}, false
			}
			// Alias is a canonical Target relation descriptor for the payload
			// graph, not a second delivery source. MayDeliver preserves the
			// payload's contents and internal aliases; it does not grant this
			// consumer an independent root, endpoint, actor, or copy/move
			// authority. Validate the alias above, then keep Placement demand
			// exclusively payload-derived. Identity and capabilities are likewise
			// descriptive labels and never change the Send policy.
			mayDeliver, outcomesOK := transferMayDeliver(operations, operation, transferID, transferIndex)
			if !outcomesOK {
				return routePlan{}, false
			}
			if !mayDeliver {
				continue
			}
			payloadOpen, payloadOK := addPayloadDemand(operations, operation, payload, packs, actual, actuals, schema, values, runtimeTail, &demands)
			if !payloadOK {
				return routePlan{}, false
			}
			allSend = allSend || payloadOpen
		}
	}
	if allSend {
		return allSendPlan(schema)
	}
	return (&routePlan{}).seal(schema, &demands)
}
