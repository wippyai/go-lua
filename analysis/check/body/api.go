package body

import (
	"context"
	"errors"
	"sync"

	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/solve/concreteflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

var (
	ErrRegistryRequired = errors.New("check: registry is required")
	ErrStaticRequired   = errors.New("check: prepared body is required")
	ErrUnsupportedCFG   = errors.New("check: unsupported cfg")
)

type Config struct {
	Registry    *axis.Registry
	Globals     []string
	GlobalTypes map[string]typ.Type
	TypeValues  *typevalue.Cache

	ExpressionValues             map[factflow.ExprRef]product.Value
	ExpressionValue              sourcevalue.ExpressionValueProvider
	VarargValue                  sourcevalue.VarargValueProvider
	CallOutcome                  callpayload.CallOutcomeProvider
	CallOutcomeFactory           CallOutcomeFactory
	SignatureArgumentType        SignatureArgumentTypeFunc
	SignatureArgumentTypeFactory SignatureArgumentTypeFactory
	SummaryInputDigests          func() []uint64
	Signatures                   signaturelookup.Source
	ModuleExports                importlookup.Source
	ModuleTypes                  typelookup.Source
	MethodReceiverTypes          map[symbol.ID]typ.Type

	Visibility *visibility.Resolver

	// StateLanes selects the State product-lattice lanes for each solve.
	// Nil uses the default lane set; a non-nil slice is the exact enabled set.
	StateLanes []state.LaneID
	Schedule   transfer.Schedule
	CompareWTO func(transfer.WTOComparison)

	EntryState             state.State
	Initial                transfer.InitialState
	ClosedDynamicAllValues []factapply.ClosedDynamicAllValueInvariant
	// Context cooperatively stops the underlying transfer worklist. Nil keeps
	// the legacy uncancelable behavior for callers that do not need a deadline.
	Context context.Context

	WidenAt    func(cfg.Point) bool
	WidenDelay func(cfg.Point) int

	// Resume is an optional run-local CFG checkpoint.  External summary users
	// supply ResumePoints after a monotone dependency change and use the point
	// hooks to rediscover point-scoped dependencies.
	Resume       *transfer.Session
	ResumePoints []cfg.Point
	BeforePoint  func(cfg.Point)
	AfterPoint   func(cfg.Point)

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
	Observation            ObservationStats
}

// ObservationStats reports deterministic seal work. Captured outputs are
// reused only when their point-input and every dynamic state read retain the
// exact solve-local revision after narrowing; all other planned outputs are
// recomputed once during the seal pass.
type ObservationStats struct {
	PlannedNodeOutputs        int
	CapturedNodeOutputs       int
	ValidatedNodeOutputs      int
	RecomputedNodeOutputs     int
	PlannedBoundaryOutputs    int
	PlannedEdgeReachability   int
	CapturedBoundaryOutputs   int
	ValidatedBoundaryOutputs  int
	RecomputedBoundaryOutputs int
	ProjectedBoundaryOutputs  int
	ProjectedEdgeReachability int
}

// Static is the reusable, entry-independent analysis artifact for one bound
// chunk or function body.
type Static struct {
	registry              *axis.Registry
	bindings              *bind.Result
	cfg                   *cfgbuild.Result
	function              *ast.FunctionExpr
	wir                   *wir.Body
	sourceStmts           []ast.Stmt
	signatures            signaturelookup.Source
	moduleTypes           typelookup.Source
	moduleLoads           importlookup.Source
	globals               []string
	globalTypes           map[string]typ.Type
	modules               moduleidentity.Projection
	signatureID           *signatureIdentityResolver
	facts                 factflow.Facts
	operationPlan         *operationplan.Plan
	symbolTypes           map[symbol.ID]typ.Type
	assignments           assignmentFactSet
	declarations          declarationFactSet
	genericFors           map[cfg.Point]GenericForFact
	genericForOperations  map[cfg.Point]factapply.GenericForOperation
	visibility            *visibility.Resolver
	sources               sourcevalue.SourceValues
	readExpressionConfig  *readexpr.Config
	customExpressionValue bool
	calleeValue           CalleeValueFunc
	receiverFn            ReceiverCallableFunc
	typeNS                *typeresolve.Resolver
	typeValues            *typevalue.Cache

	entrySeeds         []state.ValueSeed
	entrySeedsPrepared bool

	callOutcomeSupplement callpayload.CallOutcomeProvider
	signatureReturnOps    effectlowering.ReturnTypeOps
	wtoPlan               *solve.WTOPlan[cfg.Point]
	concreteFlow          *concreteflow.Plan

	// resultVersionPrefix is the digest state after immutable prepared-body
	// inputs. A Static is solved many times across prepass, summary convergence,
	// contexts, and materialization; re-encoding its WIR and imported manifests
	// on every solve is pure duplication. Per-solve state is appended to a copy
	// of this writer by computeResultVersion.
	resultVersionPrefixMu    sync.Mutex
	resultVersionPrefix      internalhash.Writer
	resultVersionPrefixReady bool

	compositionEligibilityOnce sync.Once
	compositionEligibility     CompositionEligibility
}

// HasCallSites reports whether the prepared body contains any statically
// extracted call-site facts. It is a static-shape query; it does not solve the
// body.
func (s *Static) HasCallSites() bool {
	return s != nil && s.facts.HasCallSites()
}

// HasDynamicIndexWrites reports whether the prepared body contains any
// statically extracted dynamic table write facts.
func (s *Static) HasDynamicIndexWrites() bool {
	return s != nil && s.facts.HasDynamicIndexWrites()
}

// KeySpace returns the structural key interner this prepared body solves under.
// Cross-summary entry states are rekeyed into it before the solve.
func (s *Static) KeySpace() *keyspace.KeySpace {
	if s == nil {
		return nil
	}
	return s.visibility.KeySpace()
}

// Graph exposes the immutable CFG identity used by a prepared body.  It is
// provided for run-local resume guards; callers must not mutate it.
func (s *Static) Graph() cfg.Graph {
	if s == nil || s.cfg == nil {
		return nil
	}
	return s.cfg.Graph
}

// SolveConfig holds per-solve inputs for a prepared body. These fields may
// close over caller summary readers or hold mutable caches, so they are never
// retained by Static preparation.
type SolveConfig struct {
	EntryState state.State
	Initial    transfer.InitialState
	// TypeValues is a fallback for manually constructed Static values. Prepared
	// bodies own the cache they were built with, and that prepared cache wins at
	// solve time so source readers, call outcomes, and transfer all share one
	// type-derived value database.
	TypeValues             *typevalue.Cache
	ClosedDynamicAllValues []factapply.ClosedDynamicAllValueInvariant
	Context                context.Context

	// StateLanes selects the State product-lattice lanes for this solve.
	// Nil uses the default lane set; a non-nil slice is the exact enabled set.
	StateLanes []state.LaneID
	Schedule   transfer.Schedule
	CompareWTO func(transfer.WTOComparison)

	CallOutcome                  callpayload.CallOutcomeProvider
	CallOutcomeFactory           CallOutcomeFactory
	SignatureArgumentType        SignatureArgumentTypeFunc
	SignatureArgumentTypeFactory SignatureArgumentTypeFactory
	// SummaryInputDigests returns content digests for summaries read during this
	// solve. It is consulted once at result-publication time.
	SummaryInputDigests func() []uint64

	WidenAt      func(cfg.Point) bool
	WidenDelay   func(cfg.Point) int
	Resume       *transfer.Session
	ResumePoints []cfg.Point
	BeforePoint  func(cfg.Point)
	AfterPoint   func(cfg.Point)

	Stats *Stats
}

type Result struct {
	registry              *axis.Registry
	bindings              *bind.Result
	cfg                   *cfgbuild.Result
	function              *ast.FunctionExpr
	wir                   *wir.Body
	sourceStmts           []ast.Stmt
	signatures            signaturelookup.Source
	moduleTypes           typelookup.Source
	modules               moduleidentity.Projection
	signatureID           *signatureIdentityResolver
	facts                 factflow.Facts
	symbolTypes           map[symbol.ID]typ.Type
	assignments           assignmentFactSet
	declarations          declarationFactSet
	genericFors           map[cfg.Point]GenericForFact
	exprRefinements       sourcevalue.ExpressionRefinements
	typeNS                *typeresolve.Resolver
	flow                  transfer.Result
	boundaryXfer          transfer.NodeTransfer
	edgeXfer              transfer.EdgeTransfer
	published             PublishedFacts
	observationPlan       ObservationPlan
	capturedNodeOutputs   map[cfg.Point]state.State
	observation           ObservationStats
	visibility            *visibility.Resolver
	sources               sourcevalue.SourceValues
	customExpressionValue bool
	callOutcome           callpayload.CallOutcomeProvider
	signatureArg          SignatureArgumentTypeFunc
	typeValues            *typevalue.Cache
	stateLanes            []state.LaneID
	functions             []*Result
	callContext           bool
	bodyParamObligations  bool
	funcTypes             FunctionValueTypes
	callExprPts           map[*ast.FuncCallExpr]cfg.Point

	queries resultQueryCache

	returnPoints     []cfg.Point
	returnPointsOK   bool
	paramValueSlots  []statekey.Value
	paramSlotsOK     bool
	reassignedParams map[statekey.Value]struct{}
	reassignedOK     bool
	returnTypeValues []product.Value
	returnTypesOK    bool

	resultVersion uint64
}

type CallOutcomeContext struct {
	Facts                       factflow.Facts
	Sources                     sourcevalue.SourceValues
	PathValue                   PathValueFunc
	ProtectedCall               func(transfer.NodeContext, factflow.CallSiteView) bool
	CalleeValue                 CalleeValueFunc
	ReceiverCallable            ReceiverCallableFunc
	ReturnPresenceRelationsPath ReturnPresenceRelationsForPathFunc
	KeySpace                    *keyspace.KeySpace
	TypeValues                  *typevalue.Cache
}

// PathValueFunc resolves one syntax-facing path through the body's canonical
// visibility and source-value semantics. Call providers use this read-only
// seam for resolvable receivers, which intentionally have no ReceiverSource.
type PathValueFunc func(transfer.NodeContext, pathdom.Path, state.State) (product.Value, bool)

type CallOutcomeFactory func(CallOutcomeContext) callpayload.CallOutcomeProvider

type CalleeValueFunc func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

type ReceiverCallableFunc func(ctx transfer.NodeContext, site factflow.CallSiteView) (*typ.Function, bool)

type ReturnPresenceRelationsForPathFunc func(point cfg.Point, p pathdom.Path) []callpayload.CallReturnPresenceRelation

type SignatureArgumentTypeFunc func(ctx transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool)

type SignatureArgumentTypeFactory func(CallOutcomeContext) SignatureArgumentTypeFunc

func newChecker(config Config) (*checker, error) {
	if config.Registry == nil {
		return nil, ErrRegistryRequired
	}
	if err := config.Signatures.Validate(); err != nil {
		return nil, err
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
	return SolvePrepared(prepared, config.SolveConfig())
}

// CheckBoundChunk checks a chunk using caller-supplied lexical bindings.
func CheckBoundChunk(stmts []ast.Stmt, bindings *bind.Result, config Config) (*Result, error) {
	prepared, err := PrepareBoundChunk(stmts, bindings, config)
	if err != nil {
		return nil, err
	}
	return SolvePrepared(prepared, config.SolveConfig())
}

func CheckFunction(fn *ast.FunctionExpr, config Config) (*Result, error) {
	prepared, err := PrepareFunction(fn, config)
	if err != nil {
		return nil, err
	}
	return SolvePrepared(prepared, config.SolveConfig())
}

// CheckBoundFunction checks a function using caller-supplied lexical bindings.
func CheckBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result, config Config) (*Result, error) {
	prepared, err := PrepareBoundFunction(fn, bindings, config)
	if err != nil {
		return nil, err
	}
	return SolvePrepared(prepared, config.SolveConfig())
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
	return prepared.solve(config)
}

// InputDigest returns the deterministic digest for a prepared body's static
// identity and solve inputs. Callers that cache a solve whose summary reads are
// discovered dynamically must pass a config without SummaryInputDigests here
// and validate those reads independently before reusing the cached value.
//
// This is deliberately the same digest used by ResultVersion, rather than a
// second, subtly different cache-key serialization.
func InputDigest(prepared *Static, config SolveConfig) uint64 {
	digest, _ := InputDigestContext(prepared, config)
	return digest
}

// InputDigestContext returns InputDigest while observing the solve context.
func InputDigestContext(prepared *Static, config SolveConfig) (uint64, error) {
	if prepared == nil {
		return 0, nil
	}
	return computeResultVersion(prepared, config, config.EntryState, config.Initial)
}

// IdentityDigest is the stable content identity of the prepared body. It
// excludes per-application inputs such as the entry state and caller summary
// environment, which are included in InputDigest and cache dependency checks.
func (s *Static) IdentityDigest() uint64 {
	digest, _ := s.IdentityDigestContext(context.Background())
	return digest
}

// IdentityDigestContext returns IdentityDigest while observing ctx.
func (s *Static) IdentityDigestContext(ctx context.Context) (uint64, error) {
	if s == nil {
		return 0, nil
	}
	return computeResultVersion(s, SolveConfig{Context: ctx}, state.State{}, nil)
}
