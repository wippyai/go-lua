package canonical

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
)

// solvePass demands every module summary from the canonical product equation
// system and returns a frozen summary snapshot for post-solve consumers.
// The summary solve query is the implementation decomposition for the point,
// demand, entry, and summary cells; it is not a post-solve precision pass.
func (d *Driver) solvePass(sess api.AnalysisSession, prog *program, queries *summary.Queries) canonicalSolveArtifact {
	artifact := canonicalSolveArtifact{
		Refs:      append([]summary.FuncRef(nil), prog.refs...),
		States:    make(map[summary.FuncRef]state.FunctionState, len(prog.refs)),
		Summaries: make(map[summary.FuncRef]summary.Summary, len(prog.refs)),
	}
	for _, ref := range prog.refs {
		artifact.Summaries[ref] = queries.Summarize(sess.Context(), ref)
		artifact.States[ref] = state.CloneFunctionState(queries.Intra(sess.Context(), ref))
	}
	rootRef, _ := prog.refByFunc(sess.RootFuncNode())
	observed := summary.SelectPostWidenObservationRefs(summary.PostWidenObservationInput{
		Refs: prog.refs,
		Root: rootRef,
		Summary: func(ref summary.FuncRef) summary.Summary {
			return artifact.Summaries[ref]
		},
		Graph: func(ref summary.FuncRef) *cfg.Graph {
			return prog.Graph(ref)
		},
		IsMethod: func(ref summary.FuncRef) bool {
			return prog.methodDef(ref) != nil
		},
		Nested: func(ref summary.FuncRef) []summary.FuncRef {
			return prog.funcTopology.NestedRefs(ref)
		},
		Parent: func(ref summary.FuncRef) (summary.FuncRef, bool) {
			return prog.funcTopology.ParentRef(ref)
		},
	})
	for _, ref := range observed {
		artifact.Summaries[ref] = queries.ObservedSummary(sess.Context(), ref)
	}
	artifact.Snapshot = queries.CanonicalSummarySnapshot(sess.Context(), artifact.Summaries)
	return artifact
}
