package body

import (
	"errors"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

var (
	ErrRegistryRequired = errors.New("check: registry is required")
	ErrStaticRequired   = errors.New("check: prepared body is required")
	ErrUnsupportedCFG   = errors.New("check: unsupported cfg")
)

type Config struct {
	Registry   *axis.Registry
	Globals    []string
	TypeValues *typevalue.Cache

	ExpressionValues             map[factflow.ExprRef]product.Value
	ExpressionValue              sourcevalue.ExpressionValueProvider
	VarargValue                  sourcevalue.VarargValueProvider
	CallOutcome                  callpayload.CallOutcomeProvider
	CallOutcomeFactory           CallOutcomeFactory
	SignatureArgumentType        SignatureArgumentTypeFunc
	SignatureArgumentTypeFactory SignatureArgumentTypeFactory
	Signatures                   signaturelookup.Source
	ModuleExports                importlookup.Source
	ModuleTypes                  typelookup.Source

	Visibility *visibility.Resolver

	EntryState state.State
	Initial    transfer.InitialState

	WidenAt    func(cfg.Point) bool
	WidenDelay func(cfg.Point) int

	Stats *Stats
}

type checker struct {
	config Config
}

// Stats holds caller-owned observational counters for body preparation and
// solving. Static preparation never retains the stats pointer.
type Stats struct {
	StaticChunkPrepares    int
	StaticFunctionPrepares int
	BodySolves             int
	Transfer               transfer.Stats
}

// Static is the reusable, entry-independent analysis artifact for one bound
// chunk or function body.
type Static struct {
	registry    *axis.Registry
	bindings    *bind.Result
	cfg         *cfgbuild.Result
	semantics   *semantics.Result
	signatures  signaturelookup.Source
	moduleTypes typelookup.Source
	moduleLoads importlookup.Source
	modules     moduleidentity.Projection
	signatureID *signatureIdentityResolver
	facts       factflow.Facts
	visibility  *visibility.Resolver
	sources     sourcevalue.SourceValues
	calleeValue CalleeValueFunc
	typeNS      *typeresolve.Resolver
	typeValues  *typevalue.Cache

	callOutcomeSupplement callpayload.CallOutcomeProvider
	signatureReturnOps    effectlowering.ReturnTypeOps
}

// HasCallSites reports whether the prepared body contains any statically
// extracted call-site facts. It is a static-shape query; it does not solve the
// body.
func (s *Static) HasCallSites() bool {
	return s != nil && s.facts.HasCallSites()
}

// SolveConfig holds per-solve inputs for a prepared body. These fields may
// close over caller summary readers or hold mutable caches, so they are never
// retained by Static preparation.
type SolveConfig struct {
	EntryState state.State
	Initial    transfer.InitialState
	TypeValues *typevalue.Cache

	CallOutcome                  callpayload.CallOutcomeProvider
	CallOutcomeFactory           CallOutcomeFactory
	SignatureArgumentType        SignatureArgumentTypeFunc
	SignatureArgumentTypeFactory SignatureArgumentTypeFactory

	WidenAt    func(cfg.Point) bool
	WidenDelay func(cfg.Point) int

	Stats *Stats
}

type Result struct {
	registry        *axis.Registry
	bindings        *bind.Result
	cfg             *cfgbuild.Result
	semantics       *semantics.Result
	signatures      signaturelookup.Source
	moduleTypes     typelookup.Source
	modules         moduleidentity.Projection
	signatureID     *signatureIdentityResolver
	facts           factflow.Facts
	exprRefinements map[factflow.ExprRef]factflow.ExpressionRefinement
	flow            transfer.Result
	boundary        map[cfg.Point]state.State
	boundaryXfer    transfer.NodeTransfer
	visibility      *visibility.Resolver
	sources         sourcevalue.SourceValues
	callOutcome     callpayload.CallOutcomeProvider
	typeValues      *typevalue.Cache
	functions       []*Result
	funcTypes       FunctionValueTypes
	callExprPts     map[*ast.FuncCallExpr]cfg.Point
}

type CallOutcomeContext struct {
	Facts                       factflow.Facts
	Sources                     sourcevalue.SourceValues
	CalleeValue                 CalleeValueFunc
	ReturnPresenceRelationsPath ReturnPresenceRelationsForPathFunc
}

type CallOutcomeFactory func(CallOutcomeContext) callpayload.CallOutcomeProvider

type CalleeValueFunc func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

type ReturnPresenceRelationsForPathFunc func(point cfg.Point, p pathdom.Path) []callpayload.CallReturnPresenceRelation

type SignatureArgumentTypeFunc func(ctx transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool)

type SignatureArgumentTypeFactory func(CallOutcomeContext) SignatureArgumentTypeFunc

func newChecker(config Config) (*checker, error) {
	if config.Registry == nil {
		return nil, ErrRegistryRequired
	}
	if config.TypeValues == nil {
		config.TypeValues = typevalue.NewCache()
	}
	return &checker{config: copyConfig(config)}, nil
}

func CheckChunk(stmts []ast.Stmt, config Config) (*Result, error) {
	prepared, err := PrepareChunk(stmts, config)
	if err != nil {
		return nil, err
	}
	return SolvePrepared(prepared, solveConfigFromConfig(config))
}

// CheckBoundChunk checks a chunk using caller-supplied lexical bindings.
func CheckBoundChunk(stmts []ast.Stmt, bindings *bind.Result, config Config) (*Result, error) {
	prepared, err := PrepareBoundChunk(stmts, bindings, config)
	if err != nil {
		return nil, err
	}
	return SolvePrepared(prepared, solveConfigFromConfig(config))
}

func CheckFunction(fn *ast.FunctionExpr, config Config) (*Result, error) {
	prepared, err := PrepareFunction(fn, config)
	if err != nil {
		return nil, err
	}
	return SolvePrepared(prepared, solveConfigFromConfig(config))
}

// CheckBoundFunction checks a function using caller-supplied lexical bindings.
func CheckBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result, config Config) (*Result, error) {
	prepared, err := PrepareBoundFunction(fn, bindings, config)
	if err != nil {
		return nil, err
	}
	return SolvePrepared(prepared, solveConfigFromConfig(config))
}

func PrepareChunk(stmts []ast.Stmt, config Config) (*Static, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.prepareChunk(stmts)
}

// PrepareBoundChunk prepares reusable static analysis for a chunk using
// caller-supplied lexical bindings.
func PrepareBoundChunk(stmts []ast.Stmt, bindings *bind.Result, config Config) (*Static, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.prepareBoundChunk(stmts, bindings)
}

func PrepareFunction(fn *ast.FunctionExpr, config Config) (*Static, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.prepareFunction(fn)
}

// PrepareBoundFunction prepares reusable static analysis for a function using
// caller-supplied lexical bindings.
func PrepareBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result, config Config) (*Static, error) {
	checker, err := newChecker(config)
	if err != nil {
		return nil, err
	}
	return checker.prepareBoundFunction(fn, bindings)
}

func SolvePrepared(prepared *Static, config SolveConfig) (*Result, error) {
	if prepared == nil {
		return nil, ErrStaticRequired
	}
	return prepared.Solve(config), nil
}
