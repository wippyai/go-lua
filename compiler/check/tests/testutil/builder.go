package testutil

import (
	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/hooks"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/stdlib"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func buildChecker(cfg *Config) *check.Checker {
	for path, manifest := range cfg.Manifests {
		cfg.Database.Connect(path, manifest)
	}

	var stdlibScope *scope.State
	globalTypes := make(map[string]typ.Type)

	if cfg.Stdlib {
		stdlibScope = scope.NewWithBuiltins()
		for sname, t := range stdlib.Library() {
			globalTypes[sname] = t
		}
	}

	for _, manifest := range cfg.Manifests {
		if stdlibScope == nil {
			stdlibScope = scope.New()
		}
		if manifest.Export != nil {
			globalTypes[manifest.Path] = manifest.Export
		}
		for tname, t := range manifest.Types {
			stdlibScope = stdlibScope.WithType(tname, t)
		}
		for gname, t := range manifest.AllGlobals() {
			globalTypes[gname] = t
		}
	}

	for tname, t := range cfg.Types {
		if stdlibScope == nil {
			stdlibScope = scope.New()
		}
		stdlibScope = stdlibScope.WithType(tname, t)
	}

	var engine *core.Engine
	if cfg.Stdlib {
		engine = core.NewEngineWithStdlib(stdlib.EngineConfig())
	} else {
		engine = core.NewEngine()
	}

	checkOpts := append(hooks.All(), cfg.CheckOptions...)
	return check.NewChecker(cfg.Database, check.Deps{
		Types:       engine,
		Stdlib:      stdlibScope,
		GlobalTypes: globalTypes,
	}, checkOpts...)
}
