package body

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/compiler/ast"
)

var (
	ErrRegistryRequired = errors.New("check: registry is required")
	ErrUnsupportedCFG   = errors.New("check: unsupported cfg")
)

type Config struct {
	Registry *axis.Registry
	Globals  []string

	ExpressionValues   map[factflow.ExprRef]product.Value
	ExpressionValue    sourcevalue.ExpressionValueProvider
	VarargValue        sourcevalue.VarargValueProvider
	CallOutcome        factapply.CallOutcomeProvider
	CallOutcomeFactory CallOutcomeFactory
	Signatures         signaturelookup.Source

	Visibility *visibility.Resolver

	EntryState state.State
	Initial    transfer.InitialState

	WidenAt    func(cfg.Point) bool
	WidenDelay func(cfg.Point) int
}

type checker struct {
	config Config
}

type Result struct {
	registry    *axis.Registry
	bindings    *bind.Result
	cfg         *cfgbuild.Result
	semantics   *semantics.Result
	signatures  signaturelookup.Source
	facts       factflow.Facts
	flow        transfer.Result
	visibility  *visibility.Resolver
	sources     sourcevalue.SourceValues
	callOutcome factapply.CallOutcomeProvider
	functions   []*Result
}

type CallOutcomeContext struct {
	Facts   factflow.Facts
	Sources sourcevalue.SourceValues
}

type CallOutcomeFactory func(CallOutcomeContext) factapply.CallOutcomeProvider

func newChecker(config Config) (*checker, error) {
	if config.Registry == nil {
		return nil, ErrRegistryRequired
	}
	return &checker{config: copyConfig(config)}, nil
}

func CheckChunk(stmts []ast.Stmt, config Config) (*Result, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.checkChunk(stmts)
}

// CheckBoundChunk checks a chunk using caller-supplied lexical bindings.
func CheckBoundChunk(stmts []ast.Stmt, bindings *bind.Result, config Config) (*Result, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.checkBoundChunk(stmts, bindings)
}

func CheckFunction(fn *ast.FunctionExpr, config Config) (*Result, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.checkFunction(fn)
}

// CheckBoundFunction checks a function using caller-supplied lexical bindings.
func CheckBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result, config Config) (*Result, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.checkBoundFunction(fn, bindings)
}
