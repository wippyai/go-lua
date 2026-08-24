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
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// programPlane is the bound Factor universe of one graph. Its fields are the
// four coordinates a bind reads: the graph-owned binding, the dense Factor
// vector, the canonical Factor index and the attached carrier.
type programPlane struct {
	runtime *runtimeBinding
	factors []runtimeFactor
	byKey   map[composition.Key]runtimeFactor
	carrier *carrier.Composition
	ordered []runtimeFactor
	// Query context addressing is attached only by CommittedProgram.Seal. The
	// ordinary factor plane remains reusable, while each sealed query row is
	// resolved against the exact committed directory/index/layout rather than a
	// process-wide or point-owner default.
	contexts      executioncontext.Directory
	contextIndex  contextfiber.Index
	contextLayout contextfiber.Layout
	pointOwners   []contextfiber.PointOwner
	contextAuth   bool
	// frozen records that the cold binding catalog was released while this plane
	// was minted. A plane that never reached that cut binds nothing.
	frozen bool
}

func (plane *programPlane) attachQueryContext(committed *CommittedProgram) bool {
	if plane == nil || committed == nil || !committed.valid() || plane.runtime == nil || plane.runtime.graph != committed.graph {
		return false
	}
	if committed.artifactBacked && !validQueryContextPlane(committed.graph, committed.contexts, committed.contextIndex, committed.contextLayout, committed.pointOwners) {
		return false
	}
	plane.contexts = committed.contexts
	plane.contextIndex = committed.contextIndex
	plane.contextLayout = committed.contextLayout
	plane.pointOwners = append([]contextfiber.PointOwner(nil), committed.pointOwners...)
	plane.contextAuth = committed.artifactBacked
	return true
}

// queryState resolves a mounted query through the complete context address
// plane. Explicit engine-only construction has a separate graph-point state
// address and does not mint a context alias for mounted execution.
func (plane *programPlane) queryState(query equation.Query) (contextfiber.StateOrdinal, bool) {
	if plane == nil || plane.runtime == nil || plane.runtime.graph == nil {
		return 0, false
	}
	if !plane.contextAuth {
		point, ok := plane.runtime.graph.PointIndex(query.Point())
		if !ok || point < 0 {
			return 0, false
		}
		// Explicit engine-only construction has no context plane by design. Its
		// state address is the graph point itself; this branch is never used for
		// a mounted artifact, which must resolve through the retained Layout.
		return contextfiber.StateOrdinal(point), true
	}
	if !plane.contexts.Available() || !plane.contextIndex.Available() || !plane.contextLayout.Available() || len(plane.pointOwners) != plane.runtime.graph.PointCount() {
		return 0, false
	}
	state, ok := queryStateOrdinalOwned(plane.runtime.graph, query, plane.contextIndex, plane.contextLayout)
	return state, ok
}

// observationState resolves the exact compact state cell for a mounted
// observation's execution Context and member-output Point. The observation
// admission carries the typed Context explicitly; this helper never selects a
// directory row by module or position.
func (plane *programPlane) observationState(point equation.Point, context executioncontext.Context) (contextfiber.StateOrdinal, bool) {
	if plane == nil || plane.runtime == nil || plane.runtime.graph == nil {
		return 0, false
	}
	if !plane.contextAuth {
		pointIndex, pointOK := plane.runtime.graph.PointIndex(point)
		if !pointOK || pointIndex < 0 {
			return 0, false
		}
		return contextfiber.StateOrdinal(pointIndex), true
	}
	if !plane.contexts.Available() || !plane.contextIndex.Available() || !plane.contextLayout.Available() ||
		len(plane.pointOwners) != plane.runtime.graph.PointCount() || !context.Available() ||
		context.LinkID() != plane.contexts.LinkID() {
		return 0, false
	}
	canonical, canonicalOK := plane.contexts.Context(context.ID())
	if !canonicalOK || !canonical.Available() || canonical.ID() != context.ID() || canonical.LinkID() != context.LinkID() || canonical.ModuleKey() != context.ModuleKey() {
		return 0, false
	}
	contextOrdinal, contextOK := plane.contextIndex.ContextOrdinal(context.ID())
	pointIndex, pointOK := plane.runtime.graph.PointIndex(point)
	if !contextOK || !pointOK || pointIndex < 0 {
		return 0, false
	}
	return plane.contextLayout.Lookup(contextOrdinal, contextfiber.PointOrdinal(pointIndex))
}

// bindProgramPlane is the sole mint of a plane. It enumerates the sealed
// binding's Factor cells by canonical ordinal, prepares the one carrier
// composition for them, and releases the cold catalog before returning, so the
// plane retains only concrete runtime handles.
func bindProgramPlane(state *schemaBindingState, graph *equation.Graph) (*programPlane, ProgramSealStage, bool) {
	runtime, ok := newSealedRuntimeBinding(state, graph)
	if !ok || runtime == nil {
		return nil, ProgramSealStageAdmission, false
	}
	// Everything past the sealed binding is the bound Factor table, and it says
	// so. A Factor that will not bind - because its algebra, its graph catalog,
	// or the family typed against it does not resolve - is not an admission
	// fault, and reporting one sends every reader to the program's inputs.
	factors, byKey, ok := bindProgramPlaneFactors(state, runtime)
	if !ok || !runtime.freezeCatalog() {
		return nil, ProgramSealStageFactorBind, false
	}
	prepared, ordered, ok := prepareRuntimeComposition(factors, runtime.guards)
	if !ok || prepared == nil {
		return nil, ProgramSealStageFactorBind, false
	}
	attached, ok := prepared.Attach()
	if !ok || attached == nil {
		return nil, ProgramSealStageFactorBind, false
	}
	for _, factor := range ordered {
		preparer, preparable := factor.(interface{ prepareRouteTransformClosure() bool })
		if !preparable || !preparer.prepareRouteTransformClosure() {
			return nil, ProgramSealStageFactorBind, false
		}
	}
	return &programPlane{runtime: runtime, factors: factors, byKey: byKey, carrier: attached, ordered: ordered, frozen: true}, ProgramSealStageNone, true
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
	if state == nil || runtime == nil || runtime.state != state || runtime.authority == nil || !runtime.valid() {
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
