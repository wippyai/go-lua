package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/functiontype"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) addExpressionPath(ref factflow.ExprRef, expr ast.Expr) {
	if ref == 0 || expr == nil || l.bindings == nil {
		return
	}
	p, ok := pathexpr.ViewOf(expr, l.bindings).AliasPath()
	if !ok || p.IsEmpty() {
		l.addDynamicIndexExpression(ref, expr)
		return
	}
	if l.expressionPaths == nil {
		l.expressionPaths = make(map[factflow.ExprRef]pathdom.Path)
	}
	l.expressionPaths[ref] = p
}

func (l *lowerer) addDynamicIndexExpression(ref factflow.ExprRef, expr ast.Expr) {
	if ref == 0 || expr == nil || l.bindings == nil {
		return
	}
	dyn, ok := l.dynamicIndexExpression(expr)
	if !ok {
		return
	}
	if l.dynamicIndexExpressions == nil {
		l.dynamicIndexExpressions = make(map[factflow.ExprRef]factflow.DynamicIndexExpression)
	}
	l.dynamicIndexExpressions[ref] = dyn
}

func (l *lowerer) dynamicIndexExpression(expr ast.Expr) (factflow.DynamicIndexExpression, bool) {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return factflow.DynamicIndexExpression{}, false
	}
	attr, ok := inner.(*ast.AttrGetExpr)
	if !ok || attr.Key == nil ||
		(attr.KeySyntax != ast.AttrKeyIndex && attr.KeySyntax != ast.AttrKeyDot) {
		return factflow.DynamicIndexExpression{}, false
	}
	keySource, ok := l.dynamicIndexKeySourceFromAST(attr)
	if !ok {
		return factflow.DynamicIndexExpression{}, false
	}
	tableSource, hasTableSource := l.expressionOperandSource(attr.Object)
	tablePath, hasTablePath := pathexpr.Resolve(attr.Object, l.bindings)
	if attr.KeySyntax == ast.AttrKeyDot {
		if aliasPath, ok := pathexpr.ResolveAlias(attr.Object, l.bindings); ok && !aliasPath.IsEmpty() {
			return factflow.DynamicIndexExpression{}, false
		}
	}
	if hasTablePath && !tablePath.IsEmpty() {
		expr, ok := factflow.NewDynamicIndexExpression(tablePath, keySource)
		if !ok {
			return factflow.DynamicIndexExpression{}, false
		}
		if hasTableSource {
			expr = expr.WithTableSource(tableSource)
		}
		return expr, true
	}
	if hasTableSource {
		return factflow.NewDynamicIndexExpressionFromSource(tableSource, keySource)
	}
	return factflow.DynamicIndexExpression{}, false
}

func (l *lowerer) addExpressionCondition(ref factflow.ExprRef, expr ast.Expr) {
	if ref == 0 || expr == nil || l.bindings == nil {
		return
	}
	var trueRefinements []factflow.PostconditionRefinement
	var falseRefinements []factflow.PostconditionRefinement
	var trueRelations []factflow.PostconditionPathRelation
	var falseRelations []factflow.PostconditionPathRelation
	l.addExpressionConditionImplications(&trueRefinements, &trueRelations, branchcond.ImpliedChecksOnEdge(expr, l.bindings, true))
	l.addExpressionConditionImplications(&falseRefinements, &falseRelations, branchcond.ImpliedChecksOnEdge(expr, l.bindings, false))
	if refinement, returnValue, ok := l.typeIsExpressionConditionRefinement(expr); ok {
		if returnValue {
			trueRefinements = append(trueRefinements, refinement)
		} else {
			falseRefinements = append(falseRefinements, refinement)
		}
	}
	condition := factflow.NewExpressionCondition(trueRefinements, falseRefinements, trueRelations, falseRelations)
	if condition.IsEmpty() {
		return
	}
	l.expressionConditions[ref] = condition
}

func (l *lowerer) addExpressionConditionImplications(
	refinements *[]factflow.PostconditionRefinement,
	relations *[]factflow.PostconditionPathRelation,
	impliedChecks []branchcond.ImpliedCheck,
) {
	for _, implied := range impliedChecks {
		if refinement, ok := l.branchImplicationRefinement(implied); ok {
			if value, ok := refinement.ValueForEdge(implied.Edge); ok {
				*refinements = append(*refinements, factflow.NewPostconditionRefinement(refinement.TargetPath(), value))
			}
		}
		for _, relation := range checkPathRelationsForImplication(implied) {
			if relation.Kind() != factflow.BranchPathRelationEqual || !relation.ActiveOnEdge(implied.Edge) {
				continue
			}
			*relations = append(*relations, factflow.NewPostconditionPathEquality(relation.LeftPath(), relation.RightPath()))
		}
	}
}

func (l *lowerer) addExpressionValue(ref factflow.ExprRef, expr ast.Expr) {
	if ref == 0 || expr == nil {
		return
	}
	l.addExpressionFunction(ref, expr)
	value, ok := l.expressionValue(expr)
	if !ok {
		l.addExpressionOperation(ref, expr)
		return
	}
	l.addExpressionOperation(ref, expr)
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	l.expressionValues[ref] = value
}

func (l *lowerer) addExpressionOperation(ref factflow.ExprRef, expr ast.Expr) {
	if ref == 0 || expr == nil {
		return
	}
	var op factflow.ExpressionOperation
	var ok bool
	switch expr := expr.(type) {
	case *ast.ArithmeticOpExpr:
		op, ok = l.binaryExpressionOperation(expr.Operator, expr.Lhs, expr.Rhs)
	case *ast.RelationalOpExpr:
		op, ok = l.binaryExpressionOperation(expr.Operator, expr.Lhs, expr.Rhs)
	case *ast.StringConcatOpExpr:
		op, ok = l.binaryExpressionOperation("..", expr.Lhs, expr.Rhs)
	case *ast.LogicalOpExpr:
		op, ok = l.binaryExpressionOperation(expr.Operator, expr.Lhs, expr.Rhs)
	case *ast.UnaryMinusOpExpr:
		op, ok = l.unaryExpressionOperation("-", expr.Expr)
	case *ast.UnaryNotOpExpr:
		op, ok = l.unaryExpressionOperation("not", expr.Expr)
	case *ast.UnaryLenOpExpr:
		op, ok = l.unaryExpressionOperation("#", expr.Expr)
	case *ast.UnaryBNotOpExpr:
		op, ok = l.unaryExpressionOperation("~", expr.Expr)
	case *ast.CastExpr:
		l.addExpressionOperation(ref, expr.Expr)
		return
	case *ast.NonNilAssertExpr:
		l.addExpressionOperation(ref, expr.Expr)
		return
	}
	if !ok {
		return
	}
	if l.expressionOperations == nil {
		l.expressionOperations = make(map[factflow.ExprRef]factflow.ExpressionOperation)
	}
	l.expressionOperations[ref] = op
}

func (l *lowerer) binaryExpressionOperation(op string, leftExpr, rightExpr ast.Expr) (factflow.ExpressionOperation, bool) {
	left, ok := l.expressionOperandSource(leftExpr)
	if !ok {
		return factflow.ExpressionOperation{}, false
	}
	right, ok := l.expressionOperandSource(rightExpr)
	if !ok {
		return factflow.ExpressionOperation{}, false
	}
	return factflow.NewBinaryExpressionOperation(op, left, right)
}

func (l *lowerer) unaryExpressionOperation(op string, expr ast.Expr) (factflow.ExpressionOperation, bool) {
	operand, ok := l.expressionOperandSource(expr)
	if !ok {
		return factflow.ExpressionOperation{}, false
	}
	return factflow.NewUnaryExpressionOperation(op, operand)
}

func (l *lowerer) expressionOperandSource(expr ast.Expr) (factflow.ValueSource, bool) {
	if expr == nil {
		return factflow.ValueSource{}, false
	}
	source := sourceprovenance.SourceForExpr(expr, sourceprovenance.NoSourceIndex, sourceprovenance.NoSourceIndex, 0, true, false, l.callPointForExpr)
	return l.valueSource(source), true
}

func (l *lowerer) addExpressionFunction(ref factflow.ExprRef, expr ast.Expr) {
	fn, ok := expr.(*ast.FunctionExpr)
	if !ok || fn == nil || l.bindings == nil {
		return
	}
	id, ok := l.bindings.FunctionSymbol(fn)
	if !ok || id == 0 {
		return
	}
	if l.expressionFunctions == nil {
		l.expressionFunctions = make(map[factflow.ExprRef]symbol.ID)
	}
	l.expressionFunctions[ref] = id
}

func (l *lowerer) expressionValue(expr ast.Expr) (product.Value, bool) {
	if cast, ok := expr.(*ast.CastExpr); ok {
		if value, ok := l.concreteCastExpressionValue(cast); ok {
			return value, true
		}
	}
	if _, ok := sourceprovenance.ProofInner(expr); !ok {
		value := l.valueFromType(typ.Any)
		return product.Set(l.registry, value, assertion.Key, assertion.Any()), true
	}
	if t, ok := valueexpr.LiteralType(expr); ok {
		return l.valueFromTypeWithWitness(t), true
	}
	if fn, ok := expr.(*ast.FunctionExpr); ok {
		value := product.NewWithPresence(l.registry, product.ShapeTop, presence.Present())
		value = product.Set(l.registry, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
		if t, ok := l.functionExpressionType(fn); ok {
			value = l.valueFromTypeWithWitness(t)
		}
		if id, ok := l.functionIdentity(fn); ok {
			value = product.Set(l.registry, value, identity.Key, identity.Singleton(id))
		}
		return value, true
	}
	kind, ok := valueexpr.RuntimeKind(expr)
	if ok {
		value := product.NewWithPresence(l.registry, product.ShapeTop, presence.Present())
		return product.Set(l.registry, value, runtimekind.Key, kind), true
	}
	if ident, ok := expr.(*ast.IdentExpr); ok {
		if t, ok := l.identType(ident); ok {
			return l.valueFromTypeWithWitness(t), true
		}
	}
	if t, ok := l.indexExpressionType(expr); ok {
		return l.valueFromTypeWithWitness(t), true
	}
	if t, ok := l.scalarOperationType(expr); ok {
		value := l.valueFromTypeWithWitness(t)
		if logical, ok := expr.(*ast.LogicalOpExpr); ok {
			value = l.refineLogicalOperationValue(logical, value)
		}
		return value, true
	}
	return product.Value{}, false
}

func (l *lowerer) concreteCastExpressionValue(expr *ast.CastExpr) (product.Value, bool) {
	if expr == nil {
		return product.Value{}, false
	}
	if expr.Syntax != ast.CastSyntaxAs && expr.Syntax != ast.CastSyntaxColonColon {
		return product.Value{}, false
	}
	t, ok := l.resolveType(expr.Type)
	if !ok || t == nil {
		return product.Value{}, false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return product.Value{}, false
	}
	return l.valueFromTypeWithWitness(t), true
}

func (l *lowerer) refineLogicalOperationValue(expr *ast.LogicalOpExpr, value product.Value) product.Value {
	if expr == nil {
		return value
	}
	op, ok := l.binaryExpressionOperation(expr.Operator, expr.Lhs, expr.Rhs)
	if !ok {
		return value
	}
	leftType, leftOK := l.expressionOperandType(expr.Lhs)
	rightType, rightOK := l.expressionOperandType(expr.Rhs)
	if !leftOK || !rightOK {
		return value
	}
	left := l.valueFromTypeWithWitness(leftType)
	right := l.valueFromTypeWithWitness(rightType)
	return luasourcevalue.RefineLogicalOperationValue(l.registry, op, value, left, right)
}

func (l *lowerer) functionIdentity(fn *ast.FunctionExpr) (identity.ID, bool) {
	if fn == nil || l.bindings == nil {
		return identity.ID{}, false
	}
	id, ok := l.bindings.FunctionSymbol(fn)
	if !ok || id == 0 {
		return identity.ID{}, false
	}
	return identity.LuaFunction(uint64(id)), true
}

func (l *lowerer) functionExpressionType(fn *ast.FunctionExpr) (typ.Type, bool) {
	if fn == nil || l == nil {
		return nil, false
	}
	resolver := l.typeResolver
	if resolver == nil {
		resolver = typeresolve.New(l.bindings)
	}
	return functiontype.ValueExpression(fn, l.bindings, resolver)
}

func (l *lowerer) scalarOperationType(expr ast.Expr) (typ.Type, bool) {
	switch expr := expr.(type) {
	case *ast.ArithmeticOpExpr:
		return l.binaryOperationType(expr.Lhs, expr.Operator, expr.Rhs)
	case *ast.RelationalOpExpr:
		if expr.Operator == "==" || expr.Operator == "~=" {
			return typ.Boolean, true
		}
		return l.binaryOperationType(expr.Lhs, expr.Operator, expr.Rhs)
	case *ast.StringConcatOpExpr:
		return l.binaryOperationType(expr.Lhs, "..", expr.Rhs)
	case *ast.LogicalOpExpr:
		return l.binaryOperationType(expr.Lhs, expr.Operator, expr.Rhs)
	case *ast.UnaryMinusOpExpr:
		return l.unaryOperationType("-", expr.Expr)
	case *ast.UnaryNotOpExpr:
		return l.unaryOperationType("not", expr.Expr)
	case *ast.UnaryLenOpExpr:
		return l.unaryOperationType("#", expr.Expr)
	case *ast.UnaryBNotOpExpr:
		return l.unaryOperationType("~", expr.Expr)
	case *ast.CastExpr:
		return l.resolveType(expr.Type)
	case *ast.NonNilAssertExpr:
		return l.scalarOperationType(expr.Expr)
	default:
		return nil, false
	}
}

func (l *lowerer) expressionOperandType(expr ast.Expr) (typ.Type, bool) {
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	if ident, ok := expr.(*ast.IdentExpr); ok {
		return l.identType(ident)
	}
	if t, ok := l.indexExpressionType(expr); ok {
		return t, true
	}
	return l.scalarOperationType(expr)
}

func (l *lowerer) indexExpressionType(expr ast.Expr) (typ.Type, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr == nil || attr.Object == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := l.expressionOperandType(attr.Object)
	if !ok {
		return nil, false
	}
	key, ok := l.indexKeyType(attr)
	if !ok {
		return nil, false
	}
	return access.RuntimeIndex(container, key)
}

func (l *lowerer) indexKeyType(attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	switch key := attr.Key.(type) {
	case *ast.StringExpr:
		return typ.LiteralString(key.Value), true
	case *ast.NumberExpr:
		return valueexpr.LiteralType(key)
	case *ast.IdentExpr:
		if attr.KeySyntax == ast.AttrKeyDot {
			return typ.LiteralString(key.Value), true
		}
		return l.identType(key)
	default:
		return l.expressionOperandType(key)
	}
}

func (l *lowerer) identType(expr *ast.IdentExpr) (typ.Type, bool) {
	if l == nil || l.bindings == nil || expr == nil {
		return nil, false
	}
	id, ok := l.bindings.SymbolOf(expr)
	if !ok || id == 0 {
		return nil, false
	}
	t, ok := l.symbolTypes[id]
	if ok && t != nil {
		return t, true
	}
	exprType, ok := l.bindings.SymbolTypeAnnotation(id)
	if !ok {
		return nil, false
	}
	return l.resolveType(exprType)
}

func (l *lowerer) resolveType(expr ast.TypeExpr) (typ.Type, bool) {
	if l == nil {
		return nil, false
	}
	resolver := l.typeResolver
	if resolver == nil {
		resolver = typeresolve.New(l.bindings)
	}
	return resolver.Type(expr)
}

func (l *lowerer) resolveDecl(decl bind.TypeDecl) (typ.Type, bool) {
	if l == nil {
		return nil, false
	}
	resolver := l.typeResolver
	if resolver == nil {
		resolver = typeresolve.New(l.bindings)
	}
	return resolver.Decl(decl)
}

func (l *lowerer) binaryOperationType(leftExpr ast.Expr, op string, rightExpr ast.Expr) (typ.Type, bool) {
	left, ok := l.expressionOperandType(leftExpr)
	if !ok {
		return nil, false
	}
	right, ok := l.expressionOperandType(rightExpr)
	if !ok {
		return nil, false
	}
	return typeoperator.BinaryOp(left, op, right)
}

func (l *lowerer) unaryOperationType(op string, expr ast.Expr) (typ.Type, bool) {
	operand, ok := l.expressionOperandType(expr)
	if !ok {
		return nil, false
	}
	return typeoperator.UnaryOp(op, operand)
}
