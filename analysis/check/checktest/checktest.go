// Package checktest contains test harness helpers for the active analysis
// stack. It is not a semantic owner; it only composes parse, check,
// diagnostics, and module manifests for repository fixtures.
package checktest

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/checktest/internal/precheck"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/exportmanifest"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/placementplan"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

type Option func(*config)

type config struct {
	stdlib           bool
	globals          []string
	manifests        map[string]*manifest.Manifest
	modules          map[string]*ModuleResult
	diagnosticPolicy diagnostic.Policy
	stateLanes       []state.LaneID
}

type Result struct {
	Diagnostics []diagnostic.Diagnostic
	checked     *program.Result
	placement   placementplan.Plan
}

type ModuleResult struct {
	Errors    []diagnostic.Diagnostic
	Manifest  *manifest.Manifest
	Placement placementplan.Plan
}

func WithStdlib() Option {
	return func(c *config) {
		c.stdlib = true
	}
}

func WithGlobals(names ...string) Option {
	return func(c *config) {
		c.globals = append(c.globals, names...)
	}
}

func WithManifest(name string, m *manifest.Manifest) Option {
	return func(c *config) {
		if name == "" || m == nil {
			return
		}
		if c.manifests == nil {
			c.manifests = make(map[string]*manifest.Manifest)
		}
		c.manifests[name] = m
	}
}

func WithModule(name string, mod *ModuleResult) Option {
	return func(c *config) {
		if name == "" || mod == nil {
			return
		}
		if c.modules == nil {
			c.modules = make(map[string]*ModuleResult)
		}
		c.modules[name] = mod
	}
}

func WithDiagnosticPolicy(policy diagnostic.Policy) Option {
	return func(c *config) {
		c.diagnosticPolicy = cloneDiagnosticPolicy(policy)
	}
}

func WithDiagnosticRule(code diagnostic.Code, rule diagnostic.Rule) Option {
	return func(c *config) {
		if c.diagnosticPolicy.Rules == nil {
			c.diagnosticPolicy.Rules = make(map[diagnostic.Code]diagnostic.Rule, 1)
		}
		c.diagnosticPolicy.Rules[code] = rule
	}
}

func WithStateLanes(lanes ...state.LaneID) Option {
	return func(c *config) {
		c.stateLanes = append([]state.LaneID{}, lanes...)
	}
}

func Check(src string, opts ...Option) Result {
	return checkSource(src, "test.lua", opts...)
}

func CheckAndExport(src, name string, opts ...Option) *ModuleResult {
	result := checkSource(src, name, opts...)
	var m *manifest.Manifest
	if result.checked != nil {
		m = exportmanifest.FromProgramResult(name, *result.checked)
	} else {
		m = manifest.New(name)
		m.SetExport(typ.Unknown)
	}
	return &ModuleResult{Errors: result.Diagnostics, Manifest: m, Placement: result.PlacementPlan()}
}

func (r Result) PlacementPlan() placementplan.Plan {
	return r.placement
}

func ChannelManifest() *manifest.Manifest {
	m := manifest.New("channel")
	m.SetExport(typ.Unknown)
	return m
}

func FuncsManifest() *manifest.Manifest {
	m := manifest.New("funcs")
	m.SetExport(typ.Unknown)
	return m
}

func ProcessManifest() *manifest.Manifest {
	m := manifest.New("process")
	m.DefineFunctionSignature("process.send", signature.Function{
		Type: typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			Param("payload", typ.Any).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Empty.With(ownership.SendParam{
			Param: effect.ParamRef{Index: 2},
		}),
	})
	m.SetExport(typ.Unknown)
	return m
}

func checkSource(src, filename string, opts ...Option) Result {
	cfg := applyOptions(opts)
	stmts, err := parse.ParseString(src, filename)
	if err != nil {
		diags := cfg.diagnosticPolicy.Apply([]diagnostic.Diagnostic{{
			Position: diagnostic.Position{File: filename},
			Code:     diagnostic.Code("parse"),
			Message:  err.Error(),
			Severity: diagnostic.SeverityError,
		}})
		return Result{Diagnostics: diags}
	}
	reg := standard.Registry()
	signatures := cfg.signatureSource()
	moduleExports := cfg.moduleExportSource()
	moduleTypes := cfg.moduleTypeSource()
	structural := precheck.Precheck(stmts)
	checked, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry:      reg,
			Globals:       cfg.globals,
			StateLanes:    cfg.stateLanes,
			Signatures:    signatures,
			ModuleExports: moduleExports,
			ModuleTypes:   moduleTypes,
		},
	})
	if err != nil {
		if errors.Is(err, body.ErrUnsupportedCFG) {
			structural = cfg.diagnosticPolicy.Apply(structural)
			setDefaultFile(structural, filename)
			diagnostic.Sort(structural)
			return Result{Diagnostics: structural}
		}
		diags := append([]diagnostic.Diagnostic{{
			Position: diagnostic.Position{File: filename},
			Code:     diagnostic.Code("check"),
			Message:  err.Error(),
			Severity: diagnostic.SeverityError,
		}}, structural...)
		diags = cfg.diagnosticPolicy.Apply(diags)
		setDefaultFile(diags, filename)
		diagnostic.Sort(diags)
		return Result{Diagnostics: diags}
	}
	structural = cfg.diagnosticPolicy.Apply(structural)
	diags := append(structural, diagnostics.ProduceWithConfig(checked.RootResult(), diagnostics.Config{
		Policy: cfg.diagnosticPolicy,
	})...)
	setDefaultFile(diags, filename)
	diagnostic.Sort(diags)
	return Result{Diagnostics: diags, checked: &checked, placement: placementplan.FromProgramResult(checked)}
}

func applyOptions(opts []Option) config {
	var cfg config
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

func cloneDiagnosticPolicy(policy diagnostic.Policy) diagnostic.Policy {
	if len(policy.Rules) == 0 {
		return diagnostic.Policy{}
	}
	out := diagnostic.Policy{Rules: make(map[diagnostic.Code]diagnostic.Rule, len(policy.Rules))}
	for code, rule := range policy.Rules {
		out.Rules[code] = rule
	}
	return out
}

func (c config) signatureSource() signaturelookup.Source {
	manifests := c.orderedManifests()
	return signaturelookup.Source{
		Manifests:     manifests,
		IncludeStdlib: c.stdlib,
	}
}

func (c config) moduleExportSource() importlookup.Source {
	return importlookup.Source{Manifests: c.orderedManifests()}
}

func (c config) moduleTypeSource() typelookup.Source {
	return typelookup.Source{Manifests: c.orderedManifests()}
}

func (c config) orderedManifests() []*manifest.Manifest {
	manifests := make([]*manifest.Manifest, 0, len(c.manifests)+len(c.modules))
	names := make([]string, 0, len(c.manifests))
	for name := range c.manifests {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		manifests = append(manifests, c.manifests[name])
	}
	names = names[:0]
	for name := range c.modules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if mod := c.modules[name]; mod != nil && mod.Manifest != nil {
			manifests = append(manifests, mod.Manifest)
		}
	}
	return manifests
}

func setDefaultFile(diags []diagnostic.Diagnostic, filename string) {
	for i := range diags {
		if diags[i].Position.File == "" {
			diags[i].Position.File = filename
		}
	}
}
