package canonical

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
)

func (d *Driver) diagnosticFunctionStates(sess api.AnalysisSession, prog *program, queries *summary.Queries, artifact canonicalSolveArtifact, diagnostics *diagnosticObservationArtifact) map[summary.FuncRef]state.FunctionState {
	states := make(map[summary.FuncRef]state.FunctionState, len(prog.refs))
	for _, ref := range prog.refs {
		// The converged per-point state is an exact observer over the already-solved
		// Summary dependencies, not a second interprocedural fixed point. Diagnostics
		// observe the same aggregate entry-value context that Summary.EntryValues uses,
		// so local helpers are checked under their solved caller-provided entry facts
		// instead of an artificial bottom/default call context.
		states[ref] = d.diagnosticState(sess, prog, queries, artifact, diagnostics, ref)
	}
	return states
}

func (d *Driver) diagnosticState(sess api.AnalysisSession, prog *program, queries *summary.Queries, artifact canonicalSolveArtifact, diagnostics *diagnosticObservationArtifact, ref summary.FuncRef) state.FunctionState {
	if sess == nil || prog == nil || queries == nil {
		return state.FunctionStateDomain.Bottom()
	}
	var contexts []summary.Key
	if diagnostics != nil {
		contexts = diagnostics.Contexts[ref]
	}
	if len(contexts) == 0 {
		reader := summary.NewSnapshotReaderWithStats(artifact.Snapshot, d.stats)
		values := prog.EntryValues(ref, reader)
		if len(values) != 0 {
			return state.CloneFunctionState(d.observeDiagnosticIntra(sess, queries, summary.NewDefaultKey(ref, values)))
		}
		return state.CloneFunctionState(d.observeDiagnosticIntra(sess, queries, summary.NewDefaultKey(ref, nil)))
	}
	out := state.FunctionStateDomain.Bottom()
	for _, key := range contexts {
		// Context discovery and final observation both read the converged canonical
		// snapshot, including exact keys the Summary query actually demanded. This
		// keeps diagnostics precise without creating or publishing a second Summary.
		fs := d.observeDiagnosticIntra(sess, queries, key)
		frozen := state.CloneFunctionState(fs)
		if diagnostics != nil {
			if diagnostics.States == nil {
				diagnostics.States = make(map[summary.Key]state.FunctionState)
			}
			diagnostics.States[key] = frozen
		}
		out = joinDiagnosticFunctionState(out, frozen)
	}
	return state.CloneFunctionState(out)
}

func (d *Driver) buildDiagnosticObservationArtifact(sess api.AnalysisSession, prog *program, queries *summary.Queries, artifact canonicalSolveArtifact) diagnosticObservationArtifact {
	if d == nil || sess == nil || prog == nil || queries == nil {
		return diagnosticObservationArtifact{}
	}
	root, ok := prog.refByFunc(sess.RootFuncNode())
	if !ok {
		return diagnosticObservationArtifact{}
	}
	result := summary.DiagnosticContextFrontier{
		Root: root,
		Refs: prog.refs,
		ValidKey: func(key summary.Key) bool {
			return d.validDiagnosticContext(prog, key)
		},
		DefaultKey: func(ref summary.FuncRef) summary.Key {
			return d.defaultDiagnosticKey(prog, artifact, ref)
		},
		Solve: func(key summary.Key) state.FunctionState {
			return d.observeDiagnosticIntra(sess, queries, key)
		},
		ProjectCalls: func(ref summary.FuncRef, fs state.FunctionState) []summary.Key {
			return d.projectDiagnosticCallContexts(prog, ref, fs)
		},
		ProjectClosures: func(ref summary.FuncRef, fs state.FunctionState) []summary.Key {
			return d.projectDiagnosticClosureContexts(prog, ref, fs)
		},
	}.Build()
	return diagnosticObservationArtifact{
		Contexts: result.Contexts,
		States:   cloneDiagnosticStates(result.States),
	}
}

func cloneDiagnosticStates(in map[summary.Key]state.FunctionState) map[summary.Key]state.FunctionState {
	if len(in) == 0 {
		return nil
	}
	out := make(map[summary.Key]state.FunctionState, len(in))
	for key, fs := range in {
		out[key] = state.CloneFunctionState(fs)
	}
	return out
}

func (d *Driver) observeDiagnosticIntra(sess api.AnalysisSession, queries *summary.Queries, key summary.Key) state.FunctionState {
	if d == nil || sess == nil || queries == nil {
		return state.FunctionStateDomain.Bottom()
	}
	if d.stats != nil {
		d.stats.RecordDiagnosticObservedState()
	}
	return queries.ObserveIntraWithKey(sess.Context(), key)
}

func (d *Driver) validDiagnosticContext(prog *program, key summary.Key) bool {
	return d != nil && prog != nil && prog.Graph(key.Ref) != nil
}

func (d *Driver) defaultDiagnosticKey(prog *program, artifact canonicalSolveArtifact, ref summary.FuncRef) summary.Key {
	if d == nil || prog == nil {
		return summary.NewDefaultKey(ref, nil)
	}
	reader := summary.NewSnapshotReaderWithStats(artifact.Snapshot, d.stats)
	values := prog.EntryValues(ref, reader)
	if len(values) != 0 {
		return summary.NewDefaultKey(ref, values)
	}
	return summary.NewDefaultKey(ref, nil)
}

func (d *Driver) projectDiagnosticCallContexts(prog *program, ref summary.FuncRef, fs state.FunctionState) []summary.Key {
	if d == nil || prog == nil {
		return nil
	}
	return prog.ProjectCallEntryContextKeys(ref, fs)
}

func (d *Driver) projectDiagnosticClosureContexts(prog *program, ref summary.FuncRef, fs state.FunctionState) []summary.Key {
	if d == nil || prog == nil {
		return nil
	}
	return summary.ClosureEntryContextProjection{
		State: fs,
		ReferencePaths: func(callee summary.FuncRef) flow.ReferencePathProjection {
			return prog.referenceProjection(callee)
		},
	}.ProjectKeys()
}

func joinDiagnosticFunctionState(a, b state.FunctionState) state.FunctionState {
	out := state.FunctionStateDomain.Join(a, b)
	out.InPoints = joinDiagnosticInPoints(a.InPoints, b.InPoints)
	return out
}

func joinDiagnosticInPoints(a, b map[cfg.Point]flow.PointState) map[cfg.Point]flow.PointState {
	if len(a) == 0 {
		return cloneInPoints(b)
	}
	if len(b) == 0 {
		return cloneInPoints(a)
	}
	out := cloneInPoints(a)
	for p, ps := range b {
		out[p] = flow.PointStateDomain.Join(out[p], ps)
	}
	return out
}

func cloneInPoints(in map[cfg.Point]flow.PointState) map[cfg.Point]flow.PointState {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]flow.PointState, len(in))
	for p, ps := range in {
		out[p] = flow.ClonePointState(ps)
	}
	return out
}
