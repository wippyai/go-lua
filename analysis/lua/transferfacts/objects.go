package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
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
	lowered := l.objectLiteral(fact)
	if len(lowered.Entries()) == 0 {
		return
	}
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
	if !ok || !reachesRecord(declared) {
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
	value := l.valueFromTypeWithWitness(declared)
	input.ObjectLiterals[exprRef] = lit.WithExpected(value)
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
	if !ok || declared == nil || !reachesRecord(declared) {
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
	value := l.valueFromTypeWithWitness(declared)
	input.ObjectLiterals[exprRef] = lit.WithExpected(value)
}

func reachesRecord(t typ.Type) bool {
	return reachesRecordDepth(t, 0)
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
