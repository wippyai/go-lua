package canonical

import (
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
)

// CaptureEntries projects captured lexical values through the callee-visible
// reference vocabulary. The value carrier and function/closure reference axes use
// the same referenceProjection so closures observe a single normalized view.
func (p *program) CaptureEntries(ref summary.FuncRef, captureExportsOf func(summary.FuncRef) flow.CaptureCells) flow.CaptureCells {
	g := p.Graph(ref)
	if g == nil || captureExportsOf == nil {
		return flow.CaptureCellsDomain.Bottom()
	}
	bindings := g.Bindings()
	fn := g.Func()
	if bindings == nil || fn == nil {
		return flow.CaptureCellsDomain.Bottom()
	}
	captured := bindings.CapturedSymbols(fn)
	if len(captured) == 0 {
		return flow.CaptureCellsDomain.Bottom()
	}
	deps := p.captureDependencyChain(ref)
	entries := make([]flow.CaptureCell, 0, len(captured))
	for _, sym := range captured {
		if !g.IsFreeSymbol(sym) {
			continue
		}
		if av, ok := p.captureEntryValue(ref, sym, deps, captureExportsOf); ok {
			entries = append(entries, flow.CaptureCell{Symbol: sym, Value: av})
		}
	}
	return flow.CaptureCellsOf(entries).ProjectPaths(p.referenceProjection(ref))
}

func (p *program) CaptureEntryRefs(ref summary.FuncRef, captureFunctionRefsOf func(summary.FuncRef) flow.FunctionRefs) flow.FunctionRefs {
	g := p.Graph(ref)
	if g == nil || captureFunctionRefsOf == nil {
		return flow.FunctionRefsDomain.Bottom()
	}
	bindings := g.Bindings()
	fn := g.Func()
	if bindings == nil || fn == nil {
		return flow.FunctionRefsDomain.Bottom()
	}
	captured := bindings.CapturedSymbols(fn)
	if len(captured) == 0 {
		return flow.FunctionRefsDomain.Bottom()
	}
	projection := p.referenceProjection(ref)
	if referenceProjectionEmpty(projection) {
		return flow.FunctionRefsDomain.Bottom()
	}
	out := flow.FunctionRefsDomain.Bottom()
	for _, dep := range p.captureDependencyChain(ref) {
		out = flow.FunctionRefsDomain.Join(out, flow.ProjectFunctionRefsByReferencePaths(captureFunctionRefsOf(dep), projection))
	}
	return out
}

func (p *program) CaptureEntryClosureRefs(ref summary.FuncRef, captureClosureRefsOf func(summary.FuncRef) flow.ClosureRefs) flow.ClosureRefs {
	g := p.Graph(ref)
	if g == nil || captureClosureRefsOf == nil {
		return flow.ClosureRefsDomain.Bottom()
	}
	projection := p.referenceProjection(ref)
	if referenceProjectionEmpty(projection) {
		return flow.ClosureRefsDomain.Bottom()
	}
	out := flow.ClosureRefsDomain.Bottom()
	for _, dep := range p.captureDependencyChain(ref) {
		out = flow.ClosureRefsDomain.Join(out, flow.ProjectClosureRefsByReferencePaths(captureClosureRefsOf(dep), projection))
	}
	return out
}

func (p *program) callEntryCells(ref summary.FuncRef, caller flow.CaptureCells) flow.CaptureCells {
	return caller.ProjectPaths(p.referenceProjection(ref))
}

func (p *program) callEntryFunctionRefs(ref summary.FuncRef, caller flow.FunctionRefs) flow.FunctionRefs {
	return flow.ProjectFunctionRefsByReferencePaths(caller, p.referenceProjection(ref))
}

func (p *program) callEntryClosureRefs(ref summary.FuncRef, caller flow.ClosureRefs) flow.ClosureRefs {
	return flow.ProjectClosureRefsByReferencePaths(caller, p.referenceProjection(ref))
}

// CallEntryContext projects caller-owned entry axes into the callee-visible
// reference vocabulary.
func (p *program) CallEntryContext(
	ref summary.FuncRef,
	cells flow.CaptureCells,
	refs flow.FunctionRefs,
	closures flow.ClosureRefs,
	values summary.EntryValues,
) canonicalcall.EntryContext {
	return p.CallEntryContextWithFacts(ref, cells, refs, closures, values, flow.BoundaryFactsDomain.Top())
}

// CallEntryContextWithFacts preserves parameter-relative path facts alongside
// the projected entry axes.
func (p *program) CallEntryContextWithFacts(
	ref summary.FuncRef,
	cells flow.CaptureCells,
	refs flow.FunctionRefs,
	closures flow.ClosureRefs,
	values summary.EntryValues,
	facts flow.BoundaryFacts,
) canonicalcall.EntryContext {
	return canonicalcall.NewEntryContext(
		ref,
		p.callEntryCells(ref, cells),
		p.callEntryFunctionRefs(ref, refs),
		p.callEntryClosureRefs(ref, closures),
		values,
		facts,
	)
}

func (p *program) referenceProjection(ref summary.FuncRef) flow.ReferencePathProjection {
	if p == nil {
		return flow.ReferencePathProjection{}
	}
	if projection, ok := p.referencePaths[ref]; ok {
		return projection
	}
	g := p.Graph(ref)
	if g == nil {
		return flow.ReferencePathProjection{}
	}
	projection := summary.ReferencePathProjectionForGraph(g)
	if p.referencePaths != nil {
		p.referencePaths[ref] = projection
	}
	return projection
}

func referenceProjectionEmpty(projection flow.ReferencePathProjection) bool {
	return len(projection.Exact) == 0 && len(projection.Subtrees) == 0
}
