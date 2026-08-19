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
	stageTransform func(*ruleExecution, identity.Generation, int) bool
	noCandidate    func(*ruleExecution, identity.Generation, int) bool
	stageSelection func(*ruleExecution, identity.Generation, int, routeOutputBatch[V]) bool
	noSelection    func(*ruleExecution, identity.Generation, int, routeOutputBatch[V]) bool
	derivation     func(outputSession) ([]RuleDisposition[V], bool)
}

// routeOutputBatch is assembled synchronously by StageSelection. It retains
// the canonical selected ordinal and its owner-issued exact Ref together with
// the route-local value, so output application never has an unpaired V path.
type routeOutputBatch[V any] struct {
	read        int
	selectionID uint64
	count       int
	// at is consumed in canonical Selection order by the Factor-owned route
	// sink. It keeps Ref/tag-derived output values paired without allocating
	// a per-row Ref×value staging plane.
	at func(int) (exactRef, V, bool)
}

type outputSession interface {
	accept(*RuleEvidence) (carrier.Patch, bool)
	discard()
	complete() bool
	hasStaged() bool
	settled(int) bool
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
	routeRead     uint64
	direct        carrier.Target
	directBinding factorRuntimeBinding
	directRaw     uint64
}

type resolvedRuleTarget struct {
	target  carrier.Target
	binding factorRuntimeBinding
	raw     uint64
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
		if write.routeRead != 0 || write.direct == (carrier.Target{}) || !write.directBinding.valid() || write.directRaw >= write.directBinding.keyLimit() || write.direct.Mode() != carrier.StrongTarget {
			return nil, false
		}
		result = append(result, resolvedRuleTarget{target: write.direct, binding: write.directBinding, raw: write.directRaw})
	}
	return result, true
}

type typedOutput[K ~uint32 | ~uint64, V any] struct {
	execution    *ruleExecution
	binding      *factbinding.Binding[K, V]
	targets      func(*ruleExecution, int) ([]resolvedRuleTarget, bool)
	routeRead    uint64
	routeTarget  func(exactRef) (carrier.Target, factorRuntimeBinding, uint64, bool)
	patch        *factbinding.Patch[K, V]
	transform    typedCarryTransform[K, V]
	routeScratch []V
	// disposition is one execution-local byte per materialized Product row.
	// It is allocated only when a transfer settles its first row, and records
	// the sole legal outcome for that row: a staged value or an explicit empty
	// successor. It is not a fact plane and never escapes the execution.
	disposition  []outputDisposition
	staged       bool
	dispositions []RuleDisposition[V]
	proofCount   int
	closed       bool
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
		stageTransform: func(execution *ruleExecution, epoch identity.Generation, row int) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.stageTransform(execution, epoch, row)
		},
		noCandidate: func(execution *ruleExecution, epoch identity.Generation, row int) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.noCandidate(execution, epoch, row)
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
		derivation: func(session outputSession) ([]RuleDisposition[V], bool) {
			typed, ok := session.(*typedOutput[K, V])
			if !ok || typed.execution == nil || typed.execution.product == nil || !typed.execution.product.requireCheckpoint() {
				return nil, false
			}
			if typed.proofCount != len(typed.dispositions) {
				return nil, false
			}
			// Targets came from sealed cold assembly and RuleTarget exposes only
			// equality. The derivation can share their immutable backing rather
			// than copying every target vector per executed row.
			return typed.dispositions, true
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
	if output == nil || output.closed || output.execution != execution || execution == nil || !execution.active.holds(epoch) || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset {
		return false
	}
	if output.routeRead != 0 || output.targets == nil || output.disposition != nil && output.disposition[row] != outputUnset || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
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
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		resolved := make([]RuleTarget, len(targets))
		for index, target := range targets {
			resolved[index] = RuleTarget{target: target.target, targetBinding: target.binding, targetRaw: target.raw}
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionStaged, value: value, guard: RuleGuard{mask: when}, targets: resolved, carryTransform: output.transform.semantic, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
	return true
}

// stageTransform settles one row whose only semantic effect is its declared
// transformed carry.  It exists so a transform-only Rule has no sentinel
// write or parallel carry publication path.
func (output *typedOutput[K, V]) stageTransform(execution *ruleExecution, epoch identity.Generation, row int) bool {
	if output == nil || output.closed || !output.transform.active() || output.execution != execution || execution == nil || !execution.active.holds(epoch) || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() || !output.applyCarryTransform(execution, when) {
		return false
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputStaged
	output.staged = true
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionStaged, guard: RuleGuard{mask: when}, carryTransform: output.transform.semantic, transformOnly: true, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
	return true
}

func (output *typedOutput[K, V]) noCandidate(execution *ruleExecution, epoch identity.Generation, row int) bool {
	if output == nil || output.closed || output.execution != execution || execution == nil || !execution.active.holds(epoch) || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
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
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionNoCandidate, guard: RuleGuard{mask: when}, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
	return true
}

func (output *typedOutput[K, V]) validRouteBatch(execution *ruleExecution, epoch identity.Generation, row int, batch routeOutputBatch[V]) bool {
	if output == nil || output.closed || output.execution != execution || execution == nil || !execution.active.holds(epoch) || execution.product == nil || !execution.product.requireCheckpoint() ||
		output.routeRead == 0 || output.routeTarget == nil || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset ||
		batch.read < 0 || uint64(batch.read)+1 != output.routeRead || batch.selectionID == 0 || batch.count < 0 || batch.count > 0 && batch.at == nil {
		return false
	}
	actual, ok := execution.product.readID(row, int(output.routeRead-1))
	return ok && actual == batch.selectionID
}

// stageSelection applies one complete selected-route batch. It authenticates
// every Ref against the output Factor, retains every ordinal pair as evidence,
// then groups equal exact targets and delegates their reduction to the
// Factor's admitted Join before a single strong Set per target.
func (output *typedOutput[K, V]) stageSelection(execution *ruleExecution, epoch identity.Generation, row int, batch routeOutputBatch[V]) bool {
	if !output.validRouteBatch(execution, epoch, row, batch) || batch.count == 0 || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() {
		return false
	}
	if !output.applyCarryTransform(execution, when) || !output.beginPatch(execution) {
		return false
	}
	var pairs []RuleOutput[V]
	if execution.owner != nil && execution.owner.requiresDerivation() {
		pairs = make([]RuleOutput[V], 0, batch.count)
	}
	var target carrier.Target
	begin := 0
	for ordinal := 0; ordinal < batch.count; ordinal++ {
		ref, value, available := batch.at(ordinal)
		current, binding, raw, resolved := output.routeTarget(ref)
		if !available || !resolved || current == (carrier.Target{}) || !binding.valid() || raw >= binding.keyLimit() || current.Mode() != carrier.StrongTarget {
			return false
		}
		if pairs != nil {
			pairs = append(pairs, RuleOutput[V]{target: RuleTarget{target: current, targetBinding: binding, targetRaw: raw}, value: value, ordinal: ordinal})
		}
		if ordinal == 0 {
			target, begin = current, ordinal
			output.routeScratch = append(output.routeScratch[:0], value)
			continue
		}
		if current.Same(target) {
			output.routeScratch = append(output.routeScratch, value)
			continue
		}
		// SelectRoute is canonical Unit→tag order and this Factor's target
		// universe is declared in the same exact-key order. A decreasing target
		// therefore proves a broken route-to-target correspondence rather than
		// asking the hot path to repair it with a sort.
		if current.Less(target) || !execution.product.requireCheckpoint() || !output.patch.WriteJoined(target, when, output.routeScratch) {
			return false
		}
		target, begin = current, ordinal
		output.routeScratch = append(output.routeScratch[:0], value)
	}
	if begin < batch.count && (!execution.product.requireCheckpoint() || !output.patch.WriteJoined(target, when, output.routeScratch)) {
		return false
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputStaged
	output.staged = true
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionStaged, guard: RuleGuard{mask: when}, outputs: pairs, carryTransform: output.transform.semantic, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
	return true
}

func (output *typedOutput[K, V]) noSelection(execution *ruleExecution, epoch identity.Generation, row int, batch routeOutputBatch[V]) bool {
	if !output.validRouteBatch(execution, epoch, row, batch) || batch.count != 0 || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
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
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionNoCandidate, guard: RuleGuard{mask: when}, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
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
	if output.execution.owner != nil && output.execution.owner.requiresDerivation() && (len(output.dispositions) != rows || output.proofCount != rows) {
		return false
	}
	return true
}

func (output *typedOutput[K, V]) hasStaged() bool {
	return output != nil && output.staged
}

func (output *typedOutput[K, V]) settled(row int) bool {
	return output != nil && output.execution != nil && output.execution.product != nil && row == output.execution.product.current && row >= 0 && row < len(output.disposition) && output.disposition[row] != outputUnset
}

func (output *typedOutput[K, V]) accept(evidence *RuleEvidence) (carrier.Patch, bool) {
	if output == nil || output.closed || output.execution == nil || output.execution.failed.Load() || !evidence.consume() {
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
