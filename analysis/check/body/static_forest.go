package body

import (
	"fmt"

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
	root      *Static
	functions map[*ast.FunctionExpr]*Static
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
	built  *cfgbuild.Result
	wir    wirlower.PreparedFunction
	static *Static
}

func (c *checker) prepareBoundForest(chunk []ast.Stmt, rootFn *ast.FunctionExpr, bindings *bind.Result) (*StaticForest, error) {
	if bindings == nil {
		return nil, ErrUnsupportedCFG
	}
	functions := bindings.Functions()
	prepared := make(map[*ast.FunctionExpr]preparedLexicalFunction, len(functions))

	// Binding order is parent-before-child, therefore reverse order makes every
	// direct child available before its parent is lowered.
	for i := len(functions) - 1; i >= 0; i-- {
		fn := functions[i]
		if err := validatePreparedChildren(bindings, fn, prepared); err != nil {
			return nil, err
		}
		built := cfgbuild.BuildFunction(fn, bindings)
		if built == nil || built.Graph == nil {
			return nil, ErrUnsupportedCFG
		}
		if c.config.Stats != nil {
			c.config.Stats.LexicalCFGBuilds++
		}
		moduleTypes := newRequireAliasTypeResolver(moduleidentity.NewRequireAliases(bindings, fn.Stmts, fn), c.config.ModuleTypes)
		resolver := typeresolve.NewWithExternal(bindings, moduleTypes)
		// "function" is the established owner-local WIR identity. Parent protos
		// carry their own lexical display name independently, so sharing this one
		// prepared body preserves every existing Static digest and call owner.
		wirBody := wirlower.LowerFunctionWithResolverAndOptions("function", fn, bindings, built, resolver, c.forestWIROptions(prepared))
		if c.config.Stats != nil {
			c.config.Stats.StaticFunctionPrepares++
			c.config.Stats.LexicalWIRLowerings++
		}
		static := c.prepare(bindings, built, fn, wirBody, resolver, functionSourceStmts(fn))
		prepared[fn] = preparedLexicalFunction{
			built:  built,
			wir:    wirlower.PreparedFunction{Body: wirBody, Graph: built.Graph},
			static: static,
		}
	}

	forest := &StaticForest{functions: make(map[*ast.FunctionExpr]*Static, len(prepared))}
	for fn, item := range prepared {
		forest.functions[fn] = item.static
	}
	if rootFn != nil {
		if forest.functions[rootFn] == nil {
			return nil, fmt.Errorf("prepare lexical forest: root function is not owned by bindings")
		}
		return forest, nil
	}

	if err := validatePreparedChildren(bindings, nil, prepared); err != nil {
		return nil, err
	}
	built := cfgbuild.BuildChunk(chunk, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	if c.config.Stats != nil {
		c.config.Stats.LexicalCFGBuilds++
	}
	moduleTypes := newRequireAliasTypeResolver(moduleidentity.NewRequireAliases(bindings, chunk, nil), c.config.ModuleTypes)
	resolver := typeresolve.NewWithExternal(bindings, moduleTypes)
	wirBody := wirlower.LowerWithResolverAndOptions("chunk", chunk, bindings, built, resolver, c.forestWIROptions(prepared))
	if c.config.Stats != nil {
		c.config.Stats.StaticChunkPrepares++
		c.config.Stats.LexicalWIRLowerings++
	}
	forest.root = c.prepare(bindings, built, nil, wirBody, resolver, chunk)
	return forest, nil
}

func validatePreparedChildren(bindings *bind.Result, parent *ast.FunctionExpr, prepared map[*ast.FunctionExpr]preparedLexicalFunction) error {
	for _, child := range bindings.NestedFunctions(parent) {
		item, ok := prepared[child]
		if !ok || item.wir.Body == nil || item.wir.Graph == nil || item.static == nil {
			symbol, _ := bindings.FunctionSymbol(child)
			return fmt.Errorf("prepare lexical forest: child function %d is not prepared before its owner", symbol)
		}
	}
	return nil
}

func (c *checker) forestWIROptions(prepared map[*ast.FunctionExpr]preparedLexicalFunction) wirlower.Options {
	return wirlower.Options{
		MethodReceiverTypes: c.config.MethodReceiverTypes,
		PreparedChild: func(fn *ast.FunctionExpr) (wirlower.PreparedFunction, bool) {
			item, ok := prepared[fn]
			return item.wir, ok
		},
	}
}
