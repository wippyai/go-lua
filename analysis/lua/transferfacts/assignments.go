package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) localAssignment(fact semantics.LocalAssignmentFact) (factflow.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return factflow.RootAssignment{}, false
	}
	target := path.NewPath(fact.Symbol, fact.Name)
	source := l.valueSource(fact.Source)
	if l.declaredValueApplies(fact) {
		if declared, ok := l.declaredValue(fact.Type); ok {
			return factflow.NewRootAssignmentWithDeclaredContractValue(factflow.RootAssignmentLocalDeclaration, fact.Symbol, target, source, declared), true
		}
	}
	return factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, fact.Symbol, target, source), true
}

func (l *lowerer) declaredValueApplies(fact semantics.LocalAssignmentFact) bool {
	if fact.Type == nil || fact.Source.Kind != sourceprovenance.SourceExpression {
		return false
	}
	if _, ok := valueexpr.LiteralType(fact.Expr); ok {
		return true
	}
	t, ok := l.resolveType(fact.Type)
	if !ok {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	if !tableConstructorExpr(fact.Expr) {
		return false
	}
	// A table constructor declared as a method-bearing record (the class/builder
	// instance pattern) carries the declared contract value. Such literals wire
	// method fields to callables defined later in the module or across modules,
	// which resolve to nothing at construction and drop out of the inferred
	// record; the declared annotation is the checked contract for the local.
	if recordWithCallableField(t) {
		return true
	}
	// A non-empty table constructor declared as an array carries the declared
	// array contract value. A homogeneous array literal infers positionally as a
	// fixed-arity tuple with per-element-precise types; the declared annotation
	// is the authoritative array contract for the local. The literal is still
	// verified element-wise against the annotation at the assignment site. An
	// empty literal is left to its inferred value so later table.insert flow
	// tracks populated indexes precisely.
	return reachesArray(t) && nonEmptyTableConstructor(fact.Expr)
}

func nonEmptyTableConstructor(expr ast.Expr) bool {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return false
	}
	table, ok := inner.(*ast.TableExpr)
	return ok && len(table.Fields) > 0
}

func reachesArray(t typ.Type) bool {
	return reachesArrayDepth(t, 0)
}

func reachesArrayDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Array:
		return true
	case *typ.Alias:
		return reachesArrayDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return reachesArrayDepth(v.Body, depth+1)
	default:
		return false
	}
}

func tableConstructorExpr(expr ast.Expr) bool {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return false
	}
	_, ok = inner.(*ast.TableExpr)
	return ok
}

func recordWithCallableField(t typ.Type) bool {
	return recordWithCallableFieldDepth(t, 0)
}

func recordWithCallableFieldDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return recordWithCallableFieldDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return recordWithCallableFieldDepth(v.Body, depth+1)
	case *typ.Record:
		for i := range v.Fields {
			if _, ok := typecall.Callable(v.Fields[i].Type); ok {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (l *lowerer) ordinaryAssignment(fact semantics.OrdinaryAssignmentFact) (factflow.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return factflow.RootAssignment{}, false
	}
	target := fact.Path
	if !fact.HasPath {
		target = path.NewPath(fact.Symbol, "")
	}
	if len(target.Segments) != 0 {
		return factflow.RootAssignment{}, false
	}
	return factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, fact.Symbol, target, l.valueSource(fact.Source)), true
}

func (l *lowerer) pathAssignment(fact semantics.OrdinaryAssignmentFact) (factflow.PathAssignment, bool) {
	if !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
		return factflow.PathAssignment{}, false
	}
	return factflow.NewPathAssignment(fact.Path, l.valueSource(fact.Source)), true
}

func (l *lowerer) pathStaticMemberWrite(fact semantics.OrdinaryAssignmentFact) (factflow.PathStaticMemberWrite, bool) {
	if !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
		return factflow.PathStaticMemberWrite{}, false
	}
	return factflow.NewPathStaticMemberWrite(fact.Path, l.valueSource(fact.Source)), true
}

func (l *lowerer) dynamicIndexWrite(fact semantics.OrdinaryAssignmentFact) (factflow.DynamicIndexWrite, bool) {
	if fact.HasPath {
		return factflow.DynamicIndexWrite{}, false
	}
	tablePath, ok := l.directDynamicIndexWriteTablePath(fact.Target)
	if !ok || tablePath.Symbol == 0 {
		return factflow.DynamicIndexWrite{}, false
	}
	keySource, readKey := l.dynamicIndexKeySource(fact.Target)
	source := l.valueSource(fact.Source)
	readValue := fact.Source.Kind != sourceprovenance.SourceUnknown
	return factflow.NewDynamicIndexWrite(
		tablePath,
		keySource,
		source,
		dynamicindex.AdmissionUnknown,
		dynamicIndexReadbackIntent(readKey, readValue),
	), true
}

func (l *lowerer) directDynamicIndexWriteTablePath(target ast.Expr) (path.Path, bool) {
	attr, ok := target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return path.Path{}, false
	}
	switch attr.Key.(type) {
	case *ast.StringExpr, *ast.NumberExpr:
		return path.Path{}, false
	}
	return pathexpr.Resolve(attr.Object, l.bindings)
}

func (l *lowerer) pathDescendantInvalidation(fact semantics.OrdinaryAssignmentFact) (factflow.PathDescendantInvalidation, bool) {
	if fact.HasPath || !fact.HasContainerPath || fact.ContainerPath.Symbol == 0 {
		return factflow.PathDescendantInvalidation{}, false
	}
	return factflow.NewPathDescendantInvalidation(fact.ContainerPath), true
}

func (l *lowerer) dynamicIndexKeySource(target ast.Expr) (factflow.ValueSource, bool) {
	attr, ok := target.(*ast.AttrGetExpr)
	if !ok || attr.Key == nil || sourceprovenance.CanProduceMultipleValues(attr.Key) {
		return factflow.NewUnknownValueSource(factflow.NoValueSourceIndex), false
	}
	shape, ok := sourceprovenance.NewSourceShape(true, false, false, false)
	if !ok {
		panic("transferfacts: invalid dynamic index key source shape")
	}
	source, ok := sourceprovenance.NewExpressionSource(
		attr.Key,
		sourceprovenance.NoSourceIndex,
		sourceprovenance.NoSourceIndex,
		0,
		shape,
	)
	if !ok {
		return factflow.NewUnknownValueSource(factflow.NoValueSourceIndex), false
	}
	return l.valueSource(source), true
}

func dynamicIndexReadbackIntent(readKey bool, readValue bool) factflow.DynamicIndexReadbackIntent {
	switch {
	case readKey && readValue:
		return factflow.DynamicIndexReadbackKeyAndValue
	case readKey:
		return factflow.DynamicIndexReadbackKey
	case readValue:
		return factflow.DynamicIndexReadbackValue
	default:
		return factflow.DynamicIndexReadbackNone
	}
}

func (l *lowerer) declaredValue(expr ast.TypeExpr) (product.Value, bool) {
	if expr == nil {
		return product.Value{}, false
	}
	t, ok := l.resolveType(expr)
	if !ok {
		return product.Value{}, false
	}
	return l.valueFromTypeWithWitness(t), true
}
