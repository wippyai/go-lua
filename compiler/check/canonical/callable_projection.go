package canonical

import (
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// callableProjector is the observation boundary for function-valued runtime
// paths. A callable value is either a closure value, whose environment is carried
// by ClosureRefs, or a bare function identity, whose environment is projected
// from the ambient point-state product. Diagnostics consume the resulting type;
// summary lookup and closure environment selection stay in this product-owned
// projection layer.
type callableProjector struct {
	prog               *program
	reader             summary.Reader
	baseSignature      func(summary.FuncRef) *typ.Function
	hasDeclaredReturns func(summary.FuncRef) bool
}

func newCallableProjector(d *Driver, prog *program, queries *summary.Queries, ctx *db.QueryContext) callableProjector {
	if d == nil {
		return callableProjector{prog: prog}
	}
	reader := summary.NewReader(queries, ctx, d.summaries)
	if d.snapshotSummaryReads {
		reader = d.summaryReader()
	}
	return callableProjector{
		prog:   prog,
		reader: reader,
		baseSignature: func(ref summary.FuncRef) *typ.Function {
			if prog == nil {
				return nil
			}
			if d.refHasDeclaredReturns(prog, ref) {
				return d.declaredSignatureForRef(prog, ref)
			}
			return d.signatureForRef(prog, ref)
		},
		hasDeclaredReturns: func(ref summary.FuncRef) bool {
			if prog == nil {
				return false
			}
			return d.refHasDeclaredReturns(prog, ref)
		},
	}
}

func (ct callTyper) FunctionValue(query flow.CallableSignatureQuery) (typ.Type, bool) {
	d := ct.d
	if d == nil || d.activeProgram == nil {
		return nil, false
	}
	projector := newCallableProjector(d, d.activeProgram, d.activeQueries, d.activeCtx)
	var sig typ.Type
	if query.Ref != (flow.FunctionRef{}) {
		sig = projector.FunctionTypeByRef(
			query.Ref,
			query.State.Cells,
			query.State.FunctionRefs,
			query.State.ClosureRefs,
		)
	} else if !query.Path.IsEmpty() {
		sig = projector.TypeAt(query.State, query.Path)
	}
	if typ.IsAbsentOrUnknown(sig) {
		return nil, false
	}
	return sig, true
}

func (p callableProjector) TypeAt(in flow.PointState, path constraint.Path) typ.Type {
	if p.prog == nil || path.Symbol == 0 {
		return nil
	}
	if t := p.closureTypeAtPath(in, path); !typ.IsAbsentOrUnknown(t) {
		return t
	}
	if _, ok := flow.ClosureRefAtPath(in.ClosureRefs, path); ok {
		return nil
	}
	refs, ok := flow.FunctionRefAtPath(in.FunctionRefs, path)
	if ok {
		return p.functionRefsType(refs, in.Cells, in.FunctionRefs, in.ClosureRefs)
	}
	if ref, ok := p.staticRefAtPath(path); ok {
		return p.signature(canonref.ToFlow(ref), in.Cells, in.FunctionRefs, in.ClosureRefs)
	}
	return nil
}

func (p callableProjector) FunctionTypeByRef(ref flow.FunctionRef, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs) typ.Type {
	if p.prog == nil {
		return nil
	}
	return p.signature(ref, cells, refs, closures)
}

func (p callableProjector) closureTypeAtPath(in flow.PointState, path constraint.Path) typ.Type {
	set, ok := flow.ClosureRefAtPath(in.ClosureRefs, path)
	if !ok {
		return nil
	}
	closures := set.Refs()
	if len(closures) == 0 {
		return nil
	}
	acc := product.Domain.Bottom()
	for _, closure := range closures {
		sig := p.closureSignature(closure, in)
		if typ.IsAbsentOrUnknown(sig) {
			continue
		}
		acc = product.Domain.Join(acc, product.FromType(sig))
	}
	if product.Domain.Equal(acc, product.Domain.Bottom()) {
		return nil
	}
	return product.ProjectValueOrUnknown(acc)
}

func (p callableProjector) closureSignature(closure flow.ClosureRef, in flow.PointState) typ.Type {
	if p.prog == nil {
		return nil
	}
	sref := canonref.FromFlow(closure.Ref)
	entry := canonicalcall.EntryContextFromClosureWithLiveContext(
		closure,
		p.prog.CallEntryContext(sref, flow.ReferenceContextFromPoint(&in), nil),
	)
	return p.signature(closure.Ref, entry.CaptureCells(), entry.FunctionRefs(), entry.ClosureRefs())
}

func (p callableProjector) functionRefsType(refs flow.FunctionRefSet, cells flow.CaptureCells, fnRefs flow.FunctionRefs, closures flow.ClosureRefs) typ.Type {
	flowRefs := refs.Refs()
	if len(flowRefs) == 0 {
		return nil
	}
	acc := product.Domain.Bottom()
	for _, ref := range flowRefs {
		sig := p.signature(ref, cells, fnRefs, closures)
		if typ.IsAbsentOrUnknown(sig) {
			continue
		}
		acc = product.Domain.Join(acc, product.FromType(sig))
	}
	if product.Domain.Equal(acc, product.Domain.Bottom()) {
		return nil
	}
	return product.ProjectValueOrUnknown(acc)
}

func (p callableProjector) staticRefAtPath(path constraint.Path) (summary.FuncRef, bool) {
	if p.prog == nil || path.Symbol == 0 {
		return summary.FuncRef{}, false
	}
	if len(path.Segments) == 0 {
		return p.prog.funcRef(path.Symbol)
	}
	if len(path.Segments) == 1 {
		field, ok := fieldkey.FromSegment(path.Segments[0])
		if !ok {
			return summary.FuncRef{}, false
		}
		return p.prog.fieldFuncRef(path.Symbol, field)
	}
	return summary.FuncRef{}, false
}

func (p callableProjector) signature(ref flow.FunctionRef, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs) typ.Type {
	sref := canonref.FromFlow(ref)
	if p.baseSignature == nil {
		return nil
	}
	sig := p.baseSignature(sref)
	if sig == nil {
		return nil
	}
	entry := p.prog.CallEntryContext(sref, flow.ReferenceContextOf(cells, refs, closures), nil)
	hasDeclaredReturns := false
	if p.hasDeclaredReturns != nil {
		hasDeclaredReturns = p.hasDeclaredReturns(sref)
	}
	sum := p.reader.SummarizeWithKey(entry.Key())
	return summary.FunctionSignatureWithProjectedReturns(sig, hasDeclaredReturns, sum)
}
