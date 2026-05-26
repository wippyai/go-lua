package observation

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TableCheckResult is the solved-state table-literal proof consumed by
// diagnostics. The observation domain owns the contextual AST-to-table product;
// hooks only format failures.
type TableCheckResult struct {
	Handled    bool
	Compatible bool
	Reason     string
}

// CheckTable validates a table literal against an expected type using the same
// contextual solved-state projection as TypeOfWithExpected.
func (p Projector) CheckTable(table *ast.TableExpr, point cfg.Point, expected typ.Type) TableCheckResult {
	if table == nil || expected == nil {
		return TableCheckResult{}
	}
	expected = p.resolveLocalRefs(expected, point)
	fields, arrayElems, _, earlyFail := p.tableFields(table, expected, point, true)
	if earlyFail {
		return TableCheckResult{Handled: true, Compatible: false, Reason: "table shape is incompatible with expected record fields"}
	}

	if u := unwrap.Union(expected); u != nil {
		bestReason := ""
		for _, member := range u.Members {
			ok, reason := checkTableWithOptionalRelax(fields, arrayElems, member)
			if ok {
				return TableCheckResult{Handled: true, Compatible: true}
			}
			if bestReason == "" && reason != "" {
				bestReason = reason
			}
		}
		return TableCheckResult{Handled: true, Compatible: false, Reason: bestReason}
	}

	ok, reason := checkTableWithOptionalRelax(fields, arrayElems, expected)
	return TableCheckResult{Handled: true, Compatible: ok, Reason: reason}
}

// TableCompatible is the predicate form used by contextual call-argument
// projection.
func (p Projector) TableCompatible(table *ast.TableExpr, point cfg.Point, expected typ.Type) bool {
	result := p.CheckTable(table, point, expected)
	return result.Handled && result.Compatible
}

func (p Projector) tableFields(table *ast.TableExpr, expected typ.Type, point cfg.Point, failDynamicRecordKeys bool) ([]ops.FieldDef, []typ.Type, int, bool) {
	recordOnly := false
	if u := unwrap.Union(expected); u != nil {
		recordOnly = unionAllRecordLike(u)
	} else if unwrap.Record(expected) != nil {
		recordOnly = true
	}

	fields := make([]ops.FieldDef, 0, len(table.Fields))
	var arrayElems []typ.Type
	fieldCount := 0

	for _, field := range table.Fields {
		if field == nil {
			continue
		}
		if field.Key == nil {
			elemExpected := ops.ExpectedTableElementType(expected, len(arrayElems))
			elemType := p.TypeOfWithExpected(field.Value, point, elemExpected)
			if elemType == nil {
				elemType = typ.Unknown
			}
			arrayElems = append(arrayElems, elemType)
			continue
		}

		var name string
		switch k := field.Key.(type) {
		case *ast.StringExpr:
			name = k.Value
		case *ast.IdentExpr:
			if failDynamicRecordKeys && recordOnly {
				return nil, nil, 0, true
			}
			name = k.Value
		case *ast.NumberExpr:
			elemExpected := ops.ExpectedTableElementType(expected, len(arrayElems))
			elemType := p.TypeOfWithExpected(field.Value, point, elemExpected)
			if elemType == nil {
				elemType = typ.Unknown
			}
			arrayElems = append(arrayElems, elemType)
			continue
		default:
			if failDynamicRecordKeys && recordOnly {
				return nil, nil, 0, true
			}
			continue
		}

		expectedFieldType := ops.ExpectedTableFieldType(expected, name)
		var ft typ.Type
		if isEmptyTableExpr(field.Value) {
			ft = promoteEmptyTableLiteral(expectedFieldType)
		}
		if ft == nil {
			ft = p.TypeOfWithExpected(field.Value, point, expectedFieldType)
		}
		if ft == nil {
			ft = typ.Unknown
		}
		fields = append(fields, ops.FieldDef{Name: name, Type: ft})
		fieldCount++
	}

	return fields, arrayElems, fieldCount, false
}

func isEmptyTableExpr(expr ast.Expr) bool {
	table, ok := expr.(*ast.TableExpr)
	return ok && table != nil && len(table.Fields) == 0
}

func promoteEmptyTableLiteral(expected typ.Type) typ.Type {
	if expected == nil {
		return nil
	}
	if a, ok := expected.(*typ.Alias); ok && a != nil {
		if promoted := promoteEmptyTableLiteral(a.Target); promoted != nil {
			return expected
		}
	}
	if opt, ok := expected.(*typ.Optional); ok && opt != nil {
		if promoteEmptyTableLiteral(opt.Inner) != nil {
			return expected
		}
	}
	switch unwrap.Alias(expected).Kind() {
	case kind.Map, kind.Array:
		return expected
	default:
		return nil
	}
}

func checkTableWithOptionalRelax(fields []ops.FieldDef, arrayElems []typ.Type, expected typ.Type) (bool, string) {
	result := ops.CheckTable(fields, arrayElems, expected)
	if len(result.Errors) == 0 {
		return true, ""
	}

	filtered := result.Errors[:0]
	for _, err := range result.Errors {
		if err.Message == "missing required field" && unwrap.IsOptionalLike(err.Expected) {
			continue
		}
		if err.Message == "field type mismatch" && unresolvedTableEvidence(err.Got) {
			continue
		}
		if err.Message == "unexpected field" {
			continue
		}
		filtered = append(filtered, err)
	}
	if len(filtered) == 0 {
		return true, ""
	}

	first := filtered[0]
	reason := first.Message
	if first.Field != "" {
		reason += " on field '" + first.Field + "'"
	}
	if first.Expected != nil {
		reason += ", expected " + typ.FormatShort(first.Expected)
	}
	if first.Got != nil {
		reason += ", got " + typ.FormatShort(first.Got)
	}
	return false, reason
}

func unresolvedTableEvidence(t typ.Type) bool {
	if typ.IsAbsentOrUnknown(t) {
		return true
	}
	rec := unwrap.Record(t)
	return rec != nil && len(rec.Fields) == 0 && !rec.HasMapComponent()
}

func unionAllRecordLike(u *typ.Union) bool {
	if u == nil {
		return false
	}
	for _, m := range u.Members {
		if unwrap.Record(m) == nil {
			return false
		}
	}
	return true
}
