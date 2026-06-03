package callarg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ReSynth is called to re-synthesize an AST argument with contextual typing.
type ReSynth func(idx int, arg ast.Expr, expected typ.Type) typ.Type

// ForArgs binds AST arguments to the pure call pipeline re-synthesis hook.
func ForArgs(args []ast.Expr, reSynth ReSynth) ops.ArgReSynth {
	if reSynth == nil || len(args) == 0 {
		return nil
	}
	return func(idx int, expected typ.Type) typ.Type {
		if idx < 0 || idx >= len(args) {
			return nil
		}
		return reSynth(idx, args[idx], expected)
	}
}

// TableCompatChecker checks if a table literal is compatible with an expected type.
type TableCompatChecker func(table *ast.TableExpr, expected typ.Type, p cfg.Point) bool

// Full creates a ReSynth for arguments whose type can safely use the callee's
// expected parameter type as local context. It deliberately avoids arbitrary
// nested function calls: those calls run their own inference pipeline, and
// forcing an outer expected type into them can erase generic payload precision
// before the inner call has completed.
func Full(
	synthWithExpected func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type,
	tableChecker TableCompatChecker,
	p cfg.Point,
) ReSynth {
	return func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		switch a := arg.(type) {
		case *ast.FunctionExpr:
			return synthWithExpected(a, p, expected)
		case *ast.TableExpr:
			if tableChecker != nil && tableChecker(a, expected, p) {
				return expected
			}
			return synthWithExpected(a, p, expected)
		case *ast.IdentExpr:
			inferred := synthWithExpected(a, p, nil)
			if shouldRefineWithExpected(inferred, expected) {
				return expected
			}
			return inferred
		case *ast.AttrGetExpr:
			inferred := synthWithExpected(a, p, nil)
			if shouldRefineWithExpected(inferred, expected) {
				return expected
			}
			return inferred
		case *ast.CastExpr, *ast.LogicalOpExpr, *ast.NonNilAssertExpr:
			return synthWithExpected(a, p, expected)
		}
		return nil
	}
}

func shouldRefineWithExpected(inferred, expected typ.Type) bool {
	if inferred == nil || expected == nil {
		return false
	}
	if typ.IsAny(inferred) || typ.IsAny(expected) || expected.Kind().IsPlaceholder() {
		return false
	}
	if typ.ContainsRecursive(inferred) || typ.ContainsRecursive(expected) {
		return false
	}
	if subtype.IsSubtype(inferred, expected) {
		return true
	}
	inferredRec := unwrap.Record(inferred)
	expectedRec := unwrap.Record(expected)
	if inferredRec == nil || expectedRec == nil {
		return false
	}
	return recordEvidenceMatchesExpected(inferredRec, expectedRec)
}

func recordEvidenceMatchesExpected(inferred, expected *typ.Record) bool {
	if inferred == nil || expected == nil {
		return false
	}
	for _, field := range inferred.Fields {
		expectedField := expected.GetField(field.Name)
		if expectedField == nil {
			if expected.Open {
				continue
			}
			return false
		}
		if unresolvedRecordEvidence(field.Type) {
			continue
		}
		inferredType := field.Type
		if field.Optional {
			inferredType = typ.NewOptional(inferredType)
		}
		expectedType := expectedField.Type
		if expectedField.Optional {
			expectedType = typ.NewOptional(expectedType)
		}
		if !subtype.IsSubtype(inferredType, expectedType) {
			return false
		}
	}
	if inferred.HasMapComponent() {
		if !expected.HasMapComponent() {
			return false
		}
		if !unresolvedRecordEvidence(inferred.MapKey) && !subtype.IsSubtype(inferred.MapKey, expected.MapKey) {
			return false
		}
		if !unresolvedRecordEvidence(inferred.MapValue) && !subtype.IsSubtype(inferred.MapValue, expected.MapValue) {
			return false
		}
	}
	return true
}

func unresolvedRecordEvidence(t typ.Type) bool {
	if typ.IsAbsentOrUnknown(t) {
		return true
	}
	rec := unwrap.Record(t)
	return rec != nil && len(rec.Fields) == 0 && !rec.HasMapComponent()
}
