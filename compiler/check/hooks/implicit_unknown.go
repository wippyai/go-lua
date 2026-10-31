package hooks

import (
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

// implicitUnknownHint reports a value known only as unknown flowing into a
// declared type. Gradual assignability accepts it, as it accepts any; the hint
// keeps the implicit boundary visible.
func implicitUnknownHint(pos diag.Position, span diag.Span, prefix string, declared typ.Type) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.SeverityHint,
		Code:     diag.HintImplicitUnknown,
		Position: pos,
		Span:     span,
		Message:  prefix + "implicit unknown flows into declared " + typ.FormatShort(declared),
	}
}

// implicitUnknownCallHint reports an argument known only as unknown, with the
// message the call check composed.
func implicitUnknownCallHint(pos diag.Position, span diag.Span, msg string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.SeverityHint,
		Code:     diag.HintImplicitUnknown,
		Position: pos,
		Span:     span,
		Message:  msg,
	}
}
