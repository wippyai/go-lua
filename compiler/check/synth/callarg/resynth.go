package callarg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/core"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ReSynth is called to re-synthesize an AST argument with contextual typing.
type ReSynth func(idx int, arg ast.Expr, expected typ.Type) typ.Type

// TypeOf synthesizes the ordinary non-contextual type of an argument.
type TypeOf func(ast.Expr) typ.Type

// FunctionLiteral resolves an argument expression to the function literal it
// denotes when the argument is a callback value.
type FunctionLiteral func(ast.Expr) *ast.FunctionExpr

// InitialInferenceTypes returns the argument vector used for the first generic
// inference pass. Callback literals are represented by shallow signatures so
// their body obligations cannot solve input generics before the callee has
// supplied the expected callback parameter types.
func InitialInferenceTypes(args []ast.Expr, typeOf TypeOf, literal FunctionLiteral) ([]typ.Type, bool) {
	if len(args) == 0 {
		return nil, false
	}
	out := make([]typ.Type, len(args))
	hasCallback := false
	for i, arg := range args {
		if fn := FunctionLiteralArg(arg, literal); fn != nil {
			out[i] = phasecore.ShallowFunctionLiteralSignature(fn)
			hasCallback = true
			continue
		}
		if typeOf != nil {
			out[i] = typeOf(arg)
		}
		if out[i] == nil {
			out[i] = typ.Unknown
		}
	}
	return out, hasCallback
}

// FunctionLiteralArg resolves direct and named callback arguments through one
// surface so all staged call paths agree on what must be kept shallow during
// initial inference.
func FunctionLiteralArg(arg ast.Expr, literal FunctionLiteral) *ast.FunctionExpr {
	if fn, ok := arg.(*ast.FunctionExpr); ok {
		return fn
	}
	if literal == nil {
		return nil
	}
	return literal(arg)
}

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

// ObserveArgsWithExpectedProofs binds AST arguments to the proof-aware call
// pipeline hook. Only an exact expected-type selection is authoritative; other
// returned types remain ordinary contextual refinements.
func ObserveArgsWithExpectedProofs(args []ast.Expr, reSynth ReSynth) ops.ArgProofObservation {
	if reSynth == nil || len(args) == 0 {
		return nil
	}
	return func(idx int, expected typ.Type) (typ.Type, bool) {
		if idx < 0 || idx >= len(args) {
			return nil, false
		}
		candidate := reSynth(idx, args[idx], expected)
		return candidate, candidate != nil && typ.TypeEquals(candidate, expected)
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
	return full(synthWithExpected, tableChecker, p, false)
}

// FullWithExpectedProofs creates a ReSynth for hard call-argument proof
// boundaries. Identifier and attribute arguments are sent through the
// expected-aware observation surface so declared/path/body-contract proofs can
// be selected there; unproved gradual values remain rejected by that surface.
func FullWithExpectedProofs(
	synthWithExpected func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type,
	tableChecker TableCompatChecker,
	p cfg.Point,
) ReSynth {
	return full(synthWithExpected, tableChecker, p, true)
}

func full(
	synthWithExpected func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type,
	tableChecker TableCompatChecker,
	p cfg.Point,
	expectedProofs bool,
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
			context := expected
			if !expectedProofs {
				context = nil
			}
			inferred := synthWithExpected(a, p, context)
			if shouldRefineWithExpected(inferred, expected) {
				return expected
			}
			return inferred
		case *ast.AttrGetExpr:
			context := expected
			if !expectedProofs {
				context = nil
			}
			inferred := synthWithExpected(a, p, context)
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
