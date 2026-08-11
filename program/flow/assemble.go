package flow

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/flow/internal/accessgeometry"
	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binaryprimitive"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/program/flow/internal/causal"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/continuation"
	"github.com/wippyai/go-lua/program/flow/internal/control"
	"github.com/wippyai/go-lua/program/flow/internal/directbinding"
	"github.com/wippyai/go-lua/program/flow/internal/directfunction"
	"github.com/wippyai/go-lua/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/program/flow/internal/executable"
	"github.com/wippyai/go-lua/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/program/flow/internal/position"
	"github.com/wippyai/go-lua/program/flow/internal/recurrence"
	"github.com/wippyai/go-lua/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/program/flow/internal/staticcheck"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

// Assemble is the one and only Flow publication transaction.  The owner
// finalizers must already be claimed by the root.  Assemble derives every
// cross-owner relation in the fixed DAG, consumes each finalizer exactly once,
// and publishes no Assembly until all four child components exist.
//
// The entry is the canonical top-level Body.  It is checked by every
// structural owner that observes it; no owner is allowed to infer a different
// root or to use an alternate entry.
func Assemble(
	sourceFinalizer source.Finalizer,
	staticFinalizer static.Finalizer,
	moduleFinalizer module.Finalizer,
	draft *Draft,
	entry keyspace.Term,
) (*Assembly, error) {
	flowFinalizer, err := draft.claim()
	if err != nil {
		return nil, err
	}

	var (
		sourceTerminal bool
		staticTerminal bool
		moduleTerminal bool
		flowTerminal   bool
	)
	abort := func() {
		abortOwners(sourceFinalizer, staticFinalizer, moduleFinalizer, flowFinalizer,
			sourceTerminal, staticTerminal, moduleTerminal, flowTerminal)
	}
	fail := func(stage string, cause error) (*Assembly, error) {
		abort()
		if cause == nil {
			cause = errors.New("unknown assembly failure")
		}
		return nil, fmt.Errorf("program/flow: %s: %w", stage, cause)
	}

	preimage := sourceFinalizer.Preimage()
	staticView := staticFinalizer.View()
	moduleView := moduleFinalizer.View()
	authoredLive := flowFinalizer.View()
	sourceID := preimage.Identity().ContentID()
	flowID := authoredLive.Cold().ContentID()
	staticID := staticView.ContentID()
	moduleID := moduleView.ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return fail("owner preflight", errors.New("one or more claimed owner views are unavailable"))
	}
	if keyspace.TermFamily(entry) != keyspace.FamilyBody || keyspace.TermOrdinal(entry) == 0 {
		return fail("owner preflight", errors.New("entry is not a canonical Body term"))
	}

	// Pre-Source-commit lane.  DirectBinding and Position deliberately consume
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
	direct, err := directbinding.Seal(preimage, authoredLive, bodies, bindings, staticView, moduleView)
	if err != nil {
		return fail("DirectBinding", err)
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
	controlGraph, err := sourcecontrol.Seal(sourceView, authoredLive, bodies, forest, shape, entry, staticID, moduleID)
	if err != nil {
		return fail("Source control", err)
	}
	recurrenceResult, err := recurrence.Seal(sourceView, authoredLive, bodies, forest, controlGraph, staticID, moduleID)
	if err != nil {
		return fail("Recurrence", err)
	}
	executableResult, err := executable.Seal(sourceView, authoredLive, forest, controlGraph, staticID, moduleID)
	if err != nil {
		return fail("Executable", err)
	}
	directFunctionResult, err := directfunction.Seal(sourceView, authoredLive, bodies, bindings, forest, controlGraph, executableResult, staticID, moduleID)
	if err != nil {
		return fail("DirectFunction", err)
	}
	candidateResult, err := candidates.Seal(sourceView.Identity(), authoredLive, executableResult, staticID, moduleID)
	if err != nil {
		return fail("Candidates", err)
	}
	accessGeometryResult, err := accessgeometry.Seal(sourceView, authoredLive, candidateResult, staticID, moduleID)
	if err != nil {
		return fail("AccessGeometry", err)
	}
	pendingResult, err := evaluation.SealPending(sourceView, authoredLive, executableResult, candidateResult, staticID, moduleID)
	if err != nil {
		return fail("Pending", err)
	}
	causalResult, err := causal.Seal(sourceView, authoredLive, bodies, forest, outcomes, controlGraph, recurrenceResult, ports, executableResult, staticID, moduleID)
	if err != nil {
		return fail("Causal", err)
	}
	binaryPrimitivesResult, err := binaryprimitive.Seal(sourceView, authoredLive, candidateResult, causalResult, staticID, moduleID)
	if err != nil {
		return fail("BinaryPrimitives", err)
	}
	continuationResult, err := continuation.Seal(sourceView, authoredLive, bodies, bindings, executableResult, candidateResult, causalResult, staticID, moduleID)
	if err != nil {
		return fail("Continuation", err)
	}

	// Reduce the only two structural projections retained by Flow before any
	// remaining owner becomes terminal. A reduction defect therefore aborts
	// every still-live owner and can never strand a partially publishable
	// quartet.
	activation, err := reduceActivation(bodies)
	if err != nil {
		return fail("activation reduction", err)
	}
	reducedContainment, err := reduceContainment(forest)
	if err != nil {
		return fail("containment reduction", err)
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
		return fail("Module commit", errors.New("Module returned no Component"))
	}

	// Static validation consumes the full transient forest/scope proof and
	// the still-live authored Flow view, then returns only Static-owned receipt
	// terms. The receipt itself is not retained by Flow or Assembly.
	receipt, err := staticcheck.Validate(sourceView, authoredLive, staticView, bodies, bindings, forest, scopeProof, direct, moduleID, entry)
	if err != nil {
		return fail("StaticCheck", err)
	}

	staticTerminal = true
	staticComponent, err := staticFinalizer.Commit(receipt)
	if err != nil {
		return fail("Static commit", err)
	}
	if staticComponent == nil {
		return fail("Static commit", errors.New("Static returned no Component"))
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
		return fail("Flow commit", errors.New("Flow returned no authored View"))
	}
	component := &Component{
		provenance:       Provenance{Source: sourceID, Flow: flowID, Static: staticID, Module: moduleID},
		authored:         authoredView,
		activation:       activation,
		containment:      reducedContainment,
		outcomes:         outcomes,
		ports:            ports,
		pending:          pendingResult,
		executable:       executableResult,
		directFunction:   directFunctionResult,
		candidates:       candidateResult,
		accessGeometry:   accessGeometryResult,
		directBinding:    direct,
		causal:           causalResult,
		binaryPrimitives: binaryPrimitivesResult,
		continuation:     continuationResult,
	}
	fragment, err := sealSemanticSourceFragment(component.View())
	if err != nil {
		return fail("Flow semantic-source fragment", err)
	}
	component.semantic = fragment
	return &Assembly{state: &assemblyState{
		source: sourceComponent,
		flow:   component,
		static: staticComponent,
		module: moduleComponent,
	}}, nil
}

func abortOwners(
	sourceFinalizer source.Finalizer,
	staticFinalizer static.Finalizer,
	moduleFinalizer module.Finalizer,
	flowFinalizer authored.Finalizer,
	sourceTerminal, staticTerminal, moduleTerminal, flowTerminal bool,
) {
	if !moduleTerminal {
		_ = moduleFinalizer.Abort()
	}
	if !staticTerminal {
		_ = staticFinalizer.Abort()
	}
	if !sourceTerminal {
		_ = sourceFinalizer.Abort()
	}
	if !flowTerminal {
		_ = flowFinalizer.Abort()
	}
}

func reduceActivation(bodies *body.Result) (activationProjection, error) {
	if bodies == nil || bodies.BodyCount() == 0 {
		return activationProjection{}, errors.New("Body activation proof is unavailable")
	}
	terms := make([]keyspace.Term, bodies.BodyCount())
	for index := range terms {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
		activation, ok := bodies.Activation(body)
		if !ok {
			return activationProjection{}, errors.New("Body activation proof is incomplete")
		}
		terms[index] = activation
	}
	return activationProjection{terms: terms}, nil
}

func reduceContainment(forest *containment.Result) (containmentProjection, error) {
	if forest == nil || forest.Count() == 0 {
		return containmentProjection{}, errors.New("containment proof is unavailable")
	}
	terms := make([]keyspace.Term, 0, forest.Count())
	parents := make([]keyspace.Term, 0, forest.Count())
	staticMarks := make([]bool, 0, forest.Count())
	var maximum [keyspace.FamilyCount]uint32
	for index := 0; index < forest.Count(); index++ {
		term, ok := forest.At(index)
		if !ok {
			return containmentProjection{}, errors.New("containment proof has an unavailable row")
		}
		if keyspace.TermFamily(term) == keyspace.FamilyBody {
			continue
		}
		parent, hasParent := forest.Parent(term)
		if !hasParent {
			parent = 0
		}
		terms = append(terms, term)
		parents = append(parents, parent)
		staticMarks = append(staticMarks, forest.Static(term))
		family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
		if ordinal > maximum[family] {
			maximum[family] = ordinal
		}
	}
	var index [keyspace.FamilyCount][]uint32
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if maximum[family] != 0 {
			index[family] = make([]uint32, maximum[family]+1)
		}
	}
	for at, term := range terms {
		family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
		if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) >= uint64(len(index[family])) {
			return containmentProjection{}, errors.New("containment reduction produced an invalid dense ordinal")
		}
		index[family][ordinal] = uint32(at + 1)
	}
	return containmentProjection{terms: terms, parents: parents, static: staticMarks, index: index}, nil
}
