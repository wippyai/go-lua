// runtime_factor.go declares the bound Factor vocabulary and its runtime methods.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

type boundUnit struct {
	unit        carrier.Unit
	kind        carrier.UnitKind
	local       uint64
	summaryKeys []uint64
}

type boundTarget struct {
	target carrier.Target
	mode   carrier.TargetMode
	local  uint64
}

// boundFactor is the private concrete half of a cold Factor. It is made by
// assembly, not declaration: Factor remains a cold owner capability and never
// receives a carrier slot, binding, Unit, Target, or Selector.
type boundFactor[K ~uint32 | ~uint64, V any] struct {
	implementation *FactorImplementation[K, V]
	binding        *factbinding.Binding[K, V]
	slot           shape.Slot
	hasSlot        bool
	reads          map[equation.Surface]boundUnit
	writes         map[equation.Surface]boundTarget
	// dynamicUnits is the one Factor-owned exact Unit universe needed by
	// staged reads. It is allocated once only when a sealed ReadSelect targets
	// this Factor; no Rule/input/root candidate product is retained.
	dynamicUnits []carrier.Unit
	// routeTargets is the Factor-owned presealed strong target universe paired
	// positionally with dynamicUnits. It exists once for a Factor that owns a
	// route write, never once per Rule member or source root.
	routeTargets     []carrier.Target
	routeTransform   factbinding.TransformClosure[K, V]
	routeTransformOK bool
	carryTargets     map[composition.Key][]carrier.Target
	carryRouteScope  map[composition.Key]bool
}

type runtimeFactor interface {
	semantic() identity.SemanticKey
	operation() carrier.FactorOperation
	runtimeSlot() (shape.Slot, bool)
	carryTargetsFor(equation.RuleMember) ([]carrier.Target, bool)
	supports(carrier.MergeKind) bool
	readUnit(equation.Surface) (carrier.Unit, bool)
	writeTarget(equation.Surface) (carrier.Target, bool)
	hasRouteUniverse() bool
	routeUniverse() []carrier.Target
	carryRouteScopeFor(equation.RuleMember) bool
	releaseColdBindings()
}

func (bound *boundFactor[K, V]) semantic() identity.SemanticKey {
	if bound == nil || bound.implementation == nil || !bound.implementation.descriptor.valid() {
		return identity.SemanticKey{}
	}
	semantic, ok := semanticKeyFromComposition(bound.implementation.descriptor.semantic)
	if !ok {
		return identity.SemanticKey{}
	}
	return semantic
}

func (bound *boundFactor[K, V]) operation() carrier.FactorOperation {
	if bound == nil {
		return nil
	}
	return bound.binding
}

// supports is cold typed recurrence metadata.  Assembly uses it to derive the
// exact Narrow-capable subset of an occurrence footprint before carrier scopes
// are sealed; it never probes a live operation or falls back after a rejected
// scope.
func (bound *boundFactor[K, V]) supports(kind carrier.MergeKind) bool {
	return bound != nil && bound.binding != nil && bound.binding.Supports(kind)
}

func (bound *boundFactor[K, V]) bindRuntimeSlot(slot shape.Slot) bool {
	if bound == nil || bound.binding == nil || bound.hasSlot || slot < 0 {
		return false
	}
	bound.slot, bound.hasSlot = slot, true
	return true
}

func (bound *boundFactor[K, V]) runtimeSlot() (shape.Slot, bool) {
	if bound == nil || !bound.hasSlot {
		return 0, false
	}
	return bound.slot, true
}

func (bound *boundFactor[K, V]) carryTargetsFor(member equation.RuleMember) ([]carrier.Target, bool) {
	if bound == nil || !member.Key().Available() {
		return nil, false
	}
	targets, ok := bound.carryTargets[member.Key()]
	if !ok {
		return nil, false
	}
	return append([]carrier.Target(nil), targets...), true
}

func (bound *boundFactor[K, V]) readUnit(surface equation.Surface) (carrier.Unit, bool) {
	if bound == nil {
		return carrier.Unit{}, false
	}
	unit, ok := bound.reads[surface]
	return unit.unit, ok
}

// summaryReadAddress returns the sealed Factor row and its form address. The
// caller already owns the declaration-time shape checks; runtime keeps the
// row directly instead of minting a second summary proof object.
func (bound *boundFactor[K, V]) summaryReadAddress(surface equation.Surface, formOrdinal uint64, semantic composition.Key) (factorRuntimeBinding, factorFormReceipt, []uint64, [32]byte, bool) {
	if bound == nil || bound.implementation == nil || !bound.implementation.binding.valid() || !semantic.Available() {
		return factorRuntimeBinding{}, factorFormReceipt{}, nil, [32]byte{}, false
	}
	binding := bound.implementation.binding
	unit, found := bound.reads[surface]
	if !found || unit.kind != carrier.SummaryUnit || len(unit.summaryKeys) == 0 || !matchesFactorReadShape(binding.schema, binding.ordinal, surface, summaryReadForm) || surface.Semantic != semantic || surface.Normalizer != semantic {
		return factorRuntimeBinding{}, factorFormReceipt{}, nil, [32]byte{}, false
	}
	formReceipt, formOK := binding.formAt(formOrdinal, SchemaFormReadSummary, semantic)
	if !formOK {
		return factorRuntimeBinding{}, factorFormReceipt{}, nil, [32]byte{}, false
	}
	digest := SummaryVectorDigest(unit.summaryKeys)
	if digest == ([32]byte{}) {
		return factorRuntimeBinding{}, factorFormReceipt{}, nil, [32]byte{}, false
	}
	return binding, formReceipt, unit.summaryKeys, digest, true
}

// stagedUnit resolves only a Factor-issued exact Ref through the predeclared
// dynamic exact universe. The Ref validates the same sealed Factor before its
// raw coordinate is used; no key, Unit, or graph lookup is exposed to the
// locator.
func (bound *boundFactor[K, V]) stagedUnit(ref exactRef) (carrier.Unit, bool) {
	if bound == nil || bound.implementation == nil || !bound.implementation.binding.valid() || len(bound.dynamicUnits) == 0 || ref == nil {
		return carrier.Unit{}, false
	}
	var raw uint64
	var ok bool
	if typed, valid := ref.(interface {
		factorBinding() factorRuntimeBinding
		rawAddress() uint64
	}); valid {
		address := typed.factorBinding()
		raw, ok = typed.rawAddress(), factorAddressMatches(address, bound.implementation.binding)
	}
	if !ok || raw >= uint64(len(bound.dynamicUnits)) {
		return carrier.Unit{}, false
	}
	unit := bound.dynamicUnits[int(raw)]
	if unit == (carrier.Unit{}) {
		return carrier.Unit{}, false
	}
	return unit, true
}

// stagedTarget resolves the same authenticated exact Ref through the
// presealed route-target universe. Its positional pairing with stagedUnit is
// established during Factor binding, so runtime never declares a target or
// reconstructs a key after sealing.
func (bound *boundFactor[K, V]) stagedTarget(ref exactRef) (carrier.Target, factorRuntimeBinding, uint64, bool) {
	if bound == nil || bound.implementation == nil || !bound.implementation.binding.valid() || len(bound.routeTargets) != len(bound.dynamicUnits) || len(bound.routeTargets) == 0 || ref == nil {
		return carrier.Target{}, factorRuntimeBinding{}, 0, false
	}
	var raw uint64
	var ok bool
	if typed, valid := ref.(interface {
		factorBinding() factorRuntimeBinding
		rawAddress() uint64
	}); valid {
		address := typed.factorBinding()
		raw, ok = typed.rawAddress(), factorAddressMatches(address, bound.implementation.binding)
	}
	if !ok || raw >= uint64(len(bound.routeTargets)) {
		return carrier.Target{}, factorRuntimeBinding{}, 0, false
	}
	target := bound.routeTargets[int(raw)]
	if target == (carrier.Target{}) || target.Mode() != carrier.StrongTarget {
		return carrier.Target{}, factorRuntimeBinding{}, 0, false
	}
	targetRaw, proven := exactWriteLocal(bound.implementation.binding, exactWriteReceiptSurface(bound.implementation.binding, raw+1))
	if !proven || targetRaw != raw {
		return carrier.Target{}, factorRuntimeBinding{}, 0, false
	}
	return target, bound.implementation.binding, targetRaw, true
}

func (bound *boundFactor[K, V]) routeUniverse() []carrier.Target {
	if bound == nil || len(bound.routeTargets) == 0 {
		return nil
	}
	// routeTargets is sealed Factor-owned data. Runtime consumers only range
	// over this immutable view or append its elements into their own scoped
	// seal buffer; returning a copy here would recreate the old per-member
	// route-universe materialization that the recurrence cut deliberately
	// moved to (Region, Factor).
	return bound.routeTargets
}

func (bound *boundFactor[K, V]) routeTransformClosure() (factbinding.TransformClosure[K, V], bool) {
	if bound == nil || !bound.routeTransformOK {
		return factbinding.TransformClosure[K, V]{}, false
	}
	return bound.routeTransform, true
}

// prepareRouteTransformClosure is called exactly once after the prepared
// carrier attaches every Factor Binding to its sealed SlotOwner. Binding's
// TransformClosure intentionally rejects cold, unattached authorities; this
// post-attach cut keeps the immutable route closure Factor-owned without
// rebuilding it for each transformed-carry member.
func (bound *boundFactor[K, V]) prepareRouteTransformClosure() bool {
	if bound == nil || bound.binding == nil {
		return false
	}
	if bound.routeTransformOK {
		return true
	}
	closure, ok := bound.binding.TransformClosure(bound.routeTargets)
	if !ok {
		return false
	}
	bound.routeTransform, bound.routeTransformOK = closure, true
	return true
}

func (bound *boundFactor[K, V]) hasRouteUniverse() bool {
	return bound != nil && bound.implementation != nil && (len(bound.routeTargets) != 0 || bound.implementation.algebra != nil && bound.implementation.algebra.KeyEnd() == 0)
}

func (bound *boundFactor[K, V]) carryRouteScopeFor(member equation.RuleMember) bool {
	return bound != nil && member.Key().Available() && bound.carryRouteScope != nil && bound.carryRouteScope[member.Key()]
}

func (bound *boundFactor[K, V]) stagedSlot() (shape.Slot, bool) {
	return bound.runtimeSlot()
}

// stagedObserve is the typed owner-side bridge from a selected opaque exact
// Unit to Product refinement. The generic engine supplies only the current
// carrier Work, input State, and guard piece; this method keeps root lookup,
// typed observation decoding, and the V payload entirely inside the Factor
// owner. One Begin/End generation surrounds exactly one selected Unit.
func (bound *boundFactor[K, V]) stagedObserve(work *carrier.Work, input carrier.State, unit carrier.Unit, within support.Mask, visit func(factbinding.Observation[V], support.Mask) bool) bool {
	_, ok := bound.stagedObserveWithFailure(work, input, unit, within, visit)
	return ok
}

type stagedObservationFailure uint8

const (
	stagedObservationFailureNone stagedObservationFailure = iota
	stagedObservationFailureArguments
	stagedObservationFailureCheckpoint
	stagedObservationFailureUnit
	stagedObservationFailureSupport
	stagedObservationFailureSlot
	stagedObservationFailureWork
	stagedObservationFailureRoot
	stagedObservationFailureCarrier
	stagedObservationFailureDecode
	stagedObservationFailureVisitor
)

// stagedObserveWithFailure keeps the same typed owner boundary as
// stagedObserve while classifying the first closed read predicate. Optional
// solve-local observations use this classification for failure telemetry;
// ordinary rule and query behavior is unchanged.
func (bound *boundFactor[K, V]) stagedObserveWithFailure(work *carrier.Work, input carrier.State, unit carrier.Unit, within support.Mask, visit func(factbinding.Observation[V], support.Mask) bool) (stagedObservationFailure, bool) {
	if bound == nil || bound.binding == nil || work == nil || visit == nil {
		return stagedObservationFailureArguments, false
	}
	if !work.Checkpoint() {
		return stagedObservationFailureCheckpoint, false
	}
	if unit == (carrier.Unit{}) {
		return stagedObservationFailureUnit, false
	}
	if !within.Valid() {
		return stagedObservationFailureSupport, false
	}
	slot, slotOK := bound.runtimeSlot()
	if !slotOK {
		return stagedObservationFailureSlot, false
	}
	slotWork, workOK := work.SlotWork(slot)
	if !workOK || !slotWork.BeginObservation() {
		return stagedObservationFailureWork, false
	}
	defer slotWork.EndObservation()
	root, rootOK := input.HandleAt(slot)
	if !rootOK {
		return stagedObservationFailureRoot, false
	}
	failure := stagedObservationFailureNone
	completed := slotWork.ObserveUnder(root, unit, within, func(row carrier.ObservationRow) bool {
		if !work.Checkpoint() {
			failure = stagedObservationFailureVisitor
			return false
		}
		observation, resolved := bound.binding.ResolveObservation(slotWork, row)
		if !resolved || !observation.Valid() {
			failure = stagedObservationFailureDecode
			return false
		}
		if !visit(observation, row.Region()) {
			failure = stagedObservationFailureVisitor
			return false
		}
		return true
	})
	if !completed {
		if failure == stagedObservationFailureNone {
			failure = stagedObservationFailureCarrier
		}
		return failure, false
	}
	return stagedObservationFailureNone, true
}

func (bound *boundFactor[K, V]) writeTarget(surface equation.Surface) (carrier.Target, bool) {
	if bound == nil {
		return carrier.Target{}, false
	}
	target, ok := bound.writes[surface]
	return target.target, ok
}

// releaseColdBindings is called only after the sole compiler has attached all
// members and queries. Those runtime objects retain concrete Units, Targets,
// and Selectors; keeping surface maps or target vectors past that cut would
// make an inert graph catalog a second hot-path authority.
func (bound *boundFactor[K, V]) releaseColdBindings() {
	if bound == nil {
		return
	}
	bound.reads = nil
	bound.writes = nil
}
