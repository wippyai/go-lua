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
	stageSelection func(*ruleExecution, identity.Generation, int, routeOutputBatch[V]) bool
	noSelection    func(*ruleExecution, identity.Generation, int, routeOutputBatch[V]) bool
}

// routeOutputBatch is fully materialized synchronously by StageSelection. It
// retains only canonical ordinal-aligned owner-issued Refs and values; no
// domain callback or frame capability survives into output application.
type routeOutputBatch[V any] struct {
	read        int
	selectionID uint64
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
	execution    *ruleExecution
	binding      *factbinding.Binding[K, V]
	targets      func(*ruleExecution, int) ([]resolvedRuleTarget, bool)
	routeRead    uint64
	routeTarget  func(exactRef) (carrier.Target, schemaFactorBinding, uint64, bool)
	patch        *factbinding.Patch[K, V]
	transform    typedCarryTransform[K, V]
	routeScratch []V
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
			// the binding scratch until the first actual StageValue.
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
	if output == nil || output.closed || output.execution != execution || execution == nil || !execution.active.Holds(epoch) || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset {
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
	if output == nil || output.closed || output.execution != execution || execution == nil || !execution.active.Holds(epoch) || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset {
		return false
	}
	if output.routeRead != 0 {
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
	return len(batch.refs) == len(batch.values) && output.validRouteToken(execution, epoch, row, batch.read, batch.selectionID)
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
	routes := make([]resolvedRuleTarget, len(batch.refs))
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
	target := routes[0].target
	output.routeScratch = append(output.routeScratch[:0], batch.values[0])
	for ordinal := 1; ordinal < len(batch.refs); ordinal++ {
		current := routes[ordinal].target
		if current.Same(target) {
			output.routeScratch = append(output.routeScratch, batch.values[ordinal])
			continue
		}
		if !execution.product.requireCheckpoint() || !output.patch.WriteJoined(target, when, output.routeScratch) {
			return false
		}
		target = current
		output.routeScratch = append(output.routeScratch[:0], batch.values[ordinal])
	}
	if !execution.product.requireCheckpoint() || !output.patch.WriteJoined(target, when, output.routeScratch) {
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
	if !output.validRouteBatch(execution, epoch, row, batch) || len(batch.refs) != 0 {
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
	if output == nil || output.closed || output.execution == nil || output.execution.product == nil || !output.execution.product.requireCheckpoint() {
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
	if output == nil || output.closed || output.execution == nil || output.execution.failed.Load() {
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
