package flow

import (
	"errors"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binaryprimitive"
	flowbody "github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/continuation"
	"github.com/wippyai/go-lua/analysis/program/flow/directfunction"
	"github.com/wippyai/go-lua/analysis/program/flow/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/functionboundary"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/provenance"
	"github.com/wippyai/go-lua/analysis/program/flow/returnprojection"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// Assembly is an opaque one-shot transfer capability for one successfully
// sealed Source/Flow/Static set plus scalar authored ModuleID. Copies share
// the same terminal fence; no component is individually queryable through
// this token.
type Assembly struct{ state *assemblyState }

type assemblyState struct {
	mu       sync.Mutex
	terminal bool
	source   *source.Component
	flow     *Component
	static   *static.Component
	moduleID identity.ContentID
}

// Take atomically consumes the transfer capability and returns the complete
// Source/Flow/Static owners plus the scalar authored Module identity. A nil,
// malformed, or previously consumed
// token returns no component. Successful and failed terminal transitions both
// clear every retained pointer before releasing the fence.
func (assembly *Assembly) Take() (*source.Component, *Component, *static.Component, identity.ContentID, error) {
	if assembly == nil || assembly.state == nil {
		return nil, nil, nil, identity.ContentID{}, errors.New("program/flow: invalid Assembly token")
	}
	state := assembly.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal {
		return nil, nil, nil, identity.ContentID{}, errors.New("program/flow: Assembly token is consumed")
	}
	state.terminal = true
	sourceComponent, flowComponent := state.source, state.flow
	staticComponent, moduleID := state.static, state.moduleID
	state.source, state.flow, state.static, state.moduleID = nil, nil, nil, identity.ContentID{}
	if sourceComponent == nil || flowComponent == nil || staticComponent == nil || !moduleID.Available() {
		return nil, nil, nil, identity.ContentID{}, errors.New("program/flow: malformed Assembly token")
	}
	return sourceComponent, flowComponent, staticComponent, moduleID, nil
}

// Component is the sole published Flow semantic authority. Its field count is
// intentionally small: authored data plus only the immutable owner results
// consumed after assembly. Body/Binding proofs, source-control geometry,
// recurrence, static scope, and all owner finalizers are seal-local; the
// immutable Body and Containment results remain because Flow publishes their
// owner-issued structural queries directly.
type Component struct {
	provenance           provenance.Provenance
	authored             authored.View
	body                 *flowbody.Result
	containment          *containment.Result
	outcomes             *outcome.Result
	ports                *evaluation.Ports
	programStructure     programStructureProjection
	pending              *evaluation.Pending
	executable           *executable.Result
	directFunction       *directfunction.Result
	candidates           *candidates.Result
	accessGeometry       *accessgeometry.Result
	binaryPrimitives     *binaryprimitive.Result
	continuation         *continuation.Result
	subjectFlow          *subjectflow.Result
	semanticPaths        *semanticpath.Certificate
	callArgumentSources  *callArgumentSourceIndex
	callResultAdmissions []callResultAdmission
	callResultTailSlots  []callResultTailSlotAdmission
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
// Each vertical below either returns an immutable owner result directly or
// keeps a narrow capability fence where the root must withhold an owner join.
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
	return component.authored.ContentID()
}

func (view View) ContentID() identity.ContentID {
	if !view.available() {
		return identity.ContentID{}
	}
	return view.component.authored.ContentID()
}

// ModuleID returns the scalar authored-import identity carried by this Flow
// component. It is independent from the historical Flow ContentID.
func (view View) ModuleID() identity.ContentID {
	if !view.available() {
		return identity.ContentID{}
	}
	return view.component.authored.ModuleID()
}

func (view View) Provenance() provenance.Provenance {
	if !view.available() {
		return provenance.Provenance{}
	}
	return view.component.provenance
}

func (view View) available() bool {
	return view.component != nil && view.component.authored.ContentID().Available()
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
	fence := component.provenance
	if !fence.Source.Available() || !fence.Flow.Available() ||
		!fence.Static.Available() || !fence.Module.Available() ||
		component.authored.ContentID() != fence.Flow {
		return false
	}
	if !outcome.Matches(component.outcomes, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!evaluation.Matches(component.ports, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!functionboundary.Matches(component.programStructure.boundaries, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!returnprojection.Matches(component.programStructure.returns, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!evaluation.MatchesPending(component.pending, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!executable.Matches(component.executable, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!directfunction.Matches(component.directFunction, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!candidates.Matches(component.candidates, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!accessgeometry.Matches(component.accessGeometry, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!causal.Matches(component.programStructure.causal, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!binaryprimitive.Matches(component.binaryPrimitives, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!continuation.Matches(component.continuation, fence.Source, fence.Flow, fence.Static, fence.Module) ||
		!subjectflow.Matches(component.subjectFlow, fence.Source, fence.Flow, fence.Static, fence.Module) {
		return false
	}
	if !flowbody.Matches(component.body, fence.Source, fence.Flow) {
		return false
	}
	return containment.Matches(component.containment, fence.Source, fence.Flow, fence.Static, fence.Module)
}

func (view View) Authored() authored.View {
	if !view.available() {
		return authored.View{}
	}
	return view.component.authored
}

// Body returns Flow's immutable sealed Body result. It is the sole published
// owner of Entry, lexical Body parents, and ordered Body roots.
func (view View) Body() *flowbody.Result {
	if !view.available() {
		return nil
	}
	fence := view.component.provenance
	if !view.projectionFence() || !flowbody.Matches(view.component.body, fence.Source, fence.Flow) {
		return nil
	}
	return view.component.body
}

// Containment returns Flow's immutable canonical child-to-parent proof. It is
// the sole published owner of parent, static-membership, and dense-term rows.
func (view View) Containment() *containment.Result {
	if !view.available() {
		return nil
	}
	fence := view.component.provenance
	if !view.projectionFence() || !containment.Matches(view.component.containment, fence.Source, fence.Flow, fence.Static, fence.Module) {
		return nil
	}
	return view.component.containment
}

func (view View) Outcomes() *outcome.Result {
	if !view.available() {
		return nil
	}
	return view.component.outcomes
}

func (view View) Ports() *evaluation.Ports {
	if !view.available() {
		return nil
	}
	return view.component.ports
}

// FunctionBoundaries is the sole published Function/Body-boundary join. It
// carries only existing Source/Flow/Outcome terms, including the explicit
// assembly root, and is fenced to the complete Flow quartet.
func (view View) FunctionBoundaries() *functionboundary.Result {
	if !view.available() {
		return nil
	}
	fence := view.component.provenance
	if !view.projectionFence() || !functionboundary.Matches(view.component.programStructure.boundaries, fence.Source, fence.Flow, fence.Static, fence.Module) {
		return nil
	}
	return view.component.programStructure.boundaries
}

// ReturnProjection is Flow's sealed Body-to-Return relation. Consumers join
// its authored terms to the already sealed boundary and causal owners.
func (view View) ReturnProjection() *returnprojection.Result {
	if !view.available() {
		return nil
	}
	fence := view.component.provenance
	structure := view.component.programStructure
	if !view.projectionFence() || !returnprojection.Matches(structure.returns, fence.Source, fence.Flow, fence.Static, fence.Module) {
		return nil
	}
	return structure.returns
}

func (view View) Pending() *evaluation.Pending {
	if !view.available() {
		return nil
	}
	return view.component.pending
}

func (view View) Executable() *executable.Result {
	if !view.available() {
		return nil
	}
	return view.component.executable
}

func (view View) DirectFunctions() *directfunction.Result {
	if !view.available() {
		return nil
	}
	return view.component.directFunction
}

func (view View) Candidates() *candidates.Result {
	if !view.available() {
		return nil
	}
	return view.component.candidates
}

func (view View) AccessGeometry() *accessgeometry.Result {
	if !view.available() {
		return nil
	}
	fence := view.component.provenance
	if !view.projectionFence() ||
		!accessgeometry.Matches(view.component.accessGeometry, fence.Source, fence.Flow, fence.Static, fence.Module) {
		return nil
	}
	return view.component.accessGeometry
}

func (view View) Causal() *causal.Result {
	if !view.available() {
		return nil
	}
	return view.component.programStructure.causal
}

// LocalWTO is the complete parent-issued Program-local schedule for this exact
// committed Flow. It contains acyclic singleton leaves and balanced
// Enter/Point/Exit events; consumers must copy this certificate rather than
// reconstruct an order from final routes.
func (view View) LocalWTO() causal.LocalWTO {
	if !view.projectionAvailable() {
		return causal.LocalWTO{}
	}
	return view.component.programStructure.causal.LocalWTO()
}

func (view View) BinaryPrimitives() *binaryprimitive.Result {
	if !view.available() {
		return nil
	}
	fence := view.component.provenance
	if !view.projectionFence() ||
		!binaryprimitive.Matches(view.component.binaryPrimitives, fence.Source, fence.Flow, fence.Static, fence.Module) {
		return nil
	}
	return view.component.binaryPrimitives
}

func (view View) projectionFence() bool {
	if !view.available() {
		return false
	}
	fence := view.component.provenance
	return fence.Source.Available() && fence.Flow.Available() &&
		fence.Static.Available() && fence.Module.Available() &&
		view.component.authored.ContentID() == fence.Flow
}

func (view View) Continuation() *continuation.Result {
	if !view.available() {
		return nil
	}
	return view.component.continuation
}

// SubjectFlow returns Flow's neutral local Define/Use/Alias facts and exact
// Yield/re-entry route pairs. It is intentionally policy-free: Placement
// domains consume these rows and decide their own allocation semantics.
func (view View) SubjectFlow() *subjectflow.Result {
	if !view.available() {
		return nil
	}
	fence := view.component.provenance
	if !view.projectionFence() || !subjectflow.Matches(view.component.subjectFlow, fence.Source, fence.Flow, fence.Static, fence.Module) {
		return nil
	}
	return view.component.subjectFlow
}
