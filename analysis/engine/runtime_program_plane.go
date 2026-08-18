// runtime_program_plane.go declares the sealed factor plane: one graph
// generation's Factor universe, bound from the sealed schema binding and
// nothing else.
//
// The plane is the substrate every member, query and observation binds against.
// A construction binds one plane for the graph it was declared over; an
// accepted activation revision binds another for the graph the published
// relation names. Because the plane derives from sealed values alone, the
// second binding needs nothing the first one accumulated: no attachment ledger,
// no transaction, no cold Composition.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// programPlane is the bound Factor universe of one graph. Its fields are the
// four coordinates a bind reads - the graph-owned binding, the dense Factor
// vector, the canonical Factor index and the attached carrier - plus the point
// index an observation needs.
type programPlane struct {
	runtime *runtimeBinding
	factors []runtimeFactor
	byKey   map[composition.Key]runtimeFactor
	carrier *carrier.Composition
	ordered []runtimeFactor
	// observationPoints is this graph's member -> output point index. It is
	// derived on first use because only an attached observation reads it.
	observationPoints map[composition.Key]equation.Point
	// frozen records that the cold binding catalog was released while this plane
	// was minted. A plane that never reached that cut binds nothing.
	frozen bool
}

// bindProgramPlane is the sole mint of a plane. It enumerates the sealed
// binding's Factor cells by canonical ordinal, prepares the one carrier
// composition for them, and releases the cold catalog before returning, so the
// plane retains only concrete runtime handles.
func bindProgramPlane(state *schemaBindingState, graph *equation.Graph) (*programPlane, bool) {
	runtime, ok := newSealedRuntimeBinding(state, graph)
	if !ok || runtime == nil {
		return nil, false
	}
	factors, byKey, ok := bindProgramPlaneFactors(state, runtime)
	if !ok || !runtime.freezeCatalog() {
		return nil, false
	}
	prepared, ordered, ok := prepareRuntimeComposition(factors, runtime.guards)
	if !ok || prepared == nil {
		return nil, false
	}
	attached, ok := prepared.Attach()
	if !ok || attached == nil {
		return nil, false
	}
	for _, factor := range ordered {
		preparer, preparable := factor.(interface{ prepareRouteTransformClosure() bool })
		if !preparable || !preparer.prepareRouteTransformClosure() {
			return nil, false
		}
	}
	return &programPlane{runtime: runtime, factors: factors, byKey: byKey, carrier: attached, ordered: ordered, frozen: true}, true
}

// releaseColdFactorBindings drops the cold declaration state every bound Factor
// held for the duration of the bind. A plane whose Factors are still cold has
// not finished binding, so the release is part of producing a runtime from a
// plane rather than a later cleanup pass.
func (plane *programPlane) releaseColdFactorBindings() bool {
	if plane == nil {
		return false
	}
	for _, factor := range plane.ordered {
		if factor == nil {
			return false
		}
		factor.releaseColdBindings()
	}
	return true
}

// bindProgramPlaneFactors binds every sealed Factor cell of the schema against
// this graph. Enumerating by ordinal is what makes the result a property of the
// sealed binding rather than of the order attachments happened to arrive in.
func bindProgramPlaneFactors(state *schemaBindingState, runtime *runtimeBinding) ([]runtimeFactor, map[composition.Key]runtimeFactor, bool) {
	if state == nil || runtime == nil || runtime.mode != runtimeBindingReceipt || runtime.state != state || runtime.authority == nil || !runtime.valid() {
		return nil, nil, false
	}
	state.mu.Lock()
	if state.phase != schemaBindingSealed || state.authority != runtime.authority || state.schema != runtime.schema {
		state.mu.Unlock()
		return nil, nil, false
	}
	cells := append([]schemaFactorBinding(nil), state.factors...)
	schema := state.schema
	state.mu.Unlock()
	if len(cells) != schemaFactorCount(schema) {
		return nil, nil, false
	}
	factors := make([]runtimeFactor, len(cells))
	byKey := make(map[composition.Key]runtimeFactor, len(cells))
	for ordinal, cell := range cells {
		if cell == nil || cell.schemaFactorOrdinal() != uint64(ordinal) || cell.schemaFactorSchema() != schema || !cell.schemaFactorComplete() {
			return nil, nil, false
		}
		factor, bound := cell.schemaFactorRuntimeBinding(runtime)
		key := schema.factorSemanticAt(uint64(ordinal))
		if !bound || factor == nil || !key.Available() || compositionKeyOf(factor.semantic()) != key {
			return nil, nil, false
		}
		if _, duplicate := byKey[key]; duplicate {
			return nil, nil, false
		}
		factors[ordinal], byKey[key] = factor, factor
	}
	return factors, byKey, true
}
