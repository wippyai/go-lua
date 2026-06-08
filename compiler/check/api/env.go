// env.go defines mode-typed environments for synthesis.
// DeclaredEnv is used for declared/static synthesis; NarrowEnv admits a
// producer-neutral flow projection through synth.Config.Flow.
// This split keeps declared and flow-refined function facts mode-explicit.
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

// SynthMode identifies which synthesis view is active.
// The mode also determines whether flow-refined types are available:
//   - SynthModeFlow enables flow-refined types
//   - all earlier modes are declared-only
type SynthMode uint8

const (
	SynthModeDeclared SynthMode = iota
	SynthModeResolve
	SynthModeFlow
)

func (m SynthMode) String() string {
	switch m {
	case SynthModeDeclared:
		return "Declared"
	case SynthModeResolve:
		return "Resolve"
	case SynthModeFlow:
		return "Flow"
	default:
		return "Unknown"
	}
}

// BaseEnv is the shared environment interface for synthesis.
// It intentionally excludes FunctionFacts to prevent cross-mode misuse.
type BaseEnv interface {
	SynthMode() SynthMode
	Graph() cfg.VersionedGraph
	Types() flow.TypeFacts
	Refinements() RefinementFacts
	TypeNames() *scope.State
	Bindings() *bind.BindingTable
	ModuleAliases() map[cfg.SymbolID]string
	ModuleAlias(sym cfg.SymbolID) string
	GlobalType(sym cfg.SymbolID) (typ.Type, bool)
	GlobalTypeOverlay() globalenv.TypeOverlay
	WithGlobalTypeOverlay(overlay globalenv.TypeOverlay) BaseEnv
}

// DeclaredEnv is the declared/static synthesis environment.
type DeclaredEnv interface {
	BaseEnv
}

// NarrowEnv is the flow-refined synthesis environment.
type NarrowEnv interface {
	BaseEnv
}

type envBase struct {
	mode          SynthMode
	graph         cfg.VersionedGraph
	bindings      *bind.BindingTable
	types         flow.TypeFacts
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

// SynthMode returns the current synthesis view.
func (c *envCommon) SynthMode() SynthMode {
	if c == nil {
		return SynthModeDeclared
	}
	return c.base.mode
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

// DeclaredEnvImpl is the concrete declared/static environment.
type DeclaredEnvImpl struct {
	envCommon
}

// NarrowEnvImpl is the concrete flow-refined environment.
type NarrowEnvImpl struct {
	envCommon
}

var _ BaseEnv = (*DeclaredEnvImpl)(nil)
var _ BaseEnv = (*NarrowEnvImpl)(nil)
var _ DeclaredEnv = (*DeclaredEnvImpl)(nil)
var _ NarrowEnv = (*NarrowEnvImpl)(nil)

// SynthMode returns the current synthesis view.
func (e *DeclaredEnvImpl) SynthMode() SynthMode {
	if e == nil {
		return SynthModeDeclared
	}
	return e.envCommon.SynthMode()
}

// SynthMode returns the current synthesis view.
func (e *NarrowEnvImpl) SynthMode() SynthMode {
	if e == nil {
		return SynthModeDeclared
	}
	return e.envCommon.SynthMode()
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

// DeclaredEnvConfig holds inputs for building a declared/static Env.
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

// NarrowEnvConfig holds inputs for building a flow-refined Env.
type NarrowEnvConfig struct {
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

func newEnvBase(
	mode SynthMode,
	graph cfg.VersionedGraph,
	bindings *bind.BindingTable,
	types flow.TypeFacts,
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
		mode:          mode,
		graph:         graph,
		bindings:      bindings,
		types:         types,
		refinements:   refinements,
		typeNames:     typeNames,
		moduleAliases: moduleAliases,
		globalTypes:   normalizedGlobals,
	}
}

// NewDeclaredEnv creates a declared/static Env.
func NewDeclaredEnv(cfg DeclaredEnvConfig) *DeclaredEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := newEnvBase(
		SynthModeDeclared,
		cfg.Graph,
		cfg.Bindings,
		typefacts.New(typefacts.Config{
			Declared:      cfg.DeclaredTypes,
			FunctionType:  cfg.FunctionType,
			Literals:      cfg.LiteralTypes,
			AnnotatedVars: flow.AnnotatedSymbolsFromMap(cfg.AnnotatedVars),
		}),
		refinementFactsOrNil(cfg.Refinements),
		cfg.BaseScope,
		cfg.ModuleAliases,
		cfg.GlobalTypes,
		cfg.GlobalOverlay,
	)
	return &DeclaredEnvImpl{envCommon: envCommon{base: base}}
}

// NewNarrowEnv creates a flow-refined Env.
func NewNarrowEnv(cfg NarrowEnvConfig) *NarrowEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := newEnvBase(
		SynthModeFlow,
		cfg.Graph,
		cfg.Bindings,
		typefacts.New(typefacts.Config{
			Declared:      cfg.DeclaredTypes,
			FunctionType:  cfg.FunctionType,
			Literals:      cfg.LiteralTypes,
			AnnotatedVars: flow.AnnotatedSymbolsFromMap(cfg.AnnotatedVars),
		}),
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

// NewReturnInferenceEnv creates a declared/static Env for return inference.
func NewReturnInferenceEnv(cfg ReturnInferenceEnvConfig) *DeclaredEnvImpl {
	if cfg.Graph == nil {
		return nil
	}
	base := newEnvBase(
		SynthModeDeclared,
		cfg.Graph,
		cfg.Bindings,
		typefacts.New(typefacts.Config{
			Declared:     cfg.DeclaredTypes,
			FunctionType: cfg.FunctionType,
		}),
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
