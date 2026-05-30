package testutil

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/hooks"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/stdlib"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// DiffEntry is one diagnostic in normalized form: the canonical key the
// differential compares on plus the original diagnostic for reporting.
type DiffEntry struct {
	// Key is the normalized comparison key (file:line:col code/severity message).
	// Two diagnostics from the two flows are "the same" iff their keys match.
	Key string
	// Diagnostic is the original diagnostic, retained for human-readable reports.
	Diagnostic diag.Diagnostic
}

// DifferentialResult is the per-source diff between the legacy and canonical
// flows: the diagnostics only one flow produced, and the ones both produced.
//
// CanonicalOnly are diagnostics the canonical flow emits that legacy does not
// (over-reports, or differently-located diagnostics). LegacyOnly are diagnostics
// legacy emits that canonical does not — the transfer-fidelity worklist: each is a
// check the canonical flow cannot yet make because the bridge defaults the fact it
// reads. Matched are diagnostics both flows emit identically.
type DifferentialResult struct {
	CanonicalOnly []DiffEntry
	LegacyOnly    []DiffEntry
	Matched       []DiffEntry

	// LegacyAll and CanonicalAll are the full normalized diagnostic lists per flow,
	// for callers that want the raw counts independent of the diff buckets.
	LegacyAll    []DiffEntry
	CanonicalAll []DiffEntry
}

// Differential runs source through BOTH the legacy and the canonical type-flow
// engines in SEPARATE sessions backed by SEPARATE db.New() instances (the
// isolation the cutover requires: a shared db would let one flow's memoized facts
// leak into the other and corrupt the comparison), collects each flow's
// diagnostics, normalizes them, and returns the diff.
//
// The two checkers register the SAME diagnostic passes (hooks.All), so any
// difference in the diff reflects only the facts the passes read — the divergence
// the harness is built to measure — not a difference in the diagnostic layer.
func Differential(source, name string, opts ...Option) DifferentialResult {
	legacy := runFlow(source, name, nil, opts)
	canonical := runFlow(source, name, []check.Option{check.WithCanonicalFlow()}, opts)

	legacyKeys := indexByKey(legacy)
	canonicalKeys := indexByKey(canonical)

	res := DifferentialResult{
		LegacyAll:    legacy,
		CanonicalAll: canonical,
	}
	for _, e := range canonical {
		if _, ok := legacyKeys[e.Key]; ok {
			res.Matched = append(res.Matched, e)
		} else {
			res.CanonicalOnly = append(res.CanonicalOnly, e)
		}
	}
	for _, e := range legacy {
		if _, ok := canonicalKeys[e.Key]; !ok {
			res.LegacyOnly = append(res.LegacyOnly, e)
		}
	}
	return res
}

// runFlow checks source with a checker built on a fresh database, returning the
// normalized diagnostics. flowOpts carries the flow selector (nil for legacy,
// WithCanonicalFlow for canonical); userOpts are the caller's testutil options
// (stdlib, manifests, types).
func runFlow(source, name string, flowOpts []check.Option, userOpts []Option) []DiffEntry {
	cfg := &Config{Database: db.New()}
	for _, opt := range userOpts {
		opt(cfg)
	}
	for _, fo := range flowOpts {
		cfg.CheckOptions = append(cfg.CheckOptions, fo)
	}

	checker := buildChecker(cfg)
	sess := checker.Check(source, name)
	return normalizeDiagnostics(sess.Diagnostics)
}

// buildChecker constructs a checker from an already-populated Config. It mirrors
// NewChecker's wiring but takes the resolved Config directly so Differential can
// inject the flow selector after applying the caller's options.
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
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
			IndexFunc: core.Index,
		},
	}, checkOpts...)
}

// normalizeDiagnostics canonicalizes a flow's diagnostics into a comparable,
// deterministically sorted slice. Sorting by the comparison key makes the diff
// order-independent: the same set of diagnostics compares equal regardless of the
// order each flow emitted them.
func normalizeDiagnostics(diags []diag.Diagnostic) []DiffEntry {
	out := make([]DiffEntry, 0, len(diags))
	for _, d := range diags {
		out = append(out, DiffEntry{Key: diagnosticKey(d), Diagnostic: d})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// diagnosticKey is the canonical comparison key for one diagnostic: source
// location, code, severity, and message. It is the identity the differential
// matches on — two diagnostics are "the same diagnostic" iff their keys are equal.
func diagnosticKey(d diag.Diagnostic) string {
	return fmt.Sprintf("%s:%d:%d|%s|%d|%s",
		d.Position.File, d.Position.Line, d.Position.Column,
		d.Code.Name(), int(d.Severity), d.Message)
}

// indexByKey maps each normalized entry by its comparison key for membership
// tests. A duplicate key (the same diagnostic emitted twice) collapses to one
// entry, which is the intended set semantics for the diff.
func indexByKey(entries []DiffEntry) map[string]DiffEntry {
	m := make(map[string]DiffEntry, len(entries))
	for _, e := range entries {
		m[e.Key] = e
	}
	return m
}
