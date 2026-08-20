package flow

import (
	"errors"
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/flow/provenance"

	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binaryprimitive"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/continuation"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/directfunction"
	"github.com/wippyai/go-lua/analysis/program/flow/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/functionboundary"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/position"
	"github.com/wippyai/go-lua/analysis/program/flow/returnprojection"
	"github.com/wippyai/go-lua/analysis/program/flow/runtimeentry"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/flow/staticcheck"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
)

// Assemble is the one and only Flow publication transaction. The Source,
// Module, and Flow owners use their construction capabilities; Static arrives
// as an immutable component and its build-time validation view remains local
// to this call. Assemble derives every cross-owner relation in the fixed DAG
// and publishes no Assembly until all four child components exist.
//
// The entry is the canonical top-level Body.  It is checked by every
// structural owner that observes it; no owner is allowed to infer a different
// root or to use an alternate entry.
func Assemble(
	sourceFinalizer source.Finalizer,
	staticComponent *static.Component,
	staticView staticquery.View,
	moduleFinalizer imports.Finalizer,
	draft *Draft,
	entry keyspace.Term,
) (*Assembly, error) {
	flowFinalizer, err := draft.claim()
	if err != nil {
		return nil, err
	}

	var (
		sourceTerminal bool
		moduleTerminal bool
		flowTerminal   bool
	)
	abort := func() {
		abortOwners(sourceFinalizer, moduleFinalizer, flowFinalizer,
			sourceTerminal, moduleTerminal, flowTerminal)
	}
	fail := func(stage string, cause error) (*Assembly, error) {
		abort()
		if cause == nil {
			cause = errors.New("unknown assembly failure")
		}
		return nil, fmt.Errorf("program/flow: %s: %w", stage, cause)
	}

	preimage := sourceFinalizer.Preimage()
	moduleView := moduleFinalizer.View()
	authoredLive := flowFinalizer.View()
	sourceID := preimage.Identity().ContentID()
	flowID := authoredLive.Cold().ContentID()
	staticID := staticView.ContentID()
	moduleID := moduleView.ContentID()
	if staticComponent == nil || staticComponent.ContentID() != staticID ||
		!sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return fail("owner preflight", errors.New("one or more claimed owner views are unavailable"))
	}
	if keyspace.TermFamily(entry) != keyspace.FamilyBody || keyspace.TermOrdinal(entry) == 0 {
		return fail("owner preflight", errors.New("entry is not a canonical Body term"))
	}

	// Pre-Source-commit lane. Position deliberately consumes
	// the Source Preimage while exact keys, bind order, and authored spans are
	// still live.  Nothing in this lane can observe a committed Source View.
	bodies, err := body.Seal(preimage, authoredLive, staticView, entry)
	if err != nil {
		return fail("Body", err)
	}
	bindings, err := binding.Seal(preimage, authoredLive, bodies, entry)
	if err != nil {
		return fail("Binding", err)
	}
	forest, scopeProof, err := containment.Prove(preimage, staticView, authoredLive, bodies, bindings, moduleView, entry)
	if err != nil {
		return fail("Containment", err)
	}
	shape, err := control.Seal(preimage, authoredLive, bodies, bindings, forest, staticID, moduleID)
	if err != nil {
		return fail("Control", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), authoredLive, bodies, shape, staticID, moduleID)
	if err != nil {
		return fail("Outcome", err)
	}
	// Ports are defined over the pre-Outcome Source denominator. Source commit
	// installs the derived Outcome family, so this proof must consume the live
	// preimage identity before that terminal transition.
	ports, err := evaluation.SealPorts(preimage.Identity(), authoredLive, forest, staticID, moduleID)
	if err != nil {
		return fail("Evaluation ports", err)
	}
	functionBoundaryResult, err := functionboundary.Seal(preimage, authoredLive, bodies, ports, outcomes, staticID, moduleID, entry)
	if err != nil {
		return fail("Function boundaries", err)
	}
	indexInput, err := position.Seal(preimage, authoredLive, bodies, forest, outcomes, entry, staticID, moduleID)
	if err != nil {
		return fail("Position", err)
	}

	// Source owns the only durable position/index representation.  Commit is
	// terminal even on malformed input, so mark the capability closed before
	// invoking it and never attempt a second terminal action in cleanup.
	sourceTerminal = true
	sourceComponent, err := sourceFinalizer.Commit(indexInput)
	if err != nil {
		return fail("Source commit", err)
	}
	if sourceComponent == nil {
		return fail("Source commit", errors.New("Source returned no Component"))
	}

	// Post-Source-commit lane.  These owners consume the committed Source View
	// and retain only their own scalar quartet.  All topology, recurrence, and
	// control proofs remain local to this call.
	sourceView := sourceComponent.View()
	cellRoles := sourceView.CellRoles()
	pathCertificate, err := semanticpath.Seal(cellRoles, sourceView, authoredLive, bodies, bindings, forest, outcomes, flowID, staticID, moduleID)
	if err != nil {
		return fail("Semantic path certificate", err)
	}
	controlGraph, err := sourcecontrol.Seal(sourceView, authoredLive, bodies, forest, shape, entry, staticID, moduleID)
	if err != nil {
		return fail("Source control", err)
	}
	vertexPaths, pathsOK := pathCertificate.VertexCatalog(sourceID, flowID, staticID, moduleID)
	vertexLease, vertexErr := controlGraph.InstallVertexCatalogLease(bodies, vertexPaths)
	if !pathsOK || vertexErr != nil || vertexLease == nil {
		return fail("Source control vertex catalog", errors.New("semantic vertex catalog paths unavailable"))
	}
	outcomePaths, outcomePathsOK := pathCertificate.OutcomePhases(sourceID, flowID, staticID, moduleID)
	outcomePhases, err := controlGraph.BuildOutcomePhases(sourceView, authoredLive, bodies, outcomes, outcomePaths)
	if !outcomePathsOK || err != nil || outcomePhases == nil {
		return fail("Source control Outcome phases", errors.New("Outcome phase issuance failed"))
	}
	executableResult, err := executable.Seal(sourceView, authoredLive, bodies, forest, controlGraph, staticID, moduleID, pathCertificate)
	if err != nil {
		return fail("Executable", err)
	}
	runtimeEntries, err := runtimeentry.Seal(sourceView, authoredLive, controlGraph, ports, executableResult, staticID, moduleID)
	if err != nil {
		return fail("Runtime entry", err)
	}
	returnProjection, err := returnprojection.Seal(sourceView, authoredLive, bodies, outcomes, executableResult, staticID, moduleID)
	if err != nil {
		return fail("Body returns", err)
	}
	causalPaths, pathsOK := pathCertificate.Causal(sourceID, flowID, staticID, moduleID)
	if !pathsOK {
		return fail("Causal structural paths", errors.New("structural path view unavailable"))
	}
	causalPreparation, err := causal.PrepareRoutePlanWithStructuralPaths(sourceView, authoredLive, bodies, forest, outcomes, controlGraph, ports, executableResult, runtimeEntries, causalPaths, outcomePhases, staticID, moduleID)
	if err != nil {
		return fail("Causal route plan", err)
	}
	causalResult, err := causalPreparation.Seal()
	if err != nil {
		return fail("Causal recurrence", err)
	}
	directFunctionResult, err := directfunction.Seal(sourceView, authoredLive, bodies, bindings, forest, controlGraph, executableResult, staticID, moduleID)
	if err != nil {
		return fail("DirectFunction", err)
	}
	// No published Flow projection may retain SourceControl's catalog/CSR or
	// opaque NodeRefs. Every Causal point/route row was copied while the
	// lease was live; DirectFunction is the last structural consumer.
	if !controlGraph.ReleaseVertexCatalog(vertexLease) {
		return fail("Source control vertex catalog release", errors.New("vertex catalog release authority was unavailable"))
	}
	candidateResult, err := candidates.Seal(sourceView.Identity(), authoredLive, executableResult, staticID, moduleID)
	if err != nil {
		return fail("Candidates", err)
	}
	accessGeometryResult, err := accessgeometry.Seal(sourceView, authoredLive, candidateResult, bodies, bindings, staticView, moduleView)
	if err != nil {
		return fail("AccessGeometry", err)
	}
	pendingResult, err := evaluation.SealPending(sourceView, authoredLive, executableResult, candidateResult, staticID, moduleID)
	if err != nil {
		return fail("Pending", err)
	}
	binaryPrimitivesResult, err := binaryprimitive.Seal(sourceView, authoredLive, candidateResult, causalResult, staticID, moduleID)
	if err != nil {
		return fail("BinaryPrimitives", err)
	}
	continuationResult, err := continuation.Seal(sourceView, authoredLive, bodies, bindings, executableResult, candidateResult, causalResult, staticID, moduleID)
	if err != nil {
		return fail("Continuation", err)
	}
	valueSourcePaths, err := sealCertificateValueSourcePaths(sourceView, authoredLive, pathCertificate, staticID, moduleID)
	if err != nil {
		return fail("Value source paths", err)
	}
	storagePaths, err := sealCertificateStoragePaths(sourceView, authoredLive, pathCertificate, staticID, moduleID)
	if err != nil {
		return fail("Storage paths", err)
	}
	allocationPaths, err := sealCertificateAllocationPaths(sourceView, executableResult, authoredLive, pathCertificate, staticID, moduleID)
	if err != nil {
		return fail("Allocation paths", err)
	}
	callPaths, err := sealCertificateCallPaths(sourceView, authoredLive, pathCertificate, staticID, moduleID)
	if err != nil {
		return fail("Call paths", err)
	}

	// Module entry is a private Flow assembly projection.  It is intentionally
	// kept in module_entry.go; Assemble only invokes it and commits its typed
	// owner input.  The helper must not return a second entry authority.
	moduleInput, err := sealModuleEntry(sourceView, authoredLive, moduleView, bodies, executableResult, directFunctionResult, staticID, entry)
	if err != nil {
		return fail("Module entry", err)
	}

	// Module entry is the last Module consumer, so Module commits immediately.
	// The successful component remains local until the complete quartet exists.
	moduleTerminal = true
	moduleComponent, err := moduleFinalizer.Commit(moduleInput)
	if err != nil {
		return fail("Module commit", err)
	}
	if moduleComponent == nil {
		return fail("Module commit", errors.New("module returned no Component"))
	}

	// Static validation consumes the full transient forest/scope proof and the
	// authored Flow view. Its scratch is discarded; the immutable Static
	// component remains local until Assembly is returned for root publication.
	err = staticcheck.Validate(sourceView, authoredLive, staticView, bodies, bindings, forest, scopeProof, accessGeometryResult, moduleID, entry)
	if err != nil {
		return fail("StaticCheck", err)
	}

	// Authored Flow commits last. Every consumer above observes the lifecycle-
	// bound authoredLive view, and no failure after this point can require that
	// view again. Its immutable View is the sole authored relation retained by
	// the final Flow Component.
	flowTerminal = true
	authoredView, err := flowFinalizer.Commit()
	if err != nil {
		return fail("Flow commit", err)
	}
	if !authoredView.Cold().ContentID().Available() {
		return fail("Flow commit", errors.New("flow returned no authored View"))
	}
	component := &Component{
		provenance:  provenance.Provenance{Source: sourceID, Flow: flowID, Static: staticID, Module: moduleID},
		authored:    authoredView,
		body:        bodies,
		containment: forest,
		outcomes:    outcomes,
		ports:       ports,
		programStructure: programStructureProjection{
			boundaries: functionBoundaryResult,
			causal:     causalResult,
			returns:    returnProjection,
		},
		pending:          pendingResult,
		executable:       executableResult,
		directFunction:   directFunctionResult,
		candidates:       candidateResult,
		accessGeometry:   accessGeometryResult,
		binaryPrimitives: binaryPrimitivesResult,
		continuation:     continuationResult,
		allocationPaths:  allocationPaths,
		semanticPaths:    pathCertificate,
		valueSourcePaths: valueSourcePaths,
		storagePaths:     storagePaths,
		callPaths:        callPaths,
	}
	return &Assembly{state: &assemblyState{
		source: sourceComponent,
		flow:   component,
		static: staticComponent,
		module: moduleComponent,
	}}, nil
}

func abortOwners(
	sourceFinalizer source.Finalizer,
	moduleFinalizer imports.Finalizer,
	flowFinalizer authored.Finalizer,
	sourceTerminal, moduleTerminal, flowTerminal bool,
) {
	if !moduleTerminal {
		_ = moduleFinalizer.Abort()
	}
	if !sourceTerminal {
		_ = sourceFinalizer.Abort()
	}
	if !flowTerminal {
		_ = flowFinalizer.Abort()
	}
}
