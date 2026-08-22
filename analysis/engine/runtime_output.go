// runtime_output.go implements output access, typed staging, the carry transform and patch acceptance.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

type outputAccess[V any] struct {
	begin          func(*ruleExecution) outputSession
	stage          func(*ruleExecution, identity.Generation, int, V) bool
	noCandidate    func(*ruleExecution, identity.Generation, int) bool
	routePreflight func(*ruleExecution, identity.Generation, int, int, uint64) bool
	routeReserve   func(*ruleExecution, identity.Generation, int, int, uint64, int) ([]exactRef, []V, uint64, bool)
	routeRelease   func(*ruleExecution, identity.Generation, int, int, uint64, uint64) bool
	stageSelection func(*ruleExecution, identity.Generation, int, routeOutputBatch[V]) bool
	noSelection    func(*ruleExecution, identity.Generation, int, routeOutputBatch[V]) bool
}

// routeOutputBatch is fully materialized synchronously by StageSelection. It
// retains only canonical ordinal-aligned owner-issued Refs and values; no
// domain callback or frame capability survives into output application.
type routeOutputBatch[V any] struct {
	read        int
	selectionID uint64
	lease       uint64
	refs        []exactRef
	values      []V
}

type outputSession interface {
	publish() (carrier.Patch, bool)
	discard()
	complete() bool
	hasStaged() bool
}

// outputRuntime is the private per-row staged-target projection. Direct
// targets and selector targets share one ordered write vector so a selector
// may consult already computed target bits but never future output state.
type outputRuntime struct{ writes []outputWriteRuntime }

func (runtime *outputRuntime) routeRead() (uint64, bool) {
	if runtime == nil {
		return 0, false
	}
	var found uint64
	for _, write := range runtime.writes {
		if write.routeRead == 0 {
			continue
		}
		if found != 0 || write.direct != (carrier.Target{}) {
			return 0, false
		}
		found = write.routeRead
	}
	return found, found != 0
}

type outputWriteRuntime struct {
	// routeRead is the one-based staged read ordinal consumed by a route batch.
	// Zero is the ordinary direct/static target form.
	routeRead uint64
	direct    carrier.Target
	directRow schemaFactorBinding
	directRaw uint64
}

type resolvedRuleTarget struct {
	target carrier.Target
	row    schemaFactorBinding
	raw    uint64
}

// forEachRouteGroup walks one already-authenticated canonical target vector.
// Each values argument is a contiguous subslice of the original batch; the
// visitor consumes it synchronously and cannot retain a route callback or
// domain capability in the output batch.
func forEachRouteGroup[V any](routes []resolvedRuleTarget, values []V, visit func(carrier.Target, []V) bool) bool {
	if len(routes) == 0 || len(routes) != len(values) || visit == nil {
		return false
	}
	target := routes[0].target
	start := 0
	for ordinal := 1; ordinal < len(routes); ordinal++ {
		current := routes[ordinal].target
		if current.Same(target) {
			continue
		}
		if !visit(target, values[start:ordinal]) {
			return false
		}
		target = current
		start = ordinal
	}
	return visit(target, values[start:])
}

func (runtime *outputRuntime) targets(execution *ruleExecution, row int) ([]resolvedRuleTarget, bool) {
	if runtime == nil || execution == nil || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || len(runtime.writes) == 0 {
		return nil, false
	}
	result := make([]resolvedRuleTarget, 0, len(runtime.writes))
	for _, write := range runtime.writes {
		if !execution.product.requireCheckpoint() {
			return nil, false
		}
		if write.routeRead != 0 || write.direct == (carrier.Target{}) || !factorRowAvailable(write.directRow) || write.directRaw >= write.directRow.schemaFactorAlgebra().KeyEnd() || write.direct.Mode() != carrier.StrongTarget {
			return nil, false
		}
		result = append(result, resolvedRuleTarget{target: write.direct, row: write.directRow, raw: write.directRaw})
	}
	return result, true
}

type typedOutput[K ~uint32 | ~uint64, V any] struct {
	execution           *ruleExecution
	binding             *factbinding.Binding[K, V]
	targets             func(*ruleExecution, int) ([]resolvedRuleTarget, bool)
	routeRead           uint64
	routeTarget         func(exactRef) (carrier.Target, schemaFactorBinding, uint64, bool)
	patch               *factbinding.Patch[K, V]
	transform           typedCarryTransform[K, V]
	routeRefs           []exactRef
	routeValues         []V
	routeTargets        []resolvedRuleTarget
	routeBusy           bool
	routeLeaseRow       int
	routeLeaseRead      int
	routeLeaseSelection uint64
	routeLease          uint64
	// disposition is one execution-local byte per materialized Product row.
	// It is allocated only when a transfer settles its first row, and records
	// the sole legal outcome for that row: a staged value or an explicit empty
	// successor. It is not a fact plane and never escapes the execution.
	disposition []outputDisposition
	staged      bool
	closed      bool
}

// typedCarryTransform is a Factor-owned map plus the immutable, precompiled
// carried target closure on which it may act.  It has no domain vocabulary;
// only the owner-specific V callback is retained here.
type typedCarryTransform[K ~uint32 | ~uint64, V any] struct {
	semantic identity.SemanticKey
	closures []factbinding.TransformClosure[K, V]
	apply    func(V) (V, bool)
}

func (transform typedCarryTransform[K, V]) active() bool {
	return transform.semantic.Available() && transform.apply != nil
}

type transformedCarryOwner[V any] interface {
	transformedCarry() (identity.SemanticKey, []carrier.Target, func(V) (V, bool), bool)
}

type transformedCarryRouteOwner interface {
	transformedCarryRoute() bool
}

type outputDisposition uint8

const (
	outputUnset outputDisposition = iota
	outputStaged
	outputNoCandidate
)

// newTypedOutputAccess closes the Factor owner's private K at cold binding
// time. Rule compilation sees only runtimeFactor plus this V-specialized
// access object, so a named K never needs reflection, an offset conversion,
// or a type-switch arm in the hot runtime.
func newTypedOutputAccess[K ~uint32 | ~uint64, V any](output *boundFactor[K, V], owner anyRule, projection *outputRuntime) (outputAccess[V], bool) {
	if output == nil || output.binding == nil || output.implementation == nil || output.implementation.algebra == nil || owner == nil || projection == nil {
		return outputAccess[V]{}, false
	}
	routeRead, routeOutput := projection.routeRead()
	if routeOutput && !output.hasRouteUniverse() {
		return outputAccess[V]{}, false
	}
	var transform typedCarryTransform[K, V]
	if transformed, present := owner.(transformedCarryOwner[V]); present {
		semantic, targets, apply, active := transformed.transformedCarry()
		if active {
			defaultValue, defaultOK := output.implementation.algebra.Default()
			mappedDefault, mappedOK := apply(defaultValue)
			if !defaultOK || !mappedOK || !output.implementation.algebra.Equal(defaultValue, mappedDefault) {
				return outputAccess[V]{}, false
			}
			closure, closed := output.binding.TransformClosure(targets)
			if !closed || !semantic.Available() || apply == nil {
				return outputAccess[V]{}, false
			}
			closures := []factbinding.TransformClosure[K, V]{closure}
			if routeOwner, routeRequired := owner.(transformedCarryRouteOwner); routeRequired && routeOwner.transformedCarryRoute() {
				if !output.hasRouteUniverse() {
					return outputAccess[V]{}, false
				}
				routeClosure, routeOK := output.routeTransformClosure()
				if !routeOK {
					return outputAccess[V]{}, false
				}
				closures = append(closures, routeClosure)
			}
			transform = typedCarryTransform[K, V]{semantic: semantic, closures: closures, apply: apply}
		}
	}
	return outputAccess[V]{
		begin: func(execution *ruleExecution) outputSession {
			if execution == nil || execution.owner != owner || execution.work == nil {
				return nil
			}
			// A no-candidate Product must not create an empty Factor patch. Delay
			// the binding scratch until the first actual Staged result.
			typed := &typedOutput[K, V]{execution: execution, binding: output.binding, targets: projection.targets, transform: transform}
			if routeOutput {
				typed.routeRead = routeRead
				typed.routeTarget = output.stagedTarget
			}
			return typed
		},
		stage: func(execution *ruleExecution, epoch identity.Generation, row int, value V) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			if !ok || !typed.stage(execution, epoch, row, value) {
				return false
			}
			return true
		},
		noCandidate: func(execution *ruleExecution, epoch identity.Generation, row int) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.noCandidate(execution, epoch, row)
		},
		routePreflight: func(execution *ruleExecution, epoch identity.Generation, row, read int, selectionID uint64) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.validRouteToken(execution, epoch, row, read, selectionID)
		},
		routeReserve: func(execution *ruleExecution, epoch identity.Generation, row, read int, selectionID uint64, count int) ([]exactRef, []V, uint64, bool) {
			if execution == nil || execution.owner != owner {
				return nil, nil, 0, false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			if !ok {
				return nil, nil, 0, false
			}
			return typed.reserveRoute(execution, epoch, row, read, selectionID, count)
		},
		routeRelease: func(execution *ruleExecution, epoch identity.Generation, row, read int, selectionID, lease uint64) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.releaseRoute(execution, epoch, row, read, selectionID, lease)
		},
		stageSelection: func(execution *ruleExecution, epoch identity.Generation, row int, batch routeOutputBatch[V]) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.stageSelection(execution, epoch, row, batch)
		},
		noSelection: func(execution *ruleExecution, epoch identity.Generation, row int, batch routeOutputBatch[V]) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.noSelection(execution, epoch, row, batch)
		},
	}, true
}

func (output *typedOutput[K, V]) beginPatch(execution *ruleExecution) bool {
	if output == nil || execution == nil || output.patch != nil {
		return output != nil && output.patch != nil
	}
	output.patch = output.binding.Begin(execution.work, execution.base.State())
	return output.patch != nil
}

// applyCarryTransform applies the one declared carry map before any ordinary
// exact writes for this Product row.  A no-candidate row never reaches this
// method.  The Patch remains unpublished until the enclosing Group finishes,
// so a later write failure discards both operations together.
func (output *typedOutput[K, V]) applyCarryTransform(execution *ruleExecution, when support.Mask) bool {
	if output == nil || !output.transform.active() {
		return true
	}
	return output.beginPatch(execution) && output.patch.TransformClosures(output.transform.closures, when, output.transform.apply)
}

func (output *typedOutput[K, V]) stage(execution *ruleExecution, epoch identity.Generation, row int, value V) bool {
	if output == nil || output.closed || output.routeBusy || output.execution != execution || execution == nil || !execution.active.Holds(epoch) || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset {
		return false
	}
	if output.routeRead != 0 || output.targets == nil || output.disposition != nil && output.disposition[row] != outputUnset {
		return false
	}
	targets, ok := output.targets(execution, row)
	if !ok {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() {
		return false
	}
	if len(targets) == 0 {
		// A Factor write must name at least one sealed target. Selector
		// emptiness is an explicit NoCandidate outcome, never a no-op stage.
		return false
	}
	if !output.applyCarryTransform(execution, when) || !output.beginPatch(execution) {
		return false
	}
	for _, target := range targets {
		if !execution.product.requireCheckpoint() {
			return false
		}
		if !output.patch.Write(target.target, when, value) {
			return false
		}
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputStaged
	output.staged = true
	return true
}

func (output *typedOutput[K, V]) noCandidate(execution *ruleExecution, epoch identity.Generation, row int) bool {
	if output == nil || output.closed || output.routeBusy || output.execution != execution || execution == nil || !execution.active.Holds(epoch) || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() {
		return false
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputNoCandidate
	return true
}

func (output *typedOutput[K, V]) validRouteToken(execution *ruleExecution, epoch identity.Generation, row, read int, selectionID uint64) bool {
	if output == nil || output.closed || output.execution != execution || execution == nil || !execution.active.Holds(epoch) || execution.product == nil || !execution.product.requireCheckpoint() ||
		output.routeRead == 0 || output.routeTarget == nil || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset ||
		read < 0 || uint64(read)+1 != output.routeRead || selectionID == 0 {
		return false
	}
	actual, ok := execution.product.readID(row, int(output.routeRead-1))
	return ok && actual == selectionID
}

func (output *typedOutput[K, V]) validRouteBatch(execution *ruleExecution, epoch identity.Generation, row int, batch routeOutputBatch[V]) bool {
	if output == nil || !output.routeBusy || len(batch.refs) == 0 || len(batch.refs) != len(batch.values) || len(output.routeRefs) != len(batch.refs) || len(output.routeValues) != len(batch.values) {
		return false
	}
	// Routed returns slices into this execution-local reservation.  A result
	// cannot be settled after its reservation was released, nor can a foreign
	// batch be mistaken for the currently leased scratch.  The backing-array
	// identity check is deliberately only a fence; all Ref/value contents are
	// still authenticated below before the Patch opens.
	if &batch.refs[0] != &output.routeRefs[0] || &batch.values[0] != &output.routeValues[0] || batch.lease == 0 || batch.lease != output.routeLease {
		return false
	}
	if !output.validRouteToken(execution, epoch, row, batch.read, batch.selectionID) {
		return false
	}
	return true
}

// reserveRoute leases one execution-local pair of typed vectors to Routed.
// The lease is held across the synchronous fold result and settlement, so a
// second Routed call cannot overwrite a result that is still being consumed.
// Capacity is retained only by this typedOutput; no pool or shared hot-rule
// state is involved.
func (output *typedOutput[K, V]) reserveRoute(execution *ruleExecution, epoch identity.Generation, row, read int, selectionID uint64, count int) ([]exactRef, []V, uint64, bool) {
	if output == nil || output.closed || output.routeBusy || count <= 0 || !output.validRouteToken(execution, epoch, row, read, selectionID) {
		return nil, nil, 0, false
	}
	lease := output.routeLease + 1
	if lease == 0 {
		return nil, nil, 0, false
	}
	if cap(output.routeRefs) < count {
		output.routeRefs = make([]exactRef, count)
	} else {
		output.routeRefs = output.routeRefs[:count]
	}
	if cap(output.routeValues) < count {
		output.routeValues = make([]V, count)
	} else {
		output.routeValues = output.routeValues[:count]
	}
	if cap(output.routeTargets) < count {
		output.routeTargets = make([]resolvedRuleTarget, count)
	} else {
		output.routeTargets = output.routeTargets[:count]
	}
	output.routeLeaseRow = row
	output.routeLeaseRead = read
	output.routeLeaseSelection = selectionID
	output.routeLease = lease
	output.routeBusy = true
	return output.routeRefs, output.routeValues, lease, true
}

// releaseRoute closes the one synchronous reservation.  Clearing both
// vectors breaks references held by a failed or escaped result while keeping
// their capacity available for the next Product row in this execution.
func (output *typedOutput[K, V]) releaseRoute(execution *ruleExecution, epoch identity.Generation, row, read int, selectionID, lease uint64) bool {
	if output == nil || !output.routeBusy || output.execution != execution || execution == nil || output.execution.epoch != epoch || !execution.active.Holds(epoch) || output.routeLeaseRow != row || output.routeLeaseRead != read || output.routeLeaseSelection != selectionID || output.routeLease != lease {
		return false
	}
	output.clearRouteReservation()
	return true
}

func (output *typedOutput[K, V]) clearRouteReservation() {
	if output == nil {
		return
	}
	clear(output.routeRefs)
	clear(output.routeValues)
	clear(output.routeTargets)
	output.routeRefs = output.routeRefs[:0]
	output.routeValues = output.routeValues[:0]
	output.routeTargets = output.routeTargets[:0]
	output.routeLeaseRow = -1
	output.routeLeaseRead = -1
	output.routeLeaseSelection = 0
	output.routeBusy = false
}

// stageSelection applies one complete selected-route batch. It authenticates
// every Ref against the output Factor, retains every ordinal pair in the batch,
// then groups equal exact targets and delegates their reduction to the
// Factor's admitted Join before a single strong Set per target.
func (output *typedOutput[K, V]) stageSelection(execution *ruleExecution, epoch identity.Generation, row int, batch routeOutputBatch[V]) bool {
	if !output.validRouteBatch(execution, epoch, row, batch) || len(batch.refs) == 0 {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() {
		return false
	}
	output.routeTargets = output.routeTargets[:len(batch.refs)]
	routes := output.routeTargets
	for ordinal := range batch.refs {
		ref := batch.refs[ordinal]
		current, row, raw, resolved := output.routeTarget(ref)
		if ref == nil || !resolved || current == (carrier.Target{}) || !factorRowAvailable(row) || raw >= row.schemaFactorAlgebra().KeyEnd() || current.Mode() != carrier.StrongTarget || !execution.product.requireCheckpoint() || ordinal > 0 && current.Less(routes[ordinal-1].target) {
			return false
		}
		routes[ordinal] = resolvedRuleTarget{target: current, row: row, raw: raw}
	}
	// Every domain value and owner-issued Ref is now materialized, authenticated,
	// and proven to follow canonical target order. Only this engine-owned phase
	// may open and mutate the transaction-local Patch.
	if !output.applyCarryTransform(execution, when) || !output.beginPatch(execution) {
		return false
	}
	if !forEachRouteGroup(routes, batch.values, func(target carrier.Target, values []V) bool {
		return execution.product.requireCheckpoint() && output.patch.WriteJoined(target, when, values)
	}) {
		return false
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputStaged
	output.staged = true
	return true
}

func (output *typedOutput[K, V]) noSelection(execution *ruleExecution, epoch identity.Generation, row int, batch routeOutputBatch[V]) bool {
	if output == nil || output.closed || output.routeBusy || batch.lease != 0 || len(batch.refs) != 0 || len(batch.values) != 0 || !output.validRouteToken(execution, epoch, row, batch.read, batch.selectionID) {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() {
		return false
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputNoCandidate
	return true
}

func (output *typedOutput[K, V]) complete() bool {
	if output == nil || output.closed || output.routeBusy || output.execution == nil || output.execution.product == nil || !output.execution.product.requireCheckpoint() {
		return false
	}
	rows := len(output.execution.product.values)
	if rows == 0 {
		return true
	}
	if len(output.disposition) != rows {
		return false
	}
	for _, disposition := range output.disposition {
		if disposition == outputUnset {
			return false
		}
	}
	return true
}

func (output *typedOutput[K, V]) hasStaged() bool {
	return output != nil && output.staged
}

func (output *typedOutput[K, V]) publish() (carrier.Patch, bool) {
	if output == nil || output.closed || output.routeBusy || output.execution == nil || output.execution.failed.Load() {
		return carrier.Patch{}, false
	}
	output.closed = true
	if output.patch == nil {
		return carrier.Patch{}, false
	}
	patch := output.patch
	accepted, ok := patch.Accept(output.execution.work)
	// Patch.Accept owns deterministic stage/pending-root cleanup on failure.
	// Retain the sole pointer here until that transaction has returned.
	output.patch = nil
	return accepted, ok
}

func (output *typedOutput[K, V]) discard() {
	if output == nil || output.closed {
		return
	}
	output.closed = true
	if output.routeBusy {
		output.clearRouteReservation()
	}
	if output.patch != nil {
		output.patch.Discard()
		output.patch = nil
	}
}

// bindMemberRule turns graph-owned member metadata into the one private
// per-row target projection. The caller supplies no write surface, selector
// candidate, dependency, or relation: the member is the sole occurrence owner.
func appendUniqueTarget(targets []carrier.Target, candidate carrier.Target) []carrier.Target {
	for _, target := range targets {
		if target.Same(candidate) {
			return targets
		}
	}
	return append(targets, candidate)
}
