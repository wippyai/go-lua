package canonical

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/diag"
)

func (d *Driver) emitScopeDepthDiagnostic(sess api.AnalysisSession, fn *ast.FunctionExpr, result *api.FuncResult) {
	if d == nil || sess == nil || result == nil || !d.cfg.EmitScopeDiag || d.cfg.MaxScopeDepth <= 0 || !result.DepthLimitExceeded {
		return
	}
	scopeState := sess.ScopeDepthDiagState()
	if scopeState[fn] {
		return
	}
	pos := diag.Position{File: sess.Source()}
	span := diag.Span{}
	if fn != nil && fn.Line() > 0 {
		pos.Line = fn.Line()
		pos.Column = fn.Column()
		span.StartLine = fn.Line()
		span.StartCol = fn.Column()
		span.EndLine = fn.LastLine()
		span.EndCol = fn.LastColumn()
	}
	sess.AppendDiagnostics(diag.Diagnostic{
		Position: pos,
		Span:     span,
		Severity: diag.SeverityWarning,
		Message:  fmt.Sprintf("scope depth limit exceeded (max=%d); analysis may be incomplete", d.cfg.MaxScopeDepth),
	})
	scopeState[fn] = true
}
