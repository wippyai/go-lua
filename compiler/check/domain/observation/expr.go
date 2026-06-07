// Package observation projects solved abstract-interpreter evidence into value
// observations without re-entering checker synthesis.
package observation

import (
	"strconv"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/callreturn"
	"github.com/wippyai/go-lua/compiler/check/domain/conditionexpr"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/check/domain/indexread"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/typepath"
	"github.com/wippyai/go-lua/compiler/check/scope"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/core"
	"github.com/wippyai/go-lua/compiler/check/synth/intercept"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// SymbolTypeLookup projects canonical function facts or other product-owned
// symbol types into the observation surface.
type SymbolTypeLookup func(sym cfg.SymbolID) typ.Type

// TypeResolver resolves annotation AST into concrete types in the lexical scope
// visible at a CFG point.
type TypeResolver func(ast.TypeExpr, *scope.State) typ.Type

type postStateTypeFacts interface {
	PostEffectiveTypeAt(point cfg.Point, sym cfg.SymbolID) flow.TypedValue
}

type postStatePathFacts interface {
	PostRefinedPathAt(point cfg.Point, path constraint.Path) flow.TypedValue
}

type callReturnFacts interface {
	CallReturnTypesAt(point cfg.Point, call *ast.FuncCallExpr, expected typ.Type) ([]typ.Type, bool)
}

// Config supplies immutable solved-state inputs for expression observation.
type Config struct {
	Graph                    *cfg.Graph
	Bindings                 *bind.BindingTable
	Scopes                   map[cfg.Point]*scope.State
	DefaultScope             *scope.State
	Facts                    flow.TypeFacts
	Inputs                   *flow.Inputs
	Flow                     api.FlowOps
	Ctx                      *db.QueryContext
	TypeOps                  querycore.TypeOps
	LiteralSignatureProvider api.LiteralSignatureLookup
	FunctionType             SymbolTypeLookup
	ResolveType              TypeResolver
	// GlobalTypeOverlay is the normalized source-global type carrier.
	GlobalTypeOverlay  globalenv.TypeOverlay
	PreferPreState     bool
	PreferPostState    bool
	PreserveProof      bool
	CallArgumentProofs bool
	GradualParamReads  bool
	// StrictContainerWrite restricts the gradual-top admission to the true gradual
	// top (an unannotated-parameter path root). A write into a typed container's
	// element slot uses it so a field/index read off a typed container whose member
	// is `any` does NOT relax the element-domain obligation: storing into invariant
	// container state is not a consistency-boundary coercion.
	StrictContainerWrite bool
	LocalCondition       *constraint.Condition

	// RecursiveFamilies is the compilation-scoped recursive-family interner used
	// to seal an observed class metatable into the same shared family the synthesis
	// engine seals, so a constructor's observed return metatable resolves to the
	// converging family handle instead of a degraded structural snapshot.
	RecursiveFamilies *typ.RecursiveFamilyInterner

	// ClassFamilyJoin is the function-aware class-family body join the seal widens
	// with. It is supplied by the pipeline so observation seals a metatable with the
	// same join the synthesis engine uses.
	ClassFamilyJoin func(existing, candidate typ.Type) typ.Type
}

// Projector observes expression value types from solved product state.
type Projector struct {
	cfg         Config
	globalTypes globalenv.TypeOverlay
}

var _ flow.PathObservationFacts = Projector{}

// New returns a solved-state expression observation projector.
func New(cfg Config) Projector {
	if cfg.Bindings == nil && cfg.Graph != nil {
		cfg.Bindings = cfg.Graph.Bindings()
	}
	return Projector{
		cfg:         cfg,
		globalTypes: globalenv.NormalizeTypeOverlay(cfg.GlobalTypeOverlay),
	}
}

// WithPreStateReads returns a projector that prefers point-entry path reads.
// This models assignment RHS boundaries where the target may also appear in
// the source expression.
func (p Projector) WithPreStateReads() Projector {
	p.cfg.PreferPreState = true
	p.cfg.PreferPostState = false
	return p
}

// WithPostStateReads returns a projector that prefers point-exit product facts.
// This models expression events that occur inside a node after same-node stage
// effects have been applied, such as call arguments inside a generic-for body
// whose iterator targets are bound by that node's transfer.
func (p Projector) WithPostStateReads() Projector {
	p.cfg.PreferPostState = true
	p.cfg.PreferPreState = false
	return p
}

// WithProofValues returns a projector for diagnostics/proofs. It preserves
// branch-proven literal refinements instead of admitting them into the
// convergent value-product representation.
func (p Projector) WithProofValues() Projector {
	p.cfg.PreserveProof = true
	return p
}

// WithCallArgumentProofs observes call arguments at hard proof boundaries. Body
// contract seeds remain available for ordinary body interpretation, but an
// unrelated branch guard must not turn a self-inferred entry assumption into
// proof for a concrete callee parameter.
func (p Projector) WithCallArgumentProofs() Projector {
	p.cfg.CallArgumentProofs = true
	return p
}

// WithStrictContainerWrite returns a projector whose gradual-top admission is
// restricted to the true gradual top (an unannotated-parameter path root). It is
// used for an index-write into a typed container, where a field/index read off a
// typed container whose member is `any` must not relax the element-domain
// obligation (a closed-domain write is a store, not a boundary coercion).
func (p Projector) WithStrictContainerWrite() Projector {
	p.cfg.StrictContainerWrite = true
	return p
}

// WithGradualParamReads returns a projector that reads an unannotated, fully
// unconstrained parameter as gradual `any` rather than the unrefined `unknown`
// inference seed. Diagnostic walkers use this so value operators on a gradual
// parameter the public surface already admits as `any?` are not rejected, while
// fact computation keeps the precise `unknown` seed for interproc refinement.
func (p Projector) WithGradualParamReads() Projector {
	p.cfg.GradualParamReads = true
	return p
}

// WithExprCondition returns a projector scoped by the truthiness/probe condition
// implied by expr at point. Diagnostic walkers use this to consume the same
// proof-query surface as expression observation instead of constructing local
// flow override maps.
func (p Projector) WithExprCondition(expr ast.Expr, point cfg.Point, truthy bool) Projector {
	return p.withExprCondition(expr, point, truthy)
}

// Field projects a named field through the projector's query engine.
func (p Projector) Field(t typ.Type, name string) (typ.Type, bool) {
	if p.cfg.TypeOps != nil {
		return p.cfg.TypeOps.Field(p.cfg.Ctx, t, name)
	}
	return querycore.Field(t, name)
}

// Index projects an indexed read through the projector's query engine.
func (p Projector) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	if p.cfg.TypeOps != nil {
		return p.cfg.TypeOps.Index(p.cfg.Ctx, t, key)
	}
	return querycore.Index(t, key)
}

// RuntimeIndex projects Lua runtime indexed-read semantics. Unlike Index, a
// missing key on a table-like value is a defined nil read rather than an
// indexing error.
func (p Projector) RuntimeIndex(t typ.Type, key typ.Type) (typ.Type, bool) {
	if p.cfg.TypeOps != nil {
		if out, ok := p.cfg.TypeOps.Index(p.cfg.Ctx, t, key); ok {
			return out, true
		}
		if querycore.MissingFieldReadsNil(t) {
			return typ.Nil, true
		}
		return nil, false
	}
	return querycore.RuntimeIndex(t, key)
}

// FromFuncResult returns the solved-state observation projector for a completed
// function analysis result.
func FromFuncResult(result *api.FuncResult, functionType SymbolTypeLookup) Projector {
	return FromSolvedObservationState(result.ObservationState(), functionType)
}

// FromAnalysisView returns the solved-state observation projector for a stable
// function-analysis view.
func FromAnalysisView(result *api.FuncAnalysisView, functionType SymbolTypeLookup) Projector {
	return FromSolvedObservationState(result.ObservationState(), functionType)
}

// FromSolvedObservationState returns an observation projector from the normalized
// API state bundle. Result-like producers should normalize once into this shape
// instead of teaching observation about their storage layout.
func FromSolvedObservationState(state api.SolvedObservationState, functionType SymbolTypeLookup) Projector {
	cfg := Config{
		Graph:                    state.Graph,
		Bindings:                 state.Bindings,
		Scopes:                   state.Scopes,
		DefaultScope:             state.DefaultScope,
		Facts:                    state.Facts,
		Inputs:                   state.Inputs,
		Flow:                     state.Flow,
		Ctx:                      state.Ctx,
		TypeOps:                  state.TypeOps,
		LiteralSignatureProvider: state.LiteralSignatureProvider,
		FunctionType:             functionType,
		ResolveType:              state.ResolveType,
		GlobalTypeOverlay:        state.GlobalTypeOverlay,
		RecursiveFamilies:        state.RecursiveFamilies,
		ClassFamilyJoin:          state.ClassFamilyJoin,
	}
	return New(cfg)
}

// ObservedArgumentType joins the current expression observation with the
// solved pre-state/path observation at a call boundary.
func ObservedArgumentType(result *api.FuncResult, point cfg.Point, arg ast.Expr, current typ.Type, bindings *bind.BindingTable) typ.Type {
	if result == nil || result.Graph == nil || arg == nil {
		return current
	}
	argPath := flowpath.FromExprWithBindings(arg, nil, bindings)
	if argPath.IsEmpty() || argPath.Symbol == 0 {
		return current
	}
	argPath = flowpath.WithVersion(argPath, result.Graph, point)
	projector := FromFuncResult(result, nil)
	obs := projector.pathObservationFacts().ObservePath(flow.PathObservationQuery{
		Point:               point,
		Path:                argPath,
		View:                flow.PathReadPre,
		AllowConditionProof: false,
		PreserveProof:       true,
	})
	if !obs.Resolved() || obs.Source == flow.PathObservationDeclared {
		return current
	}
	return paramevidence.MergeArgumentObservation(current, obs.Type)
}

// TypeOf observes the single-value type of expr at point p.
func (p Projector) TypeOf(expr ast.Expr, point cfg.Point) typ.Type {
	return p.typeOf(expr, point, nil)
}

// TypeOfWithExpected observes expr using an optional contextual type.
func (p Projector) TypeOfWithExpected(expr ast.Expr, point cfg.Point, expected typ.Type) typ.Type {
	return p.typeOf(expr, point, expected)
}

// ExprSatisfiesContract proves that expr satisfies a native parameter contract
// using both structural type evidence and canonical path facts. It is the
// diagnostics-facing proof predicate for body-summary obligations; callers do
// not need to collapse ParamContract to a broad projected type first.
func (p Projector) ExprSatisfiesContract(
	expr ast.Expr,
	point cfg.Point,
	contract paramevidence.ParamContract,
	satisfies paramevidence.TypeSatisfaction,
) bool {
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() || path.Symbol == 0 {
		return false
	}
	actual := p.TypeOf(expr, point)
	return paramevidence.ContractSatisfiedByTypeOrProof(
		contract,
		actual,
		satisfies,
		contractPathProof{projector: p, point: point, root: path},
	)
}

type contractPathProof struct {
	projector Projector
	point     cfg.Point
	root      constraint.Path
}

func (p contractPathProof) PathSatisfies(segments []constraint.Segment, _ paramevidence.ParamContract, expected typ.Type) bool {
	if expected == nil {
		return false
	}
	path := p.root
	for _, seg := range segments {
		path = path.Append(seg)
	}
	observed := p.projector.pathTypeAtPath(path, p.point)
	return !typ.IsAbsentOrUnknown(observed) && subtype.IsSubtype(observed, expected)
}

func (p contractPathProof) ElementFieldSatisfies(array []constraint.Segment, field []constraint.Segment, contract paramevidence.ParamContract, expected typ.Type) bool {
	if len(field) == 0 || expected == nil {
		return false
	}
	arrayPath := p.root
	for _, seg := range array {
		arrayPath = arrayPath.Append(seg)
	}
	arrayKey := flow.StablePathKey(arrayPath)
	if arrayKey == "" {
		return false
	}
	provider := p.projector.appendElementFieldOriginProvider()
	if provider == nil {
		return false
	}
	for _, use := range provider.AppendElementFieldSourcesAt(p.point, arrayKey, field) {
		sourceType := p.projector.appendElementFieldOriginSourceType(p.point, use, nil, nil)
		if !typ.IsAbsentOrUnknown(sourceType) && subtype.IsSubtype(sourceType, expected) {
			return true
		}
		if paramevidence.ContractSatisfiedByTypeOrProof(contract, sourceType, subtype.Consistent, p) {
			return true
		}
	}
	return false
}

// MultiTypeOf observes the Lua return vector of expr at point p.
func (p Projector) MultiTypeOf(expr ast.Expr, point cfg.Point) []typ.Type {
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		return p.callReturns(call, point)
	}
	t := p.TypeOf(expr, point)
	if t == nil {
		t = typ.Unknown
	}
	return []typ.Type{t}
}

func (p Projector) typeOf(expr ast.Expr, point cfg.Point, expected typ.Type) typ.Type {
	if expr == nil {
		return typ.Unknown
	}
	if attr, ok := expr.(*ast.AttrGetExpr); ok {
		return p.attrTypeWithExpected(attr, point, expected)
	}
	pathType := p.pathType(expr, point)
	contractType := p.bodyContractPathType(expr, point)
	if !typ.IsAbsentOrUnknown(pathType) && (!typ.IsAny(pathType) || typ.IsAbsentOrUnknown(contractType)) {
		return p.finishExpectedObservation(pathType, expected, expr, point)
	}
	if !typ.IsAbsentOrUnknown(contractType) {
		return p.finishExpectedObservation(contractType, expected, expr, point)
	}
	if !typ.IsAbsentOrUnknown(pathType) {
		return p.finishExpectedObservation(pathType, expected, expr, point)
	}
	switch e := expr.(type) {
	case *ast.NilExpr:
		return typ.Nil
	case *ast.TrueExpr:
		return p.contextualLiteral(typ.LiteralBool(true), expected)
	case *ast.FalseExpr:
		return p.contextualLiteral(typ.LiteralBool(false), expected)
	case *ast.NumberExpr:
		return p.contextualLiteral(ops.ParseNumber(e.Value), expected)
	case *ast.StringExpr:
		return p.contextualLiteral(typ.LiteralString(e.Value), expected)
	case *ast.IdentExpr:
		return p.identTypeWithExpected(e, point, expected)
	case *ast.TableExpr:
		return p.tableType(e, point, expected)
	case *ast.FunctionExpr:
		return p.functionType(e, expected)
	case *ast.FuncCallExpr:
		return firstType(p.callReturnsWithExpected(e, point, expected))
	case *ast.CastExpr:
		return p.castType(e, point)
	case *ast.LogicalOpExpr:
		return p.logicalType(e, point, expected)
	case *ast.NonNilAssertExpr:
		return narrow.RemoveNil(p.TypeOf(e.Expr, point))
	case *ast.RelationalOpExpr:
		return p.binaryType(p.TypeOf(e.Lhs, point), e.Operator, p.TypeOf(e.Rhs, point), typ.Boolean)
	case *ast.StringConcatOpExpr:
		return p.binaryType(p.TypeOf(e.Lhs, point), "..", p.TypeOf(e.Rhs, point), typ.String)
	case *ast.ArithmeticOpExpr:
		return p.binaryType(p.TypeOf(e.Lhs, point), e.Operator, p.TypeOf(e.Rhs, point), typ.Number)
	case *ast.UnaryMinusOpExpr:
		return p.unaryType("-", p.TypeOf(e.Expr, point), typ.Number)
	case *ast.UnaryBNotOpExpr:
		return p.unaryType("~", p.TypeOf(e.Expr, point), typ.Integer)
	case *ast.UnaryLenOpExpr:
		return p.unaryType("#", p.TypeOf(e.Expr, point), typ.Integer)
	case *ast.UnaryNotOpExpr:
		return typ.Boolean
	case *ast.Comma3Expr:
		return typ.Unknown
	default:
		return typ.Unknown
	}
}

func (p Projector) castType(expr *ast.CastExpr, point cfg.Point) typ.Type {
	if expr == nil {
		return typ.Unknown
	}
	if p.cfg.ResolveType != nil {
		if t := p.cfg.ResolveType(expr.Type, p.scopeAt(point)); t != nil {
			return t
		}
	}
	return p.TypeOf(expr.Expr, point)
}

func (p Projector) scopeAt(point cfg.Point) *scope.State {
	if p.cfg.Scopes != nil {
		if sc := p.cfg.Scopes[point]; sc != nil {
			return sc
		}
		if p.cfg.Graph != nil {
			if sc := p.cfg.Scopes[p.cfg.Graph.Entry()]; sc != nil {
				return sc
			}
		}
	}
	return p.cfg.DefaultScope
}

func (p Projector) binaryType(left typ.Type, op string, right typ.Type, fallback typ.Type) typ.Type {
	if left == nil || right == nil {
		return fallback
	}
	if p.cfg.TypeOps != nil {
		if t := p.cfg.TypeOps.BinaryOp(p.cfg.Ctx, left, op, right); t != nil {
			return t
		}
	} else if t := querycore.BinaryOp(left, op, right); t != nil {
		return t
	}
	return fallback
}

func (p Projector) unaryType(op string, operand typ.Type, fallback typ.Type) typ.Type {
	if operand == nil {
		return fallback
	}
	if p.cfg.TypeOps != nil {
		if t := p.cfg.TypeOps.UnaryOp(p.cfg.Ctx, op, operand); t != nil {
			return t
		}
	} else if t := querycore.UnaryOp(op, operand); t != nil {
		return t
	}
	return fallback
}

func (p Projector) contextualLiteral(lit typ.Type, expected typ.Type) typ.Type {
	if p.cfg.PreserveProof || lit == nil || expected == nil {
		return lit
	}
	if subtype.IsSubtype(lit, expected) {
		return expected
	}
	return lit
}

func (p Projector) identType(expr *ast.IdentExpr, point cfg.Point) typ.Type {
	if expr == nil {
		return typ.Unknown
	}
	if p.cfg.Bindings != nil {
		if sym, ok := p.cfg.Bindings.SymbolOf(expr); ok {
			if t := p.symbolType(point, sym); t != nil {
				if p.cfg.GradualParamReads && typ.IsUnknown(t) && p.isUnannotatedParamSymbol(sym) {
					return typ.Any
				}
				return t
			}
		}
	}
	if t := p.globalNameType(point, expr.Value); t != nil {
		return t
	}
	return typ.Unknown
}

func (p Projector) globalNameType(point cfg.Point, name string) typ.Type {
	if name == "" {
		return nil
	}
	if p.cfg.Graph != nil {
		if sym, ok := p.cfg.Graph.GlobalSymbol(name); ok && sym != 0 {
			if t := p.symbolType(point, sym); t != nil {
				return t
			}
		}
	}
	t, _ := p.globalTypes.Type(name)
	return t
}

// isUnannotatedParamSymbol reports whether sym is a source parameter of the
// current function that carries no type annotation. Such a parameter, left at
// the unrefined `unknown` seed, is the gradual `any?` the public contract
// already exposes.
func (p Projector) isUnannotatedParamSymbol(sym cfg.SymbolID) bool {
	if sym == 0 || p.cfg.Graph == nil {
		return false
	}
	if p.cfg.Inputs != nil && p.cfg.Inputs.AnnotatedVars[sym] {
		return false
	}
	if p.cfg.Facts != nil && p.cfg.Facts.IsAnnotated(sym) {
		return false
	}
	fn := p.cfg.Graph.Func()
	if fn == nil || fn.ParList == nil {
		return false
	}
	for _, slot := range p.cfg.Graph.ParamSlotsReadOnly() {
		if slot.Symbol != sym {
			continue
		}
		if slot.IsImplicitSelf {
			return true
		}
		sourceIdx, hasSource := slot.SourceParamIndex()
		if !hasSource {
			return false
		}
		if sourceIdx < 0 || sourceIdx >= len(fn.ParList.Names) {
			return false
		}
		if sourceIdx >= len(fn.ParList.Types) {
			return true
		}
		return fn.ParList.Types[sourceIdx] == nil
	}
	return false
}

func (p Projector) identTypeWithExpected(expr *ast.IdentExpr, point cfg.Point, expected typ.Type) typ.Type {
	t := p.identType(expr, point)
	return p.finishExpectedObservation(t, expected, expr, point)
}

func (p Projector) finishExpectedObservation(observed typ.Type, expected typ.Type, expr ast.Expr, point cfg.Point) typ.Type {
	if refined, ok := p.refineWithExpectedProof(point, expr, observed, expected); ok {
		return refined
	}
	observed = p.callArgumentProofObservation(observed, expected, expr, point)
	return p.coerceGradualToExpected(observed, expected, expr, point)
}

func (p Projector) provesExprType(point cfg.Point, expr ast.Expr, expected typ.Type) bool {
	proofs := p.conditionProofFacts()
	if proofs == nil || expr == nil || expected == nil {
		return false
	}
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() {
		return false
	}
	if p.cfg.LocalCondition != nil {
		if t := proofs.ConditionedTypeAt(point, path, *p.cfg.LocalCondition); !typ.IsAbsentOrUnknown(t) {
			return subtype.IsSubtype(t, expected)
		}
	}
	return proofs.ProvesTypeAt(point, path, expected)
}

func (p Projector) refineWithExpectedProof(point cfg.Point, expr ast.Expr, observed typ.Type, expected typ.Type) (typ.Type, bool) {
	if expected == nil {
		return nil, false
	}
	if declared := p.declaredPathProofType(point, expr); declared != nil && subtype.IsSubtype(declared, expected) {
		return expected, true
	}
	if !p.provesExprType(point, expr, expected) {
		return nil, false
	}
	observed = unwrap.Alias(observed)
	if typ.IsAbsentOrUnknown(observed) || typ.IsAny(observed) || subtype.IsSubtype(expected, observed) {
		return expected, true
	}
	return nil, false
}

func (p Projector) declaredPathProofType(point cfg.Point, expr ast.Expr) typ.Type {
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() || path.Symbol == 0 || p.cfg.Facts == nil {
		return nil
	}
	if !p.cfg.Facts.IsAnnotated(path.Symbol) {
		return nil
	}
	declared := p.cfg.Facts.DeclaredAt(point, path.Symbol)
	if declared.State != flow.StateResolved || declared.Type == nil || typ.IsUnknown(declared.Type) {
		return nil
	}
	if len(path.Segments) == 0 {
		return declared.Type
	}
	return p.pathDeclaredType(point, path)
}

func (p Projector) attrTypeWithExpected(expr *ast.AttrGetExpr, point cfg.Point, expected typ.Type) typ.Type {
	pathType := p.pathType(expr, point)
	contractType := p.bodyContractPathType(expr, point)
	if !typ.IsAbsentOrUnknown(pathType) && (!typ.IsAny(pathType) || typ.IsAbsentOrUnknown(contractType)) {
		return p.finishExpectedObservation(pathType, expected, expr, point)
	}
	if !typ.IsAbsentOrUnknown(contractType) {
		return p.finishExpectedObservation(contractType, expected, expr, point)
	}
	if !typ.IsAbsentOrUnknown(pathType) {
		return p.finishExpectedObservation(pathType, expected, expr, point)
	}
	t := p.attrType(expr, point)
	return p.finishExpectedObservation(t, expected, expr, point)
}

// coerceGradualToExpected used to apply gradual consistency by rewriting
// gradual-top `any` to the expected concrete type at typed boundaries. That is
// now deliberately disabled: expected types may guide synthesis, and
// refineWithExpectedProof may use real path/predicate evidence, but unproved
// `any` is representation rather than evidence. Keeping this hook as a no-op
// documents the boundary and prevents call/assignment/return checks from
// quietly accepting `any` as proof.
func (p Projector) coerceGradualToExpected(t typ.Type, expected typ.Type, expr ast.Expr, point cfg.Point) typ.Type {
	return t
}

func (p Projector) callArgumentProofObservation(observed typ.Type, expected typ.Type, expr ast.Expr, point cfg.Point) typ.Type {
	if !p.cfg.CallArgumentProofs || typ.IsAbsentOrUnknown(observed) ||
		typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
		return observed
	}
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() || path.Symbol == 0 {
		return observed
	}
	if len(path.Segments) == 0 {
		if p.isUnannotatedParamSymbol(path.Symbol) {
			return expected
		}
	}
	contractPathType := p.bodyContractPathTypeAtPath(point, path, nil)
	if typ.IsAbsentOrUnknown(contractPathType) || !subtype.IsSubtype(contractPathType, expected) {
		return observed
	}
	return expected
}

type bodyContractFacts interface {
	BodyContracts() paramevidence.Contracts
}

func (p Projector) bodyContractPathType(expr ast.Expr, point cfg.Point) typ.Type {
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() || path.Symbol == 0 {
		return nil
	}
	return p.bodyContractPathTypeAtPath(point, path, nil)
}

func (p Projector) bodyContractPathTypeAtPath(point cfg.Point, path constraint.Path, seen map[constraint.PathKey]bool) typ.Type {
	if path.IsEmpty() || path.Symbol == 0 {
		return nil
	}
	key := flow.StablePathKey(path)
	if key == "" {
		return nil
	}
	if seen == nil {
		seen = make(map[constraint.PathKey]bool)
	}
	if seen[key] {
		return nil
	}
	seen[key] = true
	defer delete(seen, key)

	if direct := p.directBodyContractPathType(path); direct != nil {
		return p.conditionBodyContractSeedType(point, path, direct)
	}

	if !p.cfg.CallArgumentProofs {
		return nil
	}
	pathAddr, ok := flow.StableAddressOfPath(path)
	if !ok {
		return nil
	}
	types := p.projectBodyContractOriginTypes(point, pathAddr, seen)
	switch len(types) {
	case 0:
		return nil
	case 1:
		return p.conditionBodyContractSeedType(point, path, types[0])
	default:
		return p.conditionBodyContractSeedType(point, path, typ.NewUnion(types...))
	}
}

func (p Projector) projectBodyContractOriginTypes(point cfg.Point, pathAddr flow.StableAddress, seen map[constraint.PathKey]bool) []typ.Type {
	var types []typ.Type
	for _, use := range p.pathAliasesAt(point).AliasesCoveringAddress(pathAddr) {
		sourcePath, ok := use.Alias.SourcePath()
		if !ok || sourcePath.Symbol == 0 {
			continue
		}
		for _, seg := range use.Remainder {
			sourcePath = sourcePath.Append(seg)
		}
		sourceType := p.bodyContractPathTypeAtPath(point, sourcePath, seen)
		if sourceType == nil {
			continue
		}
		types = append(types, sourceType)
	}
	origins := p.valueOriginsAt(point)
	if !origins.IsBottom() {
		for _, use := range origins.OriginsCoveringAddress(pathAddr) {
			sourcePath, ok := use.Origin.SourcePath()
			if !ok || sourcePath.Symbol == 0 {
				continue
			}
			if use.Origin.Kind == flow.ValueOriginIndexedIterator && use.Origin.VarIndex == 1 && len(use.Remainder) > 0 {
				types = append(types, p.appendElementFieldOriginTypes(point, sourcePath, use.Remainder, seen)...)
			}
			sourceType := p.bodyContractPathTypeAtPath(point, sourcePath, seen)
			if sourceType == nil {
				continue
			}
			localType := p.valueOriginLocalType(use.Origin, sourceType)
			if localType == nil {
				continue
			}
			if len(use.Remainder) > 0 {
				localType = p.typeAtSegments(localType, use.Remainder)
			}
			if localType != nil {
				types = append(types, localType)
			}
		}
	}
	return types
}

func (p Projector) appendElementFieldOriginTypes(point cfg.Point, arrayPath constraint.Path, field []constraint.Segment, seen map[constraint.PathKey]bool) []typ.Type {
	if arrayPath.IsEmpty() || len(field) == 0 {
		return nil
	}
	arrayKey := flow.StablePathKey(arrayPath)
	if arrayKey == "" {
		return nil
	}
	var provider appendElementFieldOriginFacts
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(appendElementFieldOriginFacts); ok {
			provider = facts
		}
	}
	if provider == nil && p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(appendElementFieldOriginFacts); ok {
			provider = facts
		}
	}
	if provider == nil {
		return nil
	}
	var types []typ.Type
	for _, use := range provider.AppendElementFieldSourcesAt(point, arrayKey, field) {
		sourceType := p.appendElementFieldOriginSourceType(point, use, nil, seen)
		if sourceType != nil {
			types = append(types, sourceType)
		}
	}
	return types
}

func (p Projector) appendElementFieldOriginSourceType(point cfg.Point, use flow.AppendElementFieldOriginUse, extra []constraint.Segment, seen map[constraint.PathKey]bool) typ.Type {
	sourcePath, ok := use.SourcePath()
	if !ok || sourcePath.Symbol == 0 {
		return nil
	}
	if len(use.SourceField) > 0 {
		segments := append([]constraint.Segment(nil), use.SourceField...)
		segments = append(segments, use.FieldRemainder...)
		segments = append(segments, extra...)
		var fallback typ.Type
		for _, sourceType := range []typ.Type{
			p.pathTypeAtPath(sourcePath, point),
			p.bodyContractPathTypeAtPath(point, sourcePath, seen),
		} {
			if typ.IsAbsentOrUnknown(sourceType) {
				continue
			}
			elemType := querycore.ElementType(sourceType)
			if typ.IsAbsentOrUnknown(elemType) {
				continue
			}
			projected := p.typeAtSegments(elemType, segments)
			if typ.IsAbsentOrUnknown(projected) {
				continue
			}
			if !typ.IsAny(projected) {
				return projected
			}
			fallback = projected
		}
		return fallback
	}
	for _, seg := range use.FieldRemainder {
		sourcePath = sourcePath.Append(seg)
	}
	for _, seg := range extra {
		sourcePath = sourcePath.Append(seg)
	}
	sourceType := p.bodyContractPathTypeAtPath(point, sourcePath, seen)
	if sourceType != nil {
		return sourceType
	}
	return p.pathTypeAtPath(sourcePath, point)
}

func (p Projector) directBodyContractPathType(path constraint.Path) typ.Type {
	if path.IsEmpty() || path.Symbol == 0 || p.cfg.Facts == nil {
		return nil
	}
	if !p.cfg.CallArgumentProofs && !p.isUnannotatedParamSymbol(path.Symbol) {
		return nil
	}
	bodyFacts, ok := p.cfg.Facts.(bodyContractFacts)
	if !ok {
		return nil
	}
	slot, ok := p.paramSlotForSymbol(path.Symbol)
	if !ok {
		return nil
	}
	contract, ok := bodyFacts.BodyContracts()[slot]
	if !ok || paramevidence.ParamContractDomain.Equal(contract, paramevidence.ParamContractDomain.Bottom()) {
		return nil
	}
	base := contract.ProjectValue()
	if typ.IsAbsentOrUnknown(base) {
		return nil
	}
	if len(path.Segments) == 0 {
		return base
	}
	return p.typeAtSegments(base, path.Segments)
}

func (p Projector) pathIsBodyContractSeed(point cfg.Point, path constraint.Path, expected typ.Type) bool {
	if path.IsEmpty() || path.Symbol == 0 || expected == nil {
		return false
	}
	contractPathType := p.bodyContractPathTypeAtPath(point, path, nil)
	return !typ.IsAbsentOrUnknown(contractPathType) && subtype.IsSubtype(contractPathType, expected)
}

func (p Projector) conditionBodyContractSeedType(point cfg.Point, path constraint.Path, seed typ.Type) typ.Type {
	if typ.IsAbsentOrUnknown(seed) || path.IsEmpty() {
		return seed
	}
	proofs := p.conditionProofFacts()
	if proofs == nil {
		return seed
	}
	conditioned := proofs.ConditionedSeedTypeAt(point, path, seed, path, constraint.TrueCondition())
	if typ.IsAbsentOrUnknown(conditioned) {
		return seed
	}
	return conditioned
}

func (p Projector) valueOriginsAt(point cfg.Point) flow.ValueOriginFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(valueOriginFacts); ok {
			return facts.ValueOriginsAt(point)
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(valueOriginFacts); ok {
			return facts.ValueOriginsAt(point)
		}
	}
	return flow.ValueOriginFacts{}
}

func (p Projector) pathAliasesAt(point cfg.Point) flow.PathAliasFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(pathAliasFacts); ok {
			return facts.PathAliasesAt(point)
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(pathAliasFacts); ok {
			return facts.PathAliasesAt(point)
		}
	}
	return flow.PathAliasFacts{}
}

func (p Projector) valueOriginLocalType(origin flow.ValueOriginFact, source typ.Type) typ.Type {
	if source == nil {
		return nil
	}
	switch origin.Kind {
	case flow.ValueOriginAssignmentAlias:
		return source
	case flow.ValueOriginIndexedIterator:
		if origin.VarIndex != 1 {
			return nil
		}
		return querycore.ElementType(source)
	case flow.ValueOriginKeyedIterator:
		switch origin.VarIndex {
		case 0:
			return querycore.EntryKeyType(source)
		case 1:
			return querycore.EntryValueType(source)
		default:
			return nil
		}
	default:
		return nil
	}
}

func (p Projector) paramSlotForSymbol(sym cfg.SymbolID) (int, bool) {
	if sym == 0 || p.cfg.Graph == nil {
		return 0, false
	}
	for i, slot := range p.cfg.Graph.ParamSlotsReadOnly() {
		if slot.Symbol == sym {
			return i, true
		}
	}
	return 0, false
}

func (p Projector) exprIsGradualTop(expr ast.Expr, point cfg.Point) bool {
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() || path.Symbol == 0 {
		return false
	}
	if gradual, ok := p.productGradualTopAt(point, path); ok {
		return gradual
	}
	return p.isUnannotatedParamSymbol(path.Symbol)
}

func (p Projector) productGradualTopAt(point cfg.Point, path constraint.Path) (bool, bool) {
	pv := p.ProductValueAtPath(point, path)
	if pv.State != flow.StateResolved {
		return false, false
	}
	return productValueIsGradualTop(pv), true
}

// ProductValueAtPath returns the solved product carrier for a normalized source
// path when the active facts expose product evidence. It is the observation
// boundary for consumers that need semantic carrier facts such as presence.
func (p Projector) ProductValueAtPath(point cfg.Point, path constraint.Path) flow.ProductValue {
	if p.cfg.Facts == nil || path.IsEmpty() || path.Symbol == 0 {
		return flow.ProductValue{State: flow.StateUnknown}
	}
	if len(path.Segments) == 0 {
		if valueFacts, ok := p.cfg.Facts.(flow.ProductFacts); ok {
			return valueFacts.RefinedValueAt(point, path.Symbol)
		}
	}
	if pathFacts, ok := p.cfg.Facts.(flow.ProductPathFacts); ok {
		return pathFacts.RefinedPathValueAt(point, path)
	}
	return flow.ProductValue{State: flow.StateUnknown}
}

// PathHasPresentProductValue reports whether solved product evidence proves a
// normalized path is definitely non-nil. It does not infer field existence from
// structural types; callers must supply a source path already derived at their
// AST boundary.
func (p Projector) PathHasPresentProductValue(point cfg.Point, path constraint.Path) bool {
	pv := p.ProductValueAtPath(point, path)
	return pv.State == flow.StateResolved && pv.Value.DefinitelyPresent()
}

func productValueIsGradualTop(pv flow.ProductValue) bool {
	return pv.State == flow.StateResolved && !pv.Value.IsZero() && pv.Value.IsGradualTop()
}

// AdmitGradualArgument is the call-argument analogue of
// coerceGradualToExpected. It is intentionally a no-op under the strict-any
// policy: concrete parameter obligations require proof, not gradual-top
// compatibility.
func (p Projector) AdmitGradualArgument(t typ.Type, arg ast.Expr, point cfg.Point, expected typ.Type) typ.Type {
	return t
}

func (p Projector) exprIsGradualCallArg(expr ast.Expr, point cfg.Point) bool {
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() || path.Symbol == 0 {
		return false
	}
	if p.isUnannotatedParamSymbol(path.Symbol) {
		return true
	}
	if gradual, ok := p.productGradualTopAt(point, path); ok {
		return gradual
	}
	return false
}

// SourceReadIsNonIndexable reports whether the source expression is an attribute
// or index read whose object type cannot be indexed at all at the point (for
// example a nil value reached on a non-dominating path). Such a read is the
// field-check's diagnostic ("cannot index type ..."); its result degrades to
// unknown, so a downstream assignment must not re-report that unknown as its own
// mismatch.
//
// Only a fundamentally non-indexable object qualifies. An indexable container
// whose specific field is merely absent (for example a union member missing the
// field) is a distinct "field missing" diagnostic and is left untouched.
func (p Projector) SourceReadIsNonIndexable(source ast.Expr, point cfg.Point) bool {
	attr, ok := source.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return false
	}
	obj := p.TypeOf(attr.Object, point)
	if obj == nil || obj.Kind().IsPlaceholder() {
		return false
	}
	return typeIsNonIndexable(obj)
}

// typeIsNonIndexable reports whether a concrete type supports no field or index
// access at all. It mirrors the NotIndexable classification used by field
// resolution: containers, records, interfaces, unions, intersections, optionals,
// type parameters and strings index; nil, never, functions and other scalars do
// not.
func typeIsNonIndexable(t typ.Type) bool {
	switch u := unwrap.Alias(t).(type) {
	case *typ.Record, *typ.Interface, *typ.Union, *typ.Intersection, *typ.Optional:
		return false
	case *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
		return false
	case *typ.TypeParam, *typ.Instantiated:
		return false
	case *typ.Function:
		return true
	case *typ.Literal:
		return u.Base != kind.String
	default:
		if u == nil {
			return false
		}
		switch u.Kind() {
		case kind.Recursive:
			return false
		case kind.String:
			return false
		case kind.Nil, kind.Never:
			return true
		}
		return true
	}
}

func (p Projector) attrType(expr *ast.AttrGetExpr, point cfg.Point) typ.Type {
	if expr == nil {
		return typ.Unknown
	}
	obj := p.TypeOf(expr.Object, point)
	key := p.TypeOf(expr.Key, point)
	if typ.IsAbsentOrUnknown(obj) {
		if proven, ok := p.IndexedReadProofType(expr.Object, expr.Key, key, point); ok {
			return proven
		}
		return typ.Unknown
	}
	if seg, ok := flowpath.StaticAttrSegmentWithConst(expr, p.constResolver(point)); ok && seg.Kind == constraint.SegmentField {
		if p.cfg.TypeOps != nil {
			if t, ok := p.cfg.TypeOps.Field(p.cfg.Ctx, obj, seg.Name); ok {
				return t
			}
		} else if t, ok := querycore.Field(obj, seg.Name); ok {
			return t
		}
		if querycore.MissingFieldReadsNil(obj) {
			return typ.Nil
		}
	}
	if t, ok := p.RuntimeIndex(obj, key); ok {
		return p.applyIndexReadProof(t, obj, expr.Object, expr.Key, key, point)
	}
	if proven, ok := p.IndexedReadProofType(expr.Object, expr.Key, key, point); ok {
		return proven
	}
	return typ.Unknown
}

// IndexedReadProofType returns the value type proven by the solved indexed-read
// proof surface for object[key], independent of the object's direct product type.
// It is the diagnostic counterpart to transfer's index-write readback reducer:
// the observation layer asks the normalized proof domain before treating a weak
// object type as a failed runtime-index read.
func (p Projector) IndexedReadProofType(obj ast.Expr, key ast.Expr, keyType typ.Type, point cfg.Point) (typ.Type, bool) {
	flowProof := p.indexReadFlow()
	if flowProof == nil {
		return nil, false
	}
	if typ.IsAbsentOrUnknown(keyType) {
		keyType = typ.Unknown
	}
	refined, ok := indexread.Refine(indexread.Query{
		Point:     point,
		View:      p.pathReadView(),
		Container: typ.Unknown,
		Result:    typ.Unknown,
		Object:    obj,
		Key:       key,
		KeyType:   keyType,
		Flow:      flowProof,
		PathOf: func(expr ast.Expr) constraint.Path {
			return p.pathOfExpr(expr, point)
		},
	})
	return refined, ok
}

func (p Projector) applyIndexReadProof(t typ.Type, objType typ.Type, obj ast.Expr, key ast.Expr, keyType typ.Type, point cfg.Point) typ.Type {
	if t == nil {
		return t
	}
	flowProof := p.indexReadFlow()
	if flowProof == nil {
		return t
	}
	if refined, ok := indexread.Refine(indexread.Query{
		Point:     point,
		View:      p.pathReadView(),
		Container: objType,
		Result:    t,
		Object:    obj,
		Key:       key,
		KeyType:   keyType,
		Flow:      flowProof,
		PathOf: func(expr ast.Expr) constraint.Path {
			return p.pathOfExpr(expr, point)
		},
	}); ok {
		return refined
	}
	return t
}

// indexReadFlow returns the index-read proof surface for the current flow. A
// concrete solver result supplies it directly. A producer that exposes only
// per-point facts answers HasKeyOf from the converged per-point condition (a
// `container[k]` read with a held KeyOf strips the optional), and numeric / length
// proofs from the numeric component so a dynamic-index read `arr[i]` with a loop
// bound or length guard drops the soundly-optional element on the diagnostic path
// too. Returns nil when no proof is available.
func (p Projector) indexReadFlow() indexread.Flow {
	if flowOps := p.pathReadFlow(); flowOps != nil {
		return flowOps
	}
	kf, hasKeyOf := p.cfg.Facts.(keyOfFacts)
	nf, hasNum := p.cfg.Facts.(flow.NumericFacts)
	lf, hasLen := p.cfg.Facts.(flow.LengthFacts)
	iw, hasIndexWrites := p.cfg.Facts.(flow.IndexWriteFacts)
	mr, _ := p.cfg.Facts.(indexread.MapReadbackFlow)
	vo, _ := p.cfg.Facts.(valueOriginFacts)
	if !hasKeyOf && !hasNum && !hasLen && !hasIndexWrites && mr == nil {
		return nil
	}
	return factsIndexReadFlow{
		keyOf:       kf,
		numeric:     nf,
		length:      lf,
		indexWrites: iw,
		mapReadback: mr,
		valueOrigin: vo,
		graph:       p.cfg.Graph,
		bindings:    p.cfg.Bindings,
	}
}

func (p Projector) solvedFlow() api.FlowOps {
	return p.cfg.Flow
}

func (p Projector) pathReadFlow() api.FlowOps {
	if p.cfg.Facts == nil {
		return p.cfg.Flow
	}
	return nil
}

func (p Projector) conditionProofFacts() flow.ConditionProofFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(flow.ConditionProofFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(flow.ConditionProofFacts); ok {
			return facts
		}
	}
	return nil
}

func (p Projector) constValueFacts() flow.ConstFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(flow.ConstFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(flow.ConstFacts); ok {
			return facts
		}
	}
	if p.cfg.Inputs != nil {
		return p.cfg.Inputs
	}
	return nil
}

// keyOfFacts is the key-presence proof exposed by a per-point facts carrier:
// HasKeyOf reports whether a KeyOf(table, key) fact holds at point p, i.e. the
// key was drawn from `pairs(table)` over the same container so the lookup is
// present.
type keyOfFacts interface {
	HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool
}

type valueOriginFacts interface {
	ValueOriginsAt(p cfg.Point) flow.ValueOriginFacts
}

type pathAliasFacts interface {
	PathAliasesAt(p cfg.Point) flow.PathAliasFacts
}

type appendElementFieldOriginFacts interface {
	AppendElementFieldSourcesAt(p cfg.Point, array constraint.PathKey, field []constraint.Segment) []flow.AppendElementFieldOriginUse
}

// factsIndexReadFlow adapts per-point Facts proofs to the indexread.Flow surface.
// HasKeyOf answers the key-presence proof; numeric bound and length queries are
// answered from the numeric component (flow.NumericFacts / flow.LengthFacts),
// resolving a variable NAME to its symbol at the read point through the graph so
// the index-var bound / length-reference of a `for i = 1, #arr` / `while i <= n`
// induction variable is consulted. A flow that implements none of these, or a
// name the graph cannot resolve, answers no proof so the read stays soundly
// optional.
type factsIndexReadFlow struct {
	keyOf       keyOfFacts
	numeric     flow.NumericFacts
	length      flow.LengthFacts
	indexWrites flow.IndexWriteFacts
	mapReadback indexread.MapReadbackFlow
	valueOrigin valueOriginFacts
	graph       *cfg.Graph
	bindings    *bind.BindingTable
}

func (f factsIndexReadFlow) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	if f.keyOf == nil {
		return false
	}
	return f.keyOf.HasKeyOf(p, tablePath, keyPath)
}

func (f factsIndexReadFlow) MapReadback(q flow.IndexWriteReadQuery) (typ.Type, bool) {
	if f.mapReadback != nil {
		if got, ok := f.mapReadback.MapReadback(q); ok {
			return got, true
		}
	}
	return f.IndexWriteAdmission(q)
}

func (f factsIndexReadFlow) IndexWriteAdmission(q flow.IndexWriteReadQuery) (typ.Type, bool) {
	if f.indexWrites == nil {
		return nil, false
	}
	if got, ok := f.indexWrites.IndexWriteAdmission(q); ok {
		return got, true
	}
	if !q.Admission.HasKeyPath || f.valueOrigin == nil {
		return nil, false
	}
	origins := f.valueOrigin.ValueOriginsAt(q.Point)
	for _, keyAddr := range flow.IdentityAliasClosure(flow.PointState{ValueOrigins: origins}, q.Admission.KeyPath) {
		if keyAddr.Equal(q.Admission.KeyPath) {
			continue
		}
		aliasQuery := q
		aliasQuery.Admission.KeyPath = keyAddr
		aliasQuery.Admission.HasKeyPath = true
		if got, ok := f.indexWrites.IndexWriteAdmission(aliasQuery); ok {
			return got, true
		}
	}
	return nil, false
}

func (f factsIndexReadFlow) BoundsAt(p cfg.Point, name string) (int64, int64, bool) {
	if f.numeric == nil {
		return 0, 0, false
	}
	sym, ok := f.symbolAt(p, name)
	if !ok {
		return 0, 0, false
	}
	return f.numeric.NumericBoundsAt(p, sym)
}

func (f factsIndexReadFlow) ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (string, int64, bool) {
	if f.numeric == nil {
		return "", 0, false
	}
	sym, ok := f.symbolAt(p, varName)
	if !ok {
		return "", 0, false
	}
	arrSym, offset, ok := f.numeric.ArrayLenRefAt(p, sym)
	if !ok {
		return "", 0, false
	}
	// indexread.Refine compares this key against the container expression's
	// constraint.Path.Key(), which is bound to the container symbol's SSA version at
	// the read point. The numeric component is version-insensitive, so the array
	// symbol's path is versioned at p the same way the container path is, yielding
	// the matching key.
	arrPath := flowpath.WithVersion(constraint.Path{Symbol: arrSym}, f.graph, p)
	return string(arrPath.Key()), offset, true
}

func (f factsIndexReadFlow) LengthBoundsAt(p cfg.Point, path constraint.Path) (int64, int64, bool) {
	if f.length == nil || path.Symbol == 0 {
		return 0, 0, false
	}
	if pathFacts, ok := f.length.(flow.PathLengthFacts); ok {
		if lower, ok := pathFacts.LengthLowerBoundForPathAt(p, path); ok {
			return lower, 0, true
		}
	}
	if len(path.Segments) != 0 {
		return 0, 0, false
	}
	lower, ok := f.length.LengthLowerBoundAt(p, path.Symbol)
	if !ok {
		return 0, 0, false
	}
	return lower, 0, true
}

// symbolAt resolves a variable name to its symbol visible at point p, the bridge
// between the indexread.Flow's name-keyed numeric queries and the symbol-keyed
// canonical numeric component.
func (f factsIndexReadFlow) symbolAt(p cfg.Point, name string) (cfg.SymbolID, bool) {
	if f.graph == nil || name == "" {
		return 0, false
	}
	sym, ok := f.graph.SymbolAt(p, name)
	if !ok || sym == 0 {
		return 0, false
	}
	return sym, true
}

func (p Projector) tableType(expr *ast.TableExpr, point cfg.Point, expected typ.Type) typ.Type {
	if expr == nil {
		return typ.Unknown
	}
	expected = p.resolveLocalRefs(expected, point)
	expected = p.discriminatedExpected(expr, expected)
	if len(expr.Fields) == 0 {
		if expectedTable(expected) {
			return expected
		}
		return typ.NewFreshEmptyRecord()
	}
	if result, ok := p.tableTypeWithUnionExpected(expr, point, expected); ok {
		return result
	}
	entries, elems, fieldCount, _ := p.tableEntries(expr, expected, point, false)
	if expected != nil {
		if result := ops.CheckTableEntries(entries, elems, expected); len(result.Errors) == 0 {
			if result.Type != nil {
				return result.Type
			}
			return expected
		}
	}
	if fieldCount == 0 && len(elems) > 0 {
		return value.AdmitObservation(typ.NewTuple(elems...))
	}
	result := ops.CheckTableEntries(entries, elems, nil).Type
	if result == nil {
		return typ.Unknown
	}
	return value.AdmitObservation(result)
}

func (p Projector) tableTypeWithUnionExpected(expr *ast.TableExpr, point cfg.Point, expected typ.Type) (typ.Type, bool) {
	u := unwrap.Union(expected)
	if u == nil {
		return nil, false
	}
	var results []typ.Type
	for _, member := range u.Members {
		entries, elems, _, _ := p.tableEntries(expr, member, point, false)
		result := ops.CheckTableEntries(entries, elems, member)
		if len(result.Errors) != 0 {
			continue
		}
		if result.Type != nil {
			results = append(results, result.Type)
			continue
		}
		results = append(results, member)
	}
	switch len(results) {
	case 0:
		return nil, false
	case 1:
		return results[0], true
	default:
		return typ.NewUnion(results...), true
	}
}

func (p Projector) discriminatedExpected(expr *ast.TableExpr, expected typ.Type) typ.Type {
	if expr == nil || expected == nil {
		return expected
	}
	if _, ok := unwrap.Alias(expected).(*typ.Union); !ok {
		return expected
	}
	if match := querycore.TryDiscriminatedUnionMember(expr, expected); match != nil {
		return match.Member
	}
	return expected
}

func expectedTable(expected typ.Type) bool {
	if expected == nil {
		return false
	}
	expected = narrow.RemoveNil(expected)
	return expected != nil && !typ.IsAbsentOrUnknown(expected) && !typ.IsAny(expected)
}

func (p Projector) functionType(fn *ast.FunctionExpr, expected typ.Type) typ.Type {
	expectedFn := phasecore.ExpectedFunctionLiteralSignature(fn, expected)
	var sourceSig *typ.Function
	if p.cfg.LiteralSignatureProvider != nil {
		sourceSig = p.cfg.LiteralSignatureProvider.Lookup(fn)
	}
	if p.cfg.Bindings != nil && p.cfg.FunctionType != nil {
		if sym, ok := p.cfg.Bindings.FuncLitSymbol(fn); ok && sym != 0 {
			if t := p.cfg.FunctionType(sym); t != nil {
				if observed := unwrap.Function(t); observed != nil {
					if sourceSig != nil && expectedFn != nil {
						return contextualFunctionLiteralTypeWithSource(fn, observed, sourceSig, expectedFn)
					}
					return contextualFunctionLiteralType(fn, observed, expectedFn)
				}
				return t
			}
		}
	}
	if sourceSig != nil {
		return contextualFunctionLiteralType(fn, sourceSig, expectedFn)
	}
	if expectedFn != nil {
		return expectedFn
	}
	return typ.Func().Build()
}

func contextualFunctionLiteralTypeWithSource(fn *ast.FunctionExpr, observed, source, expected *typ.Function) *typ.Function {
	if source == nil {
		return contextualFunctionLiteralType(fn, observed, expected)
	}
	sourceView := contextualFunctionLiteralType(fn, source, expected)
	observedView := contextualFunctionLiteralType(fn, observed, expected)
	if observedView == nil {
		return sourceView
	}
	builder := typ.Func().ReserveParams(len(sourceView.Params))
	for _, tp := range observedView.TypeParams {
		if tp != nil {
			builder.TypeParamRef(tp)
		}
	}
	for _, param := range sourceView.Params {
		if param.Type == nil {
			param.Type = typ.Unknown
		}
		if param.Optional {
			builder.OptParam(param.Name, param.Type)
		} else {
			builder.Param(param.Name, param.Type)
		}
	}
	if sourceView.Variadic != nil {
		builder.Variadic(sourceView.Variadic)
	}
	returns := observedView.Returns
	if !typ.HasKnownType(returns) && len(sourceView.Returns) > 0 {
		returns = sourceView.Returns
	}
	if len(returns) > 0 {
		builder.Returns(returns...)
	}
	return builder.
		Effects(observedView.Effects).
		Spec(observedView.Spec).
		WithRefinement(observedView.Refinement).
		Build()
}

func contextualFunctionLiteralType(fn *ast.FunctionExpr, observed, expected *typ.Function) *typ.Function {
	if observed == nil {
		if expected != nil {
			return expected
		}
		return typ.Func().Build()
	}
	if expected == nil {
		return observed
	}

	builder := typ.Func().ReserveParams(len(observed.Params))
	for _, tp := range observed.TypeParams {
		if tp != nil {
			builder.TypeParamRef(tp)
		}
	}
	for _, param := range contextualFunctionLiteralParams(fn, observed, expected) {
		if param.Type == nil {
			param.Type = typ.Unknown
		}
		if param.Optional {
			builder.OptParam(param.Name, param.Type)
		} else {
			builder.Param(param.Name, param.Type)
		}
	}
	if variadic := contextualFunctionLiteralVariadic(fn, observed, expected); variadic != nil {
		builder.Variadic(variadic)
	}
	returns := observed.Returns
	if !typ.HasKnownType(returns) && len(expected.Returns) > 0 {
		returns = expected.Returns
	}
	if len(returns) > 0 {
		builder.Returns(returns...)
	}
	return builder.
		Effects(observed.Effects).
		Spec(observed.Spec).
		WithRefinement(observed.Refinement).
		Build()
}

func contextualFunctionLiteralParams(fn *ast.FunctionExpr, observed, expected *typ.Function) []typ.Param {
	if observed == nil || len(observed.Params) == 0 {
		return nil
	}
	params := make([]typ.Param, len(observed.Params))
	copy(params, observed.Params)
	for i := range params {
		if !sourceParamAcceptsContext(fn, i, len(observed.Params)) {
			continue
		}
		if i >= len(expected.Params) {
			continue
		}
		context := expected.Params[i]
		if context.Type == nil {
			continue
		}
		name := params[i].Name
		if name == "" {
			name = context.Name
		}
		params[i] = typ.Param{Name: name, Type: context.Type, Optional: context.Optional}
	}
	return params
}

func contextualFunctionLiteralVariadic(fn *ast.FunctionExpr, observed, expected *typ.Function) typ.Type {
	if observed == nil {
		return nil
	}
	if expected != nil && expected.Variadic != nil && sourceVariadicAcceptsContext(fn) {
		return expected.Variadic
	}
	return observed.Variadic
}

func sourceParamAcceptsContext(fn *ast.FunctionExpr, effectiveIdx int, observedParamCount int) bool {
	if fn == nil || fn.ParList == nil {
		return false
	}
	sourceIdx := effectiveIdx
	if observedParamCount == len(fn.ParList.Names)+1 && len(fn.ParList.Names) > 0 && fn.ParList.Names[0] != "self" && effectiveIdx == 0 {
		return true
	}
	if observedParamCount == len(fn.ParList.Names)+1 && len(fn.ParList.Names) > 0 && fn.ParList.Names[0] != "self" {
		sourceIdx = effectiveIdx - 1
	}
	if sourceIdx < 0 || sourceIdx >= len(fn.ParList.Names) {
		return false
	}
	return fn.ParList.Types == nil || sourceIdx >= len(fn.ParList.Types) || fn.ParList.Types[sourceIdx] == nil
}

func sourceVariadicAcceptsContext(fn *ast.FunctionExpr) bool {
	if fn == nil || fn.ParList == nil || !fn.ParList.HasVargs {
		return false
	}
	return fn.ParList.VarargType == nil
}

func (p Projector) logicalType(expr *ast.LogicalOpExpr, point cfg.Point, expected typ.Type) typ.Type {
	if expr == nil {
		return typ.Unknown
	}
	switch expr.Operator {
	case "or":
		left := p.TypeOfWithExpected(expr.Lhs, point, expected)
		right := p.withExprCondition(expr.Lhs, point, false).TypeOfWithExpected(expr.Rhs, point, expected)
		return ops.LogicalOrTyped(left, right)
	case "and":
		left := p.TypeOf(expr.Lhs, point)
		right := p.withExprCondition(expr.Lhs, point, true).TypeOfWithExpected(expr.Rhs, point, expected)
		return ops.LogicalAndTyped(left, right)
	default:
		return typ.Unknown
	}
}

func (p Projector) withExprCondition(expr ast.Expr, point cfg.Point, truthy bool) Projector {
	return p.withLocalCondition(p.exprCondition(expr, point, truthy))
}

func (p Projector) withLocalCondition(cond constraint.Condition) Projector {
	if !cond.HasConstraints() && !cond.IsFalse() {
		return p
	}
	if p.cfg.LocalCondition != nil {
		cond = constraint.And(*p.cfg.LocalCondition, cond)
	}
	p.cfg.LocalCondition = &cond
	return p
}

func (p Projector) exprCondition(expr ast.Expr, point cfg.Point, truthy bool) constraint.Condition {
	inputs := p.cfg.Inputs
	if inputs == nil && p.cfg.Graph != nil {
		inputs = &flow.Inputs{Graph: p.cfg.Graph}
	}
	return (conditionexpr.Extractor{
		P:             point,
		SC:            p.scopeAt(point),
		Inputs:        inputs,
		Bindings:      p.cfg.Bindings,
		Graph:         p.cfg.Graph,
		ConstResolver: p.constResolver(point),
	}).ConditionForTruth(expr, truthy)
}

func (p Projector) callReturns(expr *ast.FuncCallExpr, point cfg.Point) []typ.Type {
	return p.callReturnsWithExpected(expr, point, nil)
}

// interceptSelectCall observes a select(...) call through the shared synth
// select intercept so the variadic-transform return (select("#", ...) -> integer,
// select(n, ...) -> the n-th vararg/element) matches the engine instead of
// degrading to unknown. Returns false for non-select calls.
func (p Projector) interceptSelectCall(expr *ast.FuncCallExpr, point cfg.Point) ([]typ.Type, bool) {
	if expr == nil {
		return nil, false
	}
	var resolver intercept.VariadicTypeResolver
	sc := p.scopeAt(point)
	if sc != nil {
		resolver = sc
	}
	chain := intercept.NewChain([]intercept.CallIntercept{
		&intercept.SelectIntercept{VariadicResolver: resolver},
		&intercept.TypeCastIntercept{},
	}, nil)
	env := intercept.CallEnv{
		Scope:   sc,
		Recurse: intercept.ExprSynth(func(e ast.Expr) typ.Type { return p.TypeOf(e, point) }),
		TypeLookup: func(name string) typ.Type {
			return p.globalNameType(point, name)
		},
	}
	if res := chain.InterceptCall(expr, env); res.Skip {
		return res.Types, true
	}
	return nil, false
}

// interceptMethodCall observes a method call through the shared synth method
// intercept so an effect-dispatched type-guard call (Type:is(x) -> (Type?, Error?))
// resolves to the same return vector the engine synthesizes instead of degrading
// to unknown. Returns false for non-intercepted method calls. The post-flow return
// summary observes a body's return list through this surface, so a local function
// whose body is `return Point:is(x)` publishes the inferred (Point?, Error?) summary.
func (p Projector) interceptMethodCall(expr *ast.FuncCallExpr, point cfg.Point) ([]typ.Type, bool) {
	if expr == nil || expr.Method == "" {
		return nil, false
	}
	sc := p.scopeAt(point)
	if sc == nil {
		return nil, false
	}
	chain := intercept.NewChain(nil, []intercept.MethodIntercept{
		&intercept.TypeIsIntercept{},
	})
	env := intercept.CallEnv{
		Scope:    sc,
		Recurse:  intercept.ExprSynth(func(e ast.Expr) typ.Type { return p.TypeOf(e, point) }),
		Bindings: p.cfg.Bindings,
		TypeLookup: func(name string) typ.Type {
			return p.globalNameType(point, name)
		},
	}
	if res := chain.InterceptMethodCall(expr, env); res.Skip {
		return res.Types, true
	}
	return nil, false
}

func (p Projector) callReturnsWithExpected(expr *ast.FuncCallExpr, point cfg.Point, expected typ.Type) []typ.Type {
	if expr == nil {
		return []typ.Type{typ.Unknown}
	}
	if types, ok := p.interceptSelectCall(expr, point); ok {
		return types
	}
	args := p.callArgTypes(expr.Args, point)
	if metatable.IsSetMetatableCall(expr, p.cfg.Bindings) {
		meta := args[1]
		if len(expr.Args) >= 2 {
			meta = p.canonicalMetatable(expr.Args[1], meta)
		}
		return []typ.Type{metatable.With(args[0], meta)}
	}
	if types, ok := p.interceptMethodCall(expr, point); ok {
		return types
	}
	if facts, ok := p.cfg.Facts.(callReturnFacts); ok {
		if types, ok := facts.CallReturnTypesAt(point, expr, expected); ok && len(types) > 0 {
			return types
		}
	}
	var callee typ.Type
	var receiver typ.Type
	if expr.Method != "" {
		receiver = p.TypeOf(expr.Receiver, point)
		if !typ.IsAbsentOrUnknown(receiver) {
			if p.cfg.TypeOps != nil {
				callee, _ = p.cfg.TypeOps.Method(p.cfg.Ctx, receiver, expr.Method)
				if callee == nil {
					callee, _ = p.cfg.TypeOps.Field(p.cfg.Ctx, receiver, expr.Method)
				}
			} else {
				callee, _ = querycore.Method(receiver, expr.Method)
				if callee == nil {
					callee, _ = querycore.Field(receiver, expr.Method)
				}
			}
		}
	} else {
		callee = p.TypeOf(expr.Func, point)
	}
	fn := unwrap.Function(callee)
	if fn == nil {
		// A callee typed as an already-applied generic alias (e.g. a parameter
		// of type Mapper<T, U> = fun(x: T) -> U) is an Instantiated, which
		// unwrap.Function deliberately does not expand. Only when the ordinary
		// unwrap yields no function do we expand the instantiated alias, so
		// generic calls that rely on the un-expanded callee are untouched.
		if expanded := subst.ExpandInstantiated(callee); expanded != callee {
			if efn := unwrap.Function(expanded); efn != nil {
				callee = expanded
				fn = efn
			}
		}
	}
	if fn == nil {
		return []typ.Type{typ.Unknown}
	}
	if p.cfg.TypeOps == nil {
		if len(fn.Returns) == 0 {
			return []typ.Type{typ.Nil}
		}
		out := make([]typ.Type, len(fn.Returns))
		copy(out, fn.Returns)
		return out
	}

	def := ops.CallDef{
		Callee: callee,
		Args:   args,
		Query:  p.cfg.TypeOps,
	}
	if expr.Method != "" {
		def.IsMethod = true
		def.Receiver = receiver
		def.MethodName = expr.Method
		def.Callee = nil
	}
	pipeline := ops.NewCallPipeline(p.cfg.Ctx, def, len(expr.Args)).
		WithReSynth(func(idx int, expected typ.Type) typ.Type {
			if idx < 0 || idx >= len(expr.Args) {
				return nil
			}
			return p.TypeOfWithExpected(expr.Args[idx], point, expected)
		})
	if expected != nil {
		pipeline = pipeline.WithExpected(expected)
	}
	result := pipeline.Run()
	returns := callreturn.ResultTypes(result.Type, result.Returns)
	return callreturn.ApplyEffectTransforms(callreturn.EffectTransformInput{
		Ctx:                 p.cfg.Ctx,
		Query:               p.cfg.TypeOps,
		Callee:              callee,
		Args:                args,
		Returns:             returns,
		Receiver:            receiver,
		IsMethod:            expr.Method != "",
		ForceMethodReceiver: false,
	})
}

// canonicalMetatable maps an observed setmetatable metatable argument to the
// shared interned class family keyed by the metatable binding's origin symbol.
// It mirrors the synthesis-side canonicalization so a constructor's observed
// return metatable resolves to the same converging family handle instead of a
// degraded structural snapshot. Returns current unchanged for a non-identifier
// metatable, an unresolved symbol, a non-class record, or when the interner is
// unavailable.
func (p Projector) canonicalMetatable(expr ast.Expr, current typ.Type) typ.Type {
	if expr == nil || current == nil || p.cfg.RecursiveFamilies == nil {
		return current
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || ident.Value == "" {
		return current
	}
	if !metatable.IsClassShaped(current) {
		return current
	}
	sym := p.metatableOriginSymbol(ident)
	if sym == 0 {
		return current
	}
	join := p.cfg.ClassFamilyJoin
	if join == nil {
		join = value.MergeForConvergence
	}
	key := typ.FamilyKey{Namespace: "class", Owner: strconv.FormatUint(uint64(sym), 10)}
	return metatable.SealClassFamilyInterned(current, key, p.cfg.RecursiveFamilies, join)
}

// metatableOriginSymbol resolves a class identifier to its binding symbol,
// preferring graph bindings and falling back to module bindings.
func (p Projector) metatableOriginSymbol(ident *ast.IdentExpr) cfg.SymbolID {
	if ident == nil {
		return 0
	}
	if p.cfg.Bindings != nil {
		if sym, ok := p.cfg.Bindings.SymbolOf(ident); ok && sym != 0 {
			return sym
		}
	}
	if p.cfg.Graph != nil {
		if gb := p.cfg.Graph.Bindings(); gb != nil && gb != p.cfg.Bindings {
			if sym, ok := gb.SymbolOf(ident); ok && sym != 0 {
				return sym
			}
		}
	}
	return 0
}

func (p Projector) callArgTypes(args []ast.Expr, point cfg.Point) []typ.Type {
	if len(args) == 0 {
		return nil
	}
	out := make([]typ.Type, len(args))
	for i, arg := range args {
		out[i] = p.TypeOf(arg, point)
		if out[i] == nil {
			out[i] = typ.Unknown
		}
	}
	return out
}

func (p Projector) pathType(expr ast.Expr, point cfg.Point) typ.Type {
	if expr == nil {
		return nil
	}
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() {
		return nil
	}
	obs := p.pathObservationFacts().ObservePath(flow.PathObservationQuery{
		Point:               point,
		Path:                path,
		View:                p.pathReadView(),
		AllowConditionProof: true,
		LocalCondition:      p.cfg.LocalCondition,
		PreserveProof:       p.cfg.PreserveProof,
		IndexRead:           p.pathObservationIndexRead(expr, point),
	})
	if !obs.Resolved() {
		return nil
	}
	if typ.IsNever(obs.Type) {
		return p.finalizeObservedPath(obs.Type)
	}
	if obs.Source == flow.PathObservationDeclared {
		return obs.Type
	}
	return p.finalizeObservedPath(obs.Type)
}

func (p Projector) pathTypeAtPath(path constraint.Path, point cfg.Point) typ.Type {
	if path.IsEmpty() || path.Symbol == 0 {
		return nil
	}
	obs := p.pathObservationFacts().ObservePath(flow.PathObservationQuery{
		Point:               point,
		Path:                path,
		View:                p.pathReadView(),
		AllowConditionProof: true,
		LocalCondition:      p.cfg.LocalCondition,
		PreserveProof:       p.cfg.PreserveProof,
	})
	if !obs.Resolved() {
		return nil
	}
	if typ.IsNever(obs.Type) || obs.Source == flow.PathObservationDeclared {
		return obs.Type
	}
	return p.finalizeObservedPath(obs.Type)
}

func (p Projector) pathReadView() flow.PathReadView {
	switch {
	case p.cfg.PreferPreState:
		return flow.PathReadPre
	case p.cfg.PreferPostState:
		return flow.PathReadPost
	default:
		return flow.PathReadCurrent
	}
}

func (p Projector) pathObservationFacts() flow.PathObservationFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(flow.PathObservationFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(flow.PathObservationFacts); ok {
			return facts
		}
	}
	return p
}

func (p Projector) appendElementFieldOriginProvider() appendElementFieldOriginFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(appendElementFieldOriginFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(appendElementFieldOriginFacts); ok {
			return facts
		}
	}
	return nil
}

func (p Projector) pathObservationIndexRead(expr ast.Expr, point cfg.Point) *flow.PathObservationIndexRead {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return nil
	}
	index, ok := indexread.Context(indexread.ContextQuery{
		Container: p.TypeOf(attr.Object, point),
		Object:    attr.Object,
		Key:       attr.Key,
		PathOf: func(candidate ast.Expr) constraint.Path {
			return p.pathOfExpr(candidate, point)
		},
	})
	if !ok {
		return nil
	}
	return &index
}

// ObservePath implements flow.PathObservationFacts by lifting the existing
// observation path-read policy behind one normalized, AST-free query surface.
func (p Projector) ObservePath(q flow.PathObservationQuery) flow.PathObservation {
	path := q.Path
	if path.IsEmpty() {
		return flow.PathObservation{}
	}
	declared := p.pathDeclaredType(q.Point, path)
	var direct flow.PathObservationCandidate
	if q.LocalCondition == nil && len(path.Segments) > 0 {
		if t, ok := p.directFactsPathTypeForView(q.Point, path, q.View); ok {
			direct = flow.PathObservationCandidate{
				Type:   t,
				Source: flow.PathObservationDirectPath,
				OK:     true,
			}
		}
	}
	if direct.OK {
		return p.withPathObservationIndexRead(q, flow.SelectPathObservationResult(flow.PathObservationSelection{
			Query:         q,
			Declared:      declared,
			Direct:        direct,
			AdmitSelected: true,
		}))
	}
	if flowOps := p.pathReadFlow(); flowOps != nil {
		var solved typ.Type
		var proof typ.Type
		if q.View == flow.PathReadPre {
			if t := flowOps.PreStateTypeAt(q.Point, path); !typ.IsAbsentOrUnknown(t) {
				solved = t
			} else if !q.StrictView {
				if t := flowOps.NarrowedTypeAt(q.Point, path); !typ.IsAbsentOrUnknown(t) {
					solved = t
				}
			}
			if q.AllowConditionProof && q.LocalCondition != nil {
				proof = p.conditionedPathTypeWithCondition(q.Point, path, solved, declared, q.LocalCondition, q.View)
			}
			if q.AllowConditionProof && typ.IsAbsentOrUnknown(solved) && typ.IsAbsentOrUnknown(proof) {
				proof = p.conditionProofType(q.Point, path)
			}
		} else {
			hasConditionProof := p.hasConditionProofAt(q.Point)
			if q.AllowConditionProof && q.PreserveProof && hasConditionProof && typ.IsAbsentOrUnknown(proof) {
				proof = p.conditionProofType(q.Point, path)
			}
			if t := flowOps.NarrowedTypeAt(q.Point, path); !typ.IsAbsentOrUnknown(t) {
				solved = t
			} else if t := flowOps.PreStateTypeAt(q.Point, path); !typ.IsAbsentOrUnknown(t) {
				solved = t
			}
			if q.AllowConditionProof && q.LocalCondition != nil {
				proof = p.conditionedPathTypeWithCondition(q.Point, path, solved, declared, q.LocalCondition, q.View)
			}
			if q.AllowConditionProof && typ.IsAbsentOrUnknown(solved) && (!q.PreserveProof || !hasConditionProof) && typ.IsAbsentOrUnknown(proof) {
				proof = p.conditionProofType(q.Point, path)
			}
		}
		var solvedCandidate flow.PathObservationCandidate
		if !typ.IsAbsentOrUnknown(solved) {
			solvedCandidate = flow.PathObservationCandidate{
				Type:   solved,
				Source: flow.PathObservationSolvedFlow,
				OK:     true,
			}
		}
		return p.withPathObservationIndexRead(q, flow.SelectPathObservationResult(flow.PathObservationSelection{
			Query:         q,
			Declared:      declared,
			Direct:        direct,
			Solved:        solvedCandidate,
			Proof:         proof,
			AdmitSelected: false,
		}))
	} else if t, ok := p.factsNarrowedPathType(q.Point, path); ok {
		return p.withPathObservationIndexRead(q, flow.SelectPathObservationResult(flow.PathObservationSelection{
			Query:    q,
			Declared: declared,
			Direct:   direct,
			Solved: flow.PathObservationCandidate{
				Type:   t,
				Source: flow.PathObservationFactProjection,
				OK:     true,
			},
			AdmitSelected: true,
		}))
	}
	return p.withPathObservationIndexRead(q, flow.SelectPathObservationResult(flow.PathObservationSelection{
		Query:         q,
		Declared:      declared,
		Direct:        direct,
		AdmitSelected: true,
	}))
}

func (p Projector) withPathObservationIndexRead(q flow.PathObservationQuery, obs flow.PathObservation) flow.PathObservation {
	if !obs.Resolved() || typ.IsNever(obs.Type) {
		return obs
	}
	obs.Type = p.applyPathObservationIndexRead(obs.Type, q)
	return obs
}

func (p Projector) applyPathObservationIndexRead(t typ.Type, q flow.PathObservationQuery) typ.Type {
	if t == nil || q.IndexRead == nil {
		return t
	}
	flowProof := p.indexReadFlow()
	if flowProof == nil {
		return t
	}
	if refined, ok := indexread.RefineObservation(indexread.ObservationQuery{
		Point:  q.Point,
		View:   q.View,
		Result: t,
		Index:  *q.IndexRead,
		Flow:   flowProof,
	}); ok {
		return refined
	}
	return t
}

func (p Projector) directFactsPathTypeForView(point cfg.Point, path constraint.Path, view flow.PathReadView) (typ.Type, bool) {
	if p.cfg.Facts == nil || path.Symbol == 0 || len(path.Segments) == 0 {
		return nil, false
	}
	if view == flow.PathReadPost {
		if post, ok := p.cfg.Facts.(postStatePathFacts); ok {
			refined := post.PostRefinedPathAt(point, path)
			if refined.State == flow.StateResolved && !typ.IsAbsentOrUnknown(refined.Type) {
				return refined.Type, true
			}
		}
	}
	pathFacts, ok := p.cfg.Facts.(flow.PathFacts)
	if !ok {
		return nil, false
	}
	refined := pathFacts.RefinedPathAt(point, path)
	if refined.State != flow.StateResolved || typ.IsAbsentOrUnknown(refined.Type) {
		return nil, false
	}
	return refined.Type, true
}

func (p Projector) conditionedPathTypeWithCondition(point cfg.Point, path constraint.Path, solved typ.Type, declared typ.Type, localCondition *constraint.Condition, view flow.PathReadView) typ.Type {
	if localCondition == nil || path.IsEmpty() {
		return nil
	}
	proofs := p.conditionProofFacts()
	if proofs == nil {
		if flowOps := p.solvedFlow(); flowOps != nil {
			return flowOps.NarrowedTypeAtWithCondition(point, path, *localCondition)
		}
		return nil
	}
	seedPath := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	seedType := solved
	if len(path.Segments) > 0 || typ.IsAbsentOrUnknown(seedType) {
		seedType = p.currentPathTypeForView(point, seedPath, view)
	}
	if typ.IsAbsentOrUnknown(seedType) && len(path.Segments) == 0 {
		seedType = declared
	}
	if typ.IsAbsentOrUnknown(seedType) {
		seedType = p.pathDeclaredType(point, seedPath)
	}
	if typ.IsAbsentOrUnknown(seedType) {
		return proofs.ConditionedTypeAt(point, path, *localCondition)
	}
	return proofs.ConditionedSeedTypeAt(point, seedPath, seedType, path, *localCondition)
}

func (p Projector) currentPathTypeForView(point cfg.Point, path constraint.Path, view flow.PathReadView) typ.Type {
	flowOps := p.solvedFlow()
	if flowOps == nil || path.IsEmpty() {
		return nil
	}
	if view == flow.PathReadPre {
		if t := flowOps.PreStateTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
			return t
		}
		return flowOps.NarrowedTypeAt(point, path)
	}
	if t := flowOps.NarrowedTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
		return t
	}
	return flowOps.PreStateTypeAt(point, path)
}

func (p Projector) hasConditionProofAt(point cfg.Point) bool {
	proofs := p.conditionProofFacts()
	if proofs == nil {
		return false
	}
	cond := proofs.ConditionAt(point)
	return cond.IsFalse() || cond.HasConstraints()
}

func (p Projector) conditionProofType(point cfg.Point, path constraint.Path) typ.Type {
	if proofs := p.conditionProofFacts(); proofs != nil {
		return proofs.ConditionTypeAt(point, path)
	}
	return nil
}

func (p Projector) pathOfExpr(expr ast.Expr, point cfg.Point) constraint.Path {
	return flowpath.FromExprWithBindingsAt(expr, p.constResolver(point), p.cfg.Bindings, p.cfg.Graph, point)
}

func (p Projector) constResolver(point cfg.Point) func(string) *flow.ConstValue {
	facts := p.constValueFacts()
	if facts == nil || p.cfg.Graph == nil {
		return nil
	}
	return func(name string) *flow.ConstValue {
		if name == "" {
			return nil
		}
		sym, ok := p.cfg.Graph.SymbolAt(point, name)
		if !ok {
			return nil
		}
		return facts.ConstValueAtSym(point, sym)
	}
}

func (p Projector) pathDeclaredType(point cfg.Point, path constraint.Path) typ.Type {
	if path.Symbol == 0 {
		return nil
	}
	base := p.declaredSymbolType(point, path.Symbol)
	if base == nil {
		base = p.symbolType(point, path.Symbol)
	}
	if base == nil || len(path.Segments) == 0 {
		return base
	}
	return p.typeAtSegments(base, path.Segments)
}

func (p Projector) typeAtSegments(base typ.Type, segments []constraint.Segment) typ.Type {
	return typepath.TypeAtSegments(base, segments, typepath.Options{
		Ctx:               p.cfg.Ctx,
		Ops:               p.cfg.TypeOps,
		MissingFieldAsNil: true,
	})
}

func (p Projector) declaredSymbolType(point cfg.Point, sym cfg.SymbolID) typ.Type {
	if sym == 0 || p.cfg.Facts == nil {
		return nil
	}
	if !p.cfg.Facts.IsAnnotated(sym) {
		return nil
	}
	tv := p.cfg.Facts.DeclaredAt(point, sym)
	if tv.State != flow.StateResolved || tv.Type == nil || typ.IsUnknown(tv.Type) {
		return nil
	}
	return tv.Type
}

func (p Projector) symbolType(point cfg.Point, sym cfg.SymbolID) typ.Type {
	if sym == 0 {
		return nil
	}
	if p.cfg.Facts != nil {
		if p.cfg.PreferPostState {
			if post, ok := p.cfg.Facts.(postStateTypeFacts); ok {
				tv := post.PostEffectiveTypeAt(point, sym)
				if tv.Type != nil && (tv.State == flow.StateResolved || !typ.IsUnknown(tv.Type)) {
					return tv.Type
				}
			}
		}
		tv := p.cfg.Facts.EffectiveTypeAt(point, sym)
		if tv.Type != nil && (tv.State == flow.StateResolved || !typ.IsUnknown(tv.Type)) {
			return tv.Type
		}
	}
	if p.cfg.FunctionType != nil {
		if t := p.cfg.FunctionType(sym); t != nil {
			return t
		}
	}
	return nil
}

func (p Projector) finalizeObservedPath(observed typ.Type) typ.Type {
	return p.finalizeObservedPathForPolicy(observed, p.cfg.PreserveProof)
}

func (p Projector) finalizeObservedPathForPolicy(observed typ.Type, preserveProof bool) typ.Type {
	if preserveProof {
		return observed
	}
	return value.AdmitObservation(observed)
}

func firstType(types []typ.Type) typ.Type {
	if len(types) == 0 || types[0] == nil {
		return typ.Unknown
	}
	return types[0]
}

func addRecordField(builder *typ.RecordBuilder, name string, ft typ.Type) {
	if builder == nil || name == "" {
		return
	}
	if ft == nil {
		ft = typ.Unknown
	}
	if inner, optional := typ.SplitNilableFieldType(ft); optional {
		builder.OptField(name, inner)
		return
	}
	if lit, ok := ft.(*typ.Literal); ok && lit.Base != kind.String && lit.Base != kind.Boolean {
		ft = value.AdmitObservation(ft)
	}
	builder.Field(name, ft)
}
