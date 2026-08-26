package replay

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/eval/subtree"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// ResultVisitor receives one semantic Apply extent for one population row.
// Population order is the owner witness order. The visitor runs synchronously;
// returning false stops the stream without retaining the population or any
// child extent in this package.
type ResultVisitor func(CoordinateEvidence, apply.Results) bool

// Full redeems one correlated Apply replay. The population driver is streamed
// through Populate. Each declared child is then handed to the generic sealed
// subtree evaluator, which recursively consumes its exact Input, Select,
// Join, and Complete occurrences. The resulting batches are assembled in
// Apply declaration order and evaluated under the population row's scope.
//
// CorrelatedSubtree is the only child-shape authority here. Full does not
// classify children as direct, shared, scalar, or joined, and it never
// reconstructs a relation, directory, scope key, or RowID-to-key inverse.
func Full(
	plan arrangement.ApplyBinding,
	mounted witness.Mounted,
	root database.Version,
	view geometry.Geometry,
	scratch *store.ReadScratch,
	visit ResultVisitor,
) (completed, valid bool) {
	if visit == nil || !plan.Available() || !mounted.Available() || !root.Available() || !view.Available() || !view.ValidFor(mounted) || scratch == nil || !scratch.Available() {
		return false, false
	}
	replay, replayOK := plan.Replay()
	if !replayOK || !replay.Available() || !plan.Correlation().Available() || !root.Mounted().Same(mounted) || !root.Fence().Same(mounted.RuntimeFence()) {
		return false, false
	}
	driver, driverOK := replay.Driver()
	if !driverOK || !driver.Available() || !driver.ValidFor(mounted.Fence()) {
		return false, false
	}
	populationReader, readerOK := read.Bind(root, driver, view, scratch)
	if !readerOK || !populationReader.Available() {
		return false, false
	}
	session, sessionOK := subtree.New(mounted, root, view, scratch)
	if !sessionOK || !session.Available() {
		return false, false
	}

	deliveries := plan.Deliveries()
	slotSource := plan.SlotSource()
	childCount := replay.ChildCount()
	if len(deliveries) == 0 || len(deliveries) != len(slotSource) || childCount == 0 {
		return false, false
	}

	// A child can be evaluated once for this Full only when every one of its
	// sealed Input and Complete sources is independent of the population row.
	// This is deliberately derived from the extent vocabulary on every call;
	// no direct/shared/scalar mode or replay-level cache is introduced.  The
	// result itself is immutable and remains local to this invocation.
	populationIndependent := make([]bool, childCount)
	for childIndex := 0; childIndex < childCount; childIndex++ {
		child, childOK := replay.ChildAt(childIndex)
		if !childOK || !child.Available() {
			return false, false
		}
		independent, independentOK := childHasPopulationIndependentSources(child)
		if !independentOK {
			return false, false
		}
		populationIndependent[childIndex] = independent
	}
	cachedResults := make([]subtree.Result, childCount)
	cached := make([]bool, childCount)

	malformed := false
	stopped := false
	completed, valid = Populate(replay, mounted, populationReader, func(evidence CoordinateEvidence) bool {
		children := make([][]tuple.Batch, childCount)
		witnesses := make([]binding.DenominatorWitness, len(deliveries))
		population, populationOK := session.ForPopulation(replay, evidence.RowID(), evidence.Scope())
		if !populationOK || !population.Available() {
			malformed = true
			return false
		}
		for childIndex := 0; childIndex < childCount; childIndex++ {
			child, childOK := replay.ChildAt(childIndex)
			if !childOK || !child.Available() {
				malformed = true
				return false
			}
			result := cachedResults[childIndex]
			resultOK := cached[childIndex]
			if !resultOK || !populationIndependent[childIndex] {
				result, resultOK = population.Evaluate(child)
				if resultOK && populationIndependent[childIndex] {
					cachedResults[childIndex] = result
					cached[childIndex] = true
				}
			}
			if !resultOK || !result.Available() {
				malformed = true
				return false
			}
			batches := result.Batches()
			if batches == nil {
				malformed = true
				return false
			}
			children[childIndex] = batches
			for deliveryIndex, source := range slotSource {
				if int(source.Child()) != childIndex {
					continue
				}
				if deliveryIndex >= len(deliveries) {
					malformed = true
					return false
				}
				input := deliveries[deliveryIndex].Requirement().Input()
				carrier, carrierOK := carrierWitness(mounted, input, result, population.Denominator())
				if !carrierOK {
					malformed = true
					return false
				}
				witnesses[deliveryIndex] = carrier
			}
		}
		values, applyOK := apply.Execute(plan, mounted, children, view, evidence.Scope(), witnesses)
		if !applyOK || !values.Available() {
			malformed = true
			return false
		}
		if !visit(evidence, values) {
			stopped = true
			return false
		}
		return true
	})
	if malformed {
		return false, false
	}
	if stopped {
		return false, true
	}
	return completed, valid
}

// childHasPopulationIndependentSources checks the complete closed extent
// vocabulary.  A child is reusable only when no Input or Complete extent has
// either a PopulationDriver or a Partition source.  The check is intentionally
// structural and local; it does not classify a child by shape or retain a
// cache beyond one Full call.
func childHasPopulationIndependentSources(child arrangement.CorrelatedSubtree) (independent, valid bool) {
	if !child.Available() {
		return false, false
	}
	dependent := false
	for index := 0; index < child.InputCount(); index++ {
		extent, extentOK := child.InputAt(index)
		if !extentOK || !extent.Available() {
			return false, false
		}
		source := extent.Source()
		if !source.Available() {
			return false, false
		}
		_, _, driver := source.PopulationDriver()
		_, partition := source.Partition()
		if driver && partition {
			return false, false
		}
		dependent = dependent || driver || partition
	}
	for index := 0; index < child.CompleteCount(); index++ {
		extent, extentOK := child.CompleteAt(index)
		if !extentOK || !extent.Available() {
			return false, false
		}
		source := extent.Source()
		if !source.Available() {
			return false, false
		}
		_, _, driver := source.PopulationDriver()
		_, partition := source.Partition()
		if driver && partition {
			return false, false
		}
		dependent = dependent || driver || partition
	}
	return !dependent, true
}

// carrierWitness redeems the delivery range authority for one evaluated
// child. PopulationDriver scalar slots use the exact population witness
// authenticated by Population. Span slots use only the child's root
// Complete range proof and its retained denominator witness. A result with no
// authenticated root Complete vector, an empty unauthenticated vector, or
// inconsistent witnesses is malformed; no mounted-denominator fallback is
// permitted here.
func carrierWitness(
	mounted witness.Mounted,
	input signature.Input,
	evaluationResult subtree.Result,
	population binding.DenominatorWitness,
) (binding.DenominatorWitness, bool) {
	if !mounted.Available() || !input.Available() || !evaluationResult.Available() || !population.Available() || !population.ValidFor(mounted.RuntimeFence()) {
		return binding.DenominatorWitness{}, false
	}
	carrier := input.CarrierDenominator()
	if !carrier.Available() {
		return binding.DenominatorWitness{}, false
	}
	if input.Delivery.IsScalar() {
		// A scalar child is represented by the PopulationDriver extent. Its
		// carrier must be the same owner population that authenticated this
		// callback; a witness for another denominator would widen the scalar
		// source beyond the exact population row.
		if !population.Matches(carrier) {
			return binding.DenominatorWitness{}, false
		}
		return population, true
	}
	if !input.Delivery.IsSpan() {
		return binding.DenominatorWitness{}, false
	}
	root := evaluationResult.Root()
	complete, completeOK := root.Complete()
	rootRange, rangeOK := complete.Range()
	if !root.Available() || !completeOK || !complete.Available() || !rangeOK || !rootRange.Available() || rootRange.Kind() != algebra.KindComplete || rootRange.Producer() != root.Digest() || rootRange.Denominator() != carrier {
		return binding.DenominatorWitness{}, false
	}
	batches := evaluationResult.Batches()
	if batches == nil || len(batches) == 0 {
		return binding.DenominatorWitness{}, false
	}
	var result binding.DenominatorWitness
	for _, batch := range batches {
		if !batch.ValidFor(mounted) {
			return binding.DenominatorWitness{}, false
		}
		authority := batch.Range()
		if !authority.Available() || authority.Kind() != algebra.KindComplete || authority.Producer() != rootRange.Producer() || authority.Denominator() != carrier {
			return binding.DenominatorWitness{}, false
		}
		value, ok := batch.DenominatorWitness()
		if !ok || !value.Available() || !value.ValidFor(mounted.RuntimeFence()) || !value.Matches(carrier) {
			return binding.DenominatorWitness{}, false
		}
		if result.Available() {
			if !result.Same(value) {
				return binding.DenominatorWitness{}, false
			}
		} else {
			result = value
		}
	}
	return result, result.Available() && result.Matches(carrier)
}
