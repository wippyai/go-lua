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
	"github.com/wippyai/go-lua/compiler/check/domain/iteration"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/provenance"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/join"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// CheckAssignments validates assignment type annotations.
func CheckAssignments(graph *cfg.Graph, evidence api.FlowEvidence, scopes map[cfg.Point]*scope.State, narrowSynth api.Synth, flowQ api.FlowQuery, sourceName string) []diag.Diagnostic {
	if graph == nil || narrowSynth == nil {
		return nil
	}

	annotated := make(map[cfg.SymbolID]typ.Type)
	assigned := make(map[cfg.SymbolID]bool)
	for _, assign := range evidence.Assignments {
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
		}
		sc := scopes[p]
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
				declaredType := narrowSynth.ResolveType(ann, sc)
				if !typ.IsAbsentOrUnknown(declaredType) {
					annotated[sym] = declaredType
				}
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
		info := assign.Info
		if info == nil {
			continue
		}
		sc := scopes[p]

		info.EachTargetSource(func(i int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent {
				if d, ok := checkStructuredAssignmentTarget(target, source, p, narrowSynth, flowQ, graph, evidence, sourceName); ok {
					diags = append(diags, d)
				}
				return
			}
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				return
			}

			sym := target.Symbol

			var declaredType typ.Type
			if ann := info.TypeAnnotationAt(i); info.IsLocal && ann != nil {
				declaredType = narrowSynth.ResolveType(ann, sc)
			} else if !info.IsLocal && sym != 0 {
				declaredType = annotated[sym]
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
				if !subtype.IsSubtype(typ.Nil, declaredType) {
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

			sourceUsesTarget := sourceUsesTargetSymbol(source, sym, graph)
			valueType := narrowSynth.SynthWithExpected(source, p, declaredType)
			if sourceUsesTarget {
				if pre := preAssignmentExprTypeForAssign(source, p, narrowSynth, graph, declaredType); pre != nil {
					if !typ.IsAbsentOrUnknown(pre) {
						valueType = pre
					}
				}
			}
			if valueType == nil {
				return
			}
			if flowQ != nil {
				sourcePath := extractSourcePath(source, graph, p)
				if !sourcePath.IsEmpty() {
					if narrowed := flowQ.NarrowedTypeAt(p, sourcePath); !typ.IsAbsentOrUnknown(narrowed) {
						valueType = preferPreciseSourcePathType(valueType, narrowed)
					}
				}
			}

			if table, ok := source.(*ast.TableExpr); ok && !sourceUsesTarget {
				if result := tableCheck(table, declaredType, narrowSynth, p); result.Handled {
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

			if flowQ != nil {
				if srcPath := extractSourcePath(source, graph, p); !srcPath.IsEmpty() && srcPath.Symbol != 0 {
					tv := flowQ.EffectiveTypeAt(p, srcPath.Symbol)
					if tv.State == flow.StateUnknown {
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
						return
					}
				}
			}

			if flowQ != nil {
				sourcePath := extractSourcePath(source, graph, p)
				if !sourcePath.IsEmpty() && flowQ.ExcludesTypeAt(p, sourcePath, declaredType) {
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
			}

			if !subtype.IsSubtype(valueType, declaredType) {
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

func checkStructuredAssignmentTarget(target cfg.AssignTarget, source ast.Expr, p cfg.Point, synth api.Synth, flowQ api.FlowQuery, graph *cfg.Graph, evidence api.FlowEvidence, sourceName string) (diag.Diagnostic, bool) {
	if source == nil || synth == nil {
		return diag.Diagnostic{}, false
	}
	expected := assignmentTargetWriteType(target, p, synth, flowQ)
	if iteratorExpected := pairedIteratorWriteExpected(target, p, flowQ, graph, evidence.Assignments); iteratorExpected != nil && nilOnlyType(expected) {
		expected = iteratorExpected
	}
	if typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
		return diag.Diagnostic{}, false
	}

	valueType := synth.SynthWithExpected(source, p, expected)
	if valueType == nil {
		valueType = synth.TypeOf(source, p)
	}
	if typ.IsAbsentOrUnknown(valueType) {
		return diag.Diagnostic{}, false
	}
	if nilOnlyType(valueType) && assignmentTargetDeleteAllowed(target, p, synth) {
		return diag.Diagnostic{}, false
	}
	if flowQ != nil {
		sourcePath := extractSourcePath(source, graph, p)
		if !sourcePath.IsEmpty() {
			if narrowed := flowQ.NarrowedTypeAt(p, sourcePath); !typ.IsAbsentOrUnknown(narrowed) {
				valueType = preferPreciseSourcePathType(valueType, narrowed)
			}
		}
	}
	if table, ok := source.(*ast.TableExpr); ok {
		if result := tableCheck(table, expected, synth, p); result.Handled {
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
		if result := tableCheck(fresh.Table, expected, synth, fresh.Point); result.Handled && result.Compatible {
			return diag.Diagnostic{}, false
		}
	}
	if subtype.IsSubtype(valueType, expected) {
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

func assignmentTargetWriteType(target cfg.AssignTarget, p cfg.Point, synth api.Synth, flowQ api.FlowQuery) typ.Type {
	switch target.Kind {
	case cfg.TargetIndex:
		if target.Base == nil {
			return nil
		}
		objType := synth.TypeOf(target.Base, p)
		keyType := typ.Type(nil)
		if target.Key != nil {
			keyType = synth.TypeOf(target.Key, p)
		}
		if expected, ok := querycore.IndexWrite(objType, keyType); ok {
			return expected
		}
		return nil
	default:
		return nil
	}
}

func assignmentTargetDeleteAllowed(target cfg.AssignTarget, p cfg.Point, synth api.Synth) bool {
	if target.Kind != cfg.TargetIndex || target.Base == nil || synth == nil {
		return false
	}
	objType := synth.TypeOf(target.Base, p)
	keyType := typ.Type(nil)
	if target.Key != nil {
		keyType = synth.TypeOf(target.Key, p)
	}
	return querycore.IndexDelete(objType, keyType)
}

func pairedIteratorWriteExpected(target cfg.AssignTarget, p cfg.Point, flowQ api.FlowQuery, graph *cfg.Graph, assignments []api.AssignmentEvidence) typ.Type {
	if target.Kind != cfg.TargetIndex || target.Base == nil || flowQ == nil || graph == nil {
		return nil
	}
	key, ok := target.Key.(*ast.IdentExpr)
	if !ok {
		return nil
	}
	pair, ok := iteration.FindKeyedPairValue(graph, assignments, target.Base, key)
	if !ok {
		return nil
	}
	valueType := flowQ.NarrowedTypeAt(p, pair.ValuePath)
	if typ.IsAbsentOrUnknown(valueType) && pair.ValuePath.Symbol != 0 {
		valueType = flowQ.EffectiveTypeAt(p, pair.ValuePath.Symbol).Type
	}
	if typ.IsAbsentOrUnknown(valueType) || typ.IsAny(valueType) {
		return nil
	}
	return valueType
}

func nilOnlyType(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return nilOnlyType(v.Target)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !nilOnlyType(member) {
				return false
			}
		}
		return true
	default:
		return v != nil && v.Kind() == typ.Nil.Kind()
	}
}

func preferPreciseSourcePathType(current, narrowed typ.Type) typ.Type {
	if typ.IsAbsentOrUnknown(current) {
		return narrowed
	}
	if subtype.IsSubtype(narrowed, current) {
		return narrowed
	}
	return current
}

func extractSourcePath(source ast.Expr, graph *cfg.Graph, _ cfg.Point) constraint.Path {
	if graph == nil {
		return constraint.Path{}
	}
	return path.FromExprWithBindings(source, nil, graph.Bindings())
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

func sourceUsesTargetSymbol(expr ast.Expr, sym cfg.SymbolID, graph *cfg.Graph) bool {
	if expr == nil || sym == 0 || graph == nil {
		return false
	}
	bindings := graph.Bindings()
	if bindings == nil {
		return false
	}
	return provenance.ExprReferencesSymbol(expr, sym, bindings)
}

func preAssignmentExprTypeForAssign(expr ast.Expr, p cfg.Point, synth api.Synth, graph *cfg.Graph, expected typ.Type) typ.Type {
	if expr == nil || synth == nil || graph == nil {
		return nil
	}
	preds := graph.Predecessors(p)
	if len(preds) == 0 {
		return nil
	}
	var candidate []typ.Type
	for _, pred := range preds {
		if t := synth.SynthWithExpected(expr, pred, expected); t != nil {
			candidate = append(candidate, t)
		}
	}
	if len(candidate) == 0 {
		return nil
	}
	return join.Types(candidate...)
}
