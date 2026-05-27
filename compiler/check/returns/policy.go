package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	synthops "github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/typ"
)

// SourceParamReceivesCallEvidence reports whether observed call arguments may
// enter the body-effective parameter-evidence channel for a source parameter.
// Hard annotations are authoritative contracts; bad calls must be rejected by
// diagnostics, not folded back into the callee.
func SourceParamReceivesCallEvidence(fn *ast.FunctionExpr, graph *cfg.Graph, idx int) bool {
	if fn == nil || fn.ParList == nil || idx < 0 {
		return true
	}
	sourceIdx := idx
	if signatureHasImplicitSelfSlot(fn, graph) {
		if idx == 0 {
			return false
		}
		sourceIdx = idx - 1
	}
	if sourceIdx >= len(fn.ParList.Names) {
		return true
	}
	if fn.ParList.Types == nil || sourceIdx >= len(fn.ParList.Types) {
		return true
	}
	return sourceAnnotationReceivesCallEvidence(fn.ParList.Types[sourceIdx])
}

// SourceParamIsUnannotated reports whether the source parameter at idx carries
// no type annotation. An unannotated callee parameter is inferred from its
// callsites, so its in-progress contract is not a concrete caller obligation.
func SourceParamIsUnannotated(fn *ast.FunctionExpr, graph *cfg.Graph, idx int) bool {
	if fn == nil || fn.ParList == nil || idx < 0 {
		return false
	}
	sourceIdx := idx
	if signatureHasImplicitSelfSlot(fn, graph) {
		if idx == 0 {
			return false
		}
		sourceIdx = idx - 1
	}
	if sourceIdx >= len(fn.ParList.Names) {
		return false
	}
	if fn.ParList.Types == nil || sourceIdx >= len(fn.ParList.Types) {
		return true
	}
	return fn.ParList.Types[sourceIdx] == nil
}

func signatureHasImplicitSelfSlot(fn *ast.FunctionExpr, graph *cfg.Graph) bool {
	if fn == nil || fn.ParList == nil || graph == nil {
		return false
	}
	slots := graph.ParamSlotsReadOnly()
	if len(slots) == 0 || slots[0].Name != "self" {
		return false
	}
	return len(fn.ParList.Names) == 0 || fn.ParList.Names[0] != "self"
}

func sourceAnnotationReceivesCallEvidence(expr ast.TypeExpr) bool {
	switch t := expr.(type) {
	case nil:
		return true
	case *ast.ArrayTypeExpr:
		return softAnnotationTypeExpr(t.Element)
	case *ast.MapTypeExpr:
		return softAnnotationTypeExpr(t.Key) || softAnnotationTypeExpr(t.Value)
	case *ast.RecordTypeExpr:
		for _, field := range t.Fields {
			if softAnnotationTypeExpr(field.Type) {
				return true
			}
		}
		return false
	case *ast.TupleTypeExpr:
		for _, elem := range t.Elements {
			if softAnnotationTypeExpr(elem) {
				return true
			}
		}
		return false
	case *ast.OptionalTypeExpr:
		return sourceAnnotationReceivesCallEvidence(t.Inner)
	default:
		return false
	}
}

func softAnnotationTypeExpr(expr ast.TypeExpr) bool {
	switch t := expr.(type) {
	case nil:
		return false
	case *ast.PrimitiveTypeExpr:
		return t.Name == "any" || t.Name == "unknown"
	case *ast.ArrayTypeExpr:
		return softAnnotationTypeExpr(t.Element)
	case *ast.MapTypeExpr:
		return softAnnotationTypeExpr(t.Key) || softAnnotationTypeExpr(t.Value)
	case *ast.RecordTypeExpr:
		for _, field := range t.Fields {
			if softAnnotationTypeExpr(field.Type) {
				return true
			}
		}
		return false
	case *ast.TupleTypeExpr:
		for _, elem := range t.Elements {
			if softAnnotationTypeExpr(elem) {
				return true
			}
		}
		return false
	case *ast.OptionalTypeExpr:
		return softAnnotationTypeExpr(t.Inner)
	default:
		return false
	}
}

// StaticArgumentShape extracts the finite literal/table shape visible directly
// at a call site before a callee-local abstract overlay exists.
func StaticArgumentShape(expr ast.Expr) typ.Type {
	return staticArgumentShapeWith(expr, false)
}

func staticArgumentShapeWith(expr ast.Expr, preserveLiteral bool) typ.Type {
	switch e := expr.(type) {
	case *ast.NumberExpr:
		if !preserveLiteral {
			return typ.Number
		}
		return synthops.ParseNumber(e.Value)
	case *ast.StringExpr:
		if !preserveLiteral {
			return typ.String
		}
		return typ.LiteralString(e.Value)
	case *ast.TrueExpr:
		if !preserveLiteral {
			return typ.Boolean
		}
		return typ.True
	case *ast.FalseExpr:
		if !preserveLiteral {
			return typ.Boolean
		}
		return typ.False
	case *ast.NilExpr:
		return typ.Nil
	case *ast.TableExpr:
		return staticTableShape(e)
	default:
		return nil
	}
}

func staticTableShape(tbl *ast.TableExpr) typ.Type {
	if tbl == nil {
		return nil
	}
	if len(tbl.Fields) == 0 {
		return typ.NewRecord().SetOpen(true).Build()
	}

	builder := typ.NewRecord()
	var arrayElements []typ.Type
	fieldCount := 0
	for _, field := range tbl.Fields {
		if field == nil {
			continue
		}
		if field.Key == nil {
			elem := staticArgumentShapeWith(field.Value, true)
			if elem == nil {
				elem = typ.Unknown
			}
			arrayElements = append(arrayElements, elem)
			continue
		}
		name := staticFieldName(field.Key)
		if name == "" {
			continue
		}
		value := staticArgumentShapeWith(field.Value, true)
		if value == nil {
			value = typ.Unknown
		}
		builder.Field(name, value)
		fieldCount++
	}
	if len(arrayElements) > 0 && fieldCount == 0 {
		return typ.NewTuple(arrayElements...)
	}
	if fieldCount > 0 {
		return builder.Build()
	}
	return nil
}

func staticFieldName(expr ast.Expr) string {
	switch k := expr.(type) {
	case *ast.StringExpr:
		return k.Value
	case *ast.IdentExpr:
		return k.Value
	default:
		return ""
	}
}
