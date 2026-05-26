package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// SynthTableCore synthesizes a type for a table constructor expression.
//
// Table constructors are complex: they can represent:
//   - Records: {name = "Alice", age = 30}
//   - Arrays: {1, 2, 3}
//   - Tuples: {"a", 1, true} (mixed types, no expected array context)
//   - Maps: (via type annotations on expected type)
//
// Delegates to SynthTableWithExpected without contextual typing.
func (s *Synthesizer) SynthTableCore(ex *ast.TableExpr, p cfg.Point, sc *scope.State, recurse ExprSynth) typ.Type {
	return s.SynthTableWithExpected(ex, p, sc, recurse, nil)
}

// SynthTableWithExpected synthesizes a table type with bidirectional type checking.
//
// When expected is provided:
//   - Union expected: Attempts discriminated union matching via literal fields
//   - Record expected: Uses expected field types for nested synthesis
//   - Array expected: Synthesizes as array (not tuple) even without vararg
//   - Function fields: Passes expected function types for callback typing
//
// Self-type inference: For tables with function fields that have a "self" first
// parameter, infers self type from either expected type or synthesized table type.
//
// Empty tables start as the finite fresh-table bottom shape. Transfer widens
// that shape through field writes, indexed writes, and table mutators.
func (s *Synthesizer) SynthTableWithExpected(ex *ast.TableExpr, p cfg.Point, sc *scope.State, recurse ExprSynth, expected typ.Type) typ.Type {
	if len(ex.Fields) == 0 {
		if result := emptyTableExpectedResult(expected); result != nil {
			return result
		}
		return typ.NewFreshEmptyRecord()
	}

	if _, isUnion := unwrap.Alias(expected).(*typ.Union); isUnion {
		if match := querycore.TryDiscriminatedUnionMember(ex, expected); match != nil {
			return s.SynthTableWithExpected(ex, p, sc, recurse, match.Member)
		}
	}

	selfType := s.tableSelfType(ex, recurse, expected)
	table := s.collectTableFields(ex, p, sc, recurse, expected, selfType)

	if len(table.arrayElements) > 0 && table.fieldCount == 0 {
		result := table.arrayResult(expected)
		if useExpectedTableResult(expected) && len(ops.CheckTable(nil, table.arrayElements, expected).Errors) == 0 {
			return expected
		}
		return result
	}

	result := table.builder.Build()
	if useExpectedTableResult(expected) && len(ops.CheckTable(table.fieldDefs, table.arrayElements, expected).Errors) == 0 {
		return expected
	}
	return result
}

type tableSynthesis struct {
	builder       *typ.RecordBuilder
	fieldDefs     []ops.FieldDef
	arrayElements []typ.Type
	hasVararg     bool
	fieldCount    int
}

func (s *Synthesizer) tableSelfType(ex *ast.TableExpr, recurse ExprSynth, expected typ.Type) typ.Type {
	if expected != nil {
		return expected
	}
	selfBuilder := typ.NewRecord()
	fieldCount := 0
	for _, field := range ex.Fields {
		name, ok := staticTableFieldName(field)
		if !ok {
			continue
		}
		if _, ok := field.Value.(*ast.FunctionExpr); ok {
			continue
		}
		ft := recurse(field.Value)
		if ft == nil {
			ft = typ.Unknown
		}
		addRecordField(selfBuilder, name, ft)
		fieldCount++
	}
	if fieldCount == 0 {
		return nil
	}
	return selfBuilder.Build()
}

func (s *Synthesizer) collectTableFields(
	ex *ast.TableExpr,
	p cfg.Point,
	sc *scope.State,
	recurse ExprSynth,
	expected typ.Type,
	selfType typ.Type,
) tableSynthesis {
	table := tableSynthesis{builder: typ.NewRecord()}
	for _, field := range ex.Fields {
		if field.Key == nil {
			table.addArrayElement(s.tableElementType(field.Value, p, sc, recurse, expected, len(table.arrayElements), selfType))
			if _, ok := field.Value.(*ast.Comma3Expr); ok {
				table.hasVararg = true
			}
			continue
		}
		if name, ok := staticTableFieldName(field); ok {
			table.addNamedField(name, s.tableFieldType(field.Value, p, sc, recurse, ops.ExpectedTableFieldType(expected, name), selfType))
			continue
		}
		if _, ok := field.Key.(*ast.NumberExpr); ok {
			table.addArrayElement(s.tableElementType(field.Value, p, sc, recurse, expected, len(table.arrayElements), selfType))
		}
	}
	return table
}

func staticTableFieldName(field *ast.Field) (string, bool) {
	if field == nil {
		return "", false
	}
	switch k := field.Key.(type) {
	case *ast.StringExpr:
		return k.Value, true
	case *ast.IdentExpr:
		return k.Value, true
	default:
		return "", false
	}
}

func (s *Synthesizer) tableFieldType(value ast.Expr, p cfg.Point, sc *scope.State, recurse ExprSynth, expected typ.Type, selfType typ.Type) typ.Type {
	ft := s.synthFieldValueWithExpected(value, p, sc, recurse, expected, selfType)
	if ft == nil {
		return typ.Unknown
	}
	return ft
}

func (s *Synthesizer) tableElementType(value ast.Expr, p cfg.Point, sc *scope.State, recurse ExprSynth, expected typ.Type, idx int, selfType typ.Type) typ.Type {
	elemExpected := ops.ExpectedTableElementType(expected, idx)
	elemType := s.synthFieldValueWithExpected(value, p, sc, recurse, elemExpected, selfType)
	if elemType == nil {
		return typ.Unknown
	}
	return elemType
}

func (t *tableSynthesis) addNamedField(name string, ft typ.Type) {
	t.fieldDefs = append(t.fieldDefs, ops.FieldDef{Name: name, Type: ft})
	addRecordField(t.builder, name, ft)
	t.fieldCount++
}

func addRecordField(builder *typ.RecordBuilder, name string, ft typ.Type) {
	if inner, optional := typ.SplitNilableFieldType(ft); optional {
		builder.OptField(name, inner)
		return
	}
	builder.Field(name, ft)
}

func (t *tableSynthesis) addArrayElement(elem typ.Type) {
	t.arrayElements = append(t.arrayElements, elem)
}

func (t tableSynthesis) arrayResult(expected typ.Type) typ.Type {
	if t.hasVararg || querycore.IsArrayLike(expected) {
		return typ.NewArray(typ.NewUnion(t.arrayElements...))
	}
	return typ.NewTuple(t.arrayElements...)
}

func emptyTableExpectedResult(expected typ.Type) typ.Type {
	if expected == nil {
		return nil
	}
	nonNil := narrow.RemoveNil(expected)
	if nonNil == nil || typ.IsNever(nonNil) || typ.IsAbsentOrUnknown(nonNil) || typ.IsAny(nonNil) {
		return nil
	}
	if !useExpectedTableResult(nonNil) {
		return nil
	}
	if len(ops.CheckTable(nil, nil, nonNil).Errors) != 0 {
		return nil
	}
	return nonNil
}

func useExpectedTableResult(expected typ.Type) bool {
	if expected == nil {
		return false
	}
	unwrapped := unwrap.Alias(expected)
	if unwrapped == nil {
		return false
	}
	return !unwrapped.Kind().IsPlaceholder()
}

// synthFieldValueWithExpected synthesizes type for a table field value with optional expected type.
func (s *Synthesizer) synthFieldValueWithExpected(value ast.Expr, p cfg.Point, sc *scope.State, recurse ExprSynth, expected typ.Type, selfType typ.Type) typ.Type {
	if tbl, ok := value.(*ast.TableExpr); ok {
		return s.SynthTableWithExpected(tbl, p, sc, recurse, expected)
	}
	if fn, ok := value.(*ast.FunctionExpr); ok {
		var expectedFn *typ.Function
		if expected != nil {
			expectedFn, _ = unwrap.Alias(expected).(*typ.Function)
		}
		bindings := s.deps.ModuleBindings
		if bindings == nil && s.deps.CheckCtx != nil {
			bindings = s.deps.CheckCtx.Bindings()
		}
		if expectedFn == nil && selfType != nil && phasecore.HasUnannotatedSelfParam(fn, bindings) {
			expectedFn = typ.Func().Param("self", selfType).Build()
		}
		return s.SynthFunctionTypeWithExpected(fn, sc, expectedFn)
	}
	if expected != nil {
		return s.SynthExprWithExpectedCore(value, sc, p, recurse, expected)
	}
	return recurse(value)
}
