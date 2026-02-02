package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TableCheckResult holds the outcome of a table literal check.
type TableCheckResult struct {
	Handled    bool
	Compatible bool
}

// tableCheck validates a table literal against an expected type with union expansion.
func tableCheck(table *ast.TableExpr, expected typ.Type, synth api.Synth, p cfg.Point) TableCheckResult {
	if table == nil || expected == nil || synth == nil {
		return TableCheckResult{}
	}

	fields, arrayElems, _, earlyFail := extractTableFields(table, expected, synth, p)
	if earlyFail {
		return TableCheckResult{Handled: true, Compatible: false}
	}

	if u := unwrap.Union(expected); u != nil {
		for _, member := range u.Members {
			if checkTableWithOptionalRelax(fields, arrayElems, member) {
				return TableCheckResult{Handled: true, Compatible: true}
			}
		}
		return TableCheckResult{Handled: true, Compatible: false}
	}

	return TableCheckResult{Handled: true, Compatible: checkTableWithOptionalRelax(fields, arrayElems, expected)}
}

// tableCompatible validates a table literal against an expected type without union expansion.
func tableCompatible(table *ast.TableExpr, expected typ.Type, synth api.Synth, p cfg.Point) bool {
	if table == nil || expected == nil || synth == nil {
		return false
	}

	fields, arrayElems, _, earlyFail := extractTableFields(table, expected, synth, p)
	if earlyFail {
		return false
	}

	return checkTableWithOptionalRelax(fields, arrayElems, expected)
}

func extractTableFields(table *ast.TableExpr, expected typ.Type, synth api.Synth, p cfg.Point) ([]ops.FieldDef, []typ.Type, bool, bool) {
	recordOnly := false
	if u := unwrap.Union(expected); u != nil {
		recordOnly = unionAllRecordLike(u)
	} else if unwrap.Record(expected) != nil {
		recordOnly = true
	}

	expectedFields := core.AllFieldTypesResolved(expected)
	fields := make([]ops.FieldDef, 0, len(table.Fields))
	var arrayElems []typ.Type

	var mapValueType typ.Type
	if m, ok := unwrap.Alias(expected).(*typ.Map); ok {
		mapValueType = m.Value
	}

	for _, field := range table.Fields {
		if field.Key == nil {
			elemType := synth.SynthWithExpected(field.Value, p, mapValueType)
			arrayElems = append(arrayElems, elemType)
			continue
		}

		var name string
		switch k := field.Key.(type) {
		case *ast.StringExpr:
			name = k.Value
		case *ast.IdentExpr:
			if recordOnly {
				return nil, nil, recordOnly, true
			}
			name = k.Value
		case *ast.NumberExpr:
			_ = k
			elemType := synth.SynthWithExpected(field.Value, p, mapValueType)
			arrayElems = append(arrayElems, elemType)
			continue
		default:
			if recordOnly {
				return nil, nil, recordOnly, true
			}
			return nil, nil, recordOnly, true
		}

		var expectedFieldType typ.Type
		if expectedFields != nil {
			expectedFieldType = expectedFields[name]
		}
		ft := synth.SynthWithExpected(field.Value, p, expectedFieldType)
		fields = append(fields, ops.FieldDef{Name: name, Type: ft})
	}

	return fields, arrayElems, recordOnly, false
}

func checkTableWithOptionalRelax(fields []ops.FieldDef, arrayElems []typ.Type, expected typ.Type) bool {
	result := ops.CheckTable(fields, arrayElems, expected)
	if len(result.Errors) == 0 {
		return true
	}

	filtered := result.Errors[:0]
	for _, err := range result.Errors {
		if err.Message == "missing required field" && unwrap.IsOptionalLike(err.Expected) {
			continue
		}
		if err.Message == "unexpected field" {
			continue
		}
		filtered = append(filtered, err)
	}
	return len(filtered) == 0
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
