package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) valueSources(sources []sourceprovenance.ASTSource) []factflow.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, len(sources))
	for i := range sources {
		out[i] = l.valueSource(sources[i])
	}
	return out
}

func (l *lowerer) valueSource(source sourceprovenance.ASTSource) factflow.ValueSource {
	exprRef, hasExpr := l.exprRef(source.Expr)
	if hasExpr {
		l.addExpressionPath(exprRef, source.Expr)
	}
	return factflow.ValueSource{
		Kind:         source.Kind,
		ExprRef:      exprRef,
		HasExpr:      hasExpr,
		ExprIndex:    source.ExprIndex,
		TargetIndex:  source.TargetIndex,
		ResultIndex:  source.ResultIndex,
		CallPoint:    source.CallPoint,
		HasCallPoint: source.HasCallPoint,
		Final:        source.Final,
		Expanded:     source.Expanded,
		Adjusted:     source.Adjusted,
		OpenTail:     source.OpenTail,
	}
}

func (l *lowerer) addExpressionPath(ref factflow.ExprRef, expr ast.Expr) {
	if ref == 0 || expr == nil || l.bindings == nil {
		return
	}
	p, ok := pathexpr.Resolve(expr, l.bindings)
	if !ok || p.IsEmpty() {
		return
	}
	if l.expressionPaths == nil {
		l.expressionPaths = make(map[factflow.ExprRef]pathdom.Path)
	}
	l.expressionPaths[ref] = p
}

func (l *lowerer) argumentValueSources(args []ast.Expr) []factflow.ValueSource {
	if len(args) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, len(args))
	for i, arg := range args {
		source := sourceprovenance.SourceForExpr(arg, i, i, 0, i == len(args)-1, false, l.callPointResolver())
		out[i] = l.valueSource(source)
	}
	return out
}

func (l *lowerer) argumentSemanticValueSources(args []ast.Expr) []sourceprovenance.ASTSource {
	return sourceprovenance.ValueListSources(args, false, l.callPointResolver())
}

func (l *lowerer) callPointResolver() sourceprovenance.CallPointResolver {
	if len(l.callPoints) == 0 {
		return nil
	}
	return func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		point, ok := l.callPoints[call]
		return point, ok
	}
}

func (l *lowerer) exprRef(expr any) (factflow.ExprRef, bool) {
	if expr == nil {
		return 0, false
	}
	if ref, ok := l.exprs[expr]; ok {
		return ref, true
	}
	ref := factflow.ExprRef(len(l.exprs) + 1)
	l.exprs[expr] = ref
	return ref, true
}

func (l *lowerer) typeRefs(types []ast.TypeExpr) []factflow.TypeRef {
	if len(types) == 0 {
		return nil
	}
	out := make([]factflow.TypeRef, len(types))
	for i := range types {
		out[i], _ = l.typeRef(types[i])
	}
	return out
}

func (l *lowerer) typeRef(typ any) (factflow.TypeRef, bool) {
	if typ == nil {
		return 0, false
	}
	if ref, ok := l.types[typ]; ok {
		return ref, true
	}
	if l.types == nil {
		l.types = make(map[any]factflow.TypeRef)
	}
	ref := factflow.TypeRef(len(l.types) + 1)
	l.types[typ] = ref
	return ref, true
}
