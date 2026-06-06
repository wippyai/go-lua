package canonical

import (
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
)

// CaptureEntryReferences projects captured lexical reference state through the
// callee-visible reference vocabulary so closure entry observes one normalized
// value/function/closure carrier.
func (p *program) CaptureEntryReferences(ref summary.FuncRef, captureReferencesOf func(summary.FuncRef) flow.ReferenceContext) flow.ReferenceContext {
	g := p.Graph(ref)
	if g == nil || captureReferencesOf == nil {
		return flow.ReferenceContextBottom()
	}
	bindings := g.Bindings()
	fn := g.Func()
	if bindings == nil || fn == nil {
		return flow.ReferenceContextBottom()
	}
	captured := bindings.CapturedSymbols(fn)
	if len(captured) == 0 {
		return flow.ReferenceContextBottom()
	}
	deps := p.captureDependencyChain(ref)
	entries := make([]flow.CaptureCell, 0, len(captured))
	for _, sym := range captured {
		if !g.IsFreeSymbol(sym) {
			continue
		}
		if av, ok := p.captureEntryValue(ref, sym, deps, func(dep summary.FuncRef) flow.CaptureCells {
			return captureReferencesOf(dep).CaptureCells()
		}); ok {
			entries = append(entries, flow.CaptureCell{Symbol: sym, Value: av})
		}
	}
	projection := p.referenceProjection(ref)
	out := flow.ReferenceContextOf(flow.CaptureCellsOf(entries), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()).ProjectPaths(projection)
	for _, dep := range p.captureDependencyChain(ref) {
		out = out.Join(captureReferencesOf(dep).ProjectPaths(projection).CallableIdentity())
	}
	return out
}

func (p *program) callEntryReferenceContext(ref summary.FuncRef, caller flow.ReferenceContext) flow.ReferenceContext {
	return caller.ProjectPaths(p.referenceProjection(ref))
}

// CallEntryContext projects caller-owned entry axes into the callee-visible
// reference vocabulary.
func (p *program) CallEntryContext(
	ref summary.FuncRef,
	references flow.ReferenceContext,
	values summary.EntryValues,
) canonicalcall.EntryContext {
	return p.CallEntryContextWithFacts(ref, references, values, flow.BoundaryFactsDomain.Top())
}

// CallEntryContextWithFacts preserves parameter-relative path facts alongside
// the projected entry axes.
func (p *program) CallEntryContextWithFacts(
	ref summary.FuncRef,
	references flow.ReferenceContext,
	values summary.EntryValues,
	facts flow.BoundaryFacts,
) canonicalcall.EntryContext {
	return canonicalcall.NewEntryContext(
		ref,
		p.callEntryReferenceContext(ref, references),
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
