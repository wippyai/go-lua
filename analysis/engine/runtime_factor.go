// runtime_factor.go declares the bound Factor vocabulary and its runtime methods.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
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
	// sourceColumns are copied from the generated relation owner while this
	// Factor is bound to a Program.  They are sealed data, not an owner
	// capability: no runtime row can reopen a domain schema or call an owner.
	sourceColumns []memberrelation.SourceColumn[V]
	sourcePresent []bool
	// families is the sealed table of installers authoring a concretely-typed
	// execution family for one rule ordinal each. It is sealed data in the same
	// sense the source columns are: the runtime asks it once, at Program seal,
	// and never during a solve.
	families *execution.RuleFamilies[K, V]
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
	// buildGeneratedFamilies compiles all generated rows owned by this typed
	// factor into one executor family per present execution form. It is an epoch
	// setup operation; execution reaches only the dense catalog address it
	// returns. foreign is the Program's Factor read table, indexed by sealed
	// Factor ordinal, so a rule that joins an axis it does not write to can
	// seal that read at the read fact's own types.
	buildGeneratedFamilies([]execution.FormRow, []execution.ForeignFactor) ([]execution.Family, []execution.FormAddress, bool)
	// foreignRead is this Factor's own read side with its types erased. It is
	// what the table above is built from, and it is the only thing one Factor
	// hands another.
	foreignRead() (execution.ForeignFactor, bool)
}

func (bound *boundFactor[K, V]) semantic() identity.SemanticKey {
	if bound == nil || bound.implementation == nil || !factorRowAvailable(bound.implementation.row) {
		return identity.SemanticKey{}
	}
	semantic, ok := semanticKeyFromComposition(bound.implementation.row.schemaFactorSemanticKey())
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

// summaryReadAddress consumes the already compiled Rule read row. The Factor
// owner supplies only the closed summary unit; no read shape or form walk is
// repeated at member bind time.
func (bound *boundFactor[K, V]) summaryReadAddress(surface equation.Surface, row *schemaRuleReadRow) (schemaFactorBinding, uint64, []uint64, [32]byte, bool) {
	if bound == nil || bound.implementation == nil || !factorRowAvailable(bound.implementation.row) || row == nil || row.kind != composition.ReadSummary || !row.semantic.Available() || row.factor != bound.implementation.row.schemaFactorSemanticKey() {
		return nil, 0, nil, [32]byte{}, false
	}
	factorRow := bound.implementation.row
	unit, found := bound.reads[surface]
	if !found || unit.kind != carrier.SummaryUnit || surface.Factor != row.factor || surface.Semantic != row.semantic || surface.Normalizer != row.normalizer {
		return nil, 0, nil, [32]byte{}, false
	}
	if row.summaryForm == nil || row.summaryForm.schemaBindingSchema() != factorRow.schemaFactorSchema() {
		return nil, 0, nil, [32]byte{}, false
	}
	digest := summaryVectorDigest(unit.summaryKeys)
	if digest == ([32]byte{}) {
		return nil, 0, nil, [32]byte{}, false
	}
	return factorRow, row.summaryOrdinal, unit.summaryKeys, digest, true
}

// stagedUnit resolves only a Factor-issued exact Ref through the predeclared
// dynamic exact universe. The Ref validates the same sealed Factor before its
// raw coordinate is used; no key, Unit, or graph lookup is exposed to the
// locator.
func (bound *boundFactor[K, V]) stagedUnit(ref exactRef) (carrier.Unit, bool) {
	if bound == nil || bound.implementation == nil || !factorRowAvailable(bound.implementation.row) || len(bound.dynamicUnits) == 0 || ref == nil {
		return carrier.Unit{}, false
	}
	var raw uint64
	var ok bool
	if typed, valid := ref.(interface {
		factorRow() schemaFactorBinding
		rawAddress() uint64
	}); valid {
		address := typed.factorRow()
		raw, ok = typed.rawAddress(), address == bound.implementation.row
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
func (bound *boundFactor[K, V]) stagedTarget(ref exactRef) (carrier.Target, schemaFactorBinding, uint64, bool) {
	if bound == nil || bound.implementation == nil || !factorRowAvailable(bound.implementation.row) || len(bound.routeTargets) != len(bound.dynamicUnits) || len(bound.routeTargets) == 0 || ref == nil {
		return carrier.Target{}, nil, 0, false
	}
	var raw uint64
	var ok bool
	if typed, valid := ref.(interface {
		factorRow() schemaFactorBinding
		rawAddress() uint64
	}); valid {
		address := typed.factorRow()
		raw, ok = typed.rawAddress(), address == bound.implementation.row
	}
	if !ok || raw >= uint64(len(bound.routeTargets)) {
		return carrier.Target{}, nil, 0, false
	}
	target := bound.routeTargets[int(raw)]
	if target == (carrier.Target{}) || target.Mode() != carrier.StrongTarget {
		return carrier.Target{}, nil, 0, false
	}
	targetRaw, proven := exactWriteLocal(bound.implementation.row, exactWriteSurface(bound.implementation.row, raw+1))
	if !proven || targetRaw != raw {
		return carrier.Target{}, nil, 0, false
	}
	return target, bound.implementation.row, targetRaw, true
}

// routeGeometry is the Factor's paired dense route universe: the staged exact
// Unit each coordinate is read at, beside the presealed strong Target it is
// written to. A Factor owns one exactly when both universes exist and are the
// same universe, which is the pairing stagedTarget follows. A Factor with only
// one of the two owns no route at all - it is a Factor no routed rule writes
// to - and answers the empty geometry rather than half of one.
func (bound *boundFactor[K, V]) routeGeometry() execution.RouteTable {
	if bound == nil || len(bound.routeTargets) == 0 || len(bound.routeTargets) != len(bound.dynamicUnits) {
		return execution.RouteTable{}
	}
	table, ok := execution.NewRouteTable(bound.dynamicUnits, bound.routeTargets)
	if !ok {
		return execution.RouteTable{}
	}
	return table
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

// stagedRow, stagedDefault and stagedTop expose the read boundary's three
// authentication and substitution endpoints for this Factor: the sealed row a
// read is bound against, the declared default of an unwritten coordinate, and
// the greatest element a widened opaque read delivers. None of them reaches a
// domain Fold; the engine consumes them while materializing the read.
func (bound *boundFactor[K, V]) stagedRow() schemaFactorBinding {
	if bound == nil || bound.implementation == nil {
		return nil
	}
	return bound.implementation.row
}

func (bound *boundFactor[K, V]) stagedDefault() (V, bool) {
	var zero V
	if bound == nil || bound.implementation == nil || bound.implementation.algebra == nil {
		return zero, false
	}
	return bound.implementation.algebra.Default()
}

func (bound *boundFactor[K, V]) stagedTop() (V, bool) {
	var zero V
	if bound == nil || bound.implementation == nil || bound.implementation.algebra == nil {
		return zero, false
	}
	return bound.implementation.algebra.Top()
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
