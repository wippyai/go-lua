package check

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factflow/apply"
	"github.com/wippyai/go-lua/analysis/engine/factflow/source"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

var (
	ErrRegistryRequired = errors.New("check: registry is required")
	ErrUnsupportedCFG   = errors.New("check: unsupported cfg")
)

type Config struct {
	Registry *axis.Registry
	Globals  []string

	ExpressionValues map[factflow.ExprRef]product.Value
	ExpressionValue  source.ExpressionValueProvider
	VarargValue      source.VarargValueProvider
	CallResults      apply.CallResultProvider

	Visibility *visibility.Resolver

	EntryState state.State
	Initial    transfer.InitialState

	WidenAt    func(cfg.Point) bool
	WidenDelay func(cfg.Point) int
}

type Checker struct {
	config Config
}

type Result struct {
	registry  *axis.Registry
	bindings  *bind.Result
	cfg       *cfgbuild.Result
	semantics *semantics.Result
	facts     factflow.Facts
	flow      transfer.Result
	functions []*Result
}

func (r *Result) Registry() *axis.Registry {
	if r == nil {
		return nil
	}
	return r.registry
}

func (r *Result) Graph() cfg.Graph {
	if r == nil || r.cfg == nil {
		return nil
	}
	return r.cfg.Graph
}

func (r *Result) StateAt(point cfg.Point) (state.State, bool) {
	if r == nil || r.flow == nil {
		return state.State{}, false
	}
	st, ok := r.flow[point]
	if !ok {
		return state.State{}, false
	}
	return st.Clone(), true
}

func (r *Result) ExitState() (state.State, bool) {
	graph := r.Graph()
	if graph == nil {
		return state.State{}, false
	}
	return r.StateAt(graph.Exit())
}

func (r *Result) ReturnPoints() []cfg.Point {
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	points := graph.RPO()
	out := make([]cfg.Point, 0, len(points))
	for _, point := range points {
		if _, ok := r.ReturnFact(point); ok {
			out = append(out, point)
		}
	}
	return out
}

func (r *Result) ReturnFact(point cfg.Point) (semantics.ReturnFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.ReturnFact{}, false
	}
	return r.semantics.Return(point)
}

func (r *Result) LocalAssignment(point cfg.Point) (semantics.LocalAssignmentFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.LocalAssignmentFact{}, false
	}
	return r.semantics.LocalAssignment(point)
}

func (r *Result) OrdinaryAssignment(point cfg.Point) (semantics.OrdinaryAssignmentFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.OrdinaryAssignmentFact{}, false
	}
	return r.semantics.OrdinaryAssignment(point)
}

func (r *Result) Call(point cfg.Point) (semantics.CallFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.CallFact{}, false
	}
	return r.semantics.Call(point)
}

func (r *Result) CallSite(point cfg.Point) (factflow.CallSite, bool) {
	if r == nil {
		return factflow.CallSite{}, false
	}
	return r.facts.CallSite(point)
}

func (r *Result) BranchCondition(point cfg.Point) (semantics.BranchConditionFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.BranchConditionFact{}, false
	}
	return r.semantics.BranchCondition(point)
}

func (r *Result) TypeDefinition(point cfg.Point) (cfgfacts.TypeDefinitionFact, bool) {
	if r == nil || r.semantics == nil {
		return cfgfacts.TypeDefinitionFact{}, false
	}
	return r.semantics.TypeDefinition(point)
}

func (r *Result) FunctionDefinition(point cfg.Point) (cfgfacts.FunctionDefinitionFact, bool) {
	if r == nil || r.semantics == nil {
		return cfgfacts.FunctionDefinitionFact{}, false
	}
	return r.semantics.FunctionDefinition(point)
}

func (r *Result) FunctionResults() []*Result {
	if r == nil || len(r.functions) == 0 {
		return nil
	}
	return append([]*Result(nil), r.functions...)
}

func (r *Result) SymbolName(id symbol.ID) string {
	if r == nil || r.bindings == nil {
		return ""
	}
	return r.bindings.Name(id)
}

func (r *Result) SymbolTypeAnnotation(id symbol.ID) (ast.TypeExpr, bool) {
	if r == nil || r.bindings == nil || id == 0 {
		return nil, false
	}
	if fn, ok := r.bindings.DeclaringFunction(id); ok {
		for _, slot := range r.bindings.ParamSlots(fn) {
			if slot.Symbol == id && slot.Type != nil {
				return slot.Type, true
			}
		}
	}
	graph := r.Graph()
	if graph == nil {
		return nil, false
	}
	for _, point := range graph.RPO() {
		fact, ok := r.LocalAssignment(point)
		if ok && fact.Symbol == id && fact.Type != nil {
			return fact.Type, true
		}
	}
	return nil, false
}

func (r *Result) ReturnArity(point cfg.Point) (int, bool) {
	if r == nil {
		return 0, false
	}
	fact, ok := r.facts.Return(point)
	if !ok {
		return 0, false
	}
	return len(fact.Sources()), true
}

func (r *Result) LocalSymbols(stmt *ast.LocalAssignStmt) []symbol.ID {
	if r == nil || r.bindings == nil {
		return nil
	}
	return r.bindings.LocalSymbols(stmt)
}

func New(config Config) (*Checker, error) {
	if config.Registry == nil {
		return nil, ErrRegistryRequired
	}
	return &Checker{config: copyConfig(config)}, nil
}

func CheckChunk(stmts []ast.Stmt, config Config) (*Result, error) {
	checker, err := New(config)
	if err != nil {
		return nil, err
	}
	return checker.CheckChunk(stmts)
}

// CheckBoundChunk checks a chunk using caller-supplied lexical bindings.
func CheckBoundChunk(stmts []ast.Stmt, bindings *bind.Result, config Config) (*Result, error) {
	checker, err := New(config)
	if err != nil {
		return nil, err
	}
	return checker.CheckBoundChunk(stmts, bindings)
}

func CheckFunction(fn *ast.FunctionExpr, config Config) (*Result, error) {
	checker, err := New(config)
	if err != nil {
		return nil, err
	}
	return checker.CheckFunction(fn)
}

// CheckBoundFunction checks a function using caller-supplied lexical bindings.
func CheckBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result, config Config) (*Result, error) {
	checker, err := New(config)
	if err != nil {
		return nil, err
	}
	return checker.CheckBoundFunction(fn, bindings)
}

func (c *Checker) CheckChunk(stmts []ast.Stmt) (*Result, error) {
	bindings := bind.BindChunk(stmts, bind.Options{Globals: c.config.Globals})
	return c.CheckBoundChunk(stmts, bindings)
}

func (c *Checker) CheckBoundChunk(stmts []ast.Stmt, bindings *bind.Result) (*Result, error) {
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		return nil, fmt.Errorf("check: extract chunk semantics: %w", err)
	}
	result := c.run(bindings, built, sem)
	c.attachFunctionResults(result, bindings, nil)
	return result, nil
}

func (c *Checker) CheckFunction(fn *ast.FunctionExpr) (*Result, error) {
	bindings := bind.BindFunction(fn, bind.Options{Globals: c.config.Globals})
	return c.CheckBoundFunction(fn, bindings)
}

func (c *Checker) CheckBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result) (*Result, error) {
	built := cfgbuild.BuildFunction(fn, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := semantics.ExtractFunction(fn, bindings, built)
	if err != nil {
		return nil, fmt.Errorf("check: extract function semantics: %w", err)
	}
	result := c.run(bindings, built, sem)
	c.attachFunctionResults(result, bindings, fn)
	return result, nil
}

func (c *Checker) run(bindings *bind.Result, built *cfgbuild.Result, sem *semantics.Result) *Result {
	config := c.config
	facts := transferfacts.Lower(sem, built.Graph, transferfacts.Config{Registry: config.Registry})
	sources := source.NewSourceValues(source.SourceValuesConfig{
		Registry:         config.Registry,
		ExpressionValues: config.ExpressionValues,
		ExpressionValue:  config.ExpressionValue,
		VarargValue:      config.VarargValue,
	})
	flow := transfer.Run(transfer.Config{
		Graph:      built.Graph,
		Registry:   config.Registry,
		EntryState: config.EntryState,
		Initial:    config.Initial,
		NodeTransfer: apply.NewFactsNodeTransfer(apply.FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sources,
			CallResults: config.CallResults,
			Visibility:  config.Visibility,
		}),
		EdgeTransfer: apply.NewFactsEdgeTransfer(apply.FactsEdgeTransferConfig{
			Facts:      facts,
			Visibility: config.Visibility,
		}),
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
	})
	return &Result{
		registry:  config.Registry,
		bindings:  bindings,
		cfg:       built,
		semantics: sem,
		facts:     facts,
		flow:      flow,
	}
}

func (c *Checker) attachFunctionResults(parent *Result, bindings *bind.Result, fn *ast.FunctionExpr) {
	if parent == nil || bindings == nil {
		return
	}
	for _, nested := range bindings.NestedFunctions(fn) {
		child, ok := c.checkNestedFunction(nested, bindings)
		if !ok {
			continue
		}
		c.attachFunctionResults(child, bindings, nested)
		parent.functions = append(parent.functions, child)
	}
}

func (c *Checker) checkNestedFunction(fn *ast.FunctionExpr, bindings *bind.Result) (*Result, bool) {
	built := cfgbuild.BuildFunction(fn, bindings)
	if built == nil || built.Graph == nil {
		return nil, false
	}
	sem, err := semantics.ExtractFunction(fn, bindings, built)
	if err != nil {
		return nil, false
	}
	return c.run(bindings, built, sem), true
}

func copyConfig(config Config) Config {
	config.Globals = append([]string(nil), config.Globals...)
	if len(config.ExpressionValues) != 0 {
		values := make(map[factflow.ExprRef]product.Value, len(config.ExpressionValues))
		for ref, value := range config.ExpressionValues {
			values[ref] = value
		}
		config.ExpressionValues = values
	}
	return config
}
