package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type cellEffectProjector struct {
	typer     callTyper
	program   *program
	driver    *Driver
	callEntry callEntryProjector
}

// cellEffectProjector is the canonical/program-owned adapter for caller-visible
// capture-cell effects. The summary package owns callback ordering and effect
// algebra; productCallFrame owns call-local callback target resolution, while
// this type supplies callback specs and summary effect lookups.
func (ct callTyper) cellEffectProjector() (cellEffectProjector, bool) {
	if ct.d == nil || ct.d.activeProgram == nil {
		return cellEffectProjector{}, false
	}
	callEntry, ok := ct.callEntryProjector()
	if !ok {
		return cellEffectProjector{}, false
	}
	return cellEffectProjector{
		typer:     ct,
		program:   ct.d.activeProgram,
		driver:    ct.d,
		callEntry: callEntry,
	}, true
}

func (p cellEffectProjector) effectsForRef(
	ref summary.FuncRef,
	references flow.ReferenceContext,
	entryValues summary.EntryValues,
	entryFacts flow.BoundaryFacts,
) flow.CaptureEffects {
	reader := p.driver.summaryReader()
	entry := canonicalcall.NewEntryContext(
		ref,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
		entryValues,
		entryFacts,
	)
	if reader.Live() {
		entry = p.program.CallEntryContextWithFacts(ref, references, entryValues, entryFacts)
	}
	return reader.SummarizeWithKey(entry.Key()).CellEffects
}

func (p cellEffectProjector) callbackSpecForCall(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) *contract.Spec {
	resolver := p.typer.callTypeResolver(exprType)
	return (canonicalcall.CallbackSpecProjection{
		Call: call,
		SummarySignature: func(call *ast.FuncCallExpr) typ.Type {
			if ref, ok := p.typer.resolveCalleeRef(call, p.program); ok {
				return p.driver.signatureForRef(p.program, ref)
			}
			return nil
		},
		Resolver: resolver,
	}).Spec()
}
