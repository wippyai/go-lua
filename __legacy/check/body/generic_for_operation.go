package body

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
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
	signatureCalls map[cfg.Point]operationplan.SignatureCallOperation,
	resolveDeclaredSource func(cfg.Point, effect.ParamRef) (int, typ.Type, bool),
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
		sources := make([]operationplan.GenericForSource, len(fact.Sources))
		for i, projected := range fact.Sources {
			source := operationplan.GenericForSource{}
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
			sources[i] = source
		}
		var source operationplan.GenericForSource
		if len(sources) != 0 {
			source = sources[0]
		}
		contracts := genericForSourceContracts(fact, types)
		var iterator iteration.Iterator
		hasIterator := false
		if source.Kind == operationplan.GenericForSourceCall && source.HasCallPoint {
			// The sealed signature-call operation is the sole producer of iterator
			// protocol vocabulary. Generic-for must not independently repeat name,
			// receiver, or signature resolution: method calls need the same typed
			// receiver proof as the call transaction itself.
			if call, ok := signatureCalls[source.CallPoint]; ok {
				iterator, hasIterator = iteration.ActiveIterator(call.Signature().Effect.Labels)
			}
		}
		contracts = genericForDeclaredSourceContracts(fact, source, iterator, hasIterator, contracts, resolveDeclaredSource)
		first := fact.Symbols[0]
		op, ok := operationplan.NewGenericForOperation(fact.VariableIndex, fact.Symbols[fact.VariableIndex], first, sources, contracts)
		if ok {
			if hasIterator {
				op = op.WithIterator(iterator)
			} else if call, exists := signatureCalls[source.CallPoint]; exists {
				if _, callable := effectlowering.CallableIteratorSignature(call.Signature()); callable {
					op = op.WithCallableIterator()
				}
			}
			out[point] = op
		}
	}
	return out
}

// genericForDeclaredSourceContracts freezes the state-independent declared
// source contract into the immutable operation plan. State-dependent source
// information is represented by the canonical Projection term; there is no
// second concrete recovery path.
func genericForDeclaredSourceContracts(
	fact GenericForFact,
	source operationplan.GenericForSource,
	iterator iteration.Iterator,
	hasIterator bool,
	contracts []typ.Type,
	resolveDeclaredSource func(cfg.Point, effect.ParamRef) (int, typ.Type, bool),
) []typ.Type {
	if !hasIterator || source.Kind != operationplan.GenericForSourceCall || !source.HasCallPoint || resolveDeclaredSource == nil {
		return contracts
	}
	index, declared, ok := resolveDeclaredSource(source.CallPoint, iterator.Source)
	if !ok || index < 0 || (index < len(contracts) && contracts[index] != nil) {
		return contracts
	}
	if !genericForIteratorSourceTypeProjects(iterator, fact.VariableIndex, declared) {
		return contracts
	}
	if len(contracts) <= index {
		contracts = append(contracts, make([]typ.Type, index+1-len(contracts))...)
	}
	contracts[index] = declared
	return contracts
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
