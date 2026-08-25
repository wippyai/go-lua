// Package formal consumes Target operation ownership metadata at ordinary
// mounted call sites and projects the resulting demand onto Placement's
// Heap-aligned allocation roots.
//
// The package is deliberately a consumer only.  Target remains the sole
// owner of FormalEffect rows, Value remains the sole owner of Value
// coordinates and rooted references, and Heap remains the sole owner of
// allocation keys.  No publication descriptor or Effect result is retained
// here.
package formal

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type formalSelectorRange struct {
	start   int
	end     int
	unknown bool
	owns    bool
	// valid distinguishes an authenticated selector from a malformed authored
	// spelling.  A formal parameter ordinal belongs to the callee declaration
	// and is independent of any one call site's arity, so an ordinal at or
	// past this mounted call's actual prefix is an authenticated empty
	// selector: Lua binds the parameter to nil and nil holds no allocation.
	// Unknown is reserved for a real open boundary (the mounted Pack tail).
	valid bool
}

func resolveFormalSelectorRange(spec vocabulary.FormalEffectSpec, actualCount int, runtimeTail bool) formalSelectorRange {
	result := formalSelectorRange{valid: true}
	if actualCount < 0 {
		result.valid = false
		return result
	}
	switch spec.Kind {
	case vocabulary.FormalEffectBorrowAll:
		return result
	case vocabulary.FormalEffectBorrow, vocabulary.FormalEffectFreeze:
		// Borrow and Freeze do not write Placement.  The -1 form is an authored
		// unresolved/non-displacing selector, and an ordinal past this call's
		// actual prefix names a parameter Lua bound to nil; both carry no
		// route.  Only a negative authored spelling is malformed.
		if spec.Param < -1 {
			result.valid = false
		}
		return result
	case vocabulary.FormalEffectRetain,
		vocabulary.FormalEffectStore,
		vocabulary.FormalEffectSendParam,
		vocabulary.FormalEffectExport,
		vocabulary.FormalEffectOpaque:
		result.owns = true
		if spec.Param == -1 {
			if runtimeTail {
				result.unknown = true
				return result
			}
			if actualCount == 0 {
				// The authored trailing selector names no supplied actual, so
				// it selects nothing.
				return result
			}
			result.start = actualCount - 1
			result.end = actualCount
			return result
		}
		if spec.Param < 0 {
			result.valid = false
			return result
		}
		if int(spec.Param) >= actualCount {
			// A mounted open Pack tail is the only authority that can place an
			// actual at this ordinal, and it widens the selector.  With a
			// closed actual list the ordinal is provably unsupplied: Lua binds
			// the parameter to nil, nil holds no allocation, and the selector
			// is therefore empty rather than malformed.
			result.unknown = runtimeTail
			return result
		}
		result.start = int(spec.Param)
		result.end = result.start + 1
		return result
	case vocabulary.FormalEffectSendSuffix:
		result.owns = true
		if spec.FromParam < 0 {
			result.valid = false
			return result
		}
		if int(spec.FromParam) > actualCount {
			// A suffix that begins past the supplied prefix sends nothing
			// unless Pack authenticates a runtime tail behind it.
			result.start = actualCount
			result.end = actualCount
			result.unknown = runtimeTail
			return result
		}
		result.start = int(spec.FromParam)
		result.end = actualCount
		if runtimeTail {
			// A runtime tail can contain an actual at every position after the
			// fixed prefix; the conservative choice is therefore unknown for
			// every suffix boundary, including one beyond the prefix.
			result.unknown = true
		}
		return result
	default:
		result.valid = false
		return result
	}
}

// FormalEscape maps an ownership-bearing formal kind to Placement's stable
// escape vocabulary. Non-displacing kinds return false.
func FormalEscape(kind vocabulary.FormalEffectKind) (placement.Escape, bool) {
	switch kind {
	case vocabulary.FormalEffectRetain:
		return placement.Retain, true
	case vocabulary.FormalEffectStore:
		return placement.Store, true
	case vocabulary.FormalEffectSendSuffix, vocabulary.FormalEffectSendParam:
		return placement.Send, true
	case vocabulary.FormalEffectExport:
		return placement.Export, true
	case vocabulary.FormalEffectOpaque:
		return placement.Opaque, true
	default:
		return placement.None, false
	}
}

type route struct {
	key     heap.Key
	escape  placement.Escape
	unknown bool
	tag     routeTag
}

type routePlan struct {
	// The common formal call has only a handful of allocation routes. Keep
	// those routes in the plan value itself; wider exact plans spill only the
	// suffix. The plan is the relation's derived state for one invocation and
	// is never retained past it, so this is invocation-local storage rather
	// than shared mutable state.
	inline [formalRouteInlineWidth]route
	extra  []route
	count  int

	// An authenticated Pack runtime tail or Value open/Top projection can widen
	// every allocation root. Retaining that fact as a mode, instead of
	// materializing one route per root, keeps the conservative path
	// allocation-free even for a wide Heap. An opaque Call arm never sets this
	// bit: it has no formal Target/Value identity authority. schema remains the
	// sole coordinate authority; it is not a copied root directory.
	allUnknown bool
	schema     placement.Schema
}

const formalRouteInlineWidth = 8

func (plan routePlan) routeCount() int {
	if plan.count < 0 {
		return 0
	}
	return plan.count
}

func (plan routePlan) routeAt(index int) (route, bool) {
	if index < 0 || index >= plan.count {
		return route{}, false
	}
	if plan.allUnknown {
		// An all-root plan holds the widening as a mode rather than a
		// materialized row per root. Indexed access derives the same route
		// from the owner schema without introducing a second allocation-root
		// index.
		if !plan.schema.Valid() {
			return route{}, false
		}
		ordinal := 0
		for dense := 0; dense < plan.schema.DenseKeyCount(); dense++ {
			key, keyOK := plan.schema.KeyAt(dense)
			if !keyOK {
				return route{}, false
			}
			if key.Kind() != heap.RootAllocation {
				continue
			}
			if ordinal == index {
				tag, tagOK := routeTagForDense(plan.schema, key, dense, placement.None, true)
				if !tagOK {
					return route{}, false
				}
				return route{key: key, unknown: true, tag: tag}, true
			}
			ordinal++
		}
		return route{}, false
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

// appendRoute appends in canonical dense order. Dense scratch sealing walks
// Heap coordinates in that order, so no route sort or second coordinate
// directory is needed.
func (plan *routePlan) appendRoute(candidate route) bool {
	if plan == nil || plan.allUnknown || plan.count < 0 {
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

func (plan *routePlan) prepare(count int) bool {
	if plan == nil || count < 0 || plan.count != 0 || plan.allUnknown {
		return false
	}
	if count <= len(plan.inline) {
		return true
	}
	plan.extra = make([]route, 0, count-len(plan.inline))
	return true
}

const (
	routeTagShift    = uint(4)
	routeTagMask     = uint64(0x0f)
	routeCodeUnknown = uint64(0x0f)
)

type routeTag uint64

func routeTagFor(schema placement.Schema, key heap.Key, escape placement.Escape, unknown bool) (routeTag, bool) {
	if !schema.Valid() || !key.Valid() || key.Kind() != heap.RootAllocation || !schema.Heap().OwnsKey(key) {
		return 0, false
	}
	dense, denseOK := schema.Heap().AllocationKeyIndex(key)
	if !denseOK || dense < 0 {
		return 0, false
	}
	return routeTagForDense(schema, key, dense, escape, unknown)
}

// routeTagForDense is the checked dense-coordinate spelling used by the
// invocation-local demand scratch. Heap remains the issuer of both the key
// and its dense ordinal; the canonical KeyAt check rejects foreign or forged
// coordinates before a tag is emitted.
func routeTagForDense(schema placement.Schema, key heap.Key, dense int, escape placement.Escape, unknown bool) (routeTag, bool) {
	if !schema.Valid() || !key.Valid() || key.Kind() != heap.RootAllocation || !schema.Heap().OwnsKey(key) || dense < 0 || dense >= schema.DenseKeyCount() {
		return 0, false
	}
	canonical, canonicalOK := schema.KeyAt(dense)
	if !canonicalOK || canonical != key {
		return 0, false
	}
	code := uint64(escape) + 1
	if unknown {
		code = routeCodeUnknown
	}
	if code == 0 || code > routeTagMask {
		return 0, false
	}
	coordinate := uint64(dense) + 1
	if coordinate > (^uint64(0) >> routeTagShift) {
		return 0, false
	}
	return routeTag(coordinate<<routeTagShift | code), true
}

func strongerEscape(left, right placement.Escape) placement.Escape {
	leftPlacement, leftOK := left.Placement()
	rightPlacement, rightOK := right.Placement()
	if !leftOK {
		return right
	}
	if !rightOK {
		return left
	}
	if placement.Join(leftPlacement, rightPlacement) == rightPlacement && leftPlacement != rightPlacement {
		return right
	}
	if placement.Join(leftPlacement, rightPlacement) == leftPlacement && leftPlacement != rightPlacement {
		return left
	}
	// Retain/Store and Send/Export/Opaque have equal Placement but retaining
	// the later canonical kind keeps provenance deterministic without making
	// it semantically stronger.
	if right > left {
		return right
	}
	return left
}

// formalDemandInlineWidth bounds the invocation-local dense demand set. The
// ordinary path is a small set of exact allocation roots, so this prefix
// keeps both demand replacement and route reduction on the caller's stack. An
// unusually wide exact demand set spills only its suffix; Value/Pack all-root
// widening does not need the suffix at all.
const formalDemandInlineWidth = 16

type denseRouteDemand struct {
	dense   int
	escape  placement.Escape
	unknown bool
}

// denseDemandScratch is deliberately invocation-local. The derivation
// can execute concurrently on the same immutable Rule, so every invocation
// owns its own scratch value. Dense is Heap's canonical coordinate, not a
// second key index.
type denseDemandScratch struct {
	inline     [formalDemandInlineWidth]denseRouteDemand
	extra      []denseRouteDemand
	count      int
	allUnknown bool
}

func (scratch denseDemandScratch) at(index int) (denseRouteDemand, bool) {
	if index < 0 || index >= scratch.count {
		return denseRouteDemand{}, false
	}
	if index < len(scratch.inline) {
		return scratch.inline[index], true
	}
	overflow := index - len(scratch.inline)
	if overflow < 0 || overflow >= len(scratch.extra) {
		return denseRouteDemand{}, false
	}
	return scratch.extra[overflow], true
}

// add joins one demand into canonical dense order. Formal rows are traversed
// in authored order, not Heap order, so insertion keeps sealing deterministic
// without map iteration or a second sort/index structure.
func (scratch *denseDemandScratch) add(candidate denseRouteDemand) bool {
	if scratch == nil || scratch.count < 0 || candidate.dense < 0 {
		return false
	}
	position := 0
	for position < scratch.count {
		current, ok := scratch.at(position)
		if !ok {
			return false
		}
		switch {
		case current.dense == candidate.dense:
			if current.unknown || candidate.unknown || scratch.allUnknown {
				current.unknown = true
			} else {
				current.escape = strongerEscape(current.escape, candidate.escape)
			}
			scratch.set(position, current)
			return true
		case current.dense > candidate.dense:
			goto insert
		default:
			position++
		}
	}

insert:
	if scratch.count < len(scratch.inline) {
		for index := scratch.count; index > position; index-- {
			scratch.inline[index] = scratch.inline[index-1]
		}
		if scratch.allUnknown {
			candidate.unknown = true
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
		if scratch.allUnknown {
			candidate.unknown = true
		}
		scratch.inline[position] = candidate
		scratch.extra = append(scratch.extra, denseRouteDemand{})
		copy(scratch.extra[1:], scratch.extra[:len(scratch.extra)-1])
		scratch.extra[0] = carried
	} else {
		overflow := position - len(scratch.inline)
		if overflow < 0 || overflow > len(scratch.extra) {
			return false
		}
		if scratch.allUnknown {
			candidate.unknown = true
		}
		scratch.extra = append(scratch.extra, denseRouteDemand{})
		copy(scratch.extra[overflow+1:], scratch.extra[overflow:len(scratch.extra)-1])
		scratch.extra[overflow] = candidate
	}
	scratch.count++
	return true
}

func (scratch *denseDemandScratch) set(index int, value denseRouteDemand) bool {
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

func addUnknownAllDense(schema placement.Schema, demands *denseDemandScratch) bool {
	// This is an identity-denominator operation, not a generic uncertainty
	// fallback. Callers may reach it only after Pack has authenticated a runtime
	// tail or Value has authenticated Top/opaque allocation identity. Call's
	// opaque dispatch arm has no path to this helper.
	if !schema.Valid() || demands == nil {
		return false
	}
	demands.allUnknown = true
	return true
}

func planAddDenseDemand(schema placement.Schema, key heap.Key, escape placement.Escape, unknown bool, demands *denseDemandScratch) bool {
	if demands == nil {
		return false
	}
	dense, denseOK := denseDemandKey(schema, key)
	if !denseOK {
		return false
	}
	return demands.add(denseRouteDemand{dense: dense, escape: escape, unknown: unknown || demands.allUnknown})
}

func addFactDemandDense(schema placement.Schema, values *valuedomain.Schema, actuals execution.SummaryVector[valuedomain.Value], ordinal int, escape placement.Escape, demands *denseDemandScratch) (unknown bool, ok bool) {
	if demands == nil || values == nil {
		return false, false
	}
	value, present, cell := actuals.At(ordinal)
	fact, factOK := values.AuthenticateFactorCell(value, present, cell)
	if !factOK {
		return false, false
	}
	projection, projectionOK := placement.ProjectValueAllocations(schema, values, fact)
	if !projectionOK {
		return false, false
	}
	if projection.IsTop() {
		return true, true
	}
	for index := 0; index < projection.ExactCount(); index++ {
		key, keyOK := projection.ExactAt(index)
		if !keyOK || !planAddDenseDemand(schema, key, escape, false, demands) {
			return false, false
		}
	}
	return projection.HasOpaque(), true
}

func addOpenTailDemandDense(schema placement.Schema, values *valuedomain.Schema, fact valuedomain.Value, demands *denseDemandScratch) bool {
	if demands == nil {
		return false
	}
	projection, projectionOK := placement.ProjectValueAllocations(schema, values, fact)
	if !projectionOK {
		return false
	}
	if projection.IsTop() {
		return addUnknownAllDense(schema, demands)
	}
	for index := 0; index < projection.ExactCount(); index++ {
		key, keyOK := projection.ExactAt(index)
		if !keyOK || !planAddDenseDemand(schema, key, placement.None, true, demands) {
			return false
		}
	}
	if projection.HasOpaque() {
		return addUnknownAllDense(schema, demands)
	}
	return true
}

func addUnknownOpenTailActualDemandDense(schema placement.Schema, values *valuedomain.Schema, actuals execution.SummaryVector[valuedomain.Value], ordinal int, demands *denseDemandScratch) bool {
	if values == nil {
		return false
	}
	value, present, cell := actuals.At(ordinal)
	fact, factOK := values.AuthenticateFactorCell(value, present, cell)
	if !factOK {
		return false
	}
	return addOpenTailDemandDense(schema, values, fact, demands)
}

func (plan routePlan) seal(schema placement.Schema, demands *denseDemandScratch) (routePlan, bool) {
	if !schema.Valid() || demands == nil || demands.count < 0 {
		return routePlan{}, false
	}
	if demands.allUnknown {
		sealed := routePlan{allUnknown: true, schema: schema}
		for dense := 0; dense < schema.DenseKeyCount(); dense++ {
			key, keyOK := schema.KeyAt(dense)
			if !keyOK {
				return routePlan{}, false
			}
			if key.Kind() == heap.RootAllocation {
				sealed.count++
			}
		}
		return sealed, true
	}
	var sealed routePlan
	if !sealed.prepare(demands.count) {
		return routePlan{}, false
	}
	for index := 0; index < demands.count; index++ {
		demand, demandOK := demands.at(index)
		if !demandOK {
			return routePlan{}, false
		}
		key, keyOK := schema.KeyAt(demand.dense)
		if !keyOK || key.Kind() != heap.RootAllocation {
			return routePlan{}, false
		}
		tag, tagOK := routeTagForDense(schema, key, demand.dense, demand.escape, demand.unknown)
		if !tagOK || !sealed.appendRoute(route{key: key, escape: demand.escape, unknown: demand.unknown, tag: tag}) {
			return routePlan{}, false
		}
	}
	return sealed, true
}

// coordinateForActual is the Value coordinate Pack's mounted row declares one
// authored actual at. The delivered vector is read at exactly these
// coordinates, and the reduction still authenticates each cell against the one
// it belongs to: a cell the owner would not admit at its own coordinate is
// malformed evidence, not a weaker fact.
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

// addFormalOperationDemand reduces one explicitly selected Target operation's
// formal rows into the invocation-local demand set. Formal rows only describe
// ownership of values supplied at the mounted call boundary: they do not issue
// an independent Heap root. An ordinal this call site does not supply selects
// nothing and contributes no demand; it is never compensated with an all-root
// demand.
func addFormalOperationDemand(
	schema placement.Schema,
	values *valuedomain.Schema,
	targetContract *contract.Contract,
	operation vocabulary.Operation,
	actualCount int,
	runtimeTail bool,
	actuals execution.SummaryVector[valuedomain.Value],
	demands *denseDemandScratch,
) bool {
	if !schema.Valid() || values == nil || !values.Valid() || targetContract == nil || operation == 0 || demands == nil {
		return false
	}
	declared, declaredOK := targetContract.Operations.OperationAt(int(operation) - 1)
	if !declaredOK || declared != operation {
		return false
	}
	tail, tailOK := targetContract.Operations.FormalEffectTail(operation)
	if !tailOK || (tail != vocabulary.RowClosed && tail != vocabulary.RowUnknownOpen) {
		return false
	}
	for effectIndex := 0; effectIndex < targetContract.Operations.FormalEffectCount(operation); effectIndex++ {
		spec, specOK := targetContract.Operations.FormalEffectAt(operation, effectIndex)
		if !specOK {
			return false
		}
		selection := resolveFormalSelectorRange(spec, actualCount, runtimeTail)
		if !selection.valid {
			return false
		}
		escape, owns := FormalEscape(spec.Kind)
		if !owns {
			continue
		}
		if selection.unknown {
			// The only source of a selector unknown is Pack's authenticated
			// runtime tail.  That tail can carry any owner-issued Value root,
			// so the complete allocation denominator is required here.
			if !addUnknownAllDense(schema, demands) {
				return false
			}
		}
		for actualIndex := selection.start; actualIndex < selection.end; actualIndex++ {
			if actualIndex < 0 || actualIndex >= actuals.Count() {
				return false
			}
			unknown, actualOK := addFactDemandDense(schema, values, actuals, actualIndex, escape, demands)
			if !actualOK {
				return false
			}
			if unknown && !addUnknownAllDense(schema, demands) {
				return false
			}
		}
	}
	if tail == vocabulary.RowUnknownOpen {
		// A formal open tail ranges over every fixed actual.  If Pack also
		// authenticates a runtime actual tail, that unrepresented suffix is
		// an arbitrary Value source and therefore widens the full denominator.
		if runtimeTail && !addUnknownAllDense(schema, demands) {
			return false
		}
		for ordinal := 0; ordinal < actuals.Count(); ordinal++ {
			if !addUnknownOpenTailActualDemandDense(schema, values, actuals, ordinal, demands) {
				return false
			}
		}
	}
	return true
}

// planFor is the one formal-to-placement reduction used by both transfer and
// derivation evidence. It accepts only already-selected Call/Value facts and
// emits exact owner-fenced allocation keys or conservative all-root routes.
func planFor(packs *packdomain.Schema, calls *calldomain.Algebra, schema placement.Schema, values *valuedomain.Schema, targetContract *contract.Contract, mounted calldomain.MountedCall, callFact calldomain.Value, actuals execution.SummaryVector[valuedomain.Value]) (routePlan, bool) {
	if packs == nil || calls == nil {
		return routePlan{}, false
	}
	_, callID, module, _, _, identityOK := calls.MountedCallIdentity(mounted)
	actual, actualOK := packs.MountedActualProjection(module, callID)
	key, keyOK := calls.KeyForMountedCall(mounted)
	actualCount := actual.ActualCount()
	_, runtimeTail := actual.TailID()
	if !calls.Valid() || !identityOK || !packs.LinkOwner().Available() || !packs.LinkOwner().Matches(calls.LinkOwner()) ||
		!actualOK || !actual.Valid() || !actual.OwnedBy(packs) || !keyOK ||
		values == nil || !values.Valid() || targetContract == nil || !schema.Valid() || !values.OwnsHeapSchema(schema.Heap()) ||
		!values.LinkOwner().Matches(calls.LinkOwner()) ||
		!calls.OwnsTargetContract(targetContract) || !actuals.Valid() || actuals.Count() != actualCount || !calls.Admits(key, callFact) {
		return routePlan{}, false
	}
	var demands denseDemandScratch
	// An open/Top Call value is authenticated Call uncertainty, but the opaque
	// arm does not issue a Target capability.  Formal rows are owned by an
	// explicitly selected operation Target, so uncertainty cannot be turned
	// into a synthetic operation/support enumeration or an all-root demand.
	// Known alternatives (including known+opaque values) are reduced below;
	// opaque-only dispatch consequently yields a valid no-route plan.
	// Every fixed actual participating in this selected-read cut must carry an
	// owner-authenticated Value factor cell. Sparse Bottom is the owner's exact
	// no-alternative default and contributes no route; it is not an unavailable
	// planner input or a request to manufacture all-root Unknown. Authenticated
	// open Pack tails are widened only by the explicit runtimeTail branch below.
	for ordinal := 0; ordinal < actuals.Count(); ordinal++ {
		value, present, cell := actuals.At(ordinal)
		fact, cellOK := values.AuthenticateFactorCell(value, present, cell)
		coordinate, coordinateOK := coordinateForActual(values, actual, ordinal)
		if !cellOK || !coordinateOK || !values.AdmitsCoordinate(coordinate, fact) {
			return routePlan{}, false
		}
	}
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK || !calls.OwnsTarget(target) {
			return routePlan{}, false
		}
		operation, operationOK := target.Operation()
		if !operationOK {
			// Body targets do not denote a Target operation and therefore do
			// not carry formal ownership metadata.
			continue
		}
		if !addFormalOperationDemand(schema, values, targetContract, operation, actualCount, runtimeTail, actuals, &demands) {
			return routePlan{}, false
		}
	}
	return (&routePlan{}).seal(schema, &demands)
}
