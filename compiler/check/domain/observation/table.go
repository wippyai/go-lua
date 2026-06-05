package observation

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/constraint"
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
	if u := unwrap.Union(expected); u != nil {
		bestReason := ""
		for _, member := range u.Members {
			entries, arrayElems, _, earlyFail := p.tableEntries(table, member, point, true)
			if earlyFail {
				if bestReason == "" {
					bestReason = "table shape is incompatible with expected record fields"
				}
				continue
			}
			ok, reason := checkTableEntriesWithOptionalRelax(entries, arrayElems, member)
			if ok {
				return TableCheckResult{Handled: true, Compatible: true}
			}
			if bestReason == "" && reason != "" {
				bestReason = reason
			}
		}
		return TableCheckResult{Handled: true, Compatible: false, Reason: bestReason}
	}
	entries, arrayElems, _, earlyFail := p.tableEntries(table, expected, point, true)
	if earlyFail {
		return TableCheckResult{Handled: true, Compatible: false, Reason: "table shape is incompatible with expected record fields"}
	}

	ok, reason := checkTableEntriesWithOptionalRelax(entries, arrayElems, expected)
	return TableCheckResult{Handled: true, Compatible: ok, Reason: reason}
}

// TableCompatible is the predicate form used by contextual call-argument
// projection.
func (p Projector) TableCompatible(table *ast.TableExpr, point cfg.Point, expected typ.Type) bool {
	result := p.CheckTable(table, point, expected)
	return result.Handled && result.Compatible
}

func (p Projector) tableEntries(table *ast.TableExpr, expected typ.Type, point cfg.Point, failDynamicRecordKeys bool) ([]ops.EntryDef, []typ.Type, int, bool) {
	recordOnly := false
	if u := unwrap.Union(expected); u != nil {
		recordOnly = unionAllRecordLike(u)
	} else if unwrap.Record(expected) != nil {
		recordOnly = true
	}

	entries := make([]ops.EntryDef, 0, len(table.Fields))
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

		if failDynamicRecordKeys && recordOnly {
			name, ok := fieldkey.RecordFieldNameFromTableField(field)
			if !ok {
				return nil, nil, 0, true
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
			entries = append(entries, ops.EntryDef{
				Key:  constraint.Segment{Kind: constraint.SegmentField, Name: name},
				Type: ft,
			})
			fieldCount++
			continue
		}

		seg, ok := flowpath.StaticFieldSegmentWithConst(field, p.constResolver(point))
		if !ok {
			if failDynamicRecordKeys && recordOnly {
				return nil, nil, 0, true
			}
			continue
		}
		key, ok := fieldkey.FromSegment(seg)
		if !ok {
			if failDynamicRecordKeys && recordOnly {
				return nil, nil, 0, true
			}
			continue
		}

		expectedFieldType := ops.ExpectedTableEntryType(expected, key)
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
		entries = append(entries, ops.EntryDef{Key: key, Type: ft})
		fieldCount++
	}

	return entries, arrayElems, fieldCount, false
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
	case kind.Map, kind.ReadonlyMap, kind.Array:
		return expected
	default:
		return nil
	}
}

func checkTableWithOptionalRelax(fields []ops.FieldDef, arrayElems []typ.Type, expected typ.Type) (bool, string) {
	result := ops.CheckTable(fields, arrayElems, expected)
	return checkTableResultWithOptionalRelax(result, expected)
}

func checkTableEntriesWithOptionalRelax(entries []ops.EntryDef, arrayElems []typ.Type, expected typ.Type) (bool, string) {
	result := ops.CheckTableEntries(entries, arrayElems, expected)
	return checkTableResultWithOptionalRelax(result, expected)
}

func checkTableResultWithOptionalRelax(result ops.CheckResult, expected typ.Type) (bool, string) {
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
