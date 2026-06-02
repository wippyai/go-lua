package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// ValueOriginEffect records that ValuePath is derived from SourcePath by a
// semantic projection such as iterator element selection. It is a point-state
// provenance fact, not a value update; backward demand later consumes it to route
// typed uses of derived locals back to source parameters.
type ValueOriginEffect struct {
	ValuePath  constraint.Path
	SourcePath constraint.Path
	Kind       flow.ValueOriginKind
	VarIndex   int
}

func (t *Transfer) applyValueOriginEffect(out *flow.PointState, effect ValueOriginEffect) bool {
	if out == nil || effect.ValuePath.IsEmpty() || effect.SourcePath.IsEmpty() || effect.Kind == 0 {
		return false
	}
	before := out.ValueOrigins
	out.ValueOrigins = out.ValueOrigins.WithPaths(effect.ValuePath, effect.SourcePath, effect.Kind, effect.VarIndex)
	return !flow.ValueOriginFactsDomain.Equal(before, out.ValueOrigins)
}

func (t *Transfer) demandExprCtx(out *flow.PointState, expr ast.Expr, ctx typ.Type, demand func(int, paramevidence.ParamContract)) {
	if demand == nil || ctx == nil || typ.IsAbsentOrUnknown(ctx) {
		return
	}
	valuePath, ok := t.demandPathForExpr(expr)
	if !ok || valuePath.Symbol == 0 {
		return
	}
	if idx, isParam := t.paramBySym[valuePath.Symbol]; isParam {
		evidence := ctx
		if len(valuePath.Segments) > 0 {
			evidence = paramevidence.PathEvidence(valuePath.Segments, ctx)
			if evidence == nil {
				return
			}
		}
		if out != nil {
			evidence, _ = paramevidence.ConditionedPathEvidence(valuePath, evidence, out.Cond)
		}
		demand(idx, paramevidence.DemandFromType(evidence))
		return
	}
	if out == nil {
		return
	}
	for _, use := range out.ValueOrigins.OriginsCoveringPath(valuePath) {
		localEvidence := paramevidence.PathEvidence(use.Remainder, ctx)
		if localEvidence == nil {
			continue
		}
		evidence := iteratorOriginEvidence(use.Origin, localEvidence)
		if evidence == nil {
			continue
		}
		t.demandSourcePathCtx(use.Origin.Source, evidence, demand)
	}
}

func (t *Transfer) demandPathForExpr(expr ast.Expr) (constraint.Path, bool) {
	sym, segments, ok := t.pathSymbol(expr)
	if !ok || sym == 0 {
		return constraint.Path{}, false
	}
	return constraint.Path{
		Symbol:   sym,
		Segments: append([]constraint.Segment(nil), segments...),
	}, true
}

func (t *Transfer) demandSourcePathCtx(source constraint.PathKey, evidence typ.Type, demand func(int, paramevidence.ParamContract)) {
	if source == "" || evidence == nil || demand == nil {
		return
	}
	sym, segments, ok := flow.ParseSymbolPathKey(source)
	if !ok || sym == 0 {
		return
	}
	idx, isParam := t.paramBySym[sym]
	if !isParam {
		return
	}
	if len(segments) > 0 {
		evidence = paramevidence.PathEvidence(segments, evidence)
		if evidence == nil {
			return
		}
	}
	demand(idx, paramevidence.DemandFromType(evidence))
}

func iteratorOriginEvidence(origin flow.ValueOriginFact, local typ.Type) typ.Type {
	switch origin.Kind {
	case flow.ValueOriginIndexedIterator:
		return paramevidence.IndexedIteratorEvidence(origin.VarIndex, local)
	case flow.ValueOriginKeyedIterator:
		return paramevidence.KeyedIteratorEvidence(origin.VarIndex, local)
	default:
		return nil
	}
}
