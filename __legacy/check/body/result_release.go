package body

import (
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
)

// ReleaseTransient drops the solver and projection working set held by this
// result. It is an ownership boundary for callers that have already copied all
// required summary, diagnostic, placement, semantic, and debug projections.
//
// It intentionally does not recurse: materialization can reuse a child result
// in a later tree while discarding its former parent. Use
// ReleaseTransientTree once the entire tree is no longer needed.
func (r *Result) ReleaseTransient() {
	if r == nil {
		return
	}
	r.flow = nil
	r.published = PublishedFacts{}
	r.observationPlan = ObservationPlan{}
	r.sources = nil
	r.signatureArg = effectlowering.SignatureArgumentTypeProgram{}
	r.exprRefinements = sourcevalue.ExpressionRefinements{}
	r.queries.reset()

	// Everything below is prepared/solved-body metadata. The public service has
	// already projected it into compact records before releasing a tree.
	r.bindings = nil
	r.cfg = nil
	r.function = nil
	r.wir = nil
	r.sourceStmts = nil
	r.signatures = signaturelookup.Source{}
	r.moduleTypes = typelookup.Source{}
	r.modules = moduleidentity.Projection{}
	r.signatureID = nil
	r.facts = factflow.Facts{}
	r.symbolTypes = nil
	r.assignments = assignmentFactSet{}
	r.declarations = declarationFactSet{}
	r.genericFors = nil
	r.typeNS = nil
	r.visibility = nil
	r.typeValues = nil
	r.stateLanes = nil
	r.funcTypes = FunctionValueTypes{}
	r.callExprPts = nil
	r.returnPoints = nil
	r.paramValueSlots = nil
	r.reassignedParams = nil
	r.returnTypeValues = nil
	r.functions = nil
}

// ReleaseTransientTree releases this result and all materialized nested
// results. Call it only after every consumer has projected its compact output.
func (r *Result) ReleaseTransientTree() {
	seen := map[*Result]struct{}{}
	var release func(*Result)
	release = func(current *Result) {
		if current == nil {
			return
		}
		if _, ok := seen[current]; ok {
			return
		}
		seen[current] = struct{}{}
		for _, child := range current.functions {
			release(child)
		}
		current.ReleaseTransient()
	}
	release(r)
}
