package check

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

var (
	ErrRegistryRequired = body.ErrRegistryRequired
	ErrUnsupportedCFG   = body.ErrUnsupportedCFG
)

type Config = body.Config
type Result = body.Result

func CheckChunk(stmts []ast.Stmt, config Config) (*Result, error) {
	result, err := program.RunChunk(stmts, program.Config{Check: config})
	if err != nil {
		return nil, err
	}
	return result.RootResult(), nil
}

// CheckBoundChunk checks a chunk using caller-supplied lexical bindings.
func CheckBoundChunk(stmts []ast.Stmt, bindings *bind.Result, config Config) (*Result, error) {
	result, err := program.RunBoundChunk(stmts, bindings, program.Config{Check: config})
	if err != nil {
		return nil, err
	}
	return result.RootResult(), nil
}

func CheckFunction(fn *ast.FunctionExpr, config Config) (*Result, error) {
	result, err := program.RunFunction(fn, program.Config{Check: config})
	if err != nil {
		return nil, err
	}
	return result.RootResult(), nil
}

// CheckBoundFunction checks a function using caller-supplied lexical bindings.
func CheckBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result, config Config) (*Result, error) {
	result, err := program.RunBoundFunction(fn, bindings, program.Config{Check: config})
	if err != nil {
		return nil, err
	}
	return result.RootResult(), nil
}
