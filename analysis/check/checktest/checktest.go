// Package checktest contains test harness helpers for the active analysis
// stack. It is not a semantic owner; it only composes parse, check,
// diagnostics, and module manifests for repository fixtures.
package checktest

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

type Option func(*config)

type config struct {
	stdlib    bool
	manifests map[string]*manifest.Manifest
	modules   map[string]*ModuleResult
}

type Result struct {
	Diagnostics []diagnostic.Diagnostic
}

type ModuleResult struct {
	Errors   []diagnostic.Diagnostic
	Manifest *manifest.Manifest
}

func WithStdlib() Option {
	return func(c *config) {
		c.stdlib = true
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

func Check(src string, opts ...Option) Result {
	return checkSource(src, "test.lua", opts...)
}

func CheckAndExport(src, name string, opts ...Option) *ModuleResult {
	result := checkSource(src, name, opts...)
	m := manifest.New(name)
	m.SetExport(typ.Unknown)
	return &ModuleResult{Errors: result.Diagnostics, Manifest: m}
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

func checkSource(src, filename string, opts ...Option) Result {
	cfg := applyOptions(opts)
	_ = cfg
	stmts, err := parse.ParseString(src, filename)
	if err != nil {
		return Result{Diagnostics: []diagnostic.Diagnostic{{
			Position: diagnostic.Position{File: filename},
			Code:     diagnostic.Code("parse"),
			Message:  err.Error(),
			Severity: diagnostic.SeverityError,
		}}}
	}
	reg := product.DefaultRegistry()
	checked, err := check.CheckChunk(stmts, check.Config{Registry: reg})
	if err != nil {
		if errors.Is(err, check.ErrUnsupportedCFG) {
			return Result{}
		}
		return Result{Diagnostics: []diagnostic.Diagnostic{{
			Position: diagnostic.Position{File: filename},
			Code:     diagnostic.Code("check"),
			Message:  err.Error(),
			Severity: diagnostic.SeverityError,
		}}}
	}
	diags := diagnostics.Produce(checked, diagnostics.Config{Registry: reg})
	setDefaultFile(diags, filename)
	return Result{Diagnostics: diags}
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

func setDefaultFile(diags []diagnostic.Diagnostic, filename string) {
	for i := range diags {
		if diags[i].Position.File == "" {
			diags[i].Position.File = filename
		}
	}
}
