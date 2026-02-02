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
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// CheckAssignments validates assignment type annotations.
func CheckAssignments(graph *cfg.Graph, scopes map[cfg.Point]*scope.State, narrowSynth api.Synth, flowQ api.FlowQuery, sourceName string) []diag.Diagnostic {
	if graph == nil || narrowSynth == nil {
		return nil
	}

	annotated := make(map[cfg.SymbolID]typ.Type)
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if !info.IsLocal {
			return
		}
		sc := scopes[p]
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
			if declaredType != nil && declaredType.Kind() != typ.Unknown.Kind() {
				annotated[sym] = declaredType
			}
		}
	})

	var diags []diag.Diagnostic

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		sc := scopes[p]

		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				continue
			}

			sym := target.Symbol

			var declaredType typ.Type
			if info.IsLocal && i < len(info.TypeAnnotations) && info.TypeAnnotations[i] != nil {
				declaredType = narrowSynth.ResolveType(info.TypeAnnotations[i], sc)
			} else if !info.IsLocal && sym != 0 {
				declaredType = annotated[sym]
			}

			if declaredType == nil || declaredType.Kind() == typ.Unknown.Kind() {
				continue
			}

			if i >= len(info.Sources) {
				continue
			}
			source := info.Sources[i]
			if source == nil {
				continue
			}

			valueType := narrowSynth.SynthWithExpected(source, p, declaredType)
			if valueType == nil {
				continue
			}

			if table, ok := source.(*ast.TableExpr); ok {
				if result := tableCheck(table, declaredType, narrowSynth, p); result.Handled {
					if result.Compatible {
						continue
					}
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
					continue
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
						continue
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
					continue
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
		}
	})

	return diags
}

func extractSourcePath(source ast.Expr, graph *cfg.Graph, _ cfg.Point) constraint.Path {
	if graph == nil {
		return constraint.Path{}
	}
	return path.FromExprWithBindings(source, nil, graph.Bindings())
}

func formatExcluded(value, declared typ.Type) string {
	return "cannot assign " + typ.FormatShort(value) + " to " + typ.FormatShort(declared) + " (type excluded by guard)"
}

func formatAssignMismatch(value, declared typ.Type) string {
	return "cannot assign " + typ.FormatShort(value) + " to " + typ.FormatShort(declared)
}
