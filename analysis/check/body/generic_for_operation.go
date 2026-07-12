package body

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// compileGenericForOperations is the sole AST-to-engine boundary for generic
// iteration. Solves retain only these immutable operations; GenericForFact is
// kept separately for syntax-facing Result accessors.
func compileGenericForOperations(
	facts map[cfg.Point]GenericForFact,
	types *typeresolve.Resolver,
	resolvePath func(ast.Expr) (pathdom.Path, bool),
	resolveIterator func(cfg.Point) (iteration.Iterator, bool),
) map[cfg.Point]operationplan.GenericForOperation {
	if len(facts) == 0 {
		return nil
	}
	out := make(map[cfg.Point]operationplan.GenericForOperation, len(facts))
	for point, fact := range facts {
		if fact.Role != GenericForRoleVariable || !fact.HasSymbols ||
			fact.VariableIndex < 0 || fact.VariableIndex >= len(fact.Symbols) {
			continue
		}
		source := operationplan.GenericForSource{}
		if len(fact.Sources) != 0 {
			projected := fact.Sources[0]
			switch projected.Kind {
			case sourceprovenance.SourceExpression:
				source.Kind = operationplan.GenericForSourceExpression
				if resolvePath != nil && projected.Expr != nil {
					source.RootPath, source.HasRootPath = resolvePath(projected.Expr)
				}
			case sourceprovenance.SourceCall:
				source.Kind = operationplan.GenericForSourceCall
				source.CallPoint = projected.CallPoint
				source.HasCallPoint = projected.HasCallPoint
			}
		}
		contracts := genericForSourceContracts(fact, types)
		first := fact.Symbols[0]
		op, ok := operationplan.NewGenericForOperation(fact.VariableIndex, fact.Symbols[fact.VariableIndex], first, source, contracts)
		if ok {
			if resolveIterator != nil && source.Kind == operationplan.GenericForSourceCall && source.HasCallPoint {
				if iterator, resolved := resolveIterator(source.CallPoint); resolved {
					op = op.WithIterator(iterator)
				}
			}
			out[point] = op
		}
	}
	return out
}

func genericForOperationExtensions(operations map[cfg.Point]operationplan.GenericForOperation) []operationplan.ExtensionInput {
	if len(operations) == 0 {
		return nil
	}
	out := make([]operationplan.ExtensionInput, 0, len(operations))
	for point, operation := range operations {
		out = append(out, operationplan.ExtensionInput{Point: point, Kind: operationplan.BodyGenericFor, GenericFor: operation})
	}
	return out
}

func genericForSourceContracts(fact GenericForFact, resolver *typeresolve.Resolver) []typ.Type {
	if resolver == nil {
		return nil
	}
	maxArgs := 0
	for _, expr := range fact.Exprs {
		if call, ok := expr.(*ast.FuncCallExpr); ok && call != nil && len(call.Args) > maxArgs {
			maxArgs = len(call.Args)
		}
	}
	if maxArgs == 0 {
		return nil
	}
	out := make([]typ.Type, maxArgs)
	for i := range out {
		arg := genericForCallArgument(fact, i)
		if arg == nil {
			continue
		}
		out[i], _ = assertedIteratorSourceType(arg, resolver)
	}
	return out
}

func genericForCallArgument(generic GenericForFact, sourceIndex int) ast.Expr {
	for _, expr := range generic.Exprs {
		call, ok := expr.(*ast.FuncCallExpr)
		if !ok || call == nil || sourceIndex >= len(call.Args) {
			continue
		}
		return call.Args[sourceIndex]
	}
	return nil
}

func assertedIteratorSourceType(expr ast.Expr, resolver *typeresolve.Resolver) (typ.Type, bool) {
	switch expr := expr.(type) {
	case *ast.CastExpr:
		t, ok := resolver.Type(expr.Type)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
			return nil, false
		}
		return t, true
	case *ast.NonNilAssertExpr:
		return assertedIteratorSourceType(expr.Expr, resolver)
	default:
		return nil, false
	}
}
