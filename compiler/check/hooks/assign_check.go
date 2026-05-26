// assign_check.go implements assignment type validation for the type checker.
//
// This pass validates that values assigned to typed variables are compatible
// with their declared types. It runs after flow analysis, using narrowed types.
//
// # VALIDATION RULES
//
// For local declarations with type annotations:
//
//	local x: string = 123  -- ERROR: cannot assign number to string
//
// For reassignments to previously annotated variables:
//
//	local x: number
//	x = "hello"  -- ERROR: cannot assign string to number
//
// # TYPE GUARD INTERACTION
//
// When flow analysis has proven a variable excludes certain types at a point,
// assignments attempting to assign excluded types are rejected:
//
//	if x ~= nil then
//	    local y: T = x  -- uses narrowed type (non-nil)
//	end
//
// # TABLE LITERAL HANDLING
//
// Table literals are checked structurally against expected record/interface types
// using bidirectional type checking to infer field types from context.
package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/domain/provenance"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// CheckAssignments validates assignment type annotations.
func CheckAssignments(graph *cfg.Graph, evidence api.FlowEvidence, declared flow.DeclaredTypes, observer observation.Projector, flowOps api.FlowOps, sourceName string) []diag.Diagnostic {
	if graph == nil {
		return nil
	}

	annotated := make(map[cfg.SymbolID]bool)
	assigned := make(map[cfg.SymbolID]bool)
	for _, assign := range evidence.Assignments {
		p := assign.Point
		if flowOps != nil && flowOps.IsPointDead(p) {
			continue
		}
		info := assign.Info
		if info == nil {
			continue
		}
		if info.IsLocal {
			for i, ann := range info.TypeAnnotations {
				if ann == nil {
					continue
				}
				if i >= len(info.Targets) {
					continue
				}
				target := info.Targets[i]
				if target.Kind != cfg.TargetIdent || target.Name == "" {
					continue
				}
				sym := target.Symbol
				if sym == 0 {
					continue
				}
				annotated[sym] = true
			}
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			if source != nil {
				assigned[target.Symbol] = true
			}
		})
	}
	for _, def := range evidence.FunctionDefinitions {
		if def.Symbol != 0 {
			assigned[def.Symbol] = true
		}
	}

	var diags []diag.Diagnostic

	for _, assign := range evidence.Assignments {
		p := assign.Point
		if flowOps != nil && flowOps.IsPointDead(p) {
			continue
		}
		info := assign.Info
		if info == nil {
			continue
		}
		info.EachTargetSource(func(i int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent {
				if d, ok := checkStructuredAssignmentTarget(target, source, p, observer, graph, evidence, sourceName); ok {
					diags = append(diags, d)
				}
				return
			}
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				return
			}

			sym := target.Symbol

			var declaredType typ.Type
			if sym != 0 {
				if info.IsLocal && info.TypeAnnotationAt(i) != nil {
					declaredType = declared[sym]
				} else if !info.IsLocal && annotated[sym] {
					declaredType = declared[sym]
				}
			}

			if typ.IsAbsentOrUnknown(declaredType) {
				return
			}

			if info.IsLocal && info.TypeAnnotationAt(i) != nil && source == nil {
				if isVarArgReturnExpr(info.LastSource()) {
					return
				}
				if sym != 0 && assigned[sym] {
					return
				}
				if !subtype.Consistent(typ.Nil, declaredType) {
					var posNode ast.PositionHolder
					if target.Expr != nil {
						posNode = target.Expr
					} else if info.Stmt != nil {
						posNode = info.Stmt
					}
					if posNode != nil {
						pos := diag.Position{File: sourceName, Line: posNode.Line(), Column: posNode.Column()}
						span := ast.SpanOf(posNode)
						msg := formatAssignMismatch(typ.Nil, declaredType)
						_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
						diags = append(diags, diag.Diagnostic{
							Severity: diag.SeverityError,
							Code:     diag.ErrTypeMismatch,
							Position: pos,
							Span:     span,
							Message:  msg,
							Help:     help,
						})
					}
				}
				return
			}

			if source == nil {
				return
			}

			valueType := observer.AssignmentSourceType(source, p, declaredType, sym)
			if valueType == nil {
				return
			}

			if table, ok := source.(*ast.TableExpr); ok {
				if result := observer.AssignmentSourceTableCheck(table, p, declaredType, sym); result.Handled {
					if result.Compatible {
						return
					}
					pos := diag.Position{File: sourceName, Line: source.Line(), Column: source.Column()}
					span := ast.SpanOf(source)
					msg := formatAssignMismatchDetailed(valueType, declaredType, result.Reason)
					_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
					diags = append(diags, diag.Diagnostic{
						Severity: diag.SeverityError,
						Code:     diag.ErrTypeMismatch,
						Position: pos,
						Span:     span,
						Message:  msg,
						Help:     help,
					})
					return
				}
			}

			if observer.ExcludesExprTypeAt(p, source, declaredType) {
				pos := diag.Position{File: sourceName, Line: source.Line(), Column: source.Column()}
				span := ast.SpanOf(source)
				msg := formatExcluded(valueType, declaredType)
				_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
				diags = append(diags, diag.Diagnostic{
					Severity: diag.SeverityError,
					Code:     diag.ErrTypeMismatch,
					Position: pos,
					Span:     span,
					Message:  msg,
					Help:     help,
				})
				return
			}

			if !subtype.Consistent(valueType, declaredType) {
				pos := diag.Position{File: sourceName, Line: source.Line(), Column: source.Column()}
				span := ast.SpanOf(source)
				msg := formatAssignMismatch(valueType, declaredType)
				_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
				diags = append(diags, diag.Diagnostic{
					Severity: diag.SeverityError,
					Code:     diag.ErrTypeMismatch,
					Position: pos,
					Span:     span,
					Message:  msg,
					Help:     help,
				})
			}
		})
	}

	return diags
}

func checkStructuredAssignmentTarget(target cfg.AssignTarget, source ast.Expr, p cfg.Point, observer observation.Projector, graph *cfg.Graph, evidence api.FlowEvidence, sourceName string) (diag.Diagnostic, bool) {
	if source == nil {
		return diag.Diagnostic{}, false
	}
	expected := observer.AssignmentTargetWriteType(target, source, p)
	if typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
		return diag.Diagnostic{}, false
	}

	valueType := observer.AssignmentSourceType(source, p, expected, target.Symbol)
	if typ.IsAbsentOrUnknown(valueType) {
		return diag.Diagnostic{}, false
	}
	if value.IsNilOnly(valueType) && observer.AssignmentTargetDeleteAllowed(target, p) {
		return diag.Diagnostic{}, false
	}
	if table, ok := source.(*ast.TableExpr); ok {
		if result := observer.AssignmentSourceTableCheck(table, p, expected, target.Symbol); result.Handled {
			if result.Compatible {
				return diag.Diagnostic{}, false
			}
			pos := diag.Position{File: sourceName, Line: source.Line(), Column: source.Column()}
			span := ast.SpanOf(source)
			msg := formatAssignMismatchDetailed(valueType, expected, result.Reason)
			_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
			return diag.Diagnostic{
				Severity: diag.SeverityError,
				Code:     diag.ErrTypeMismatch,
				Position: pos,
				Span:     span,
				Message:  msg,
				Help:     help,
			}, true
		}
	}
	var bindings provenance.IdentBindingLookup
	if graph != nil {
		bindings = graph.Bindings()
	}
	if fresh, ok := provenance.CurrentFreshTableLiteral(source, p, bindings, evidence.FreshTableLiterals); ok {
		if result := observer.CheckTable(fresh.Table, fresh.Point, expected); result.Handled && result.Compatible {
			return diag.Diagnostic{}, false
		}
	}
	if subtype.Consistent(valueType, expected) {
		return diag.Diagnostic{}, false
	}

	pos := diag.Position{File: sourceName, Line: source.Line(), Column: source.Column()}
	span := ast.SpanOf(source)
	msg := formatAssignMismatch(valueType, expected)
	_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
	return diag.Diagnostic{
		Severity: diag.SeverityError,
		Code:     diag.ErrTypeMismatch,
		Position: pos,
		Span:     span,
		Message:  msg,
		Help:     help,
	}, true
}

func isVarArgReturnExpr(expr ast.Expr) bool {
	switch ex := expr.(type) {
	case *ast.FuncCallExpr:
		return !ex.AdjustRet
	case *ast.Comma3Expr:
		return !ex.AdjustRet
	}
	return false
}

func formatExcluded(value, declared typ.Type) string {
	return "cannot assign " + typ.FormatShort(value) + " to " + typ.FormatShort(declared) + " (type excluded by guard)"
}

func formatAssignMismatch(value, declared typ.Type) string {
	return "cannot assign " + typ.FormatShort(value) + " to " + typ.FormatShort(declared)
}

func formatAssignMismatchDetailed(value, declared typ.Type, reason string) string {
	msg := formatAssignMismatch(value, declared)
	if reason == "" {
		return msg
	}
	return msg + ": " + reason
}
