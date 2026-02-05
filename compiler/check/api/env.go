// env.go defines phase-typed environments for synthesis.
// DeclaredEnv is used pre-flow; NarrowEnv is used post-flow.
// This split prevents pre-flow return summaries from being accessed in narrowing.
package api

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// Phase identifies which pipeline stage is active.
// The phase also determines whether flow-refined types are available:
//   - PhaseNarrowing enables flow-refined types
//   - all earlier phases are declared-only
type Phase uint8

const (
	PhaseScopeCompute Phase = iota
	PhaseTypeResolution
	PhaseNarrowing
)

func (p Phase) String() string {
	switch p {
	case PhaseScopeCompute:
		return "ScopeCompute"
	case PhaseTypeResolution:
		return "TypeResolution"
	case PhaseNarrowing:
		return "Narrowing"
	default:
		return "Unknown"
	}
}

// BaseEnv is the shared environment interface for synthesis.
// It intentionally excludes return summaries to prevent cross-phase misuse.
type BaseEnv interface {
	Phase() Phase
	Graph() cfg.VersionedGraph
	Types() flow.TypeFacts
	Consts() *flow.Solution
	Effects() EffectFacts
	TypeNames() *scope.State
	Bindings() *bind.BindingTable
	ModuleAliases() map[cfg.SymbolID]string
	ModuleAlias(sym cfg.SymbolID) string
	GlobalType(sym cfg.SymbolID) (typ.Type, bool)
	GlobalTypes() map[string]typ.Type
	WithGlobalOverlay(overlay map[string]typ.Type) BaseEnv
}

// DeclaredEnv provides access to pre-flow return summaries.
type DeclaredEnv interface {
	BaseEnv
	ReturnSummaries() map[cfg.SymbolID][]typ.Type
}

// NarrowEnv provides access to post-flow return summaries.
type NarrowEnv interface {
	BaseEnv
	NarrowReturnSummaries() map[cfg.SymbolID][]typ.Type
}

type envBase struct {
	phase         Phase
	graph         cfg.VersionedGraph
	bindings      *bind.BindingTable
	types         flow.TypeFacts
	solution      *flow.Solution
	effects       EffectFacts
	typeNames     *scope.State
	moduleAliases map[cfg.SymbolID]string
	globalTypes   map[string]typ.Type
}

func (b *envBase) withGlobalOverlay(overlay map[string]typ.Type) *envBase {
	if b == nil || len(overlay) == 0 {
		return b
	}
	merged := make(map[string]typ.Type, len(b.globalTypes)+len(overlay))
	for k, v := range b.globalTypes {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	next := *b
	next.globalTypes = merged
	return &next
}

// DeclaredEnvImpl is the concrete declared-phase environment.
type DeclaredEnvImpl struct {
	base            *envBase
	returnSummaries map[cfg.SymbolID][]typ.Type
}

// NarrowEnvImpl is the concrete narrowing-phase environment.
type NarrowEnvImpl struct {
	base          *envBase
	narrowReturns map[cfg.SymbolID][]typ.Type
}

var _ BaseEnv = (*DeclaredEnvImpl)(nil)
var _ BaseEnv = (*NarrowEnvImpl)(nil)
var _ DeclaredEnv = (*DeclaredEnvImpl)(nil)
var _ NarrowEnv = (*NarrowEnvImpl)(nil)

// Phase returns the current checking phase.
func (e *DeclaredEnvImpl) Phase() Phase {
	if e == nil || e.base == nil {
		return PhaseScopeCompute
	}
	return e.base.phase
}

// Phase returns the current checking phase.
func (e *NarrowEnvImpl) Phase() Phase {
	if e == nil || e.base == nil {
		return PhaseScopeCompute
	}
	return e.base.phase
}

// Graph returns the versioned CFG graph.
func (e *DeclaredEnvImpl) Graph() cfg.VersionedGraph {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.graph
}

// Graph returns the versioned CFG graph.
func (e *NarrowEnvImpl) Graph() cfg.VersionedGraph {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.graph
}

// Types returns the type facts provider.
func (e *DeclaredEnvImpl) Types() flow.TypeFacts {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.types
}

// Types returns the type facts provider.
func (e *NarrowEnvImpl) Types() flow.TypeFacts {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.types
}

// Consts returns the flow solution for constant value lookup.
func (e *DeclaredEnvImpl) Consts() *flow.Solution {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.solution
}

// Consts returns the flow solution for constant value lookup.
func (e *NarrowEnvImpl) Consts() *flow.Solution {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.solution
}

// Effects returns the effect facts provider.
func (e *DeclaredEnvImpl) Effects() EffectFacts {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.effects
}

// Effects returns the effect facts provider.
func (e *NarrowEnvImpl) Effects() EffectFacts {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.effects
}

// TypeNames returns the scope state for type name resolution.
func (e *DeclaredEnvImpl) TypeNames() *scope.State {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.typeNames
}

// TypeNames returns the scope state for type name resolution.
func (e *NarrowEnvImpl) TypeNames() *scope.State {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.typeNames
}

// Bindings returns the binding table for AST-based symbol resolution.
func (e *DeclaredEnvImpl) Bindings() *bind.BindingTable {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.bindings
}

// Bindings returns the binding table for AST-based symbol resolution.
func (e *NarrowEnvImpl) Bindings() *bind.BindingTable {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.bindings
}

// ModuleAliases returns the module alias map (symbol -> module path).
func (e *DeclaredEnvImpl) ModuleAliases() map[cfg.SymbolID]string {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.moduleAliases
}

// ModuleAliases returns the module alias map (symbol -> module path).
func (e *NarrowEnvImpl) ModuleAliases() map[cfg.SymbolID]string {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.moduleAliases
}

// ModuleAlias returns the module path for a symbol assigned from require().
func (e *DeclaredEnvImpl) ModuleAlias(sym cfg.SymbolID) string {
	if e == nil || e.base == nil || e.base.moduleAliases == nil {
		return ""
	}
	return e.base.moduleAliases[sym]
}

// ModuleAlias returns the module path for a symbol assigned from require().
func (e *NarrowEnvImpl) ModuleAlias(sym cfg.SymbolID) string {
	if e == nil || e.base == nil || e.base.moduleAliases == nil {
		return ""
	}
	return e.base.moduleAliases[sym]
}

// GlobalType returns the global type for a symbol if it is a confirmed global.
func (e *DeclaredEnvImpl) GlobalType(sym cfg.SymbolID) (typ.Type, bool) {
	if e == nil || e.base == nil || e.base.globalTypes == nil || sym == 0 {
		return nil, false
	}
	if e.base.bindings != nil {
		kind, ok := e.base.bindings.Kind(sym)
		if !ok || kind != cfg.SymbolGlobal {
			return nil, false
		}
		if name := e.base.bindings.Name(sym); name != "" {
			if t, found := e.base.globalTypes[name]; found && t != nil {
				return t, true
			}
		}
		return nil, false
	}
	return nil, false
}

// GlobalType returns the global type for a symbol if it is a confirmed global.
func (e *NarrowEnvImpl) GlobalType(sym cfg.SymbolID) (typ.Type, bool) {
	if e == nil || e.base == nil || e.base.globalTypes == nil || sym == 0 {
		return nil, false
	}
	if e.base.bindings != nil {
		kind, ok := e.base.bindings.Kind(sym)
		if !ok || kind != cfg.SymbolGlobal {
			return nil, false
		}
		if name := e.base.bindings.Name(sym); name != "" {
			if t, found := e.base.globalTypes[name]; found && t != nil {
				return t, true
			}
		}
		return nil, false
	}
	return nil, false
}

// GlobalTypes returns the global type map.
func (e *DeclaredEnvImpl) GlobalTypes() map[string]typ.Type {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.globalTypes
}

// GlobalTypes returns the global type map.
func (e *NarrowEnvImpl) GlobalTypes() map[string]typ.Type {
	if e == nil || e.base == nil {
		return nil
	}
	return e.base.globalTypes
}

// WithGlobalOverlay returns a derived Env with additional globals merged in.
func (e *DeclaredEnvImpl) WithGlobalOverlay(overlay map[string]typ.Type) BaseEnv {
	if e == nil {
		return e
	}
	if len(overlay) == 0 {
		return e
	}
	next := *e
	next.base = e.base.withGlobalOverlay(overlay)
	return &next
}

// WithGlobalOverlay returns a derived Env with additional globals merged in.
func (e *NarrowEnvImpl) WithGlobalOverlay(overlay map[string]typ.Type) BaseEnv {
	if e == nil {
		return e
	}
	if len(overlay) == 0 {
		return e
	}
	next := *e
	next.base = e.base.withGlobalOverlay(overlay)
	return &next
}

// ReturnSummaries returns the return type summaries for sibling functions.
func (e *DeclaredEnvImpl) ReturnSummaries() map[cfg.SymbolID][]typ.Type {
	if e == nil {
		return nil
	}
	return e.returnSummaries
}

// NarrowReturnSummaries returns post-flow return summaries for narrowing.
func (e *NarrowEnvImpl) NarrowReturnSummaries() map[cfg.SymbolID][]typ.Type {
	if e == nil {
		return nil
	}
	return e.narrowReturns
}

// DeclaredEnvConfig holds inputs for building a declared-phase Env.
type DeclaredEnvConfig struct {
	Graph           cfg.VersionedGraph
	Bindings        *bind.BindingTable
	DeclaredTypes   flow.DeclaredTypes
	AnnotatedVars   map[cfg.SymbolID]bool
	BaseScope       *scope.State
	EffectStore     EffectStore
	ModuleAliases   map[cfg.SymbolID]string
	GlobalTypes     map[string]typ.Type
	SiblingTypes    map[cfg.SymbolID]typ.Type
	LiteralTypes    flow.DeclaredTypes
	ReturnSummaries map[cfg.SymbolID][]typ.Type
}

// NarrowEnvConfig holds inputs for building a narrowing-phase Env.
type NarrowEnvConfig struct {
	Graph                 cfg.VersionedGraph
	Bindings              *bind.BindingTable
	DeclaredTypes         flow.DeclaredTypes
	AnnotatedVars         map[cfg.SymbolID]bool
	Solution              *flow.Solution
	BaseScope             *scope.State
	EffectStore           EffectStore
	ModuleAliases         map[cfg.SymbolID]string
	GlobalTypes           map[string]typ.Type
	SiblingTypes          map[cfg.SymbolID]typ.Type
	LiteralTypes          flow.DeclaredTypes
	NarrowReturnSummaries map[cfg.SymbolID][]typ.Type
}

// NewDeclaredEnv creates a declared-phase Env.
func NewDeclaredEnv(cfg DeclaredEnvConfig) *DeclaredEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := &envBase{
		phase:         PhaseScopeCompute,
		graph:         cfg.Graph,
		bindings:      cfg.Bindings,
		types:         newUnifiedTypeFacts(cfg.Graph, cfg.DeclaredTypes, cfg.SiblingTypes, cfg.LiteralTypes, cfg.AnnotatedVars, nil),
		solution:      nil,
		effects:       NewEffectFacts(cfg.EffectStore),
		typeNames:     cfg.BaseScope,
		moduleAliases: cfg.ModuleAliases,
		globalTypes:   cfg.GlobalTypes,
	}
	return &DeclaredEnvImpl{base: base, returnSummaries: cfg.ReturnSummaries}
}

// NewNarrowEnv creates a narrowing-phase Env.
func NewNarrowEnv(cfg NarrowEnvConfig) *NarrowEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := &envBase{
		phase:         PhaseNarrowing,
		graph:         cfg.Graph,
		bindings:      cfg.Bindings,
		types:         newUnifiedTypeFacts(cfg.Graph, cfg.DeclaredTypes, cfg.SiblingTypes, cfg.LiteralTypes, cfg.AnnotatedVars, cfg.Solution),
		solution:      cfg.Solution,
		effects:       NewEffectFacts(cfg.EffectStore),
		typeNames:     cfg.BaseScope,
		moduleAliases: cfg.ModuleAliases,
		globalTypes:   cfg.GlobalTypes,
	}
	return &NarrowEnvImpl{base: base, narrowReturns: cfg.NarrowReturnSummaries}
}

// ReturnInferenceEnvConfig holds inputs for return type inference.
type ReturnInferenceEnvConfig struct {
	Graph           cfg.VersionedGraph
	Bindings        *bind.BindingTable
	BaseScope       *scope.State
	DeclaredTypes   flow.DeclaredTypes
	GlobalTypes     map[string]typ.Type
	ModuleAliases   map[cfg.SymbolID]string
	ReturnSummaries map[cfg.SymbolID][]typ.Type
}

// NewReturnInferenceEnv creates a declared-phase Env for return inference.
func NewReturnInferenceEnv(cfg ReturnInferenceEnvConfig) *DeclaredEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := &envBase{
		phase:         PhaseScopeCompute,
		graph:         cfg.Graph,
		bindings:      cfg.Bindings,
		types:         newUnifiedTypeFacts(cfg.Graph, cfg.DeclaredTypes, nil, nil, nil, nil),
		solution:      nil,
		effects:       NewEffectFacts(nil),
		typeNames:     cfg.BaseScope,
		moduleAliases: cfg.ModuleAliases,
		globalTypes:   cfg.GlobalTypes,
	}
	return &DeclaredEnvImpl{base: base, returnSummaries: cfg.ReturnSummaries}
}

// unifiedTypeFacts implements flow.TypeFacts with layered type source lookup.
type unifiedTypeFacts struct {
	graph         cfg.VersionedGraph
	declaredTypes flow.DeclaredTypes
	siblingTypes  map[cfg.SymbolID]typ.Type
	literalTypes  flow.DeclaredTypes
	annotatedVars map[cfg.SymbolID]bool
	solution      *flow.Solution
}

func newUnifiedTypeFacts(
	graph cfg.VersionedGraph,
	declared flow.DeclaredTypes,
	siblings map[cfg.SymbolID]typ.Type,
	literals flow.DeclaredTypes,
	annotated map[cfg.SymbolID]bool,
	solution *flow.Solution,
) flow.TypeFacts {
	return &unifiedTypeFacts{
		graph:         graph,
		declaredTypes: declared,
		siblingTypes:  siblings,
		literalTypes:  literals,
		annotatedVars: annotated,
		solution:      solution,
	}
}

// DeclaredAt returns the declared type for a symbol at a CFG point.
// Searches: literal types, sibling function types, then declared types.
func (f *unifiedTypeFacts) DeclaredAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if sym == 0 {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	// For explicitly annotated symbols, prefer the declared type over literal overlays.
	if f.annotatedVars != nil && f.annotatedVars[sym] {
		if f.declaredTypes != nil {
			if t, ok := f.declaredTypes[sym]; ok && t != nil {
				return f.toTypedValue(t)
			}
		}
	}
	if f.literalTypes != nil {
		if f.annotatedVars == nil || !f.annotatedVars[sym] {
			if t, ok := f.literalTypes[sym]; ok && t != nil {
				return f.toTypedValue(t)
			}
		}
	}
	if f.siblingTypes != nil {
		if t, ok := f.siblingTypes[sym]; ok && t != nil {
			return f.toTypedValue(t)
		}
	}
	if f.declaredTypes != nil {
		if t, ok := f.declaredTypes[sym]; ok && t != nil {
			return f.toTypedValue(t)
		}
	}
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

// RefinedAt returns the flow-refined type for a symbol at a CFG point.
// Returns nil type if no solution is available or symbol is unknown.
func (f *unifiedTypeFacts) RefinedAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if f == nil || sym == 0 {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	if f.solution == nil {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	return f.solution.RefinedAt(p, sym)
}

// EffectiveTypeAt returns the effective type for a symbol at a CFG point.
// Prefers refined (narrowed) type if available, otherwise falls back to declared.
func (f *unifiedTypeFacts) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	refined := f.RefinedAt(p, sym)
	if refined.Type != nil && refined.State == flow.StateResolved {
		return refined
	}
	return f.DeclaredAt(p, sym)
}

// IsAnnotated returns true if the symbol has an explicit type annotation.
func (f *unifiedTypeFacts) IsAnnotated(sym cfg.SymbolID) bool {
	if f.annotatedVars == nil {
		return false
	}
	return f.annotatedVars[sym]
}

func (f *unifiedTypeFacts) toTypedValue(t typ.Type) flow.TypedValue {
	if t.Kind() == typ.Unknown.Kind() {
		return flow.TypedValue{Type: t, State: flow.StateUnknown}
	}
	return flow.TypedValue{Type: t, State: flow.StateResolved}
}
