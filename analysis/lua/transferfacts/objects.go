package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
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
		l.addAssertionOverlaysForSource(input, entry.Source)
	}
}

func (l *lowerer) objectLiteral(fact semantics.ObjectLiteralFact) factflow.ObjectLiteral {
	entries := make([]factflow.ObjectEntry, 0, len(fact.Entries))
	for _, entry := range fact.Entries {
		entries = append(entries, factflow.NewObjectEntry(entry.Suffix, l.valueSource(entry.Source)))
	}
	return factflow.NewObjectLiteral(entries)
}
