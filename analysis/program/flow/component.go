package flow

import (
	"errors"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binaryprimitive"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/directfunction"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/functionboundary"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/returnprojection"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// Assembly is an opaque one-shot transfer capability for one successfully
// sealed owner quartet. Copies share the same terminal fence; no component is
// individually queryable through this token.
type Assembly struct{ state *assemblyState }

type assemblyState struct {
	mu       sync.Mutex
	terminal bool
	source   *source.Component
	flow     *Component
	static   *static.Component
	module   *imports.Component
}

// Take atomically consumes the transfer capability and returns the complete
// Source/Flow/Static/Module quartet. A nil, malformed, or previously consumed
// token returns no component. Successful and failed terminal transitions both
// clear every retained pointer before releasing the fence.
func (assembly *Assembly) Take() (*source.Component, *Component, *static.Component, *imports.Component, error) {
	if assembly == nil || assembly.state == nil {
		return nil, nil, nil, nil, errors.New("program/flow: invalid Assembly token")
	}
	state := assembly.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal {
		return nil, nil, nil, nil, errors.New("program/flow: Assembly token is consumed")
	}
	state.terminal = true
	sourceComponent, flowComponent := state.source, state.flow
	staticComponent, moduleComponent := state.static, state.module
	state.source, state.flow, state.static, state.module = nil, nil, nil, nil
	if sourceComponent == nil || flowComponent == nil || staticComponent == nil || moduleComponent == nil {
		return nil, nil, nil, nil, errors.New("program/flow: malformed Assembly token")
	}
	return sourceComponent, flowComponent, staticComponent, moduleComponent, nil
}

// Component is the sole published Flow semantic authority.  Its field count
// is intentionally small: authored data plus only the projections that are
// consumed after assembly.  Body/Binding/Containment proofs, source-control
// geometry, recurrence, static scope, and all owner finalizers are seal-local
// and are absent from this struct.
type Component struct {
	provenance       Provenance
	authored         authored.View
	activation       activationProjection
	containment      containmentProjection
	outcomes         *outcome.Result
	ports            *evaluation.Ports
	programStructure programStructureProjection
	pending          *evaluation.Pending
	executable       *executable.Result
	directFunction   *directfunction.Result
	candidates       *candidates.Result
	accessGeometry   *accessgeometry.Result
	binaryPrimitives *binaryprimitive.Result
	continuation     *continuation.Result
	allocationPaths  [keyspace.FamilyCount][]allocationPath
	semanticPaths    *semanticpath.Certificate
	valueSourcePaths [keyspace.FamilyCount][]identity.ContentID
	storagePaths     [keyspace.FamilyCount][]identity.ContentID
	callPaths        []identity.ContentID
}

// Provenance is the explicit four-owner fence for the published Flow
// assembly. It is not a composite build ID and carries no owner pointer.
type Provenance struct {
	Source identity.ContentID
	Flow   identity.ContentID
	Static identity.ContentID
	Module identity.ContentID
}

type activationProjection struct {
	terms []keyspace.Term
}

type containmentProjection struct {
	terms   []keyspace.Term
	parents []keyspace.Term
	static  []bool
	index   [keyspace.FamilyCount][]uint32
}

// programStructureProjection is Flow's sole retained Program-local structural
// aggregate. Its boundary and causal relations are sibling owner-issued
// projections: neither is derived from, or reconstructed through, the other.
type programStructureProjection struct {
	boundaries *functionboundary.Result
	causal     *causal.Result
	returns    *returnprojection.Result
}

// View is the typed public query surface over one committed Flow component.
// Each vertical below is a narrow projection over the one Component; no
// internal Result type or child owner is exposed.
type View struct{ component *Component }

func (component *Component) View() View {
	if component == nil {
		return View{}
	}
	return View{component: component}
}

func (component *Component) ContentID() identity.ContentID {
	if component == nil {
		return identity.ContentID{}
	}
	return component.authored.Cold().ContentID()
}

func (view View) ContentID() identity.ContentID {
	if !view.available() {
		return identity.ContentID{}
	}
	return view.component.authored.Cold().ContentID()
}

func (view View) Provenance() Provenance {
	if !view.available() {
		return Provenance{}
	}
	return view.component.provenance
}

func (view View) available() bool {
	return view.component != nil && view.component.authored.Cold().ContentID().Available()
}

// projectionAvailable is the composition fence for Flow's post-assembly
// projections. The ordinary authored availability fence above remains
// intentionally weaker because artifact writers consume authored Flow without
// requiring every projection.
func (view View) projectionAvailable() bool {
	if !view.available() {
		return false
	}
	component := view.component
	provenance := component.provenance
	if !provenance.Source.Available() || !provenance.Flow.Available() ||
		!provenance.Static.Available() || !provenance.Module.Available() ||
		component.authored.Cold().ContentID() != provenance.Flow {
		return false
	}
	if !outcome.Matches(component.outcomes, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!evaluation.Matches(component.ports, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!functionboundary.Matches(component.programStructure.boundaries, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!returnprojection.Matches(component.programStructure.returns, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!evaluation.MatchesPending(component.pending, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!executable.Matches(component.executable, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!directfunction.Matches(component.directFunction, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!candidates.Matches(component.candidates, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!accessgeometry.Matches(component.accessGeometry, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!causal.Matches(component.programStructure.causal, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!binaryprimitive.Matches(component.binaryPrimitives, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!continuation.Matches(component.continuation, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) {
		return false
	}
	bodyCount := component.executable.FamilyCount(keyspace.FamilyBody)
	if bodyCount <= 0 || len(component.activation.terms) != bodyCount {
		return false
	}
	for _, activation := range component.activation.terms {
		if activation != 0 && (keyspace.TermFamily(activation) != keyspace.FamilyFunction || keyspace.TermOrdinal(activation) == 0) {
			return false
		}
	}
	return len(component.containment.terms) == len(component.containment.parents) &&
		len(component.containment.terms) == len(component.containment.static)
}

func (view View) Authored() Authored {
	if !view.available() {
		return Authored{}
	}
	return Authored{view: view.component.authored}
}

func (view View) Activation() Activation {
	if !view.available() {
		return Activation{}
	}
	return Activation{projection: &view.component.activation}
}

func (view View) Containment() Containment {
	if !view.available() {
		return Containment{}
	}
	return Containment{projection: &view.component.containment}
}

func (view View) Outcomes() Outcomes {
	if !view.available() {
		return Outcomes{}
	}
	return Outcomes{result: view.component.outcomes}
}

func (view View) Ports() Ports {
	if !view.available() {
		return Ports{}
	}
	return Ports{result: view.component.ports}
}

// FunctionBoundaries is the sole published Function/Body-boundary join. It
// carries only existing Source/Flow/Outcome terms, including the explicit
// assembly root, and is fenced to the complete Flow quartet.
func (view View) FunctionBoundaries() FunctionBoundaries {
	if !view.available() {
		return FunctionBoundaries{}
	}
	provenance := view.component.provenance
	if !view.projectionFence() || !functionboundary.Matches(view.component.programStructure.boundaries, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) {
		return FunctionBoundaries{}
	}
	return FunctionBoundaries{result: view.component.programStructure.boundaries}
}

// BodyReturns is the sole sealed projection from a Body boundary to its
// targetless OutcomeReturn and ordered executable Values alternatives.
func (view View) BodyReturns() BodyReturns {
	if !view.available() {
		return BodyReturns{}
	}
	provenance := view.component.provenance
	structure := view.component.programStructure
	if !view.projectionFence() || !returnprojection.Matches(structure.returns, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!functionboundary.Matches(structure.boundaries, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) ||
		!causal.Matches(structure.causal, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) {
		return BodyReturns{}
	}
	return BodyReturns{result: structure.returns, causal: structure.causal, boundaries: structure.boundaries}
}

func (view View) Pending() Pending {
	if !view.available() {
		return Pending{}
	}
	return Pending{result: view.component.pending}
}

func (view View) Executable() Executable {
	if !view.available() {
		return Executable{}
	}
	return Executable{result: view.component.executable}
}

func (view View) DirectFunctions() DirectFunctions {
	if !view.available() {
		return DirectFunctions{}
	}
	return DirectFunctions{result: view.component.directFunction}
}

func (view View) Candidates() Candidates {
	if !view.available() {
		return Candidates{}
	}
	return Candidates{result: view.component.candidates}
}

func (view View) AccessGeometry() AccessGeometry {
	if !view.available() {
		return AccessGeometry{}
	}
	provenance := view.component.provenance
	if !view.projectionFence() ||
		!accessgeometry.Matches(view.component.accessGeometry, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) {
		return AccessGeometry{}
	}
	return AccessGeometry{result: view.component.accessGeometry, available: true}
}

func (view View) Causal() Causal {
	if !view.available() {
		return Causal{}
	}
	return Causal{result: view.component.programStructure.causal}
}

func (view View) BinaryPrimitives() BinaryPrimitives {
	if !view.available() {
		return BinaryPrimitives{}
	}
	provenance := view.component.provenance
	if !view.projectionFence() ||
		!binaryprimitive.Matches(view.component.binaryPrimitives, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) {
		return BinaryPrimitives{}
	}
	return BinaryPrimitives{result: view.component.binaryPrimitives, available: true}
}

func (view View) projectionFence() bool {
	if !view.available() {
		return false
	}
	provenance := view.component.provenance
	return provenance.Source.Available() && provenance.Flow.Available() &&
		provenance.Static.Available() && provenance.Module.Available() &&
		view.component.authored.Cold().ContentID() == provenance.Flow
}

func (view View) Continuation() Continuation {
	if !view.available() {
		return Continuation{}
	}
	return Continuation{result: view.component.continuation}
}
