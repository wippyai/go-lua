package body

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/compiler/ast"
)

// StaticForest owns the immutable prepared artifacts for one source lexical
// forest. Every lexical function has exactly one CFG, WIR body and Static;
// parent closure protos point at those same immutable child artifacts.
type StaticForest struct {
	root         *Static
	functions    map[*ast.FunctionExpr]*Static
	callTopology operationplan.CallTopology
}

// Root returns the prepared chunk root. Function-root forests have no separate
// chunk root; their selected root is returned by Function.
func (f *StaticForest) Root() *Static {
	if f == nil {
		return nil
	}
	return f.root
}

// Function returns the unique prepared artifact owned by fn.
func (f *StaticForest) Function(fn *ast.FunctionExpr) *Static {
	if f == nil || fn == nil {
		return nil
	}
	return f.functions[fn]
}

// PrepareBoundChunkForest prepares a chunk and every lexically nested function
// as one source-owned immutable forest. Unlike preparing every function after
// recursively lowering the chunk, this performs exactly one CFG build and one
// WIR lowering per lexical body.
func PrepareBoundChunkForest(stmts []ast.Stmt, bindings *bind.Result, config Config) (*StaticForest, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.prepareBoundForest(stmts, nil, bindings)
}

// PrepareBoundFunctionForest prepares fn and its binding-owned lexical forest
// without a separate chunk body.
func PrepareBoundFunctionForest(fn *ast.FunctionExpr, bindings *bind.Result, config Config) (*StaticForest, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.prepareBoundForest(nil, fn, bindings)
}

type preparedLexicalFunction struct {
	built    *cfgbuild.Result
	wir      wirlower.PreparedFunction
	resolver *typeresolve.Resolver
}

func (c *checker) prepareBoundForest(chunk []ast.Stmt, rootFn *ast.FunctionExpr, bindings *bind.Result) (*StaticForest, error) {
	if bindings == nil {
		return nil, ErrBindingsRequired
	}
	functions := bindings.Functions()
	prepared := make(map[*ast.FunctionExpr]preparedLexicalFunction, len(functions))
	sealedLuaTypeChecks := luaTypePredicateChecksSealedForLowering(bindings, c.config.Signatures, c.config.GlobalTypes)
	cfgOptions := cfgbuild.Options{SealedLuaTypeChecks: sealedLuaTypeChecks}

	// Binding order is parent-before-child, therefore reverse order makes every
	// direct child available before its parent is lowered.
	for i := len(functions) - 1; i >= 0; i-- {
		fn := functions[i]
		if err := validatePreparedChildren(bindings, fn, prepared); err != nil {
			return nil, err
		}
		built := cfgbuild.BuildFunctionWithOptions(fn, bindings, cfgOptions)
		if built == nil || built.Graph == nil {
			return nil, ErrCFGRequired
		}
		if c.config.Stats != nil {
			c.config.Stats.LexicalCFGBuilds++
		}
		moduleTypes := newRequireAliasTypeResolver(moduleidentity.NewRequireAliases(bindings, fn.Stmts, fn), c.config.ModuleTypes)
		resolver := typeresolve.NewWithExternal(bindings, moduleTypes)
		// "function" is the established owner-local WIR identity. Parent protos
		// carry their own lexical display name independently, so sharing this one
		// prepared body preserves every existing Static digest and call owner.
		wirBody := wirlower.LowerFunctionWithResolverAndOptions("function", fn, bindings, built, resolver, c.forestWIROptions(prepared, sealedLuaTypeChecks))
		if c.config.Stats != nil {
			c.config.Stats.LexicalWIRLowerings++
		}
		prepared[fn] = preparedLexicalFunction{
			built:    built,
			wir:      wirlower.PreparedFunction{Body: wirBody, Graph: built.Graph},
			resolver: resolver,
		}
	}

	if rootFn != nil {
		root, ok := prepared[rootFn]
		if !ok || root.built == nil || root.wir.Body == nil {
			return nil, fmt.Errorf("prepare lexical forest: root function is not owned by bindings")
		}
		namespace := c.config.UnitNamespace
		if namespace == (lexicalidentity.UnitNamespace{}) {
			namespace = standaloneLexicalUnitNamespace(bindings, root.built, root.wir.Body)
		}
		forest, err := c.prepareLexicalStatics(bindings, functions, prepared, namespace)
		if err != nil {
			return nil, err
		}
		if err := forest.sealCallTopology(bindings); err != nil {
			return nil, err
		}
		return forest, nil
	}

	if err := validatePreparedChildren(bindings, nil, prepared); err != nil {
		return nil, err
	}
	built := cfgbuild.BuildChunkWithOptions(chunk, bindings, cfgOptions)
	if built == nil || built.Graph == nil {
		return nil, ErrCFGRequired
	}
	if c.config.Stats != nil {
		c.config.Stats.LexicalCFGBuilds++
	}
	moduleTypes := newRequireAliasTypeResolver(moduleidentity.NewRequireAliases(bindings, chunk, nil), c.config.ModuleTypes)
	resolver := typeresolve.NewWithExternal(bindings, moduleTypes)
	wirBody := wirlower.LowerWithResolverAndOptions("chunk", chunk, bindings, built, resolver, c.forestWIROptions(prepared, sealedLuaTypeChecks))
	if c.config.Stats != nil {
		c.config.Stats.LexicalWIRLowerings++
	}
	namespace := c.config.UnitNamespace
	if namespace == (lexicalidentity.UnitNamespace{}) {
		namespace = standaloneLexicalUnitNamespace(bindings, built, wirBody)
	}
	forest, err := c.prepareLexicalStatics(bindings, functions, prepared, namespace)
	if err != nil {
		return nil, err
	}
	if c.config.Stats != nil {
		c.config.Stats.StaticChunkPrepares++
	}
	forest.root, err = c.prepare(bindings, built, nil, wirBody, resolver, chunk)
	if err != nil {
		return nil, err
	}
	if err := forest.sealCallTopology(bindings); err != nil {
		return nil, err
	}
	return forest, nil
}

// prepareLexicalStatics is the second forest phase. Every body is prepared
// exactly once after the selected root has established one shared unit
// namespace for the complete lexical forest.
func (c *checker) prepareLexicalStatics(
	bindings *bind.Result,
	functions []*ast.FunctionExpr,
	prepared map[*ast.FunctionExpr]preparedLexicalFunction,
	namespace lexicalidentity.UnitNamespace,
) (*StaticForest, error) {
	if namespace == (lexicalidentity.UnitNamespace{}) {
		return nil, fmt.Errorf("prepare lexical forest: root has no stable unit namespace")
	}
	c.config.UnitNamespace = namespace
	forest := &StaticForest{functions: make(map[*ast.FunctionExpr]*Static, len(prepared))}
	for _, fn := range functions {
		item, ok := prepared[fn]
		if !ok || item.built == nil || item.wir.Body == nil || item.resolver == nil {
			return nil, fmt.Errorf("prepare lexical forest: function is absent from lowered forest")
		}
		if c.config.Stats != nil {
			c.config.Stats.StaticFunctionPrepares++
		}
		static, err := c.prepare(bindings, item.built, fn, item.wir.Body, item.resolver, functionSourceStmts(fn))
		if err != nil {
			return nil, err
		}
		forest.functions[fn] = static
	}
	return forest, nil
}

func validatePreparedChildren(bindings *bind.Result, parent *ast.FunctionExpr, prepared map[*ast.FunctionExpr]preparedLexicalFunction) error {
	for _, child := range bindings.NestedFunctions(parent) {
		item, ok := prepared[child]
		if !ok || item.wir.Body == nil || item.wir.Graph == nil || item.built == nil || item.resolver == nil {
			symbol, _ := bindings.FunctionSymbol(child)
			return fmt.Errorf("prepare lexical forest: child function %d is not prepared before its owner", symbol)
		}
	}
	return nil
}

func (c *checker) forestWIROptions(prepared map[*ast.FunctionExpr]preparedLexicalFunction, sealedLuaTypeChecks bool) wirlower.Options {
	return wirlower.Options{
		MethodReceiverTypes: c.config.MethodReceiverTypes,
		SealedLuaTypeChecks: sealedLuaTypeChecks,
		PreparedChild: func(fn *ast.FunctionExpr) (wirlower.PreparedFunction, bool) {
			item, ok := prepared[fn]
			return item.wir, ok
		},
	}
}
