package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (l *lowerer) addObjectLiteral(input *factflow.FactsInput, result *semantics.Result, source sourceprovenance.ASTSource) {
	fact, ok := result.ObjectLiteral(source.Expr)
	if !ok {
		return
	}
	exprRef, hasExpr := l.exprRef(fact.Expr)
	if !hasExpr {
		return
	}
	lowered := l.objectLiteral(fact).WithIdentity(identity.LuaTableLiteral(l.graphID, uint64(exprRef)))
	if input.ObjectLiterals == nil {
		input.ObjectLiterals = make(map[factflow.ExprRef]factflow.ObjectLiteral)
	}
	input.ObjectLiterals[exprRef] = lowered
	for _, entry := range fact.Entries {
		l.addAssertionRefinementsForSource(input, entry.Source)
		l.addObjectLiteral(input, result, entry.Source)
	}
}

// addObjectLiteralExpectedType attaches the declared record type of an annotated
// local to the object literal sidecar at its constructor source, so the body
// evaluator can fill literal fields that are otherwise untypeable from that
// record. It only fires when the declared type resolves to a record (directly or
// through Alias/Recursive/Instantiated wrappers).
func (l *lowerer) addObjectLiteralExpectedType(input *factflow.FactsInput, fact semantics.LocalAssignmentFact) {
	if fact.Type == nil || fact.Source.Kind != sourceprovenance.SourceExpression {
		return
	}
	if !tableConstructorExpr(fact.Expr) {
		return
	}
	declared, ok := l.resolveType(fact.Type)
	if !ok || !reachesTableContract(declared) {
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
	if !ok || declared == nil || !reachesTableContract(declared) {
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
		if !ok || !reachesTableContract(declaredType) {
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

func reachesRecord(t typ.Type) bool {
	return reachesRecordDepth(t, 0)
}

func reachesTableContract(t typ.Type) bool {
	return reachesTableContractDepth(t, 0)
}

func reachesTableContractDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
		return true
	case *typ.Alias:
		return reachesTableContractDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return reachesTableContractDepth(v.Body, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return false
		}
		return reachesTableContractDepth(expanded, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if reachesTableContractDepth(member, depth+1) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func reachesRecordDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record:
		return true
	case *typ.Alias:
		return reachesRecordDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return reachesRecordDepth(v.Body, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return false
		}
		return reachesRecordDepth(expanded, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if reachesRecordDepth(member, depth+1) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (l *lowerer) objectLiteral(fact semantics.ObjectLiteralFact) factflow.ObjectLiteral {
	entries := make([]factflow.ObjectEntry, 0, len(fact.Entries))
	for _, entry := range fact.Entries {
		entries = append(entries, factflow.NewObjectEntry(entry.Suffix, l.valueSource(entry.Source)))
	}
	return factflow.NewObjectLiteral(entries)
}

func (l *lowerer) objectLiteralWithExpectedType(lit factflow.ObjectLiteral, declared typ.Type) factflow.ObjectLiteral {
	root := l.valueFromTypeWithWitness(declared)
	entries := lit.Entries()
	for i, entry := range entries {
		projected, ok := luatypeprojection.ApplySegments(declared, entry.Suffix().Segments)
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
