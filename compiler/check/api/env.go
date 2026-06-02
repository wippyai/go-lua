// env.go defines phase-typed environments for synthesis.
// DeclaredEnv is used pre-flow; NarrowEnv is used post-flow.
// This split keeps declared and flow-refined function facts phase-explicit.
package api

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/check/domain/typefacts"
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
// It intentionally excludes FunctionFacts to prevent cross-phase misuse.
type BaseEnv interface {
	Phase() Phase
	Graph() cfg.VersionedGraph
	Types() flow.TypeFacts
	Consts() *flow.Solution
	Refinements() RefinementFacts
	TypeNames() *scope.State
	Bindings() *bind.BindingTable
	ModuleAliases() map[cfg.SymbolID]string
	ModuleAlias(sym cfg.SymbolID) string
	GlobalType(sym cfg.SymbolID) (typ.Type, bool)
	GlobalTypeOverlay() globalenv.TypeOverlay
	WithGlobalTypeOverlay(overlay globalenv.TypeOverlay) BaseEnv
}

// DeclaredEnv is the declared-phase synthesis environment.
type DeclaredEnv interface {
	BaseEnv
}

// NarrowEnv is the narrowing-phase synthesis environment.
type NarrowEnv interface {
	BaseEnv
}

type envBase struct {
	phase         Phase
	graph         cfg.VersionedGraph
	bindings      *bind.BindingTable
	types         flow.TypeFacts
	solution      *flow.Solution
	refinements   RefinementFacts
	typeNames     *scope.State
	moduleAliases map[cfg.SymbolID]string
	globalTypes   globalenv.TypeOverlay
}

func (b envBase) withGlobalTypeOverlay(overlay globalenv.TypeOverlay) envBase {
	if len(overlay) == 0 {
		return b
	}
	next := b
	next.globalTypes = globalenv.OverrideTypeOverlay(
		b.globalTypes,
		overlay,
	)
	return next
}

type envCommon struct {
	base envBase
}

func (c *envCommon) withGlobalTypeOverlay(overlay globalenv.TypeOverlay) envCommon {
	if c == nil || len(overlay) == 0 {
		if c == nil {
			return envCommon{}
		}
		return *c
	}
	return envCommon{base: c.base.withGlobalTypeOverlay(overlay)}
}

// Phase returns the current checking phase.
func (c *envCommon) Phase() Phase {
	if c == nil {
		return PhaseScopeCompute
	}
	return c.base.phase
}

// Graph returns the versioned CFG graph.
func (c *envCommon) Graph() cfg.VersionedGraph {
	if c == nil {
		return nil
	}
	return c.base.graph
}

// Types returns the type facts provider.
func (c *envCommon) Types() flow.TypeFacts {
	if c == nil {
		return nil
	}
	return c.base.types
}

// Consts returns the flow solution for constant value lookup.
func (c *envCommon) Consts() *flow.Solution {
	if c == nil {
		return nil
	}
	return c.base.solution
}

// Refinements returns the refinement facts provider.
func (c *envCommon) Refinements() RefinementFacts {
	if c == nil {
		return nil
	}
	return c.base.refinements
}

// TypeNames returns the scope state for type name resolution.
func (c *envCommon) TypeNames() *scope.State {
	if c == nil {
		return nil
	}
	return c.base.typeNames
}

// Bindings returns the binding table for AST-based symbol resolution.
func (c *envCommon) Bindings() *bind.BindingTable {
	if c == nil {
		return nil
	}
	return c.base.bindings
}

// ModuleAliases returns the module alias map (symbol -> module path).
func (c *envCommon) ModuleAliases() map[cfg.SymbolID]string {
	if c == nil {
		return nil
	}
	return c.base.moduleAliases
}

// ModuleAlias returns the module path for a symbol assigned from require().
func (c *envCommon) ModuleAlias(sym cfg.SymbolID) string {
	if c == nil || c.base.moduleAliases == nil {
		return ""
	}
	return c.base.moduleAliases[sym]
}

// GlobalType returns the global type for a symbol if it is a confirmed global.
func (c *envCommon) GlobalType(sym cfg.SymbolID) (typ.Type, bool) {
	if c == nil || len(c.base.globalTypes) == 0 || sym == 0 {
		return nil, false
	}
	if c.base.bindings == nil {
		return nil, false
	}
	kind, ok := c.base.bindings.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		return nil, false
	}
	if name := c.base.bindings.Name(sym); name != "" {
		return c.base.globalTypes.Type(name)
	}
	return nil, false
}

// GlobalTypeOverlay returns the normalized global type overlay.
func (c *envCommon) GlobalTypeOverlay() globalenv.TypeOverlay {
	if c == nil {
		return nil
	}
	return c.base.globalTypes.Clone()
}

// DeclaredEnvImpl is the concrete declared-phase environment.
type DeclaredEnvImpl struct {
	envCommon
}

// NarrowEnvImpl is the concrete narrowing-phase environment.
type NarrowEnvImpl struct {
	envCommon
}

var _ BaseEnv = (*DeclaredEnvImpl)(nil)
var _ BaseEnv = (*NarrowEnvImpl)(nil)
var _ DeclaredEnv = (*DeclaredEnvImpl)(nil)
var _ NarrowEnv = (*NarrowEnvImpl)(nil)

// Phase returns the current checking phase.
func (e *DeclaredEnvImpl) Phase() Phase {
	if e == nil {
		return PhaseScopeCompute
	}
	return e.envCommon.Phase()
}

// Phase returns the current checking phase.
func (e *NarrowEnvImpl) Phase() Phase {
	if e == nil {
		return PhaseScopeCompute
	}
	return e.envCommon.Phase()
}

// Graph returns the versioned CFG graph.
func (e *DeclaredEnvImpl) Graph() cfg.VersionedGraph {
	if e == nil {
		return nil
	}
	return e.envCommon.Graph()
}

// Graph returns the versioned CFG graph.
func (e *NarrowEnvImpl) Graph() cfg.VersionedGraph {
	if e == nil {
		return nil
	}
	return e.envCommon.Graph()
}

// Types returns the type facts provider.
func (e *DeclaredEnvImpl) Types() flow.TypeFacts {
	if e == nil {
		return nil
	}
	return e.envCommon.Types()
}

// Types returns the type facts provider.
func (e *NarrowEnvImpl) Types() flow.TypeFacts {
	if e == nil {
		return nil
	}
	return e.envCommon.Types()
}

// Consts returns the flow solution for constant value lookup.
func (e *DeclaredEnvImpl) Consts() *flow.Solution {
	if e == nil {
		return nil
	}
	return e.envCommon.Consts()
}

// Consts returns the flow solution for constant value lookup.
func (e *NarrowEnvImpl) Consts() *flow.Solution {
	if e == nil {
		return nil
	}
	return e.envCommon.Consts()
}

// Refinements returns the refinement facts provider.
func (e *DeclaredEnvImpl) Refinements() RefinementFacts {
	if e == nil {
		return nil
	}
	return e.envCommon.Refinements()
}

// Refinements returns the refinement facts provider.
func (e *NarrowEnvImpl) Refinements() RefinementFacts {
	if e == nil {
		return nil
	}
	return e.envCommon.Refinements()
}

// TypeNames returns the scope state for type name resolution.
func (e *DeclaredEnvImpl) TypeNames() *scope.State {
	if e == nil {
		return nil
	}
	return e.envCommon.TypeNames()
}

// TypeNames returns the scope state for type name resolution.
func (e *NarrowEnvImpl) TypeNames() *scope.State {
	if e == nil {
		return nil
	}
	return e.envCommon.TypeNames()
}

// Bindings returns the binding table for AST-based symbol resolution.
func (e *DeclaredEnvImpl) Bindings() *bind.BindingTable {
	if e == nil {
		return nil
	}
	return e.envCommon.Bindings()
}

// Bindings returns the binding table for AST-based symbol resolution.
func (e *NarrowEnvImpl) Bindings() *bind.BindingTable {
	if e == nil {
		return nil
	}
	return e.envCommon.Bindings()
}

// ModuleAliases returns the module alias map (symbol -> module path).
func (e *DeclaredEnvImpl) ModuleAliases() map[cfg.SymbolID]string {
	if e == nil {
		return nil
	}
	return e.envCommon.ModuleAliases()
}

// ModuleAliases returns the module alias map (symbol -> module path).
func (e *NarrowEnvImpl) ModuleAliases() map[cfg.SymbolID]string {
	if e == nil {
		return nil
	}
	return e.envCommon.ModuleAliases()
}

// ModuleAlias returns the module path for a symbol assigned from require().
func (e *DeclaredEnvImpl) ModuleAlias(sym cfg.SymbolID) string {
	if e == nil {
		return ""
	}
	return e.envCommon.ModuleAlias(sym)
}

// ModuleAlias returns the module path for a symbol assigned from require().
func (e *NarrowEnvImpl) ModuleAlias(sym cfg.SymbolID) string {
	if e == nil {
		return ""
	}
	return e.envCommon.ModuleAlias(sym)
}

// GlobalType returns the global type for a symbol if it is a confirmed global.
func (e *DeclaredEnvImpl) GlobalType(sym cfg.SymbolID) (typ.Type, bool) {
	if e == nil {
		return nil, false
	}
	return e.envCommon.GlobalType(sym)
}

// GlobalType returns the global type for a symbol if it is a confirmed global.
func (e *NarrowEnvImpl) GlobalType(sym cfg.SymbolID) (typ.Type, bool) {
	if e == nil {
		return nil, false
	}
	return e.envCommon.GlobalType(sym)
}

// GlobalTypeOverlay returns the normalized global type overlay.
func (e *DeclaredEnvImpl) GlobalTypeOverlay() globalenv.TypeOverlay {
	if e == nil {
		return nil
	}
	return e.envCommon.GlobalTypeOverlay()
}

// GlobalTypeOverlay returns the normalized global type overlay.
func (e *NarrowEnvImpl) GlobalTypeOverlay() globalenv.TypeOverlay {
	if e == nil {
		return nil
	}
	return e.envCommon.GlobalTypeOverlay()
}

// WithGlobalTypeOverlay returns a derived Env with normalized globals merged in.
func (e *DeclaredEnvImpl) WithGlobalTypeOverlay(overlay globalenv.TypeOverlay) BaseEnv {
	if e == nil {
		return e
	}
	if len(overlay) == 0 {
		return e
	}
	next := *e
	next.envCommon = e.withGlobalTypeOverlay(overlay)
	return &next
}

// WithGlobalTypeOverlay returns a derived Env with normalized globals merged in.
func (e *NarrowEnvImpl) WithGlobalTypeOverlay(overlay globalenv.TypeOverlay) BaseEnv {
	if e == nil {
		return e
	}
	if len(overlay) == 0 {
		return e
	}
	next := *e
	next.envCommon = e.withGlobalTypeOverlay(overlay)
	return &next
}

// DeclaredEnvConfig holds inputs for building a declared-phase Env.
type DeclaredEnvConfig struct {
	Graph         cfg.VersionedGraph
	Bindings      *bind.BindingTable
	DeclaredTypes flow.DeclaredTypes
	AnnotatedVars map[cfg.SymbolID]bool
	BaseScope     *scope.State
	Refinements   RefinementFacts
	ModuleAliases map[cfg.SymbolID]string
	GlobalTypes   map[string]typ.Type
	GlobalOverlay globalenv.TypeOverlay
	LiteralTypes  flow.DeclaredTypes
	FunctionType  typefacts.FunctionTypeLookup
}

// NarrowEnvConfig holds inputs for building a narrowing-phase Env.
type NarrowEnvConfig struct {
	Graph         cfg.VersionedGraph
	Bindings      *bind.BindingTable
	DeclaredTypes flow.DeclaredTypes
	AnnotatedVars map[cfg.SymbolID]bool
	Solution      *flow.Solution
	BaseScope     *scope.State
	Refinements   RefinementFacts
	ModuleAliases map[cfg.SymbolID]string
	GlobalTypes   map[string]typ.Type
	GlobalOverlay globalenv.TypeOverlay
	LiteralTypes  flow.DeclaredTypes
	FunctionType  typefacts.FunctionTypeLookup
}

func newEnvBase(
	phase Phase,
	graph cfg.VersionedGraph,
	bindings *bind.BindingTable,
	types flow.TypeFacts,
	solution *flow.Solution,
	refinements RefinementFacts,
	typeNames *scope.State,
	moduleAliases map[cfg.SymbolID]string,
	globalTypes map[string]typ.Type,
	globalOverlay globalenv.TypeOverlay,
) envBase {
	normalizedGlobals := globalenv.OverrideTypeOverlay(
		globalenv.TypeOverlayFromMap(globalTypes),
		globalOverlay,
	)
	return envBase{
		phase:         phase,
		graph:         graph,
		bindings:      bindings,
		types:         types,
		solution:      solution,
		refinements:   refinements,
		typeNames:     typeNames,
		moduleAliases: moduleAliases,
		globalTypes:   normalizedGlobals,
	}
}

// NewDeclaredEnv creates a declared-phase Env.
func NewDeclaredEnv(cfg DeclaredEnvConfig) *DeclaredEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := newEnvBase(
		PhaseScopeCompute,
		cfg.Graph,
		cfg.Bindings,
		typefacts.New(typefacts.Config{
			Declared:      cfg.DeclaredTypes,
			FunctionType:  cfg.FunctionType,
			Literals:      cfg.LiteralTypes,
			AnnotatedVars: cfg.AnnotatedVars,
		}),
		nil,
		refinementFactsOrNil(cfg.Refinements),
		cfg.BaseScope,
		cfg.ModuleAliases,
		cfg.GlobalTypes,
		cfg.GlobalOverlay,
	)
	return &DeclaredEnvImpl{envCommon: envCommon{base: base}}
}

// NewNarrowEnv creates a narrowing-phase Env.
func NewNarrowEnv(cfg NarrowEnvConfig) *NarrowEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := newEnvBase(
		PhaseNarrowing,
		cfg.Graph,
		cfg.Bindings,
		typefacts.New(typefacts.Config{
			Declared:      cfg.DeclaredTypes,
			FunctionType:  cfg.FunctionType,
			Literals:      cfg.LiteralTypes,
			AnnotatedVars: cfg.AnnotatedVars,
			Solution:      cfg.Solution,
		}),
		cfg.Solution,
		refinementFactsOrNil(cfg.Refinements),
		cfg.BaseScope,
		cfg.ModuleAliases,
		cfg.GlobalTypes,
		cfg.GlobalOverlay,
	)
	return &NarrowEnvImpl{envCommon: envCommon{base: base}}
}

// ReturnInferenceEnvConfig holds inputs for return type inference.
type ReturnInferenceEnvConfig struct {
	Graph         cfg.VersionedGraph
	Bindings      *bind.BindingTable
	BaseScope     *scope.State
	DeclaredTypes flow.DeclaredTypes
	GlobalTypes   map[string]typ.Type
	GlobalOverlay globalenv.TypeOverlay
	ModuleAliases map[cfg.SymbolID]string
	FunctionType  typefacts.FunctionTypeLookup
}

// NewReturnInferenceEnv creates a declared-phase Env for return inference.
func NewReturnInferenceEnv(cfg ReturnInferenceEnvConfig) *DeclaredEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := newEnvBase(
		PhaseScopeCompute,
		cfg.Graph,
		cfg.Bindings,
		typefacts.New(typefacts.Config{
			Declared:     cfg.DeclaredTypes,
			FunctionType: cfg.FunctionType,
		}),
		nil,
		nilRefinementFacts{},
		cfg.BaseScope,
		cfg.ModuleAliases,
		cfg.GlobalTypes,
		cfg.GlobalOverlay,
	)
	return &DeclaredEnvImpl{envCommon: envCommon{base: base}}
}

func refinementFactsOrNil(f RefinementFacts) RefinementFacts {
	if f == nil {
		return nilRefinementFacts{}
	}
	return f
}
