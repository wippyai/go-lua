package synth

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// FunctionLiteralTypes synthesizes declared types for function literals and tables.
//
// This function extracts type information from function and table literals assigned
// to local variables, building a DeclaredTypes map that the flow analysis uses as
// initial type information before narrowing.
//
// Processing order (via CFG RPO traversal):
//  1. Local assignments with function literal RHS: Synthesizes and records function type
//  2. Local assignments with table literal RHS: Synthesizes and records table/record type
//  3. Function definitions (function field assignments, method definitions):
//     Extends receiver types with method fields
//
// The resulting map provides declared types that flow analysis uses as the base
// types before applying any narrowing from control flow.
//
// Returns nil if graph or synth is nil, or if no literals are found.
func FunctionLiteralTypes(graph *cfg.Graph, synth api.ExprSynth) flow.DeclaredTypes {
	if graph == nil || synth == nil {
		return nil
	}

	types := make(flow.DeclaredTypes)

	for _, p := range graph.RPO() {
		info := graph.Assign(p)
		if info == nil || !info.IsLocal {
			continue
		}

		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent {
				return
			}
			sym := target.Symbol
			if sym == 0 {
				return
			}

			switch v := source.(type) {
			case *ast.FunctionExpr:
				if t := synth(source, p); t != nil {
					types[sym] = t
				}
			case *ast.TableExpr:
				if len(v.Fields) == 0 {
					return
				}
				if t := synth(source, p); t != nil {
					types[sym] = t
				}
			}
		})
	}

	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if info == nil {
			return
		}
		if info.TargetKind == cfg.FuncDefGlobal {
			if info.Symbol == 0 || info.FuncExpr == nil || len(info.FuncExpr.ReturnTypes) > 0 {
				return
			}
			if t := synth(info.FuncExpr, p); t != nil {
				types[info.Symbol] = t
			}
			return
		}
		if info.TargetKind != cfg.FuncDefField && info.TargetKind != cfg.FuncDefMethod {
			return
		}
		if info.Name == "" {
			return
		}
		receiverSym := info.ReceiverSymbol
		if receiverSym == 0 {
			return
		}
		fnType := typ.Unknown
		if info.FuncExpr != nil {
			if t := synth(info.FuncExpr, p); t != nil {
				fnType = t
			}
		}
		baseType := types[receiverSym]
		if baseType == nil && info.Receiver != nil {
			baseType = synth(info.Receiver, p)
		}
		types[receiverSym] = typ.ExtendRecordWithField(baseType, info.Name, fnType)
	})

	if len(types) == 0 {
		return nil
	}
	return types
}

// FunctionLiteralSignatures builds a map of function expressions to their signatures.
//
// Unlike FunctionLiteralTypes which produces DeclaredTypes keyed by symbol,
// this function produces signatures keyed by AST node. This is used by the type
// checker to look up the signature of any function literal in the code.
//
// Processing:
//  1. Local assignments: Extracts signatures from function literals and nested
//     function fields in table literals
//  2. Function definitions: Handles field and method definitions on receivers
//  3. Return statements: If declaredReturns is provided, uses expected return
//     types to guide function literal inference at return positions
//
// Expected type propagation:
//   - For table fields, uses expected table type to infer field function signatures
//   - For methods with "self" parameter, infers self type from receiver
//   - For return expressions, uses declared return types as expected types
//
// Returns nil if graph or engine is nil, or if no function literals found.
func FunctionLiteralSignatures(graph *cfg.Graph, engine LiteralSynth, declaredReturns []typ.Type) map[*ast.FunctionExpr]*typ.Function {
	if graph == nil || engine == nil {
		return nil
	}

	out := make(map[*ast.FunctionExpr]*typ.Function)
	scopes := engine.Scopes()
	entry := engine.Entry()
	callCtx := db.NewQueryContext(db.New())

	addSig := func(fn *ast.FunctionExpr, p cfg.Point, expected *typ.Function) {
		if fn == nil {
			return
		}
		sc := scopes[p]
		if sc == nil {
			sc = scopes[entry]
		}
		sig := engine.SynthFunctionTypeWithExpected(fn, sc, expected)
		if sig != nil {
			out[fn] = sig
		}
	}
	receiverSelfType := func(expr ast.Expr, p cfg.Point) typ.Type {
		sc := scopes[p]
		if sc == nil {
			sc = scopes[entry]
		}
		if ident, ok := expr.(*ast.IdentExpr); ok && ident != nil && sc != nil {
			if named, ok := sc.LookupType(ident.Value); ok && named != nil {
				return named
			}
		}
		return widenMutableReceiverState(engine.TypeOf(expr, p))
	}

	var collectExpr func(expr ast.Expr, p cfg.Point, expected typ.Type)
	var collectTable func(tbl *ast.TableExpr, p cfg.Point, expected typ.Type)

	collectExpr = func(expr ast.Expr, p cfg.Point, expected typ.Type) {
		if expr == nil {
			return
		}
		switch v := expr.(type) {
		case *ast.FunctionExpr:
			expectedFn := phasecore.ExpectedFunctionLiteralSignature(v, expected)
			addSig(v, p, expectedFn)
		case *ast.TableExpr:
			collectTable(v, p, expected)
		}
	}

	collectTable = func(tbl *ast.TableExpr, p cfg.Point, expected typ.Type) {
		if tbl == nil {
			return
		}
		expectedFields := querycore.AllFieldTypesResolved(expected)
		selfType := expected
		if selfType == nil {
			selfBuilder := typ.NewRecord()
			fieldCount := 0
			for _, field := range tbl.Fields {
				if field.Key == nil {
					continue
				}
				if _, ok := field.Value.(*ast.FunctionExpr); ok {
					continue
				}
				switch k := field.Key.(type) {
				case *ast.StringExpr:
					selfBuilder.Field(k.Value, engine.TypeOf(field.Value, p))
					fieldCount++
				case *ast.IdentExpr:
					selfBuilder.Field(k.Value, engine.TypeOf(field.Value, p))
					fieldCount++
				}
			}
			if fieldCount > 0 {
				selfType = widenMutableReceiverState(selfBuilder.Build())
			}
		}

		for _, field := range tbl.Fields {
			if field.Key == nil {
				continue
			}
			switch v := field.Value.(type) {
			case *ast.FunctionExpr:
				var expectedFn *typ.Function
				switch k := field.Key.(type) {
				case *ast.StringExpr:
					if expectedFields != nil {
						if ft := expectedFields[k.Value]; ft != nil {
							expectedFn, _ = unwrap.Alias(ft).(*typ.Function)
						}
					}
				case *ast.IdentExpr:
					if expectedFields != nil {
						if ft := expectedFields[k.Value]; ft != nil {
							expectedFn, _ = unwrap.Alias(ft).(*typ.Function)
						}
					}
				}
				if expectedFn == nil && selfType != nil && phasecore.HasUnannotatedSelfParam(v, graph.Bindings()) {
					expectedFn = typ.Func().Param("self", selfType).Build()
				}
				addSig(v, p, expectedFn)
			case *ast.TableExpr:
				var nestedExpected typ.Type
				switch k := field.Key.(type) {
				case *ast.StringExpr:
					if expectedFields != nil {
						nestedExpected = expectedFields[k.Value]
					}
				case *ast.IdentExpr:
					if expectedFields != nil {
						nestedExpected = expectedFields[k.Value]
					}
				}
				collectTable(v, p, nestedExpected)
			}
		}
	}

	for _, p := range graph.RPO() {
		info := graph.Assign(p)
		if info == nil || !info.IsLocal {
			continue
		}
		info.EachSource(func(_ int, source ast.Expr) {
			switch v := source.(type) {
			case *ast.FunctionExpr:
				addSig(v, p, nil)
			case *ast.TableExpr:
				collectTable(v, p, nil)
			}
		})
	}

	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if info == nil || info.FuncExpr == nil {
			return
		}
		var expectedFn *typ.Function
		if info.TargetKind == cfg.FuncDefField || info.TargetKind == cfg.FuncDefMethod {
			if info.Receiver != nil {
				recvType := receiverSelfType(info.Receiver, p)
				if recvType != nil && phasecore.HasSelfParam(info.FuncExpr, graph.Bindings()) {
					expectedFn = typ.Func().Param("self", recvType).Build()
				}
			}
		}
		addSig(info.FuncExpr, p, expectedFn)
	})

	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		expectedArgs, expectedVariadic := expectedArgsForDirectCallbackLiteral(callCtx, engine, graph, info, p)
		if len(expectedArgs) == 0 && expectedVariadic == nil {
			return
		}
		for i, arg := range info.Args {
			fn, ok := arg.(*ast.FunctionExpr)
			if !ok {
				continue
			}
			var expected typ.Type
			if i < len(expectedArgs) {
				expected = expectedArgs[i]
			} else {
				expected = expectedVariadic
			}
			if expectedFn := phasecore.ExpectedFunctionLiteralSignature(fn, expected); expectedFn != nil {
				addSig(fn, p, expectedFn)
			}
		}
	})

	if len(declaredReturns) > 0 {
		graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
			if info == nil {
				return
			}
			for i, expr := range info.Exprs {
				if i >= len(declaredReturns) {
					break
				}
				expected := declaredReturns[i]
				collectExpr(expr, p, expected)
			}
		})
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

type callQueryProvider interface {
	GetCallQuery() querycore.TypeOps
}

func expectedArgsForDirectCallbackLiteral(
	ctx *db.QueryContext,
	engine LiteralSynth,
	graph *cfg.Graph,
	info *cfg.CallInfo,
	p cfg.Point,
) ([]typ.Type, typ.Type) {
	provider, ok := engine.(callQueryProvider)
	if !ok || provider == nil || graph == nil || info == nil {
		return nil, nil
	}
	query := provider.GetCallQuery()
	if query == nil {
		return nil, nil
	}

	def := ops.CallDef{
		Args:                shallowCallArgTypes(engine, info.Args, p),
		Query:               query,
		ForceMethodReceiver: callsite.ForceMethodReceiverAtPoint(graph.Bindings(), graph, p, info.Call),
	}
	if callsite.IsMethodCallInfo(info) {
		def.IsMethod = true
		def.Receiver = engine.TypeOf(info.Receiver, p)
		def.MethodName = info.Method
	} else {
		def.Callee = engine.TypeOf(info.Callee, p)
	}
	inferred := ops.InferCall(ctx, def)
	return inferred.ExpectedArgs, inferred.ExpectedVariadic
}

func shallowCallArgTypes(engine LiteralSynth, args []ast.Expr, p cfg.Point) []typ.Type {
	if len(args) == 0 {
		return nil
	}
	out := make([]typ.Type, len(args))
	for i, arg := range args {
		if fn, ok := arg.(*ast.FunctionExpr); ok {
			out[i] = phasecore.ShallowFunctionLiteralSignature(fn)
			continue
		}
		out[i] = engine.TypeOf(arg, p)
	}
	return out
}

func widenMutableReceiverState(t typ.Type) typ.Type {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok {
		return t
	}

	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	for _, f := range rec.Fields {
		fieldType := widenMutableReceiverField(f.Type)
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, fieldType)
		case f.Optional:
			builder.OptField(f.Name, fieldType)
		case f.Readonly:
			builder.ReadonlyField(f.Name, fieldType)
		default:
			builder.Field(f.Name, fieldType)
		}
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	return builder.Build()
}

func widenMutableReceiverField(t typ.Type) typ.Type {
	if t == nil {
		return typ.Unknown
	}
	unaliased := unwrap.Alias(t)
	if unaliased == nil {
		return typ.Unknown
	}
	if unaliased.Kind() == kind.Nil {
		return typ.Unknown
	}
	if v, ok := unaliased.(*typ.Literal); ok {
		switch v.Base {
		case kind.Boolean:
			return typ.Boolean
		case kind.String:
			return typ.String
		case kind.Integer:
			return typ.Integer
		case kind.Number:
			return typ.Number
		}
	}
	return t
}
