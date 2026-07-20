package factapply

import (
	"context"
	"fmt"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ExternalCallResultBinding is one sealed call-result index to carrier root.
// K is address syntax only; the factor law is otherwise carrier-neutral.
type ExternalCallResultBinding[K comparable] struct {
	Index int
	Root  K
}

// ExternalCallFactorProgram is the carrier-neutral outer transaction for an
// external call. It owns the opaque-call boundary, exact result-slot
// mutations, identity-keyed outcome effects, diagnostics and reachability.
// Path-relative effects are composed into this transaction by the prepared
// post-return programs; they are never reinterpreted through State.
//
// The program contains no State, inventory callback, or provider callback.
// Its factor inventory and result roots are sealed once at the call site.
type ExternalCallFactorProgram[K comparable] struct {
	domain        state.ProductDomain
	point         cfg.Point
	results       []ExternalCallResultBinding[K]
	resultOrdinal map[int]int
	factors       []state.ProductLane
	factorOrdinal map[state.LaneID]int
	boundary      []int
}

// ExternalCallResultMutation is one exact finite Values point update. The
// surrounding carrier applies these mutations to their named roots and
// structurally carries every unmentioned Values slot.
type ExternalCallResultMutation[K comparable] struct {
	Root  K
	Value product.Value
}

// ExternalCallFactorFrame is the exact outer carrier. Factors must match
// program order exactly. ResultMutations contains only the program's sealed
// result roots; it never materializes or clones the complete Values map.
// Diagnostics is kept separate from product factors because it is a sibling
// fixpoint component.
type ExternalCallFactorFrame[K comparable] struct {
	Factors         []state.LaneFactor
	ResultMutations []ExternalCallResultMutation[K]
	Diagnostics     callpayload.DiagnosticOutput
}

// PrepareExternalCallFactorProgram seals the observable result inventory and
// every registered opaque-call participant. bind must be injective across
// distinct result indexes; aliases are rejected rather than made
// order-dependent.
func PrepareExternalCallFactorProgram[K comparable](
	domain state.ProductDomain,
	access state.TransferAccess,
	point cfg.Point,
	resultIndices []int,
	bind func(point uint32, result uint32) (K, bool),
) (ExternalCallFactorProgram[K], error) {
	if !domain.Valid() || !access.Valid() || bind == nil || access.LaneCarry() < 0 {
		return ExternalCallFactorProgram[K]{}, fmt.Errorf("factapply: invalid external-call factor program")
	}
	program := ExternalCallFactorProgram[K]{
		domain:        domain,
		point:         point,
		resultOrdinal: make(map[int]int, len(resultIndices)),
		factorOrdinal: make(map[state.LaneID]int),
	}
	reads := access.LaneCarryReads()
	writes := access.LaneWrites()
	for _, lane := range domain.LaneInventory() {
		if !writes.Has(lane.ID()) {
			continue
		}
		if !reads.Has(lane.ID()) {
			return ExternalCallFactorProgram[K]{}, fmt.Errorf("factapply: external-call write lane %q is not readable from lane carry", lane.ID())
		}
		program.factorOrdinal[lane.ID()] = len(program.factors)
		program.factors = append(program.factors, lane)
	}
	for _, lane := range domain.CallBoundaryFactorLanes() {
		ordinal := -1
		for index, candidate := range program.factors {
			if candidate == lane {
				ordinal = index
				break
			}
		}
		if ordinal < 0 {
			return ExternalCallFactorProgram[K]{}, fmt.Errorf("factapply: external-call boundary lane %q is outside write ownership", lane.ID())
		}
		program.boundary = append(program.boundary, ordinal)
	}
	seenRoots := make(map[K]int, len(resultIndices))
	for _, index := range resultIndices {
		if index < 0 {
			return ExternalCallFactorProgram[K]{}, fmt.Errorf("factapply: invalid external-call result index %d", index)
		}
		if _, duplicate := program.resultOrdinal[index]; duplicate {
			continue
		}
		root, ok := bind(uint32(point), uint32(index))
		if !ok {
			return ExternalCallFactorProgram[K]{}, fmt.Errorf("factapply: unresolved external-call result root %d", index)
		}
		if prior, duplicate := seenRoots[root]; duplicate && prior != index {
			return ExternalCallFactorProgram[K]{}, fmt.Errorf("factapply: external-call result indexes %d and %d share one Values root", prior, index)
		}
		seenRoots[root] = index
		program.resultOrdinal[index] = len(program.results)
		program.results = append(program.results, ExternalCallResultBinding[K]{Index: index, Root: root})
	}
	return program, nil
}

func (p ExternalCallFactorProgram[K]) ResultBindings() []ExternalCallResultBinding[K] {
	return append([]ExternalCallResultBinding[K](nil), p.results...)
}

func (p ExternalCallFactorProgram[K]) ResultCount() int { return len(p.results) }

func (p ExternalCallFactorProgram[K]) Lanes() []state.ProductLane {
	return append([]state.ProductLane(nil), p.factors...)
}

func (p ExternalCallFactorProgram[K]) LaneCount() int { return len(p.factors) }

// Apply executes the prefix transaction atomically. The resolved CallOutcome
// is scratch syntax, not a retained formal carrier. Empty outcomes materialize
// owned results as Top; any resolved outcome owns the result tuple and clears
// omitted indexes to Bottom, matching the concrete call-producer law.
func (p ExternalCallFactorProgram[K]) Apply(
	ctx context.Context,
	token *cancellation.Token,
	input ExternalCallFactorFrame[K],
	outcome callpayload.CallOutcome,
) (ExternalCallFactorFrame[K], error) {
	if ctx == nil || !p.domain.Valid() || len(input.Factors) != len(p.factors) {
		return input, fmt.Errorf("factapply: invalid external-call factor execution")
	}
	for index, factor := range input.Factors {
		if factor.Lane() != p.factors[index] {
			return input, fmt.Errorf("factapply: reordered external-call factor %d", index)
		}
	}
	if err := externalCallFactorCanceled(ctx, token); err != nil {
		return input, err
	}
	reg := p.domain.Registry()
	diagnostics := callpayload.DiagnosticOutputFromCallOutcome(reg, outcome)
	if !diagnostics.Valid(reg) {
		return input, fmt.Errorf("factapply: invalid external-call diagnostic payload")
	}
	for _, result := range outcome.Results {
		if _, owned := p.resultOrdinal[result.Index]; owned && !product.BelongsToRegistry(reg, result.Value) {
			return input, fmt.Errorf("factapply: foreign external-call result %d", result.Index)
		}
	}

	out := ExternalCallFactorFrame[K]{
		Factors:     append([]state.LaneFactor(nil), input.Factors...),
		Diagnostics: diagnostics,
	}
	var err error
	for _, index := range p.boundary {
		out.Factors[index], err = p.domain.ApplyCallBoundaryFactor(out.Factors[index])
		if err != nil {
			return input, err
		}
	}
	if !outcome.Empty() {
		if err = p.applyIdentityOutcomeFactors(&out, outcome); err != nil {
			return input, err
		}
	}
	if len(p.results) != 0 {
		initial := product.Bottom(reg)
		if outcome.Empty() {
			initial = product.Top()
		}
		out.ResultMutations = make([]ExternalCallResultMutation[K], len(p.results))
		for index, binding := range p.results {
			out.ResultMutations[index] = ExternalCallResultMutation[K]{Root: binding.Root, Value: initial}
		}
		for _, result := range outcome.Results {
			ordinal, owned := p.resultOrdinal[result.Index]
			if !owned {
				continue
			}
			out.ResultMutations[ordinal].Value = result.Value
		}
	}
	if err := externalCallFactorCanceled(ctx, token); err != nil {
		return input, err
	}
	return out, nil
}

// applyIdentityOutcomeFactors owns descriptor families whose post-return law
// is independent of caller paths. Path-relative facts are applied by the
// prepared post-return factor program in the same outer transaction.
func (p ExternalCallFactorProgram[K]) applyIdentityOutcomeFactors(
	out *ExternalCallFactorFrame[K],
	outcome callpayload.CallOutcome,
) error {
	applyAt := func(id state.LaneID, apply func(state.LaneFactor) (state.LaneFactor, error)) error {
		ordinal, owned := p.factorOrdinal[id]
		if !owned || ordinal < 0 || ordinal >= len(out.Factors) {
			return fmt.Errorf("factapply: external-call outcome requires unowned lane %q", id)
		}
		next, err := apply(out.Factors[ordinal])
		if err != nil {
			return err
		}
		out.Factors[ordinal] = next
		return nil
	}
	if len(outcome.HeapTableObjects) != 0 {
		if err := applyAt(state.LaneHeapTableIdentity, func(factor state.LaneFactor) (state.LaneFactor, error) {
			var err error
			for id, object := range outcome.HeapTableObjects {
				factor, err = p.domain.ApplyCallOutcomeHeapObjectFactor(factor, id, object, identity.IsReturnedAllocation(id))
				if err != nil {
					return state.LaneFactor{}, err
				}
			}
			return factor, nil
		}); err != nil {
			return err
		}
	}
	if len(outcome.Placements) != 0 {
		if err := applyAt(state.LanePlacement, func(factor state.LaneFactor) (state.LaneFactor, error) {
			var err error
			for id, value := range outcome.Placements {
				factor, err = p.domain.ApplyCallOutcomePlacementFactor(factor, id, value)
				if err != nil {
					return state.LaneFactor{}, err
				}
			}
			return factor, nil
		}); err != nil {
			return err
		}
	}
	if !outcome.ProtectedCallTypestate.Empty() {
		protected := outcome.ProtectedCallTypestate
		if err := applyAt(state.LaneTypestates, func(factor state.LaneFactor) (state.LaneFactor, error) {
			return p.domain.ApplyProtectedCallTypestateFactor(
				factor, protected.Normal, protected.HasNormal,
				protected.Exceptional, protected.HasExceptional,
			)
		}); err != nil {
			return err
		}
	}
	return nil
}

// prepareConcreteExternalCallFactorProgram is the concrete address adapter.
// Call-site enumeration happens only at preparation; Apply sees the sealed
// dense result inventory.
func prepareConcreteExternalCallFactorProgram(
	domain state.ProductDomain,
	point cfg.Point,
	site factflow.CallSiteView,
) (ExternalCallFactorProgram[statekey.Value], error) {
	indices := make([]int, 0, site.ResultTargetCount())
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if target.ResultIndex() >= 0 {
			indices = append(indices, target.ResultIndex())
		}
		return true
	})
	boundary := domain.CallBoundaryFactorLanes()
	boundaryIDs := make([]state.LaneID, len(boundary))
	for index, lane := range boundary {
		boundaryIDs[index] = lane.ID()
	}
	access, err := state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs: []state.TransferInputAccess{{}},
		LaneCarryReads: state.NewLaneSet(boundaryIDs...), LaneWrites: state.NewLaneSet(boundaryIDs...),
		ValueCarry: 0, LaneCarry: 0,
		DiagnosticCarry: 0, ReachableCarry: 0,
	})
	if err != nil {
		return ExternalCallFactorProgram[statekey.Value]{}, err
	}
	return PrepareExternalCallFactorProgram(domain, access, point, indices, func(point, result uint32) (statekey.Value, bool) {
		return statekey.CallResult(point, result), true
	})
}

// applyConcreteExternalCallFactorPrefix transposes concrete State into the
// same factor program used by a formal carrier, then patches it only after the
// whole transaction succeeds. DiagnosticOutput is returned separately
// because it is a sibling fixpoint component, never a hidden State lane.
func applyConcreteExternalCallFactorPrefix(
	ctx context.Context,
	token *cancellation.Token,
	domain state.ProductDomain,
	point cfg.Point,
	site factflow.CallSiteView,
	input state.State,
	outcome callpayload.CallOutcome,
) (state.State, callpayload.DiagnosticOutput, error) {
	if domain.Lattice().Equal(input, domain.Lattice().Bottom()) {
		// Concrete reachability lives in State; formal execution makes the same
		// choice through Care before this factor transaction is invoked.
		return input, callpayload.DiagnosticOutputFromCallOutcome(domain.Registry(), outcome), nil
	}
	program, err := prepareConcreteExternalCallFactorProgram(domain, point, site)
	if err != nil {
		return input, callpayload.DiagnosticOutput{}, err
	}
	factors, err := domain.DecomposeLanes(input, program.Lanes())
	if err != nil {
		return input, callpayload.DiagnosticOutput{}, err
	}
	output, err := program.Apply(ctx, token, ExternalCallFactorFrame[statekey.Value]{
		Factors: factors,
	}, outcome)
	if err != nil {
		return input, callpayload.DiagnosticOutput{}, err
	}
	out, err := domain.PatchLaneFactors(input, output.Factors)
	if err != nil {
		return input, callpayload.DiagnosticOutput{}, err
	}
	edit := out.EditValues(domain.Registry())
	for _, mutation := range output.ResultMutations {
		edit.Write(mutation.Root, mutation.Value)
	}
	return state.Reachable(edit.Done()), output.Diagnostics, nil
}

func externalCallFactorCanceled(ctx context.Context, token *cancellation.Token) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if token != nil && token.Canceled() {
		return context.Canceled
	}
	return nil
}
