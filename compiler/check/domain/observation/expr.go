// Package observation projects solved abstract-interpreter evidence into value
// observations without re-entering checker synthesis.
package observation

import (
	"fmt"
	"os"
	"strconv"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/conditionexpr"
	"github.com/wippyai/go-lua/compiler/check/domain/indexread"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/intercept"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
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

// zprobeExpr traces observation read-surface narrowing when ZNARROW is set.
func zprobeExpr(format string, args ...interface{}) {
	if os.Getenv("ZNARROW") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[ZEXPR] "+format+"\n", args...)
}

// ZProbeRefined exposes the flow-refined symbol type at a point for the
// narrowing-precision probe. Debug helper.
func (p Projector) ZProbeRefined(point cfg.Point, sym cfg.SymbolID) (typ.Type, flow.TypeState) {
	if p.cfg.Facts == nil {
		return nil, flow.StateUnknown
	}
	tv := p.cfg.Facts.RefinedAt(point, sym)
	return tv.Type, tv.State
}

// SymbolTypeLookup projects canonical function facts or other product-owned
// symbol types into the observation surface.
type SymbolTypeLookup func(sym cfg.SymbolID) typ.Type

// TypeResolver resolves annotation AST into concrete types in the lexical scope
// visible at a CFG point.
type TypeResolver func(ast.TypeExpr, *scope.State) typ.Type

// Config supplies immutable solved-state inputs for expression observation.
type Config struct {
	Graph             *cfg.Graph
	Bindings          *bind.BindingTable
	Scopes            map[cfg.Point]*scope.State
	Facts             flow.TypeFacts
	Inputs            *flow.Inputs
	Solution          *flow.Solution
	Ctx               *db.QueryContext
	TypeOps           querycore.TypeOps
	LiteralSignatures map[*ast.FunctionExpr]*typ.Function
	FunctionType      SymbolTypeLookup
	ResolveType       TypeResolver
	GlobalTypes       map[string]typ.Type
	PreferPreState    bool
	PreserveProof     bool
	GradualParamReads bool
	LocalCondition    *constraint.Condition

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
	cfg Config
}

// New returns a solved-state expression observation projector.
func New(cfg Config) Projector {
	if cfg.Bindings == nil && cfg.Graph != nil {
		cfg.Bindings = cfg.Graph.Bindings()
	}
	return Projector{cfg: cfg}
}

// WithPreStateReads returns a projector that prefers point-entry path reads.
// This models assignment RHS boundaries where the target may also appear in
// the source expression.
func (p Projector) WithPreStateReads() Projector {
	p.cfg.PreferPreState = true
	return p
}

// WithProofValues returns a projector for diagnostics/proofs. It preserves
// branch-proven literal refinements instead of admitting them into the
// convergent value-product representation.
func (p Projector) WithProofValues() Projector {
	p.cfg.PreserveProof = true
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

// FromFuncResult returns the solved-state observation projector for a completed
// function analysis result.
func FromFuncResult(result *api.FuncResult, functionType SymbolTypeLookup) Projector {
	cfg := Config{FunctionType: functionType}
	if result != nil {
		cfg.Graph = result.Graph
		cfg.Bindings = result.ModuleBindings
		if result.Graph != nil && result.Graph.Bindings() != nil {
			cfg.Bindings = result.Graph.Bindings()
		}
		cfg.Scopes = result.Scopes
		cfg.Facts = result.Facts
		cfg.Inputs = result.FlowInputs
		cfg.Solution = result.FlowSolution
		cfg.LiteralSignatures = result.LiteralSignatures
		cfg.Ctx = result.QueryContext
		cfg.TypeOps = result.TypeOps
		cfg.GlobalTypes = result.GlobalTypes
		cfg.RecursiveFamilies = result.RecursiveFamilies
		cfg.ClassFamilyJoin = result.ClassFamilyJoin
		if result.NarrowSynth != nil {
			cfg.ResolveType = result.NarrowSynth.ResolveType
		}
	}
	return New(cfg)
}

// FromAnalysisView returns the solved-state observation projector for a stable
// function-analysis view.
func FromAnalysisView(result *api.FuncAnalysisView, functionType SymbolTypeLookup) Projector {
	cfg := Config{FunctionType: functionType}
	if result != nil {
		cfg.Graph = result.Graph
		cfg.Scopes = result.Scopes
		cfg.Facts = result.Facts
		cfg.Inputs = result.FlowInputs
		cfg.Solution = result.FlowSolution
		cfg.Ctx = result.QueryContext
		cfg.TypeOps = result.TypeOps
		cfg.GlobalTypes = result.GlobalTypes
		cfg.LiteralSignatures = result.LiteralSignatures
		cfg.RecursiveFamilies = result.RecursiveFamilies
		cfg.ClassFamilyJoin = result.ClassFamilyJoin
		if result.NarrowSynth != nil {
			cfg.ResolveType = result.NarrowSynth.ResolveType
		}
	}
	return New(cfg)
}

// ObservedArgumentType joins the current expression observation with the
// solved pre-state/path observation at a call boundary.
func ObservedArgumentType(result *api.FuncResult, point cfg.Point, arg ast.Expr, current typ.Type, bindings *bind.BindingTable) typ.Type {
	if result == nil || result.FlowSolution == nil || result.Graph == nil || arg == nil {
		return current
	}
	argPath := flowpath.FromExprWithBindings(arg, nil, bindings)
	if argPath.IsEmpty() || argPath.Symbol == 0 {
		return current
	}
	argPath = flowpath.WithVersion(argPath, result.Graph, point)
	declared := FromFuncResult(result, nil).pathDeclaredType(point, argPath)
	if t := result.FlowSolution.PreStateTypeAt(point, argPath); !typ.IsAbsentOrUnknown(t) {
		if selected, ok := value.SelectPathObservation(t, nil, declared); ok {
			return paramevidence.MergeArgumentObservation(current, selected)
		}
		return paramevidence.MergeArgumentObservation(current, t)
	}
	if t := result.FlowSolution.NarrowedTypeAt(point, argPath); !typ.IsAbsentOrUnknown(t) {
		if selected, ok := value.SelectPathObservation(t, nil, declared); ok {
			return paramevidence.MergeArgumentObservation(current, selected)
		}
		return paramevidence.MergeArgumentObservation(current, t)
	}
	return current
}

// TypeOf observes the single-value type of expr at point p.
func (p Projector) TypeOf(expr ast.Expr, point cfg.Point) typ.Type {
	return p.typeOf(expr, point, nil)
}

// TypeOfWithExpected observes expr using an optional contextual type.
func (p Projector) TypeOfWithExpected(expr ast.Expr, point cfg.Point, expected typ.Type) typ.Type {
	return p.typeOf(expr, point, expected)
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
	if t := p.pathType(expr, point); !typ.IsAbsentOrUnknown(t) {
		return p.coerceGradualToExpected(t, expected)
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
	if len(p.cfg.Scopes) == 0 {
		return nil
	}
	return p.cfg.Scopes[point]
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
	if p.cfg.GlobalTypes != nil {
		if t := p.cfg.GlobalTypes[expr.Value]; t != nil {
			return t
		}
	}
	return typ.Unknown
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
	fn := p.cfg.Graph.Func()
	if fn == nil || fn.ParList == nil {
		return false
	}
	for _, slot := range p.cfg.Graph.ParamSlotsReadOnly() {
		if slot.Symbol != sym {
			continue
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
	if refined, ok := p.refineWithExpectedProof(point, expr, t, expected); ok {
		return refined
	}
	return p.coerceGradualToExpected(t, expected)
}

func (p Projector) provesExprType(point cfg.Point, expr ast.Expr, expected typ.Type) bool {
	if p.cfg.Solution == nil || expr == nil || expected == nil {
		return false
	}
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() {
		return false
	}
	if p.cfg.LocalCondition != nil {
		if t := p.cfg.Solution.ConditionedTypeAt(point, path, *p.cfg.LocalCondition); !typ.IsAbsentOrUnknown(t) {
			return subtype.IsSubtype(t, expected)
		}
	}
	return p.cfg.Solution.ProvesTypeAt(point, path, expected)
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
	if t := p.pathType(expr, point); !typ.IsAbsentOrUnknown(t) {
		if refined, ok := p.refineWithExpectedProof(point, expr, t, expected); ok {
			return refined
		}
		return p.coerceGradualToExpected(t, expected)
	}
	t := p.attrType(expr, point)
	if refined, ok := p.refineWithExpectedProof(point, expr, t, expected); ok {
		return refined
	}
	return p.coerceGradualToExpected(t, expected)
}

// coerceGradualToExpected applies gradual-typing's consistency at a typed
// boundary: a value observed as the gradual top `any` (an unannotated parameter,
// or a field/index read off one) is consistent with every type, so against a
// concrete expected type it observes as that expected type. This mirrors the
// proven-path refinement refineWithExpectedProof already performs for `any`, for
// the unproven gradual case the path-type short-circuit otherwise bypasses. It is
// gradual-`any` admission, not error suppression: only the gradual top is
// coerced, and only toward a concrete expected target (never `unknown`, the
// opaque inference seed, which keeps its strict rejection).
func (p Projector) coerceGradualToExpected(t typ.Type, expected typ.Type) typ.Type {
	if expected == nil || !typ.IsAny(t) {
		return t
	}
	if typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
		return t
	}
	return expected
}

// AdmitGradualArgument applies the gradual-top-any admission for the
// call-argument diagnostic boundary, mirroring the assignment source and the
// return boundary: an argument observed as the gradual top `any` is consistent
// with a concrete expected parameter type. It is gated by sourceAnyIsGradualTop
// so a declared-`any` symbol read (an `any` the cast-guard contract requires to
// stay strict) and the opaque `unknown` seed are untouched and keep their
// rejection.
func (p Projector) AdmitGradualArgument(t typ.Type, arg ast.Expr, point cfg.Point, expected typ.Type) typ.Type {
	if zNoGradualBoundary() {
		return t
	}
	if !typ.IsAny(t) || !p.sourceAnyIsGradualTop(arg, point) {
		return t
	}
	return p.coerceGradualToExpected(t, expected)
}

// zNoGradualBoundary disables the assignment/call-argument gradual-top admission
// for the baseline measurement probe, leaving the pre-existing return coercion
// untouched. Debug helper.
func zNoGradualBoundary() bool {
	return os.Getenv("ZNOGRADUAL_BOUNDARY") != ""
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
	case *typ.Map, *typ.Array, *typ.Tuple:
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
	if typ.IsAbsentOrUnknown(obj) {
		return typ.Unknown
	}
	if seg, ok := flowpath.StaticAttrKeySegmentWithConst(expr.Key, p.constResolver(point)); ok && seg.Kind == constraint.SegmentField {
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
	key := p.TypeOf(expr.Key, point)
	if p.cfg.TypeOps != nil {
		if t, ok := p.cfg.TypeOps.Index(p.cfg.Ctx, obj, key); ok {
			return p.applyIndexReadProof(t, obj, expr.Object, expr.Key, point)
		}
	} else if t, ok := querycore.Index(obj, key); ok {
		return p.applyIndexReadProof(t, obj, expr.Object, expr.Key, point)
	}
	if querycore.MissingFieldReadsNil(obj) {
		return typ.Nil
	}
	return typ.Unknown
}

func (p Projector) applyIndexReadProof(t typ.Type, objType typ.Type, obj ast.Expr, key ast.Expr, point cfg.Point) typ.Type {
	if t == nil || p.cfg.Solution == nil {
		return t
	}
	if refined, ok := indexread.Refine(indexread.Query{
		Point:     point,
		Container: objType,
		Result:    t,
		Object:    obj,
		Key:       key,
		Flow:      p.cfg.Solution,
		PathOf: func(expr ast.Expr) constraint.Path {
			return p.pathOfExpr(expr, point)
		},
	}); ok {
		return refined
	}
	return t
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
	fields, elems, fieldCount, _ := p.tableFields(expr, expected, point, false)
	if expected != nil {
		if result := ops.CheckTable(fields, elems, expected); len(result.Errors) == 0 {
			if result.Type != nil {
				return result.Type
			}
			return expected
		}
	}
	if fieldCount == 0 && len(elems) > 0 {
		return value.AdmitObservation(typ.NewTuple(elems...))
	}
	builder := typ.NewRecord()
	for _, field := range fields {
		addRecordField(builder, field.Name, field.Type)
	}
	return value.AdmitObservation(builder.Build())
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
	expectedFn := unwrap.Function(expected)
	if p.cfg.Bindings != nil && p.cfg.FunctionType != nil {
		if sym, ok := p.cfg.Bindings.FuncLitSymbol(fn); ok && sym != 0 {
			if t := p.cfg.FunctionType(sym); t != nil {
				if observed := unwrap.Function(t); observed != nil {
					return contextualFunctionLiteralType(fn, observed, expectedFn)
				}
				return t
			}
		}
	}
	if p.cfg.LiteralSignatures != nil {
		if sig := p.cfg.LiteralSignatures[fn]; sig != nil {
			return contextualFunctionLiteralType(fn, sig, expectedFn)
		}
	}
	if expectedFn != nil {
		return expectedFn
	}
	return typ.Func().Build()
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
			builder.TypeParam(tp.Name, tp.Constraint)
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
	if expr == nil || p.cfg.GlobalTypes == nil {
		return nil, false
	}
	var resolver intercept.VariadicTypeResolver
	sc := p.cfg.Scopes[point]
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
			if t, ok := p.cfg.GlobalTypes[name]; ok {
				return t
			}
			return nil
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
	sc := p.cfg.Scopes[point]
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
			if p.cfg.GlobalTypes == nil {
				return nil
			}
			if t, ok := p.cfg.GlobalTypes[name]; ok {
				return t
			}
			return nil
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
	returns := callResultReturns(result)
	return applyEffectReturnTransforms(p.cfg.Ctx, p.cfg.TypeOps, callee, args, returns, receiver, expr.Method != "", false)
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

func callResultReturns(result ops.CallResult) []typ.Type {
	if len(result.Returns) > 0 {
		out := make([]typ.Type, len(result.Returns))
		copy(out, result.Returns)
		return out
	}
	if tuple, ok := result.Type.(*typ.Tuple); ok {
		out := make([]typ.Type, len(tuple.Elements))
		copy(out, tuple.Elements)
		return out
	}
	return []typ.Type{result.Type}
}

func applyEffectReturnTransforms(ctx *db.QueryContext, query querycore.TypeOps, callee typ.Type, args []typ.Type, returns []typ.Type, receiver typ.Type, isMethod bool, forceMethodReceiver bool) []typ.Type {
	fn := unwrap.Function(callee)
	if fn == nil || len(returns) == 0 {
		return returns
	}
	effectArgs := ops.RuntimeArgsForEffects(ctx, query, callee, args, receiver, isMethod, forceMethodReceiver)
	var out []typ.Type
	for i := range returns {
		transformed := transform.ApplyEffectTransform(fn, effectArgs, i, returns[i])
		if transformed == nil || transformed == returns[i] {
			continue
		}
		if out == nil {
			out = make([]typ.Type, len(returns))
			copy(out, returns)
		}
		out[i] = transformed
	}
	if out != nil {
		return out
	}
	return returns
}

func (p Projector) pathType(expr ast.Expr, point cfg.Point) typ.Type {
	if expr == nil {
		return nil
	}
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() {
		return nil
	}
	declared := p.pathDeclaredType(point, path)
	if p.cfg.Solution != nil {
		var solved typ.Type
		var proof typ.Type
		if p.cfg.PreferPreState {
			if t := p.cfg.Solution.PreStateTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
				solved = t
			} else if t := p.cfg.Solution.NarrowedTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
				solved = t
			}
			if p.cfg.LocalCondition != nil {
				proof = p.conditionedPathType(point, path, solved, declared)
			}
			if typ.IsAbsentOrUnknown(solved) && typ.IsAbsentOrUnknown(proof) {
				proof = p.cfg.Solution.ConditionTypeAt(point, path)
			}
		} else {
			hasConditionProof := p.hasConditionProofAt(point)
			if p.cfg.PreserveProof && hasConditionProof && typ.IsAbsentOrUnknown(proof) {
				proof = p.cfg.Solution.ConditionTypeAt(point, path)
			}
			if t := p.cfg.Solution.NarrowedTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
				solved = t
			} else if t := p.cfg.Solution.PreStateTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
				solved = t
			}
			if p.cfg.LocalCondition != nil {
				proof = p.conditionedPathType(point, path, solved, declared)
			}
			if typ.IsAbsentOrUnknown(solved) && (!p.cfg.PreserveProof || !hasConditionProof) && typ.IsAbsentOrUnknown(proof) {
				proof = p.cfg.Solution.ConditionTypeAt(point, path)
			}
		}
		// A provably-empty (never) flow narrowing is authoritative: do not let a
		// declared annotation re-widen it back to the declared type. Annotated
		// locals carry a declared fallback here while unannotated symbols do not,
		// so without this both must observe the same genuine never narrowing.
		if typ.IsNever(solved) {
			return p.finalizeObservedPath(solved)
		}
		// A short-circuit local condition that proves the path empty is
		// authoritative: the expression is on a control branch that cannot
		// execute for this value (e.g. msg.content[1] guarded by
		// type(msg.content) == "table" when msg.content is a string). The proof
		// surface (ConditionedTypeAt/ConditionedSeedTypeAt) only yields Never as
		// genuine bottom for ConditionAt(point) AND LocalCondition; projection
		// imprecision falls back to nil, not Never, so this never converts an
		// unknown read into a spurious unreachable.
		if p.cfg.LocalCondition != nil && typ.IsNever(proof) {
			return p.finalizeObservedPath(proof)
		}
		if selected, ok := value.SelectPathObservation(solved, proof, declared); ok {
			selected = p.applyPathPresenceProof(selected, expr, point)
			return p.finalizeObservedPath(selected)
		}
	} else if t, ok := p.factsNarrowedPathType(point, path); ok {
		zprobeExpr("pathType point=%v root=%d segs=%v factsNarrowed=%v declared=%v", point, path.Symbol, path.Segments, t, declared)
		// No Solution: the canonical flow's narrowing lives in the per-point Facts.
		// A read of a path narrowed by a branch guard (a nil-check fall-through, a
		// type guard, a discriminant) observes the flow-refined type here rather than
		// the flow-insensitive declared type, the same surface the assignment-source
		// read consults. The merge-point env LUB recovers the unnarrowed value where
		// both edges meet, so a narrowing never survives past its guard.
		//
		// A provably-empty (never) narrowing is authoritative: the read is on an
		// impossible edge (a discriminant guard pinned the value to the other
		// variant), so a declared annotation must not re-widen it back, mirroring the
		// Solution-present never branch above.
		if typ.IsNever(t) {
			return p.finalizeObservedPath(t)
		}
		selected := p.reconcileObservedPath(t, declared)
		selected = p.applyPathPresenceProof(selected, expr, point)
		return p.finalizeObservedPath(selected)
	}
	if declared != nil {
		return p.applyPathPresenceProof(declared, expr, point)
	}
	return nil
}

func (p Projector) applyPathPresenceProof(t typ.Type, expr ast.Expr, point cfg.Point) typ.Type {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return t
	}
	return p.applyIndexReadProof(t, p.TypeOf(attr.Object, point), attr.Object, attr.Key, point)
}

func (p Projector) conditionedPathType(point cfg.Point, path constraint.Path, solved typ.Type, declared typ.Type) typ.Type {
	if p.cfg.LocalCondition == nil || p.cfg.Solution == nil || path.IsEmpty() {
		return nil
	}
	seedPath := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	seedType := solved
	if len(path.Segments) > 0 || typ.IsAbsentOrUnknown(seedType) {
		seedType = p.currentPathType(point, seedPath)
	}
	if typ.IsAbsentOrUnknown(seedType) && len(path.Segments) == 0 {
		seedType = declared
	}
	if typ.IsAbsentOrUnknown(seedType) {
		seedType = p.pathDeclaredType(point, seedPath)
	}
	if typ.IsAbsentOrUnknown(seedType) {
		return p.cfg.Solution.ConditionedTypeAt(point, path, *p.cfg.LocalCondition)
	}
	return p.cfg.Solution.ConditionedSeedTypeAt(point, seedPath, seedType, path, *p.cfg.LocalCondition)
}

func (p Projector) currentPathType(point cfg.Point, path constraint.Path) typ.Type {
	if p.cfg.Solution == nil || path.IsEmpty() {
		return nil
	}
	if p.cfg.PreferPreState {
		if t := p.cfg.Solution.PreStateTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
			return t
		}
		return p.cfg.Solution.NarrowedTypeAt(point, path)
	}
	if t := p.cfg.Solution.NarrowedTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
		return t
	}
	return p.cfg.Solution.PreStateTypeAt(point, path)
}

func (p Projector) hasConditionProofAt(point cfg.Point) bool {
	if p.cfg.Solution == nil {
		return false
	}
	cond := p.cfg.Solution.ConditionAt(point)
	return cond.IsFalse() || cond.HasConstraints()
}

func (p Projector) pathOfExpr(expr ast.Expr, point cfg.Point) constraint.Path {
	return flowpath.FromExprWithBindingsAt(expr, p.constResolver(point), p.cfg.Bindings, p.cfg.Graph, point)
}

func (p Projector) constResolver(point cfg.Point) func(string) *flow.ConstValue {
	if p.cfg.Solution == nil {
		return nil
	}
	return func(name string) *flow.ConstValue {
		return p.cfg.Solution.ConstValueAt(point, name)
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
	current := base
	for _, segment := range path.Segments {
		var next typ.Type
		switch segment.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			if p.cfg.TypeOps != nil {
				next, _ = p.cfg.TypeOps.Field(p.cfg.Ctx, current, segment.Name)
				if next == nil {
					next, _ = p.cfg.TypeOps.Index(p.cfg.Ctx, current, typ.LiteralString(segment.Name))
				}
			} else {
				next, _ = querycore.Field(current, segment.Name)
				if next == nil {
					next, _ = querycore.Index(current, typ.LiteralString(segment.Name))
				}
			}
		case constraint.SegmentIndexInt:
			key := typ.LiteralInt(int64(segment.Index))
			if p.cfg.TypeOps != nil {
				next, _ = p.cfg.TypeOps.Index(p.cfg.Ctx, current, key)
			} else {
				next, _ = querycore.Index(current, key)
			}
		default:
			return nil
		}
		if next == nil {
			if querycore.MissingFieldReadsNil(current) {
				next = typ.Nil
			} else {
				return nil
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	return current
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

func (p Projector) reconcileObservedPath(observed, declared typ.Type) typ.Type {
	zprobeExpr("reconcileObservedPath observed=%v declared=%v", observed, declared)
	if t, ok := value.ReconcilePathFactWithDeclaredRead(observed, declared); ok {
		if p.cfg.PreserveProof {
			return t
		}
		return value.AdmitObservation(t)
	}
	if p.cfg.PreserveProof {
		return observed
	}
	return value.AdmitObservation(observed)
}

func (p Projector) finalizeObservedPath(observed typ.Type) typ.Type {
	if p.cfg.PreserveProof {
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
