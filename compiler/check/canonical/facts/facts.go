// Package facts derives module-level semantic facts consumed by the canonical
// abstract interpreter.
//
// These facts are not diagnostics and not driver policy. They are finite,
// monotone evidence computed after name resolution and before the product-flow
// solve so transfers can consume them uniformly. Keeping them here prevents the
// canonical driver from accumulating one-off graph walks for each precision
// feature.
package facts

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

// Program is the resolved module view needed to derive facts.
type Program struct {
	Refs []ref.FuncRef
	// ModuleAliases are manifest-backed require() alias exports, already lowered
	// from module path strings to stable alias symbols and export types by the
	// driver boundary.
	ModuleAliases []topology.ModuleAlias

	Graph func(ref.FuncRef) *cfg.Graph
	// Evidence returns the graph's already-extracted flow evidence.
	Evidence func(*cfg.Graph) api.FlowEvidence
	// ResolveCallee resolves a call in g to a module function, if any.
	ResolveCallee func(g *cfg.Graph, call *ast.FuncCallExpr) (ref.FuncRef, bool)
	// RefForFuncSymbol resolves a symbol denoting a module-local function literal
	// to its stable canonical ref. Function identity is normalized by program
	// discovery; facts never ask the driver to resolve AST pointers.
	RefForFuncSymbol func(cfg.SymbolID) (ref.FuncRef, bool)
	// DeclaredReturnTypes returns ref's resolved source-declared return vector.
	// Expected function-literal entry facts use this declaration-only boundary; no
	// inferred summary returns may enter pre-transfer facts.
	DeclaredReturnTypes func(ref.FuncRef) []typ.Type
	// NestedFuncRefs returns the refs of functions directly nested in ref.
	NestedFuncRefs func(ref.FuncRef) []ref.FuncRef
	// CallbackOverlaysForRef returns declared/source callback environment overlays
	// for a module-local callee. Facts consume the normalized overlay carrier rather
	// than a full function signature so immutable fact extraction cannot read
	// inferred return summaries.
	CallbackOverlaysForRef func(ref.FuncRef) callbackenv.Overlays
	// CalleeCallbackOverlays returns imported/captured callback environment
	// overlays for a call when it does not resolve to a module-local ref.
	CalleeCallbackOverlays func(g *cfg.Graph, call *ast.FuncCallExpr) callbackenv.Overlays
	// TypeByName resolves a source type name in the module scope. Type-check guard
	// facts use it for `T:is(x)` assignments; facts owns the assignment scan while
	// the driver owns the module-scope service.
	TypeByName func(name string) typ.Type
	// SetupExprType resolves the pre-solve, declaration-only type of an expression
	// assigned into a temporary callback global (`_G.name = expr`) while inferring
	// callback environment overlays. It must not read solved summaries.
	SetupExprType func(g *cfg.Graph, expr ast.Expr, point cfg.Point) typ.Type
}

// Module is the fact set derived for one canonical module run.
type Module struct {
	noReturn []ref.FuncRef

	moduleAliases      []topology.ModuleAlias
	predicateByFunc    []guard.PredicateFunction
	predicateByCondSym []predicateResultRow
	callbackEnv        []callbackEnvRow
	keysCollectors     []keysCollectorRow
	typeChecks         []typeCheckBindRow
	functionBindings   []topology.FunctionBinding
	fieldFunctions     []topology.FieldFunction
	entrySeeds         []entrySeedRow
	metatableIndexes   []metatable.Index
	methodReceivers    []methodReceiverEntry
	prototypeMethods   []metatable.PrototypeMethod
	setMetatableSites  []setMetatableSiteEntry
}

type predicateResultRow struct {
	FuncRef ref.FuncRef
	Result  guard.PredicateResult
}

type typeCheckBindRow struct {
	FuncRef ref.FuncRef
	Bind    guard.TypeCheckBind
}

type callbackEnvRow struct {
	FuncRef ref.FuncRef
	Binding callbackenv.GlobalBinding
}

type keysCollectorRow struct {
	FuncRef ref.FuncRef
	Info    keyscoll.KeysCollector
}

// FunctionEntrySeed records a declaration-time entry value for one function
// parameter slot.
type FunctionEntrySeed struct {
	Slot int
	Type typ.Type
}

type entrySeedRow struct {
	FuncRef ref.FuncRef
	Seed    FunctionEntrySeed
	Order   int
}

type methodReceiverEntry struct {
	FuncRef ref.FuncRef
	Info    metatable.MethodReceiver
}

type setMetatableSiteEntry struct {
	FuncRef ref.FuncRef
	Info    metatable.SetMetatableSite
}

// BuildPreTransfer derives the finite graph/signature facts needed to construct
// transfers. These facts do not depend on transfer-discovered body effects, so
// they can be computed before transfer.New and supplied as immutable config.
func BuildPreTransfer(p Program) Module {
	m := Module{
		moduleAliases:   append([]topology.ModuleAlias(nil), p.ModuleAliases...),
		predicateByFunc: collectPredicateFacts(p),
	}
	m.metatableIndexes = collectMetatableIndexes(p)
	m.functionBindings = collectFunctionBindings(p)
	m.fieldFunctions = collectFieldFunctions(p)
	m.entrySeeds = append(m.entrySeeds, collectEntrySelfSeeds(p)...)
	m.entrySeeds = append(m.entrySeeds, collectExpectedFunctionEntrySeeds(p)...)
	m.methodReceivers = collectMethodReceivers(p)
	m.prototypeMethods = collectPrototypeMethods(m.fieldFunctions)
	m.setMetatableSites = collectSetMetatableSites(p, m.metatableIndexes)
	m.typeChecks = collectTypeCheckBinds(p)
	if len(m.predicateByFunc) > 0 {
		for _, ref := range p.Refs {
			g := graphOf(p, ref)
			if g == nil {
				continue
			}
			for _, guard := range predicateCondSymBinds(ref, g, m.predicateByFunc) {
				m.predicateByCondSym = append(m.predicateByCondSym, guard)
			}
		}
	}
	sortModuleFacts(&m)
	return m
}

// Build derives the complete finite fact set for p.
func Build(p Program) Module {
	m := BuildPreTransfer(p)
	m.noReturn = computeNoReturn(p)
	m.callbackEnv = collectCallbackEnv(p)
	m.keysCollectors = collectKeysCollectors(p)
	sortModuleFacts(&m)
	return m
}

func graphOf(p Program, ref ref.FuncRef) *cfg.Graph {
	if p.Graph == nil {
		return nil
	}
	return p.Graph(ref)
}

// ModuleAliasType returns the imported module export type for alias symbol sym.
func (m Module) ModuleAliasType(sym cfg.SymbolID) (typ.Type, bool) {
	if sym == 0 || len(m.moduleAliases) == 0 {
		return nil, false
	}
	idx, ok := slices.BinarySearchFunc(m.moduleAliases, topology.ModuleAlias{Symbol: sym}, compareModuleAliasEntry)
	if !ok {
		return nil, false
	}
	t := m.moduleAliases[idx].Type
	if t == nil || typ.IsAbsentOrUnknown(t) {
		return nil, false
	}
	return t, true
}

// HasNoReturn reports whether r is proven never to return normally.
func (m Module) HasNoReturn(r ref.FuncRef) bool {
	_, ok := slices.BinarySearchFunc(m.noReturn, r, compareFuncRef)
	return ok
}

// PredicateFacts returns a copy of the sorted predicate-function facts.
func (m Module) PredicateFacts() []guard.PredicateFunction {
	return append([]guard.PredicateFunction(nil), m.predicateByFunc...)
}

// PredicateGuards returns a copy of the sorted assigned predicate guards for r.
func (m Module) PredicateGuards(r ref.FuncRef) []guard.PredicateResult {
	if len(m.predicateByCondSym) == 0 {
		return nil
	}
	start, _ := slices.BinarySearchFunc(m.predicateByCondSym, predicateResultRow{FuncRef: r}, comparePredicateResultRowRefOnly)
	var out []guard.PredicateResult
	for i := start; i < len(m.predicateByCondSym) && compareFuncRef(m.predicateByCondSym[i].FuncRef, r) == 0; i++ {
		out = append(out, m.predicateByCondSym[i].Result)
	}
	return out
}

// KeysCollector returns r's keys-collector fact, if any.
func (m Module) KeysCollector(r ref.FuncRef) (keyscoll.KeysCollector, bool) {
	if len(m.keysCollectors) == 0 {
		return keyscoll.KeysCollector{}, false
	}
	idx, ok := slices.BinarySearchFunc(m.keysCollectors, keysCollectorRow{FuncRef: r}, compareKeysCollectorRowRefOnly)
	if !ok {
		return keyscoll.KeysCollector{}, false
	}
	return m.keysCollectors[idx].Info, true
}

// TypeChecks returns a copy of r's type-check guard binds.
func (m Module) TypeChecks(r ref.FuncRef) []guard.TypeCheckBind {
	if len(m.typeChecks) == 0 {
		return nil
	}
	start, _ := slices.BinarySearchFunc(m.typeChecks, typeCheckBindRow{FuncRef: r}, compareTypeCheckBindEntryRefOnly)
	var out []guard.TypeCheckBind
	for i := start; i < len(m.typeChecks) && compareFuncRef(m.typeChecks[i].FuncRef, r) == 0; i++ {
		out = append(out, cloneTypeCheckBind(m.typeChecks[i].Bind))
	}
	return out
}

// FunctionRef returns the module-local function bound to sym, if any.
func (m Module) FunctionRef(sym cfg.SymbolID) (ref.FuncRef, bool) {
	if sym == 0 || len(m.functionBindings) == 0 {
		return ref.FuncRef{}, false
	}
	idx, ok := slices.BinarySearchFunc(m.functionBindings, topology.FunctionBinding{Symbol: sym}, compareFunctionBindingEntrySymbolOnly)
	if !ok {
		return ref.FuncRef{}, false
	}
	return m.functionBindings[idx].FuncRef, true
}

// FunctionBindings returns a copy of the sorted symbol -> FuncRef facts.
func (m Module) FunctionBindings() []topology.FunctionBinding {
	return append([]topology.FunctionBinding(nil), m.functionBindings...)
}

// FunctionBindingTypes projects normalized function-binding facts through a
// caller-provided signature resolver. The facts package owns the symbol -> ref
// carrier and deterministic iteration; callers own only how a FuncRef becomes a
// typ.Type at their boundary.
func (m Module) FunctionBindingTypes(signatureFor func(ref.FuncRef) typ.Type) map[cfg.SymbolID]typ.Type {
	if len(m.functionBindings) == 0 || signatureFor == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]typ.Type, len(m.functionBindings))
	for _, binding := range m.functionBindings {
		if binding.Symbol == 0 {
			continue
		}
		sig := signatureFor(binding.FuncRef)
		if sig == nil || typ.IsAbsentOrUnknown(sig) {
			continue
		}
		out[binding.Symbol] = sig
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FieldFuncRef returns the function ref statically stored at container.field.
func (m Module) FieldFuncRef(container cfg.SymbolID, field fieldkey.Key) (ref.FuncRef, bool) {
	if container == 0 || field == (fieldkey.Key{}) || len(m.fieldFunctions) == 0 {
		return ref.FuncRef{}, false
	}
	key := topology.FieldFunction{ContainerSym: container, Field: field}
	start, ok := slices.BinarySearchFunc(m.fieldFunctions, key, compareFieldFuncEntryKeyOnly)
	if !ok {
		return ref.FuncRef{}, false
	}
	return m.fieldFunctions[start].FuncRef, true
}

// FunctionEntrySeeds returns declaration-context entry seeds for r.
func (m Module) FunctionEntrySeeds(r ref.FuncRef) []FunctionEntrySeed {
	if len(m.entrySeeds) == 0 {
		return nil
	}
	start, _ := slices.BinarySearchFunc(m.entrySeeds, entrySeedRow{FuncRef: r}, compareEntrySeedRowRefOnly)
	var out []FunctionEntrySeed
	for i := start; i < len(m.entrySeeds) && compareFuncRef(m.entrySeeds[i].FuncRef, r) == 0; i++ {
		seed := m.entrySeeds[i].Seed
		out = append(out, FunctionEntrySeed{Slot: seed.Slot, Type: seed.Type})
	}
	return out
}

// CallbackEnv returns a copy of r's callback-scoped global bindings.
func (m Module) CallbackEnv(r ref.FuncRef) []callbackenv.GlobalBinding {
	if len(m.callbackEnv) == 0 {
		return nil
	}
	start, _ := slices.BinarySearchFunc(m.callbackEnv, callbackEnvRow{FuncRef: r}, compareCallbackEnvRowRefOnly)
	var out []callbackenv.GlobalBinding
	for i := start; i < len(m.callbackEnv) && compareFuncRef(m.callbackEnv[i].FuncRef, r) == 0; i++ {
		out = append(out, m.callbackEnv[i].Binding)
	}
	return out
}

// MetatableIndexes returns a copy of the sorted metatable -> prototype facts.
func (m Module) MetatableIndexes() []metatable.Index {
	return append([]metatable.Index(nil), m.metatableIndexes...)
}

// PrototypeForMetatable returns the prototype symbol named by mt's __index fact.
func (m Module) PrototypeForMetatable(mt cfg.SymbolID) (cfg.SymbolID, bool) {
	idx, ok := slices.BinarySearchFunc(m.metatableIndexes, mt, func(e metatable.Index, target cfg.SymbolID) int {
		return cmp.Compare(e.MetatableSym, target)
	})
	if !ok {
		return 0, false
	}
	return m.metatableIndexes[idx].PrototypeSym, true
}

// MethodReceivers returns a copy of r's method receiver facts.
func (m Module) MethodReceivers(r ref.FuncRef) []metatable.MethodReceiver {
	if len(m.methodReceivers) == 0 {
		return nil
	}
	start, _ := slices.BinarySearchFunc(m.methodReceivers, methodReceiverEntry{FuncRef: r}, compareMethodReceiverEntryRefOnly)
	var out []metatable.MethodReceiver
	for i := start; i < len(m.methodReceivers) && compareFuncRef(m.methodReceivers[i].FuncRef, r) == 0; i++ {
		out = append(out, m.methodReceivers[i].Info)
	}
	return out
}

// PrototypeMethods returns a copy of the sorted prototype-method topology facts.
func (m Module) PrototypeMethods() []metatable.PrototypeMethod {
	return append([]metatable.PrototypeMethod(nil), m.prototypeMethods...)
}

// SetMetatableSites returns a copy of r's setmetatable receiver sites.
func (m Module) SetMetatableSites(r ref.FuncRef) []metatable.SetMetatableSite {
	if len(m.setMetatableSites) == 0 {
		return nil
	}
	start, _ := slices.BinarySearchFunc(m.setMetatableSites, setMetatableSiteEntry{FuncRef: r}, compareSetMetatableSiteEntryRefOnly)
	var out []metatable.SetMetatableSite
	for i := start; i < len(m.setMetatableSites) && compareFuncRef(m.setMetatableSites[i].FuncRef, r) == 0; i++ {
		out = append(out, m.setMetatableSites[i].Info)
	}
	return out
}

func sortModuleFacts(m *Module) {
	slices.SortFunc(m.moduleAliases, compareModuleAliasEntry)
	m.moduleAliases = compactModuleAliasEntries(m.moduleAliases)
	slices.SortFunc(m.noReturn, compareFuncRef)
	m.noReturn = compactFuncRefs(m.noReturn)
	slices.SortFunc(m.predicateByFunc, comparePredicateFuncEntry)
	m.predicateByFunc = compactPredicateFuncEntries(m.predicateByFunc)
	slices.SortFunc(m.predicateByCondSym, comparePredicateResultRow)
	m.predicateByCondSym = compactPredicateResultRows(m.predicateByCondSym)
	slices.SortFunc(m.callbackEnv, compareCallbackEnvRow)
	m.callbackEnv = compactCallbackEnvRows(m.callbackEnv)
	slices.SortFunc(m.keysCollectors, compareKeysCollectorRow)
	m.keysCollectors = compactKeysCollectorRows(m.keysCollectors)
	slices.SortFunc(m.typeChecks, compareTypeCheckBindEntry)
	m.typeChecks = compactTypeCheckBindEntries(m.typeChecks)
	slices.SortFunc(m.functionBindings, compareFunctionBindingEntry)
	m.functionBindings = compactFunctionBindingEntries(m.functionBindings)
	slices.SortFunc(m.fieldFunctions, compareFieldFuncEntry)
	m.fieldFunctions = compactFieldFuncEntries(m.fieldFunctions)
	slices.SortFunc(m.entrySeeds, compareEntrySeedRow)
	slices.SortFunc(m.metatableIndexes, compareMetatableIndexEntry)
	m.metatableIndexes = compactMetatableIndexEntries(m.metatableIndexes)
	slices.SortFunc(m.methodReceivers, compareMethodReceiverEntry)
	m.methodReceivers = compactMethodReceiverEntries(m.methodReceivers)
	slices.SortFunc(m.prototypeMethods, comparePrototypeMethodEntry)
	m.prototypeMethods = compactPrototypeMethodEntries(m.prototypeMethods)
	slices.SortFunc(m.setMetatableSites, compareSetMetatableSiteEntry)
	m.setMetatableSites = compactSetMetatableSiteEntries(m.setMetatableSites)
}

func compareModuleAliasEntry(a, b topology.ModuleAlias) int {
	return cmp.Compare(a.Symbol, b.Symbol)
}

func compareFuncRef(a, b ref.FuncRef) int {
	return ref.CompareFuncRef(a, b)
}

func comparePredicateFuncEntry(a, b guard.PredicateFunction) int {
	return cmp.Compare(a.FuncSym, b.FuncSym)
}

func comparePredicateResultRow(a, b predicateResultRow) int {
	if c := compareFuncRef(a.FuncRef, b.FuncRef); c != 0 {
		return c
	}
	return cmp.Compare(a.Result.CondSym, b.Result.CondSym)
}

func comparePredicateResultRowRefOnly(a, b predicateResultRow) int {
	return compareFuncRef(a.FuncRef, b.FuncRef)
}

func compareCallbackEnvRow(a, b callbackEnvRow) int {
	if c := compareFuncRef(a.FuncRef, b.FuncRef); c != 0 {
		return c
	}
	return cmp.Compare(a.Binding.Symbol, b.Binding.Symbol)
}

func compareCallbackEnvRowRefOnly(a, b callbackEnvRow) int {
	return compareFuncRef(a.FuncRef, b.FuncRef)
}

func compareKeysCollectorRow(a, b keysCollectorRow) int {
	if c := compareFuncRef(a.FuncRef, b.FuncRef); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Info.ParamIndex, b.Info.ParamIndex); c != 0 {
		return c
	}
	return cmp.Compare(a.Info.ReturnIndex, b.Info.ReturnIndex)
}

func compareKeysCollectorRowRefOnly(a, b keysCollectorRow) int {
	return compareFuncRef(a.FuncRef, b.FuncRef)
}

func compareTypeCheckBindEntry(a, b typeCheckBindRow) int {
	if c := compareFuncRef(a.FuncRef, b.FuncRef); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Bind.ErrSym, b.Bind.ErrSym); c != 0 {
		return c
	}
	if c := cmp.Compare(typeHash(a.Bind.Type), typeHash(b.Bind.Type)); c != 0 {
		return c
	}
	return compareSymbolIDs(a.Bind.NarrowSyms, b.Bind.NarrowSyms)
}

func compareTypeCheckBindEntryRefOnly(a, b typeCheckBindRow) int {
	return compareFuncRef(a.FuncRef, b.FuncRef)
}

func compareFunctionBindingEntry(a, b topology.FunctionBinding) int {
	if c := cmp.Compare(a.Symbol, b.Symbol); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Order, b.Order); c != 0 {
		return c
	}
	return compareFuncRef(a.FuncRef, b.FuncRef)
}

func compareFunctionBindingEntrySymbolOnly(a, b topology.FunctionBinding) int {
	return cmp.Compare(a.Symbol, b.Symbol)
}

func compareFieldFuncEntry(a, b topology.FieldFunction) int {
	if c := compareFieldFuncEntryKeyOnly(a, b); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Order, b.Order); c != 0 {
		return c
	}
	return compareFuncRef(a.FuncRef, b.FuncRef)
}

func compareFieldFuncEntryKeyOnly(a, b topology.FieldFunction) int {
	if c := cmp.Compare(a.ContainerSym, b.ContainerSym); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Field.Kind, b.Field.Kind); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Field.Name, b.Field.Name); c != 0 {
		return c
	}
	return cmp.Compare(a.Field.Index, b.Field.Index)
}

func compareEntrySeedRow(a, b entrySeedRow) int {
	if c := compareFuncRef(a.FuncRef, b.FuncRef); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Seed.Slot, b.Seed.Slot); c != 0 {
		return c
	}
	if c := cmp.Compare(typeHash(a.Seed.Type), typeHash(b.Seed.Type)); c != 0 {
		return c
	}
	return cmp.Compare(a.Order, b.Order)
}

func compareEntrySeedRowRefOnly(a, b entrySeedRow) int {
	return compareFuncRef(a.FuncRef, b.FuncRef)
}

func compareMetatableIndexEntry(a, b metatable.Index) int {
	if c := cmp.Compare(a.MetatableSym, b.MetatableSym); c != 0 {
		return c
	}
	return cmp.Compare(a.PrototypeSym, b.PrototypeSym)
}

func compareMethodReceiverEntry(a, b methodReceiverEntry) int {
	if c := compareFuncRef(a.FuncRef, b.FuncRef); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Info.PrototypeSym, b.Info.PrototypeSym); c != 0 {
		return c
	}
	return cmp.Compare(a.Info.SelfSlot, b.Info.SelfSlot)
}

func compareMethodReceiverEntryRefOnly(a, b methodReceiverEntry) int {
	return compareFuncRef(a.FuncRef, b.FuncRef)
}

func comparePrototypeMethodEntry(a, b metatable.PrototypeMethod) int {
	if c := cmp.Compare(a.PrototypeSym, b.PrototypeSym); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Field.Kind, b.Field.Kind); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Field.Name, b.Field.Name); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Field.Index, b.Field.Index); c != 0 {
		return c
	}
	if c := cmp.Compare(a.FuncRef.GraphID, b.FuncRef.GraphID); c != 0 {
		return c
	}
	return cmp.Compare(a.FuncRef.ParentHash, b.FuncRef.ParentHash)
}

func compareSetMetatableSiteEntry(a, b setMetatableSiteEntry) int {
	if c := compareFuncRef(a.FuncRef, b.FuncRef); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Info.Point, b.Info.Point); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Info.MetatableSym, b.Info.MetatableSym); c != 0 {
		return c
	}
	return cmp.Compare(a.Info.PrototypeSym, b.Info.PrototypeSym)
}

func compareSetMetatableSiteEntryRefOnly(a, b setMetatableSiteEntry) int {
	return compareFuncRef(a.FuncRef, b.FuncRef)
}

func compareSegments(a, b []constraint.Segment) int {
	if c := cmp.Compare(len(a), len(b)); c != 0 {
		return c
	}
	for i := range a {
		if c := cmp.Compare(a[i].Kind, b[i].Kind); c != 0 {
			return c
		}
		if c := cmp.Compare(a[i].Name, b[i].Name); c != 0 {
			return c
		}
		if c := cmp.Compare(a[i].Index, b[i].Index); c != 0 {
			return c
		}
	}
	return 0
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a && b:
		return -1
	default:
		return 1
	}
}

func compareSymbolIDs(a, b []cfg.SymbolID) int {
	if c := cmp.Compare(len(a), len(b)); c != 0 {
		return c
	}
	for i := range a {
		if c := cmp.Compare(a[i], b[i]); c != 0 {
			return c
		}
	}
	return 0
}

func typeHash(t typ.Type) uint64 {
	if t == nil {
		return 0
	}
	return t.Hash()
}

func compactModuleAliasEntries(in []topology.ModuleAlias) []topology.ModuleAlias {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev topology.ModuleAlias
	for i, e := range in {
		if i > 0 && compareModuleAliasEntry(prev, e) == 0 {
			if e.Type != nil {
				out[len(out)-1].Type = value.JoinPrecise(out[len(out)-1].Type, e.Type)
				prev = out[len(out)-1]
			}
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

func compactFuncRefs(in []ref.FuncRef) []ref.FuncRef {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev ref.FuncRef
	for i, r := range in {
		if i > 0 && compareFuncRef(prev, r) == 0 {
			continue
		}
		out = append(out, r)
		prev = r
	}
	return out
}

func compactPredicateFuncEntries(in []guard.PredicateFunction) []guard.PredicateFunction {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev cfg.SymbolID
	for i, e := range in {
		if i > 0 && e.FuncSym == prev {
			continue
		}
		out = append(out, e)
		prev = e.FuncSym
	}
	return out
}

func compactPredicateResultRows(in []predicateResultRow) []predicateResultRow {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev predicateResultRow
	for i, e := range in {
		if i > 0 && comparePredicateResultRow(prev, e) == 0 {
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

func compactCallbackEnvRows(in []callbackEnvRow) []callbackEnvRow {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev callbackEnvRow
	for i, e := range in {
		if i > 0 && compareCallbackEnvRow(prev, e) == 0 {
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

func compactKeysCollectorRows(in []keysCollectorRow) []keysCollectorRow {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev keysCollectorRow
	for i, e := range in {
		if i > 0 && compareKeysCollectorRow(prev, e) == 0 {
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

func compactTypeCheckBindEntries(in []typeCheckBindRow) []typeCheckBindRow {
	if len(in) < 2 {
		for i := range in {
			in[i].Bind = cloneTypeCheckBind(in[i].Bind)
		}
		return in
	}
	out := in[:0]
	var prev typeCheckBindRow
	for i, e := range in {
		e.Bind = cloneTypeCheckBind(e.Bind)
		if i > 0 && compareFuncRef(prev.FuncRef, e.FuncRef) == 0 && prev.Bind.ErrSym == e.Bind.ErrSym {
			dst := &out[len(out)-1]
			dst.Bind.NarrowSyms = mergeSymbolIDs(dst.Bind.NarrowSyms, e.Bind.NarrowSyms)
			if e.Bind.Type != nil {
				dst.Bind.Type = value.JoinPrecise(dst.Bind.Type, e.Bind.Type)
			}
			prev = *dst
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

func cloneTypeCheckBind(b guard.TypeCheckBind) guard.TypeCheckBind {
	if len(b.NarrowSyms) > 0 {
		b.NarrowSyms = append([]cfg.SymbolID(nil), b.NarrowSyms...)
	}
	return b
}

func mergeSymbolIDs(a, b []cfg.SymbolID) []cfg.SymbolID {
	if len(a) == 0 {
		return append([]cfg.SymbolID(nil), b...)
	}
	if len(b) == 0 {
		return append([]cfg.SymbolID(nil), a...)
	}
	out := append(append([]cfg.SymbolID(nil), a...), b...)
	slices.Sort(out)
	return slices.Compact(out)
}

func compactFunctionBindingEntries(in []topology.FunctionBinding) []topology.FunctionBinding {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev topology.FunctionBinding
	for i, e := range in {
		if i > 0 && compareFunctionBindingEntrySymbolOnly(prev, e) == 0 {
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

func compactFieldFuncEntries(in []topology.FieldFunction) []topology.FieldFunction {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	seen := make(map[fieldFuncIdentity]bool, len(in))
	for _, e := range in {
		id := fieldFuncIdentity{
			container: e.ContainerSym,
			field:     e.Field,
			ref:       e.FuncRef,
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, e)
	}
	return out
}

type fieldFuncIdentity struct {
	container cfg.SymbolID
	field     fieldkey.Key
	ref       ref.FuncRef
}

func compactMetatableIndexEntries(in []metatable.Index) []metatable.Index {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev metatable.Index
	for i, e := range in {
		if i > 0 && compareMetatableIndexEntry(prev, e) == 0 {
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

func compactMethodReceiverEntries(in []methodReceiverEntry) []methodReceiverEntry {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev methodReceiverEntry
	for i, e := range in {
		if i > 0 && compareMethodReceiverEntry(prev, e) == 0 {
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

func compactPrototypeMethodEntries(in []metatable.PrototypeMethod) []metatable.PrototypeMethod {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	for _, e := range in {
		if len(out) > 0 && comparePrototypeMethodEntry(out[len(out)-1], e) == 0 {
			continue
		}
		out = append(out, e)
	}
	return out
}

func compactSetMetatableSiteEntries(in []setMetatableSiteEntry) []setMetatableSiteEntry {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev setMetatableSiteEntry
	for i, e := range in {
		if i > 0 && compareSetMetatableSiteEntry(prev, e) == 0 {
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

// collectPredicateFacts accumulates module-wide local type-predicate facts from
// flow evidence, keyed by predicate function symbol.
func collectPredicateFacts(p Program) []guard.PredicateFunction {
	if p.Evidence == nil {
		return nil
	}
	byFunc := make(map[cfg.SymbolID]guard.PredicateFunction)
	for _, ref := range p.Refs {
		g := graphOf(p, ref)
		if g == nil {
			continue
		}
		for _, pred := range p.Evidence(g).LocalTypePredicates {
			if pred.Symbol == 0 || pred.Kind == "" || pred.ParamIndex < 0 {
				continue
			}
			if _, seen := byFunc[pred.Symbol]; seen {
				continue
			}
			byFunc[pred.Symbol] = guard.PredicateFunction{
				FuncSym:    pred.Symbol,
				ParamIndex: pred.ParamIndex,
				Kind:       pred.Kind,
			}
		}
	}
	if len(byFunc) == 0 {
		return nil
	}
	out := make([]guard.PredicateFunction, 0, len(byFunc))
	for _, fact := range byFunc {
		out = append(out, fact)
	}
	slices.SortFunc(out, comparePredicateFuncEntry)
	return compactPredicateFuncEntries(out)
}

// predicateCondSymBinds derives a graph's assigned-result predicate guards.
func predicateCondSymBinds(r ref.FuncRef, g *cfg.Graph, facts []guard.PredicateFunction) []predicateResultRow {
	byFunc := predicateFuncMap(facts)
	if g == nil || len(byFunc) == 0 {
		return nil
	}
	bindings := g.Bindings()
	var out []predicateResultRow
	g.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i := range info.Targets {
			target := info.Targets[i]
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			call, retIdx := info.CallForTarget(i)
			if call == nil || retIdx != 0 || call.Call == nil {
				continue
			}
			argSym, kind, ok := predicateCallInfoNarrow(call, byFunc)
			if !ok {
				argSym, kind, ok = predicateCallNarrow(call.Call, byFunc, bindings)
			}
			if !ok {
				continue
			}
			out = append(out, predicateResultRow{
				FuncRef: r,
				Result:  guard.PredicateResult{CondSym: target.Symbol, NarrowSym: argSym, Kind: kind},
			})
		}
	})
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, comparePredicateResultRow)
	return compactPredicateResultRows(out)
}

func predicateCallInfoNarrow(call *cfg.CallInfo, byFunc map[cfg.SymbolID]guard.PredicateFunction) (cfg.SymbolID, string, bool) {
	if call == nil || len(byFunc) == 0 || call.Method != "" {
		return 0, "", false
	}
	fnSym := call.CalleeSymbol
	if fnSym == 0 || callsite.IsMethodLikeExpr(call.Call) {
		return 0, "", false
	}
	fact, ok := byFunc[fnSym]
	if !ok || fact.Kind == "" {
		return 0, "", false
	}
	if fact.ParamIndex < 0 || fact.ParamIndex >= len(call.ArgSymbols) {
		return 0, "", false
	}
	argSym := call.ArgSymbols[fact.ParamIndex]
	if argSym == 0 {
		return 0, "", false
	}
	return argSym, fact.Kind, true
}

func predicateFuncMap(facts []guard.PredicateFunction) map[cfg.SymbolID]guard.PredicateFunction {
	if len(facts) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]guard.PredicateFunction, len(facts))
	for _, e := range facts {
		if e.FuncSym == 0 || e.Kind == "" {
			continue
		}
		out[e.FuncSym] = e
	}
	return out
}

func predicateCallNarrow(call *ast.FuncCallExpr, byFunc map[cfg.SymbolID]guard.PredicateFunction, bindings *bind.BindingTable) (cfg.SymbolID, string, bool) {
	if call == nil || bindings == nil || callsite.IsMethodLikeExpr(call) {
		return 0, "", false
	}
	fnIdent, ok := call.Func.(*ast.IdentExpr)
	if !ok || fnIdent == nil {
		return 0, "", false
	}
	fnSym, ok := bindings.SymbolOf(fnIdent)
	if !ok || fnSym == 0 {
		return 0, "", false
	}
	fact, ok := byFunc[fnSym]
	if !ok || fact.Kind == "" {
		return 0, "", false
	}
	if fact.ParamIndex < 0 || fact.ParamIndex >= len(call.Args) {
		return 0, "", false
	}
	argIdent, ok := call.Args[fact.ParamIndex].(*ast.IdentExpr)
	if !ok || argIdent == nil {
		return 0, "", false
	}
	argSym, ok := bindings.SymbolOf(argIdent)
	if !ok || argSym == 0 {
		return 0, "", false
	}
	return argSym, fact.Kind, true
}
