// Package input assembles the inputs the canonical intraprocedural solver needs
// for one function: the CFG and graph topology, the raw graph-event evidence,
// and the declared scope facts (parameter symbols, names, and declared types).
//
// It reuses ONLY the sound leaf extractors of the legacy pipeline:
//
//   - the CFG and its binding table (compiler/cfg.Build / a session's
//     GetOrBuildCFG produce the same *cfg.Graph with bindings attached);
//   - the raw graph-event evidence (compiler/check/abstract/trace.GraphEvidence,
//     the same stream a session's store.EvidenceForGraph caches) — assignments,
//     calls, returns, branches, function definitions, identifier and parameter
//     uses;
//   - declared parameter facts read off the function's own parameter list, with
//     a caller-supplied resolver for the type annotations (annotations, aliases,
//     and base scopes are the caller's; this package does not own them).
//
// It does NOT call InferLocalTypes, CollectSpecNarrowedTypes,
// buildPreflowBranchSolution, or SolveConditionView, and it does not build the
// legacy flow.Inputs. Local inference is not an input here — it is produced by
// the value/condition/numeric transfer equations the solver runs over these
// inputs. Edge conditions and numeric conditions are transfer INPUTS the
// per-node transfer reads from the raw evidence and CFG, not pre-solved facts.
package input

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/constprop"
	abstractcore "github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/scope"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/propagate"
	"github.com/wippyai/go-lua/types/typ"
)

// Inputs is everything the canonical solver needs for one function. It is a
// passive carrier: the equation.Builder owns the topology walk, and the injected
// NodeTransfer reads these fields to compute per-node effects.
type Inputs struct {
	// Graph is the function's control-flow graph. Its Entry/Successors/
	// Predecessors drive the equation graph; its per-point node info (Assign,
	// Return, Branch, Call) is the syntactic source the transfer interprets.
	Graph *cfg.Graph

	// Evidence is the raw graph-event trace: assignments, calls, returns,
	// branches, identifier uses, parameter uses, and function definitions. The
	// transfer reads it as syntactic evidence, never as solved facts.
	Evidence api.FlowEvidence

	// Scope holds the declared facts: parameter symbols and names in declaration
	// order, resolved parameter annotations, and symbol-keyed declared types. Body
	// uses refine parameters into contract demand; declared types seed entry state
	// and provide the authoritative narrowing base for annotated locals.
	Scope ScopeFacts

	// ConditionDemand seeds the SSA-version liveness abstraction used to project
	// dead path-condition facts out of the canonical PointState.Cond component.
	// It is graph/evidence-derived input, not a precision pass: the equation
	// builder applies it as a sound point-state abstraction inside the single
	// fixed point.
	ConditionDemand *propagate.Demand

	// LoopAppendLengths are conservative loop-summary facts derived from CFG
	// topology and source syntax before interpretation. They are passive input:
	// transfer consumes them at the loop exit inside the canonical fixed point.
	LoopAppendLengths []LoopAppendLengthFact

	// ConstValues are passive, point-indexed constant facts used only to normalize
	// paths before interpretation (`obj[key]` where key is proven literal). They
	// are deterministic CFG/evidence facts, not a replacement for value solving.
	ConstValues map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue

	// VariantFieldOrigins are passive return-transform provenance facts. They
	// link a field of a returned variant to a source path under a first-class
	// finite origin family/case relation, so branch condition extraction can
	// lower identity tests into ordinary condition-axis constraints inside the
	// single fixed point.
	VariantFieldOrigins []flow.VariantFieldOrigin
}

// ScopeFacts are declared, non-flow facts about a function's symbols. Parameter
// facts come from the parameter list and a caller-supplied annotation resolver;
// callers may add resolved local/global annotations to DeclaredTypes before
// constructing transfer. No flow solving is involved.
type ScopeFacts struct {
	// ParamSymbols are the parameter symbol IDs in declaration order.
	ParamSymbols []cfg.SymbolID
	// ParamNames are the parameter names in declaration order, parallel to
	// ParamSymbols.
	ParamNames []string
	// DeclaredParamTypes maps a parameter index to its resolved declared type.
	// An absent index is an unannotated parameter whose type the solver infers
	// from body use.
	DeclaredParamTypes map[int]typ.Type
	// DeclaredTypes maps annotated symbols to their resolved declared type. It is
	// the symbol-keyed authoritative type fact transfer reads for entry seeding,
	// declared-container handling, and discriminant/typeof narrowing bases.
	DeclaredTypes map[cfg.SymbolID]typ.Type
	// CellSymbols are symbols declared by this function whose lexical locations
	// are captured by nested closures. They are sorted unique immutable input facts:
	// transfer stores them in PointState.Cells instead of Env so owner and child
	// closures share the same abstract location inside the product fixed point.
	CellSymbols []cfg.SymbolID
}

// NumParams is the parameter count, the number of contract cells the equation
// graph allocates.
func (s ScopeFacts) NumParams() int {
	return len(s.ParamSymbols)
}

// TypeResolver resolves a declared parameter type annotation in the function's
// scope. It is the single seam to the caller's annotation/alias/base-scope
// machinery; this package does not own type resolution. A nil resolver, or a
// resolver returning nil for an annotation, leaves that parameter unannotated
// (inferred from body use).
type TypeResolver func(expr ast.TypeExpr, sc *scope.State) typ.Type

// Build assembles Inputs for the function whose graph is g, resolving parameter
// annotations through resolve in the function's base scope baseScope.
//
// g must already carry its binding table (cfg.Build attaches it; a session's
// GetOrBuildCFG returns such a graph). evidence is the raw graph-event trace; a
// caller with a session passes store.EvidenceForGraph(g), and BuildFromFunction
// derives it from the graph when none is supplied.
func Build(g *cfg.Graph, evidence api.FlowEvidence, resolve TypeResolver, baseScope *scope.State) Inputs {
	in := Inputs{
		Graph:    g,
		Evidence: evidence,
		Scope:    scopeFacts(g, resolve, baseScope),
	}
	in.ConstValues = BuildConstValues(g, evidence)
	in.ConditionDemand = BuildConditionDemand(in)
	in.LoopAppendLengths = BuildLoopAppendLengths(in)
	return in
}

// BuildConstValues derives point-local literal bindings used by canonical path
// normalization. It reuses the existing constant-propagation leaf extractor as a
// passive input producer; transfer still owns all value interpretation in the
// single canonical fixed point.
func BuildConstValues(g *cfg.Graph, evidence api.FlowEvidence) map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue {
	if g == nil {
		return nil
	}
	temp := &flow.Inputs{}
	fc := &abstractcore.FlowContext{Graph: g, Evidence: evidence}
	constprop.CollectConstAssignments(fc, temp)
	constprop.PropagateAllConstValues(fc, temp)
	return temp.ConstValues
}

func (in Inputs) constResolver(p cfg.Point) func(string) *flow.ConstValue {
	if in.Graph == nil || len(in.ConstValues) == 0 {
		return nil
	}
	return func(name string) *flow.ConstValue {
		sym, ok := in.Graph.SymbolAt(p, name)
		if !ok || sym == 0 {
			if bindings := in.Graph.Bindings(); bindings != nil {
				symbols := bindings.SymbolsByName(name)
				if len(symbols) == 1 {
					sym = symbols[0]
				}
			}
			if sym == 0 {
				return nil
			}
		}
		at := in.ConstValues[sym]
		if at == nil {
			return nil
		}
		val := at[p]
		if val == nil || val.Kind == flow.ConstUnknown {
			return nil
		}
		return val
	}
}

// BuildFromFunction is the standalone path: it builds the CFG and its binding
// table from fn (the same cfg.Build a session uses), derives the raw evidence
// from the resulting graph (trace.GraphEvidence, the same stream a session
// caches), and assembles Inputs. globals are the predeclared global names the
// binder seeds. It is the entry point for callers that hold an *ast.FunctionExpr
// rather than a session-built graph.
func BuildFromFunction(fn *ast.FunctionExpr, resolve TypeResolver, baseScope *scope.State, globals ...string) Inputs {
	g := cfg.Build(fn, globals...)
	if g == nil {
		return Inputs{}
	}
	evidence := trace.GraphEvidence(g, g.Bindings())
	return Build(g, evidence, resolve, baseScope)
}

// BuildConditionDemand derives the read/def demand that bounds canonical
// path-condition vocabulary. It is the canonical counterpart of the legacy
// flow condition-demand builder: every real expression/guard/return read is a
// use, whole-symbol definitions and phi targets are defs, and return points keep
// exported roots live for summary projection.
func BuildConditionDemand(in Inputs) *propagate.Demand {
	g := in.Graph
	if g == nil {
		return nil
	}
	uses := make(map[basecfg.Point][]constraint.Path)
	defs := make(map[basecfg.Point][]basecfg.Version)

	addUse := func(p basecfg.Point, path constraint.Path) {
		if path.Symbol == 0 {
			return
		}
		uses[p] = append(uses[p], path)
	}
	addExpr := func(p basecfg.Point, expr ast.Expr) {
		walkConditionDemandExpr(g, p, expr, in.constResolver(p), addUse)
	}

	g.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if info == nil {
			return
		}
		addExpr(p, info.Condition)
	})
	g.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for _, expr := range info.Sources {
			addExpr(p, expr)
		}
		for _, expr := range info.IterExprs {
			addExpr(p, expr)
		}
		if info.NumericFor != nil {
			addExpr(p, info.NumericFor.Init)
			addExpr(p, info.NumericFor.Limit)
			addExpr(p, info.NumericFor.Step)
		}
		for _, target := range info.Targets {
			switch target.Kind {
			case cfg.TargetIdent:
				if target.Symbol != 0 {
					if ver := g.VisibleVersion(p, target.Symbol); !ver.IsZero() {
						defs[p] = append(defs[p], ver)
					}
				}
			case cfg.TargetField, cfg.TargetIndex:
				addExpr(p, target.Base)
				addExpr(p, target.Key)
			}
		}
	})
	g.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		addExpr(p, info.Callee)
		addExpr(p, info.Receiver)
		for _, arg := range info.Args {
			addExpr(p, arg)
		}
		if !info.CalleePath.IsEmpty() {
			addUse(p, withVisibleVersion(g, p, info.CalleePath))
		}
		if info.TypeCheckPath.Symbol != 0 {
			addUse(p, withVisibleVersion(g, p, info.TypeCheckPath))
		}
	})
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		for _, expr := range info.Exprs {
			addExpr(p, expr)
		}
	})

	for _, phi := range g.PhiNodes() {
		for _, op := range phi.Operands {
			if op.Version.Symbol != 0 {
				addUse(op.From, versionPath(op.Version))
			}
		}
		if phi.Target.Symbol != 0 && phi.Target.ID != 0 {
			defs[phi.Point] = append(defs[phi.Point], phi.Target)
		}
	}

	for _, p := range g.RPO() {
		node := g.Node(p)
		if node == nil {
			continue
		}
		if node.Kind != basecfg.NodeReturn && node.Kind != basecfg.NodeExit {
			continue
		}
		for sym, ver := range g.AllVisibleVersions(p) {
			if sym == 0 || ver.IsZero() || !returnSummaryKind(g, sym) {
				continue
			}
			addUse(p, versionPath(ver))
		}
	}

	if len(uses) == 0 && len(defs) == 0 {
		return nil
	}
	return &propagate.Demand{Uses: uses, Defs: defs}
}

func walkConditionDemandExpr(g *cfg.Graph, p cfg.Point, expr ast.Expr, constResolver func(string) *flow.ConstValue, addUse func(basecfg.Point, constraint.Path)) {
	if expr == nil || g == nil || addUse == nil {
		return
	}
	record := func(e ast.Expr) {
		path := domainpath.FromExprWithBindingsAt(e, constResolver, g.Bindings(), g, p)
		if path.Symbol != 0 {
			addUse(p, path)
		}
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		record(e)
	case *ast.AttrGetExpr:
		record(e)
		walkConditionDemandExpr(g, p, e.Object, constResolver, addUse)
		walkConditionDemandExpr(g, p, e.Key, constResolver, addUse)
	case *ast.FuncCallExpr:
		walkConditionDemandExpr(g, p, e.Func, constResolver, addUse)
		walkConditionDemandExpr(g, p, e.Receiver, constResolver, addUse)
		for _, arg := range e.Args {
			walkConditionDemandExpr(g, p, arg, constResolver, addUse)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			walkConditionDemandExpr(g, p, field.Key, constResolver, addUse)
			walkConditionDemandExpr(g, p, field.Value, constResolver, addUse)
		}
	case *ast.LogicalOpExpr:
		walkConditionDemandExpr(g, p, e.Lhs, constResolver, addUse)
		walkConditionDemandExpr(g, p, e.Rhs, constResolver, addUse)
	case *ast.RelationalOpExpr:
		walkConditionDemandExpr(g, p, e.Lhs, constResolver, addUse)
		walkConditionDemandExpr(g, p, e.Rhs, constResolver, addUse)
	case *ast.StringConcatOpExpr:
		walkConditionDemandExpr(g, p, e.Lhs, constResolver, addUse)
		walkConditionDemandExpr(g, p, e.Rhs, constResolver, addUse)
	case *ast.ArithmeticOpExpr:
		walkConditionDemandExpr(g, p, e.Lhs, constResolver, addUse)
		walkConditionDemandExpr(g, p, e.Rhs, constResolver, addUse)
	case *ast.UnaryMinusOpExpr:
		walkConditionDemandExpr(g, p, e.Expr, constResolver, addUse)
	case *ast.UnaryNotOpExpr:
		walkConditionDemandExpr(g, p, e.Expr, constResolver, addUse)
	case *ast.UnaryLenOpExpr:
		walkConditionDemandExpr(g, p, e.Expr, constResolver, addUse)
	case *ast.UnaryBNotOpExpr:
		walkConditionDemandExpr(g, p, e.Expr, constResolver, addUse)
	case *ast.CastExpr:
		walkConditionDemandExpr(g, p, e.Expr, constResolver, addUse)
	case *ast.NonNilAssertExpr:
		walkConditionDemandExpr(g, p, e.Expr, constResolver, addUse)
	}
}

func withVisibleVersion(g *cfg.Graph, p cfg.Point, path constraint.Path) constraint.Path {
	if path.Symbol == 0 {
		return path
	}
	ver := g.VisibleVersion(p, path.Symbol)
	if ver.IsZero() {
		return path
	}
	path.Version = ver.ID
	return path
}

func versionPath(v basecfg.Version) constraint.Path {
	return constraint.Path{Root: v.Root, Symbol: v.Symbol, Version: v.ID}
}

func returnSummaryKind(g *cfg.Graph, sym basecfg.SymbolID) bool {
	k, ok := g.SymbolKind(sym)
	if !ok {
		return true
	}
	switch k {
	case basecfg.SymbolParam, basecfg.SymbolUpvalue, basecfg.SymbolGlobal:
		return true
	default:
		return false
	}
}

// scopeFacts reads the declared parameter facts off the graph's function and
// resolves the declared parameter types through resolve.
func scopeFacts(g *cfg.Graph, resolve TypeResolver, baseScope *scope.State) ScopeFacts {
	if g == nil {
		return ScopeFacts{}
	}
	facts := ScopeFacts{
		ParamSymbols:       g.ParamSymbols(),
		DeclaredParamTypes: make(map[int]typ.Type),
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
	}
	fn := g.Func()
	if fn == nil || fn.ParList == nil {
		return facts
	}
	facts.ParamNames = append([]string(nil), fn.ParList.Names...)
	if resolve == nil {
		return facts
	}
	for i, ann := range fn.ParList.Types {
		if ann == nil {
			continue
		}
		if t := resolve(ann, baseScope); t != nil {
			facts.DeclaredParamTypes[i] = t
		}
	}
	for _, slot := range g.ParamSlotsReadOnly() {
		if slot.Symbol == 0 || slot.TypeAnnotation == nil {
			continue
		}
		if t := resolve(slot.TypeAnnotation, baseScope); t != nil {
			facts.DeclaredTypes[slot.Symbol] = t
		}
	}
	return facts
}
