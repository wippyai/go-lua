package body

import "github.com/wippyai/go-lua/compiler/ast"

// OrdinaryAssignmentTargetSpan returns the syntax span of an ordinary
// assignment target. Body owns this projection so readmodel consumers do not
// inspect AST nodes just to render mutation evidence.
func OrdinaryAssignmentTargetSpan(fact OrdinaryAssignmentFact) (SourceSpan, bool) {
	if fact.Target == nil {
		return SourceSpan{}, false
	}
	return sourceSpanFromAST(ast.SpanOf(fact.Target)), true
}
