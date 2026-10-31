// Package nestedinfer processes nested function definitions during type analysis.
//
// Nested functions (closures) require special handling because they:
//   - Capture variables from enclosing scopes
//   - May be called before their definition is reached
//   - Can form mutual recursion with siblings
//
// The [Processor] gathers nested function definitions from a parent graph,
// groups them by scope, and analyzes each group with the appropriate parent
// context. This includes:
//   - Computing enriched parent scopes with sibling function types
//   - Propagating captured field assignments back to parent scopes
//   - Recursively processing nested functions within nested functions
//
// The processor integrates with the fixpoint loop by storing interprocedural
// facts (literal signatures, captured assignments) that may affect other
// functions in subsequent iterations.
package nestedinfer

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/infer/captured"
	"github.com/wippyai/go-lua/compiler/check/nested"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/siblings"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CheckFunc analyzes a nested function with a given parent scope.
type CheckFunc func(fn *ast.FunctionExpr, parent *scope.State)

// ResultFunc returns the analysis result for a function literal.
type ResultFunc func(fn *ast.FunctionExpr) *api.FuncResultView

// Config holds dependencies for nested processing.
type Config struct {
	Stdlib        *scope.State
	Store         api.NestedStore
	Graphs        api.GraphProvider
	Check         CheckFunc
	ResultForFunc ResultFunc
}

// Processor analyzes nested functions for a parent graph.
type Processor struct {
	stdlib           *scope.State
	store            api.NestedStore
	graphs           api.GraphProvider
	check            CheckFunc
	resultForFunc    ResultFunc
	classSelf        map[cfg.SymbolID]typ.Type
	classReceiver    map[cfg.SymbolID]typ.Type
	instanceContexts map[cfg.SymbolID]map[string]bool
}

// New creates a nested processor.
func New(cfg Config) *Processor {
	return &Processor{
		stdlib:        cfg.Stdlib,
		store:         cfg.Store,
		graphs:        cfg.Graphs,
		check:         cfg.Check,
		resultForFunc: cfg.ResultForFunc,
	}
}

// ProcessNestedFunctions analyzes all nested function definitions within a parent graph.
func (p *Processor) ProcessNestedFunctions(graph *cfg.Graph, parentResult *api.FuncResultView) {
	if parentResult == nil {
		return
	}

	scopes := parentResult.Scopes
	if scopes == nil {
		return
	}

	// Gather nested function definitions.
	gathered := nested.GatherChildren(graph, scopes, p.stdlib)
	if len(gathered) == 0 {
		return
	}

	// Find the parent function for this graph.
	parentFunc := (*ast.FunctionExpr)(nil)
	if p.store != nil {
		parentFunc = p.store.FuncForGraph(graph)
	}
	p.bindClassTables(graph, gathered, parentResult)

	// Group by scope and build FuncInfo entries.
	groups := p.groupNestedByScope(gathered)

	// Process each scope group.
	for _, group := range groups {
		p.processNestedGroup(graph, scopes, group, parentResult, parentFunc)
	}
}

// bindClassTables gives each class table of graph one recursion snapshot for
// this parent analysis. A class table is a table that nested functions
// are stored into; every nested function observes it through the snapshot,
// both as self and as a captured upvalue, so method signatures and captured
// views all refer to the table's stable recursion identity.
func (p *Processor) bindClassTables(graph *cfg.Graph, children []nested.Child, parentResult *api.FuncResultView) {
	p.classSelf = make(map[cfg.SymbolID]typ.Type)
	p.classReceiver = make(map[cfg.SymbolID]typ.Type)
	p.instanceContexts = make(map[cfg.SymbolID]map[string]bool)
	if p.store == nil || graph == nil || graph.Bindings() == nil || parentResult == nil {
		return
	}
	p.instanceContexts = p.instanceFieldContexts(graph, children, parentResult.NarrowSynth)
	ordered := append([]nested.Child(nil), children...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].NF.Point < ordered[j].NF.Point })
	for _, child := range ordered {
		info := &nested.FuncInfo{Child: child}
		if info.FuncDef != nil && (info.FuncDef.TargetKind == cfg.FuncDefField || info.FuncDef.TargetKind == cfg.FuncDefMethod) && len(info.FuncDef.TargetPath.Segments) != 1 {
			continue
		}
		sym := nested.MethodOwner(graph, info.NF.Func, info.FuncDef, info.NF.Point)
		if sym == 0 || p.classSelf[sym] != nil || hasDeclaredMethodSelf(info) {
			continue
		}
		body := p.classTableType(graph, sym, parentResult)
		if body == nil {
			continue
		}
		p.classSelf[sym] = p.store.BindClassSelf(graph, info.NF.Point, sym, graph.NameOf(sym), body)
		if nested.ReceiverComplete(p.moduleGraphs(), sym) {
			p.classReceiver[sym] = p.classSelf[sym]
		} else {
			p.classReceiver[sym] = p.store.BindClassReceiver(graph, info.NF.Point, sym, graph.NameOf(sym), body)
		}
	}
}

// moduleGraphs lists the graphs of the analyzed module in a stable order.
func (p *Processor) moduleGraphs() []*cfg.Graph {
	if p.store == nil {
		return nil
	}
	graphs := p.store.Graphs()
	ids := make([]uint64, 0, len(graphs))
	for id := range graphs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]*cfg.Graph, 0, len(ids))
	for _, id := range ids {
		if g := graphs[id]; g != nil {
			out = append(out, g)
		}
	}
	return out
}

// classTableType is the type of class table sym for its nested functions:
// its solved type where the parent graph completes, after every field the
// parent assigns to it. Nested functions run once the parent has populated
// the table, so they observe that state.
func (p *Processor) classTableType(graph *cfg.Graph, sym cfg.SymbolID, parentResult *api.FuncResultView) typ.Type {
	if parentResult.FlowSolution == nil {
		return nil
	}
	body := parentResult.FlowSolution.TypeAt(graph.Exit(), constraint.Path{Symbol: sym})
	if body == nil || typ.IsAny(body) || typ.IsUnknown(body) {
		return nil
	}
	return body
}

func hasDeclaredMethodSelf(info *nested.FuncInfo) bool {
	if info == nil || info.FuncDef == nil || info.FuncDef.ReceiverName == "" || info.DefScope == nil {
		return false
	}
	named, ok := info.DefScope.LookupValueType(info.FuncDef.ReceiverName)
	return ok && named != nil
}

// nestedGroup holds a group of functions sharing the same parent scope.
type nestedGroup struct {
	Hash     uint64
	Funcs    []*nested.FuncInfo
	MinPoint cfg.Point
}

// groupNestedByScope groups nested function children by their defining scope hash.
func (p *Processor) groupNestedByScope(gathered []nested.Child) []*nestedGroup {
	scopeGroups := make(map[uint64][]*nested.FuncInfo)

	for i := range gathered {
		child := &gathered[i]
		scopeHash := child.DefScope.GroupHash()

		info := &nested.FuncInfo{Child: *child}

		scopeGroups[scopeHash] = append(scopeGroups[scopeHash], info)
	}

	// Collect groups in deterministic order.
	groups := make([]*nestedGroup, 0, len(scopeGroups))
	for scopeHash, funcs := range scopeGroups {
		if len(funcs) == 0 {
			continue
		}
		sort.SliceStable(funcs, func(i, j int) bool {
			return funcs[i].NF.Point < funcs[j].NF.Point
		})
		groups = append(groups, &nestedGroup{
			Hash:     scopeHash,
			Funcs:    funcs,
			MinPoint: funcs[0].NF.Point,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].MinPoint != groups[j].MinPoint {
			return groups[i].MinPoint < groups[j].MinPoint
		}
		return groups[i].Hash < groups[j].Hash
	})

	return groups
}

// processNestedGroup processes all functions in a scope group.
func (p *Processor) processNestedGroup(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	group *nestedGroup,
	parentResult *api.FuncResultView,
	parentFunc *ast.FunctionExpr,
) {
	// Build sibling types for this group.
	siblingTypes := p.buildSiblingTypesForGroup(graph, scopes, group.Hash, group.Funcs, parentResult)
	if siblingTypes == nil {
		siblingTypes = make(map[cfg.SymbolID]typ.Type)
	}

	// Process each function in the group.
	for _, info := range group.Funcs {
		p.processNestedFunction(graph, scopes, info, siblingTypes, parentResult, parentFunc)
	}
}

// processNestedFunction analyzes a single nested function.
func (p *Processor) processNestedFunction(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	info *nested.FuncInfo,
	siblingTypes map[cfg.SymbolID]typ.Type,
	parentResult *api.FuncResultView,
	parentFunc *ast.FunctionExpr,
) {
	baseParentScope := scopes[info.NF.Point]
	if baseParentScope == nil {
		baseParentScope = p.stdlib
	}

	parentScope := baseParentScope
	var nestedGraph *cfg.Graph
	if p.graphs != nil {
		nestedGraph = p.graphs.GetOrBuildCFG(info.NF.Func)
	}

	captureContext := captured.ParentContext{
		ParentGraph: graph,
		ChildGraph:  nestedGraph,
		Point:       info.NF.Point,
		Facts:       parentResult.Facts,
		Solution:    parentResult.FlowSolution,
		Classes:     p.store,
		Mutations:   p.store,
	}
	if parentResult.NarrowSynth != nil {
		captureContext.TypeOf = parentResult.NarrowSynth.TypeOf
	}
	capturedTypes := captured.Types(captureContext)
	if selfType := p.methodSelfType(graph, info); selfType != nil {
		parentScope = parentScope.WithSelf(selfType).WithLocalName("self")
	}

	if nestedGraph != nil && len(capturedTypes) > 0 && p.store != nil {
		p.persistCapturedTypesForNestedGraph(nestedGraph, parentScope, capturedTypes)
	}

	// Check the function.
	if p.check != nil {
		p.check(info.NF.Func, parentScope)
	}

	// Get the result for constructor detection and sibling updates.
	result := (*api.FuncResultView)(nil)
	if p.resultForFunc != nil {
		result = p.resultForFunc(info.NF.Func)
	}
	if result == nil {
		return
	}

	// Constructors and methods contribute to the same instance field facts.
	if result.Graph != nil && p.store != nil {
		classSym, selfSym := nested.DetectConstructorPattern(result.Graph, graph, info.NF.Func, info.FuncDef)
		isConstructor := classSym != 0
		if classSym == 0 && !hasDeclaredMethodSelf(info) && p.methodSelfType(graph, info) != nil {
			classSym = nested.MethodOwner(graph, info.NF.Func, info.FuncDef, info.NF.Point)
			for _, slot := range result.Graph.ParamSlotsReadOnly() {
				if slot.IsImplicitSelf || slot.Name == "self" {
					selfSym = slot.Symbol
					break
				}
			}
		}
		if classSym != 0 && selfSym != 0 {
			previous := p.store.LookupConstructorFields(classSym)
			context := make(map[string]typ.Type)
			for name, eligible := range p.instanceContexts[classSym] {
				if eligible {
					context[name] = previous[name]
				}
			}
			fields := nested.CollectInstanceFields(result.Graph, selfSym, result.NarrowSynth, context)
			if !isConstructor {
				for name := range fields {
					if !p.instanceContexts[classSym][name] {
						delete(fields, name)
					}
				}
			}
			if len(fields) > 0 {
				p.store.StoreConstructorFields(classSym, fields)
			}
		}
	}

	// Update sibling types with the fully-inferred function type.
	if info.IsLocal && info.FuncSym != 0 && result.NarrowSynth != nil {
		if inferredType := result.NarrowSynth.FunctionType(info.NF.Func, parentScope); inferredType != nil {
			siblingTypes[info.FuncSym] = returns.MergeFunctionFactType(siblingTypes[info.FuncSym], inferredType)
		}
	}
}

// methodSelfType is the type of self in a method: the declared type named
// by the receiver of `function T:m`, or the snapshot of the class table the
// method is stored into.
func (p *Processor) methodSelfType(graph *cfg.Graph, info *nested.FuncInfo) typ.Type {
	isMethod := info.FuncDef != nil && info.FuncDef.IsMethod
	if !isMethod && !phasecore.HasUnannotatedSelfParam(info.NF.Func, graph.Bindings()) {
		return nil
	}
	// The receiver is any table that uses the method table, so the method
	// table's own fields describe it partially.
	if isMethod && info.FuncDef.ReceiverName != "" && info.DefScope != nil {
		if named, ok := info.DefScope.LookupValueType(info.FuncDef.ReceiverName); ok && named != nil {
			return typ.PartialView(nested.NormalizeMethodSelfType(named))
		}
	}
	if info.FuncDef != nil && (info.FuncDef.TargetKind == cfg.FuncDefField || info.FuncDef.TargetKind == cfg.FuncDefMethod) && len(info.FuncDef.TargetPath.Segments) != 1 {
		return nil
	}
	if sym := nested.MethodOwner(graph, info.NF.Func, info.FuncDef, info.NF.Point); sym != 0 {
		return p.classReceiver[sym]
	}
	return nil
}

func (p *Processor) persistCapturedTypesForNestedGraph(
	nestedGraph *cfg.Graph,
	parentScope *scope.State,
	capturedTypes map[cfg.SymbolID]typ.Type,
) {
	if p.store == nil || nestedGraph == nil || parentScope == nil || len(capturedTypes) == 0 {
		return
	}
	if p.store.GraphParentHashOf(nestedGraph.ID()) == 0 {
		if setter, ok := p.store.(interface {
			SetGraphParentHash(graphID, parentHash uint64)
		}); ok {
			setter.SetGraphParentHash(nestedGraph.ID(), parentScope.Hash())
		}
	}
	key, ok := p.store.GraphKeyFor(nestedGraph, parentScope)
	if !ok {
		return
	}
	nextCaptured := make(api.CapturedTypes, len(capturedTypes))
	for _, sym := range cfg.SortedSymbolIDs(capturedTypes) {
		t := capturedTypes[sym]
		if sym == 0 || t == nil {
			continue
		}
		nextCaptured[sym] = t
	}
	if len(nextCaptured) == 0 {
		return
	}
	p.store.UpdateInterprocFactsNext(key, func(facts *api.Facts) {
		facts.CapturedTypes = returns.WidenCapturedTypes(facts.CapturedTypes, nextCaptured)
	})
}

// buildSiblingTypesForGroup computes sibling function types for a scope group.
func (p *Processor) buildSiblingTypesForGroup(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	groupHash uint64,
	funcs []*nested.FuncInfo,
	parentResult *api.FuncResultView,
) map[cfg.SymbolID]typ.Type {
	if p.store == nil || graph == nil || len(funcs) == 0 {
		return nil
	}

	entries := make([]siblings.FuncEntry, len(funcs))
	for i, info := range funcs {
		entries[i] = siblings.FuncEntry{
			Func:    info.NF.Func,
			Point:   info.NF.Point,
			Symbol:  info.FuncSym,
			IsLocal: info.IsLocal,
		}
	}

	bindings := graph.Bindings()

	buildCfg := siblings.BuildConfig{
		Funcs:     entries,
		GroupHash: groupHash,
	}

	// Use canonical local function types (signatures + param hints + return summaries).
	var parentScope *scope.State
	if len(funcs) > 0 {
		parentScope = funcs[0].DefScope
	}
	buildCfg.FuncTypes = p.store.GetLocalFuncTypesSnapshot(graph, parentScope)

	buildCfg.Services = siblings.BuildServicesFuncs{
		CapturedSymbolsFn: func(fn *ast.FunctionExpr) []cfg.SymbolID {
			if bindings == nil {
				return nil
			}
			return bindings.CapturedSymbols(fn)
		},
		TypeAtPointFn: func(point cfg.Point, sym cfg.SymbolID) typ.Type {
			if parentResult == nil || parentResult.Facts == nil {
				return nil
			}
			ref := parentResult.Facts.RefinedAt(point, sym)
			decl := parentResult.Facts.DeclaredAt(point, sym)

			var chosen typ.Type
			if ref.Type != nil && !typ.IsSoft(ref.Type, typ.SoftAnnotationPolicy) {
				chosen = ref.Type
			} else if decl.Type != nil && !typ.IsSoft(decl.Type, typ.SoftAnnotationPolicy) {
				chosen = decl.Type
			} else if ref.State == flow.StateResolved && ref.Type != nil {
				chosen = ref.Type
			} else if decl.State == flow.StateResolved && decl.Type != nil {
				chosen = decl.Type
			} else {
				return nil
			}

			return chosen
		},
	}

	return siblings.Build(buildCfg)
}

// instanceFieldContexts proves that an inferred receiver slot starts in fresh
// constructor literals and every replacement is fresh or has a declared domain.
// A shared unannotated source keeps the original constructor-only inference.
func (p *Processor) instanceFieldContexts(parent *cfg.Graph, children []nested.Child, synth api.BaseSynth) map[cfg.SymbolID]map[string]bool {
	result := make(map[cfg.SymbolID]map[string]bool)
	freshInitializers := make(map[cfg.SymbolID]map[string]bool)
	blocked := make(map[cfg.SymbolID]bool)
	if p.graphs == nil {
		return result
	}
	for _, child := range children {
		g := p.graphs.GetOrBuildCFG(child.NF.Func)
		if g == nil {
			continue
		}
		class, self := nested.DetectConstructorPattern(g, parent, child.NF.Func, child.FuncDef)
		constructor := class != 0
		if !constructor {
			info := &nested.FuncInfo{Child: child}
			if hasDeclaredMethodSelf(info) || child.FuncDef == nil || !child.FuncDef.IsMethod {
				continue
			}
			class = nested.MethodOwner(parent, child.NF.Func, child.FuncDef, child.NF.Point)
			for _, slot := range g.ParamSlotsReadOnly() {
				if slot.IsImplicitSelf {
					self = slot.Symbol
					break
				}
			}
		}
		if class == 0 || self == 0 {
			continue
		}
		if result[class] == nil {
			result[class] = make(map[string]bool)
			freshInitializers[class] = make(map[string]bool)
		}
		declared := make(map[cfg.SymbolID]bool)
		for _, slot := range g.ParamSlotsReadOnly() {
			if slot.TypeAnnotation != nil && synth != nil {
				declared[slot.Symbol] = unwrap.IsContainer(unwrap.Optional(synth.ResolveType(slot.TypeAnnotation, child.DefScope)))
			}
		}
		g.EachAssign(func(_ cfg.Point, assignment *cfg.AssignInfo) {
			assignment.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
				if target.BaseSymbol != self {
					return
				}
				if target.Kind == cfg.TargetField && len(target.FieldPath) > 1 {
					return // A member write does not replace the containing slot.
				}
				if target.Kind == cfg.TargetIndex && len(flowpath.FromExprWithBindings(target.Base, nil, g.Bindings()).Segments) > 0 {
					return
				}
				if target.Kind != cfg.TargetField || len(target.FieldPath) != 1 {
					blocked[class] = true
					return
				}
				name := target.FieldPath[0]
				_, fresh := source.(*ast.TableExpr)
				safe := fresh
				if !constructor {
					if ident, ok := source.(*ast.IdentExpr); ok && g.Bindings() != nil {
						if sym, ok := g.Bindings().SymbolOf(ident); ok {
							safe = declared[sym]
						}
					}
				}
				if existing, seen := result[class][name]; !seen {
					result[class][name] = safe
				} else {
					result[class][name] = existing && safe
				}
				if constructor {
					freshInitializers[class][name] = fresh
				}
			})
		})
	}
	for class, fields := range result {
		for name := range fields {
			fields[name] = fields[name] && freshInitializers[class][name] && !blocked[class]
		}
	}
	return result
}
