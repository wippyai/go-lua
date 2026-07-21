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
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
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
	ErrBindingsRequired = errors.New("check: lexical bindings are required")
	ErrCFGRequired      = errors.New("check: cfg builder returned no graph")
)

type Config struct {
	Registry *axis.Registry
	// UnitNamespace is the stable logical-unit namespace used for lexical
	// identities. Artifact revisions and policy inputs are fenced separately by
	// cache digests. Zero selects deterministic standalone derivation from the
	// canonical semantic body.
	UnitNamespace lexicalidentity.UnitNamespace
	Globals       []string
	GlobalTypes   map[string]typ.Type
	TypeValues    *typevalue.Cache

	ExpressionValues             map[factflow.ExprRef]product.Value
	ExpressionValue              sourcevalue.ExpressionValueProvider
	VarargValue                  sourcevalue.VarargValueProvider
	CallOutcome                  callpayload.CallOutcomeProgram
	CallOutcomeFactory           CallOutcomeFactory
	SignatureArgumentType        effectlowering.SignatureArgumentTypeProgram
	SignatureArgumentTypeFactory SignatureArgumentTypeFactory
	SummaryInputDigests          func() []uint64
	// SummaryInputs is the identity-preserving form of SummaryInputDigests.
	// New interprocedural owners must use this provider so ResultVersion records
	// which exact summary key was present or missing, not only an anonymous
	// payload-digest multiset. SummaryInputDigests remains as a compatibility
	// seam for callers that cannot yet name their dependencies.
	SummaryInputs func() []SummaryInput
	// SummaryInputsComplete asserts that SummaryInputs is the complete dynamic
	// read set. It is publication metadata and never changes ResultVersion.
	SummaryInputsComplete bool
	Signatures            signaturelookup.Source
	ModuleExports         importlookup.Source
	ModuleTypes           typelookup.Source
	MethodReceiverTypes   map[symbol.ID]typ.Type

	Visibility *visibility.Resolver

	// StateLanes selects the State product-lattice lanes for each solve.
	// Nil uses the default lane set; a non-nil slice is the exact enabled set.
	StateLanes []state.LaneID

	EntryState             state.State
	Initial                transfer.InitialState
	ClosedDynamicAllValues []factapply.ClosedDynamicAllValueInvariant
	// Context cooperatively stops the underlying transfer worklist. Nil keeps
	// the legacy uncancelable behavior for callers that do not need a deadline.
	Context context.Context

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
	// LexicalCFGBuilds and LexicalWIRLowerings count source-owned forest
	// construction. They exclude pointer attachment of an already prepared child
	// into its parent proto, making accidental triangular subtree rebuilding
	// directly observable.
	LexicalCFGBuilds    int
	LexicalWIRLowerings int
	Observation         ObservationStats
}

// ObservationStats reports the finite stabilized-coordinate projection owned
// by result publication. No transfer is replayed while collecting these facts.
type ObservationStats struct {
	PlannedNodeOutputs        int
	PlannedBoundaryOutputs    int
	PlannedEdgeReachability   int
	ProjectedBoundaryOutputs  int
	ProjectedEdgeReachability int
}

// Static is the reusable, entry-independent analysis artifact for one bound
// chunk or function body.
type Static struct {
	lexicalBodyID                 lexicalidentity.StableLexicalBodyID
	tableLiteralSite              identity.TableLiteralSite
	registry                      *axis.Registry
	bindings                      *bind.Result
	cfg                           *cfgbuild.Result
	function                      *ast.FunctionExpr
	wir                           *wir.Body
	sourceStmts                   []ast.Stmt
	signatures                    signaturelookup.Source
	moduleTypes                   typelookup.Source
	moduleLoads                   importlookup.Source
	globals                       []string
	globalTypes                   map[string]typ.Type
	modules                       moduleidentity.Projection
	signatureID                   *signatureIdentityResolver
	sealedLuaTypeChecks           bool
	facts                         factflow.Facts
	operationPlan                 *operationplan.Plan
	readGraph                     ReadGraph
	symbolTypes                   map[symbol.ID]typ.Type
	assignments                   assignmentFactSet
	declarations                  declarationFactSet
	genericFors                   map[cfg.Point]GenericForFact
	visibility                    *visibility.Resolver
	sources                       sourcevalue.SourceValues
	readExpressionConfig          *readexpr.Config
	customExpressionValue         bool
	customExpressionValueProvider sourcevalue.ExpressionValueProvider
	calleeValue                   CalleeValueFunc
	receiverFn                    ReceiverCallableFunc
	typeNS                        *typeresolve.Resolver
	typeValues                    *typevalue.Cache

	entrySeeds         []state.ValueSeed
	entrySeedsPrepared bool

	callOutcomeSupplement callpayload.CallOutcomeProgram
	signatureReturnOps    effectlowering.ReturnTypeOps
	// The immutable digests cache canonical prepared-body inputs. A Static is
	// solved many times across prepass, summary convergence, contexts, and
	// materialization; re-encoding its WIR and imported manifests on every solve
	// is pure duplication. Per-solve state is appended to resultVersionPrefix by
	// computeResultVersion.
	immutableDigestMu              sync.Mutex
	resultVersionPrefix            internalhash.Writer
	resultVersionWidePrefix        []byte
	resultVersionPrefixReady       bool
	boundaryEnvironmentDigest      BoundaryEnvironmentDigest
	boundaryEnvironmentDigestReady bool

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

// PathSemanticAuthority returns the immutable prepared path authority shared
// by replacement tuple-engine semantic transactions. It does not expose a body
// transfer or retain solve State.
func (s *Static) PathSemanticAuthority() *factapply.PathSemanticAuthority {
	if s == nil {
		return nil
	}
	return factapply.NewPathSemanticAuthorityWithWiden(s.visibility, luaPathTypeProjector, luaCovariantWiden, s.typeValues)
}

// Registry returns the immutable value-axis registry owned by this prepared
// body. Interprocedural adapters use it to install exact read tracking before
// constructing per-solve providers.
func (s *Static) Registry() *axis.Registry {
	if s == nil {
		return nil
	}
	return s.registry
}

// Graph exposes the immutable CFG identity used by a prepared body.  It is
// provided for run-local resume guards; callers must not mutate it.
func (s *Static) Graph() cfg.Graph {
	if s == nil || s.cfg == nil {
		return nil
	}
	return s.cfg.Graph
}

// StableLexicalBodyID returns the deterministic semantic identity of this
// prepared lexical body.
func (s *Static) StableLexicalBodyID() lexicalidentity.StableLexicalBodyID {
	if s == nil {
		return lexicalidentity.StableLexicalBodyID{}
	}
	return s.lexicalBodyID
}

// OperationPlan returns the immutable, preparation-owned semantic plan. It is
// the handoff for compositional analyzers; callers must not mutate its graph or
// retained fact payloads.
func (s *Static) OperationPlan() *operationplan.Plan {
	if s == nil {
		return nil
	}
	return s.operationPlan
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

	CallOutcome                  callpayload.CallOutcomeProgram
	CallOutcomeFactory           CallOutcomeFactory
	SignatureArgumentType        effectlowering.SignatureArgumentTypeProgram
	SignatureArgumentTypeFactory SignatureArgumentTypeFactory
	// SummaryInputDigests returns content digests for summaries read during this
	// solve. It is consulted once at result-publication time.
	SummaryInputDigests func() []uint64
	// SummaryInputs returns exact, identity-bearing summary dependencies read by
	// this solve. It is consulted once at result-publication time.
	SummaryInputs func() []SummaryInput
	// SummaryInputsComplete asserts that SummaryInputs is the complete dynamic
	// read set. It is publication metadata and never changes ResultVersion.
	SummaryInputsComplete bool

	WidenAt    func(cfg.Point) bool
	WidenDelay func(cfg.Point) int
	Stats      *Stats
}

type Result struct {
	lexicalBodyID         lexicalidentity.StableLexicalBodyID
	tableLiteralSite      identity.TableLiteralSite
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
	sealedLuaTypeChecks   bool
	facts                 factflow.Facts
	operationPlan         *operationplan.Plan
	symbolTypes           map[symbol.ID]typ.Type
	assignments           assignmentFactSet
	declarations          declarationFactSet
	genericFors           map[cfg.Point]GenericForFact
	exprRefinements       sourcevalue.ExpressionRefinements
	typeNS                *typeresolve.Resolver
	flow                  transfer.Result
	published             PublishedFacts
	observationPlan       ObservationPlan
	observation           ObservationStats
	visibility            *visibility.Resolver
	sources               sourcevalue.SourceValues
	customExpressionValue bool
	calleeValue           CalleeValueFunc
	signatureArg          effectlowering.SignatureArgumentTypeProgram
	typeValues            *typevalue.Cache
	stateLanes            []state.LaneID
	functions             []*Result
	bodyParamObligations  bool
	funcTypes             FunctionValueTypes
	callExprPts           map[*ast.FuncCallExpr]cfg.Point
	diagnosticOutput      callpayload.DiagnosticOutput
	formalPathValue       FormalPathValueObservation

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
	resultLineage ResultVersionLineage
}

// FormalPathValueObservation is the selected formal read-model observation
// for one point-local path. boundary selects the sealed node-output coordinate
// when the consumer needs same-point boundary facts. It intentionally exposes
// a product value, never a State-shaped compatibility carrier.
type FormalPathValueObservation func(point cfg.Point, p pathdom.Path, boundary bool) (product.Value, bool)

type CallOutcomeContext struct {
	LexicalBodyID               lexicalidentity.StableLexicalBodyID
	Facts                       factflow.Facts
	OperationPlan               *operationplan.Plan
	Sources                     sourcevalue.SourceValues
	PathValue                   PathValueFunc
	DynamicRead                 DynamicReadFunc
	DynamicTableRead            DynamicReadFunc
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

// DynamicReadFunc resolves one syntax-free dynamic table read through the
// body's canonical visibility, heap, and type-index semantics. tableValue is
// either the owner at tablePath's root (DynamicRead) or the already-projected
// table itself (DynamicTableRead).
type DynamicReadFunc func(transfer.NodeContext, pathdom.Path, product.Value, product.Value, state.State) (product.Value, bool)

type CallOutcomeFactory func(CallOutcomeContext) callpayload.CallOutcomeProgram

type CalleeValueFunc func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

type ReceiverCallableFunc func(ctx transfer.NodeContext, site factflow.CallSiteView) (*typ.Function, bool)

type ReturnPresenceRelationsForPathFunc func(point cfg.Point, p pathdom.Path) []callpayload.CallReturnPresenceRelation

type SignatureArgumentTypeFactory func(CallOutcomeContext, effectlowering.SignatureOutcomeInputProgram) effectlowering.SignatureArgumentTypeProgram

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

// InputDigest returns the deterministic digest for a prepared body's static
// identity and solve inputs. Callers that cache a solve whose summary reads are
// discovered dynamically must pass a config without SummaryInputDigests here
// and validate those reads independently before reusing the cached value.
//
// This is deliberately the same digest used by ResultVersion, rather than a
// second, subtly different cache-key serialization. The 64-bit digest is a
// cache index, not reuse authority: callers must validate exact normalized
// state and dynamic dependency witnesses before adopting a cached solve.
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

// InputLineageContext returns the full immutable input-lineage record produced
// by the ResultVersion encoder while observing the solve context. It performs
// no body solve or semantic scan beyond the existing digest traversal.
func InputLineageContext(prepared *Static, config SolveConfig) (ResultVersionLineage, error) {
	if prepared == nil {
		return ResultVersionLineage{}, nil
	}
	return computeResultVersionLineage(prepared, config, config.EntryState, config.Initial)
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
