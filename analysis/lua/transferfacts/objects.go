package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) addObjectLiteral(input *factflow.FactsInput, result *semantics.Result, source sourceprovenance.ASTSource) {
	if source.Kind != sourceprovenance.SourceExpression || source.Expr == nil {
		return
	}
	l.addObjectLiteralExpr(input, result, source.Expr)
}

func (l *lowerer) addObjectLiteralExpr(input *factflow.FactsInput, result *semantics.Result, expr ast.Expr) {
	if expr == nil {
		return
	}
	if result == nil {
		return
	}
	fact, ok := result.ObjectLiteral(expr)
	if ok {
		exprRef, hasExpr := l.exprRef(fact.Expr)
		if !hasExpr {
			return
		}
		lowered := l.objectLiteral(fact).WithIdentity(identity.LuaTableLiteral(l.graphID, uint64(exprRef)))
		if expected, ok := l.objectLiteralCastExpectedType(expr); ok {
			lowered = l.objectLiteralWithExpectedType(lowered, expected)
		}
		if input.ObjectLiterals == nil {
			input.ObjectLiterals = make(map[factflow.ExprRef]factflow.ObjectLiteral)
		}
		input.ObjectLiterals[exprRef] = lowered
		for _, entry := range fact.Entries {
			l.addAssertionRefinementsForSource(input, entry.Source)
			l.addObjectLiteral(input, result, entry.Source)
		}
		return
	}
	for _, child := range objectLiteralChildExprs(expr) {
		l.addObjectLiteralExpr(input, result, child)
	}
}

func objectLiteralChildExprs(expr ast.Expr) []ast.Expr {
	switch e := expr.(type) {
	case *ast.LogicalOpExpr:
		return []ast.Expr{e.Lhs, e.Rhs}
	case *ast.RelationalOpExpr:
		return []ast.Expr{e.Lhs, e.Rhs}
	case *ast.StringConcatOpExpr:
		return []ast.Expr{e.Lhs, e.Rhs}
	case *ast.ArithmeticOpExpr:
		return []ast.Expr{e.Lhs, e.Rhs}
	case *ast.UnaryMinusOpExpr:
		return []ast.Expr{e.Expr}
	case *ast.UnaryNotOpExpr:
		return []ast.Expr{e.Expr}
	case *ast.UnaryLenOpExpr:
		return []ast.Expr{e.Expr}
	case *ast.UnaryBNotOpExpr:
		return []ast.Expr{e.Expr}
	case *ast.CastExpr:
		return []ast.Expr{e.Expr}
	case *ast.NonNilAssertExpr:
		return []ast.Expr{e.Expr}
	case *ast.AttrGetExpr:
		return []ast.Expr{e.Object, e.Key}
	case *ast.FuncCallExpr:
		out := make([]ast.Expr, 0, 2+len(e.Args))
		out = append(out, e.Func, e.Receiver)
		out = append(out, e.Args...)
		return out
	case *ast.TableExpr:
		out := make([]ast.Expr, 0, len(e.Fields)*2)
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			out = append(out, field.Key, field.Value)
		}
		return out
	default:
		return nil
	}
}

func (l *lowerer) objectLiteralCastExpectedType(expr ast.Expr) (typ.Type, bool) {
	cast, ok := expr.(*ast.CastExpr)
	if !ok || !tableConstructorExpr(cast.Expr) {
		return nil, false
	}
	expected, ok := l.resolveType(cast.Type)
	if !ok || expected == nil || !luatypeprojection.ReachesTableContract(expected) {
		return nil, false
	}
	return expected, true
}

// addObjectLiteralExpectedType attaches the declared record type of an annotated
// local to the object literal sidecar at its constructor source, so the body
// evaluator can fill literal fields that are otherwise untypeable from that
// record. It only fires when the declared type resolves to a record (directly or
// through Alias/Recursive/Instantiated wrappers).
func (l *lowerer) addObjectLiteralExpectedType(input *factflow.FactsInput, fact semantics.LocalAssignmentFact) {
	if fact.Source.Kind != sourceprovenance.SourceExpression {
		return
	}
	declared, ok := l.localObjectLiteralExpectedType(fact)
	if !ok || !luatypeprojection.ReachesTableContract(declared) {
		return
	}
	l.addResultObjectLiteralExpectedTypes(input, fact.Source.Expr, declared)
}

func (l *lowerer) localObjectLiteralExpectedType(fact semantics.LocalAssignmentFact) (typ.Type, bool) {
	if fact.Type != nil {
		return l.resolveType(fact.Type)
	}
	return l.returnLocalObjectLiteralContract(fact)
}

// addOrdinaryObjectLiteralExpectedType attaches the declared record type of an
// assignment target to the object literal sidecar when an annotated local is
// re-assigned a table constructor (target = {...}). The declared symbol type is
// the checked contract for the local, so the literal's fields adopt that record's
// field types rather than their narrow inferred literal types.
func (l *lowerer) addOrdinaryObjectLiteralExpectedType(input *factflow.FactsInput, fact semantics.OrdinaryAssignmentFact) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return
	}
	if fact.Source.Kind != sourceprovenance.SourceExpression {
		return
	}
	if !tableConstructorExpr(fact.Source.Expr) {
		return
	}
	declared, ok := l.symbolTypes[fact.Symbol]
	if !ok || declared == nil || !luatypeprojection.ReachesTableContract(declared) {
		return
	}
	exprRef, hasExpr := l.exprRef(fact.Source.Expr)
	if !hasExpr {
		return
	}
	if input.ObjectLiterals == nil {
		return
	}
	lit, ok := input.ObjectLiterals[exprRef]
	if !ok {
		return
	}
	input.ObjectLiterals[exprRef] = l.objectLiteralWithExpectedType(lit, declared)
}

func (l *lowerer) addDynamicIndexObjectLiteralExpectedTypes(input *factflow.FactsInput, fact semantics.OrdinaryAssignmentFact) {
	if fact.Source.Kind != sourceprovenance.SourceExpression || fact.Source.Expr == nil {
		return
	}
	expected, ok := l.dynamicIndexWriteValueType(fact)
	if !ok || !luatypeprojection.ReachesTableContract(expected) {
		return
	}
	l.addResultObjectLiteralExpectedTypes(input, fact.Source.Expr, expected)
}

func (l *lowerer) dynamicIndexWriteValueType(fact semantics.OrdinaryAssignmentFact) (typ.Type, bool) {
	expected, ok := l.indexExpressionType(fact.Target)
	if ok && expected != nil {
		if unwrapped := unwrap.Optional(expected); unwrapped != nil {
			expected = unwrapped
		}
		return expected, expected != nil
	}
	tablePath, ok := l.directDynamicIndexWriteTablePath(fact.Target)
	if !ok {
		return nil, false
	}
	container, ok := l.aliasPathType(tablePath)
	if !ok {
		return nil, false
	}
	return dynamicIndexMapValueType(container, 0)
}

func dynamicIndexMapValueType(container typ.Type, depth int) (typ.Type, bool) {
	if container == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch t := unwrap.Alias(container).(type) {
	case *typ.Map:
		return t.Value, t.Value != nil
	case *typ.ReadonlyMap:
		return t.Value, t.Value != nil
	case *typ.Record:
		if t.HasMapComponent() {
			return t.MapValue, t.MapValue != nil
		}
		return nil, false
	case *typ.Union:
		values := make([]typ.Type, 0, len(t.Members))
		for _, member := range t.Members {
			value, ok := dynamicIndexMapValueType(member, depth+1)
			if !ok {
				return nil, false
			}
			values = append(values, value)
		}
		return normalize.UnionForEvidence(values...), len(values) != 0
	default:
		return nil, false
	}
}

func (l *lowerer) addResultObjectLiteralExpectedTypes(input *factflow.FactsInput, expr ast.Expr, expected typ.Type) {
	if input == nil || input.ObjectLiterals == nil || expr == nil || expected == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.TableExpr:
		ref, ok := l.existingExprRef(e)
		if !ok {
			return
		}
		lit, ok := input.ObjectLiterals[ref]
		if !ok {
			return
		}
		input.ObjectLiterals[ref] = l.objectLiteralWithExpectedType(lit, expected)
	case *ast.LogicalOpExpr:
		l.addResultObjectLiteralExpectedTypes(input, e.Lhs, expected)
		l.addResultObjectLiteralExpectedTypes(input, e.Rhs, expected)
	case *ast.CastExpr:
		l.addResultObjectLiteralExpectedTypes(input, e.Expr, expected)
	case *ast.NonNilAssertExpr:
		l.addResultObjectLiteralExpectedTypes(input, e.Expr, expected)
	}
}

func (l *lowerer) existingExprRef(expr any) (factflow.ExprRef, bool) {
	if l == nil || expr == nil || l.exprs == nil {
		return 0, false
	}
	ref, ok := l.exprs[expr]
	return ref, ok
}

func (l *lowerer) addReturnObjectLiteralExpectedTypes(input *factflow.FactsInput, result *semantics.Result, fact semantics.ReturnFact) {
	if input == nil || result == nil {
		return
	}
	declared := declaredReturnTypes(result)
	if len(declared) == 0 || len(fact.Sources) == 0 {
		return
	}
	for i, source := range fact.Sources {
		if i >= len(declared) || source.Kind != sourceprovenance.SourceExpression || !tableConstructorExpr(source.Expr) {
			continue
		}
		declaredType, ok := l.resolveType(declared[i])
		if !ok || !luatypeprojection.ReachesTableContract(declaredType) {
			continue
		}
		exprRef, hasExpr := l.exprRef(source.Expr)
		if !hasExpr {
			continue
		}
		lit, ok := input.ObjectLiterals[exprRef]
		if !ok {
			continue
		}
		input.ObjectLiterals[exprRef] = l.objectLiteralWithExpectedType(lit, declaredType)
	}
}

// addObjectLiteralFieldExposures records covariant exposures for object-literal
// entries that store an aliased narrow object into a declared container slot
// (holder = {ref = narrow}, sink = {narrow}) whose declared slot type strictly
// widens the stored object. The container retains the stored object at the slot,
// so a write through the wider slot view can launder a wide value back into the
// source object; the source object is widened to the slot's declared contract.
func (l *lowerer) addObjectLiteralFieldExposures(input *factflow.FactsInput, result *semantics.Result, point cfg.Point, source sourceprovenance.ASTSource, declared typ.Type) {
	if source.Kind != sourceprovenance.SourceExpression || declared == nil {
		return
	}
	if typ.IsAny(declared) || typ.IsUnknown(declared) {
		return
	}
	fact, ok := result.ObjectLiteral(source.Expr)
	if !ok {
		return
	}
	for _, entry := range fact.Entries {
		slotType, ok := luatypeprojection.ApplySegments(declared, entry.Suffix.Segments)
		if !ok || slotType == nil {
			continue
		}
		l.addAliasExposureToContractType(input, point, entry.Source, slotType)
	}
}

func (l *lowerer) objectLiteral(fact semantics.ObjectLiteralFact) factflow.ObjectLiteral {
	entries := make([]factflow.ObjectEntry, 0, len(fact.Entries))
	for _, entry := range fact.Entries {
		entries = append(entries, factflow.NewObjectEntryWithMetadata(
			entry.Suffix,
			l.valueSource(entry.Source),
			sourceSpan(entry.ValueSpan),
			entry.ValueLabel,
		))
	}
	return factflow.NewObjectLiteral(entries)
}

func sourceSpan(span semantics.SourceSpan) factflow.SourceSpan {
	return factflow.SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func (l *lowerer) objectLiteralWithExpectedType(lit factflow.ObjectLiteral, declared typ.Type) factflow.ObjectLiteral {
	rootType := luatypeprojection.PresentConstructorRoot(declared)
	root := l.valueFromTypeWithWitness(rootType)
	entries := lit.Entries()
	for i, entry := range entries {
		projected, ok := luatypeprojection.ApplyConstructorSegments(rootType, entry.Suffix().Segments)
		if !ok || projected == nil {
			continue
		}
		entries[i] = entry.WithExpected(l.valueFromTypeWithWitness(projected))
	}
	out := factflow.NewObjectLiteral(entries).WithExpected(root)
	if id, ok := lit.Identity(); ok {
		out = out.WithIdentity(id)
	}
	return out
}
