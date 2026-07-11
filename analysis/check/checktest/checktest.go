// Package checktest contains test harness helpers for the active analysis
// stack. It is not a semantic owner; it only composes parse, check,
// diagnostics, and module manifests for repository fixtures.
package checktest

import (
	"context"
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
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

type Option func(*config)

type config struct {
	context          context.Context
	stdlib           bool
	globals          []string
	manifests        map[string]*manifest.Manifest
	modules          map[string]*ModuleResult
	diagnosticPolicy diagnostic.Policy
	stateLanes       []state.LaneID
	schedule         transfer.Schedule
	compareWTO       func(transfer.WTOComparison)
	stats            *program.Stats
}

// WithContext makes the check's fixed-point worklists cooperatively
// cancelable. Nil keeps the helper's legacy uncancelable behavior.
func WithContext(ctx context.Context) Option {
	return func(c *config) {
		c.context = ctx
	}
}

// WithSchedule selects an opt-in body schedule for fixture/benchmark runs.
// FIFO remains the default. compare may be nil.
func WithSchedule(schedule transfer.Schedule, compare func(transfer.WTOComparison)) Option {
	return func(c *config) {
		c.schedule = schedule
		c.compareWTO = compare
	}
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
	bodies    []*body.Result
}

func WithStdlib() Option {
	return func(c *config) {
		c.stdlib = true
	}
}

func WithGlobals(names ...string) Option {
	selected := append([]string{}, names...)
	return func(c *config) {
		c.globals = append(c.globals, selected...)
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
	selected := cloneDiagnosticPolicy(policy)
	return func(c *config) {
		c.diagnosticPolicy = cloneDiagnosticPolicy(selected)
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
	selected := state.NewLaneSet(lanes...).IDs()
	return func(c *config) {
		c.stateLanes = state.NewLaneSet(selected...).IDs()
	}
}

func WithStats(stats *program.Stats) Option {
	return func(c *config) {
		c.stats = stats
	}
}

func Check(src string, opts ...Option) Result {
	return checkSource(src, "test.lua", opts...)
}

func CheckFile(src, filename string, opts ...Option) Result {
	if filename == "" {
		filename = "test.lua"
	}
	return checkSource(src, filename, opts...)
}

func CheckAndExport(src, name string, opts ...Option) *ModuleResult {
	result := checkSource(src, name, opts...)
	return moduleResultFromCheck(name, result)
}

func CheckFileAndExport(src, name, filename string, opts ...Option) *ModuleResult {
	if filename == "" {
		filename = name
	}
	result := checkSource(src, filename, opts...)
	return moduleResultFromCheck(name, result)
}

func moduleResultFromCheck(name string, result Result) *ModuleResult {
	var m *manifest.Manifest
	if result.checked != nil {
		m = exportmanifest.FromProgramResult(name, *result.checked)
	} else {
		m = manifest.New(name)
		m.SetExport(typ.Unknown)
	}
	return &ModuleResult{
		Errors:    result.Diagnostics,
		Manifest:  m,
		Placement: result.PlacementPlan(),
		bodies:    result.BodyResults(),
	}
}

func (r Result) PlacementPlan() placementplan.Plan {
	return r.placement
}

// RootResult returns the solved entry body, when checking reached analysis.
func (r Result) RootResult() *body.Result {
	if r.checked == nil {
		return nil
	}
	return r.checked.RootResult()
}

// BodyResults returns the solved entry body and all materialized nested
// function bodies. It is a harness/migration view; semantic consumers should
// use readmodel.Reader over each returned body.
func (r Result) BodyResults() []*body.Result {
	return collectBodyResults(r.RootResult())
}

func collectBodyResults(root *body.Result) []*body.Result {
	if root == nil {
		return nil
	}
	out := []*body.Result{root}
	for _, child := range root.FunctionResults() {
		out = append(out, collectBodyResults(child)...)
	}
	return out
}

func ChannelManifest() *manifest.Manifest {
	m := manifest.New("channel")
	channelType := typ.Instantiate(ambient.ChannelGeneric(), typ.Any)
	newType := typ.Func().
		OptParam("buffer", typ.Number).
		Returns(channelType).
		Build()
	selectResultType := typetable.NewRecord().
		Field("channel", typ.Any).
		Field("value", typ.Unknown).
		Field("ok", typ.Boolean).
		Build()
	selectType := typ.Func().
		Param("cases", typ.Any).
		OptParam("default", typ.Boolean).
		Returns(selectResultType).
		Build()
	m.DefineType("Channel", ambient.ChannelGeneric())
	m.DefineFunctionSignature("channel.new", signature.Function{Type: newType})
	m.DefineFunctionSignature("channel.select", signature.Function{Type: selectType})
	m.SetExport(typetable.NewRecord().
		Field("new", newType).
		Field("select", selectType).
		Build())
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
	globals := cfg.globals
	globalTypes := manifestGlobalTypes(cfg.orderedManifests())
	for _, m := range cfg.orderedManifests() {
		if m != nil {
			globals = append(globals, m.Globals...)
		}
	}
	checked, err := program.RunChunk(stmts, program.Config{
		Context: cfg.context,
		Check: body.Config{
			Registry:      reg,
			Globals:       globals,
			GlobalTypes:   globalTypes,
			StateLanes:    cfg.stateLanes,
			Schedule:      cfg.schedule,
			CompareWTO:    cfg.compareWTO,
			Signatures:    signatures,
			ModuleExports: moduleExports,
			ModuleTypes:   moduleTypes,
		},
		Stats: cfg.stats,
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
	diags := append(structural, diagnostics.ProduceWithConfig(checked.RootResult(), diagnostics.Config{Policy: cfg.diagnosticPolicy})...)
	setDefaultFile(diags, filename)
	diagnostic.Sort(diags)
	return Result{Diagnostics: diags, checked: &checked, placement: placementplan.FromProgramResult(checked)}
}

func manifestGlobalTypes(manifests []*manifest.Manifest) map[string]typ.Type {
	out := make(map[string]typ.Type)
	for _, m := range manifests {
		if m == nil {
			continue
		}
		for name, t := range m.GlobalTypes {
			if name != "" && t != nil {
				out[name] = t
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
