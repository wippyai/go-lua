// Package check is a legacy facade over the current analysis checker.
package check

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/exportmanifest"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	typemanifest "github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

type Option func(*Checker)

type Deps struct {
	GlobalTypes map[string]typ.Type
}

type Checker struct {
	db               *db.DB
	deps             Deps
	diagnosticPolicy diagnostic.Policy
}

type Session struct {
	Diagnostics []diag.Diagnostic
	manifest    *typemanifest.Manifest
}

func NewChecker(database *db.DB, deps Deps, opts ...Option) *Checker {
	c := &Checker{db: database, deps: deps}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// WithDiagnosticPolicy configures analyzer diagnostic enablement and severity
// without changing the fixed-point analysis itself.
func WithDiagnosticPolicy(policy diagnostic.Policy) Option {
	return func(c *Checker) {
		c.diagnosticPolicy = cloneDiagnosticPolicy(policy)
	}
}

func (c *Checker) ClearCache() {}

func (c *Checker) CheckChunk(chunk []ast.Stmt, entryID string) *Session {
	return c.checkChunk(chunk, entryID, nil)
}

// CheckChunkWithImports checks chunk as an entry with the provided import alias
// bindings. Wippy entry imports are dependency-injected globals: the alias is
// directly visible in the entry, and the imported module's export type seeds
// that global. The checker DB may contain additional manifests for lookup and
// cache reuse, but only this per-entry import map becomes lexical globals.
func (c *Checker) CheckChunkWithImports(chunk []ast.Stmt, entryID string, imports map[string]*typemanifest.Manifest) *Session {
	return c.checkChunk(chunk, entryID, imports)
}

func (c *Checker) checkChunk(chunk []ast.Stmt, entryID string, imports map[string]*typemanifest.Manifest) *Session {
	manifests := append(c.canonicalManifests(), currentImportManifests(imports)...)
	globalTypes := mergedGlobalTypes(c.deps.GlobalTypes, importedAliasGlobalTypes(imports), importedAmbientGlobalTypes(manifests))
	checked, err := program.RunChunk(chunk, program.Config{
		ObservationContracts: append(append([]transformer.ObservationContract{
			program.SummaryProjectionObservationContract(),
		}, diagnostics.ObservationContracts()...), exportmanifest.ObservationContract()),
		Check: body.Config{
			Registry:    standard.Registry(),
			Globals:     configuredGlobals(globalTypes, manifests, imports),
			GlobalTypes: globalTypes,
			Signatures: signaturelookup.Source{
				Manifests:     manifests,
				IncludeStdlib: true,
			},
			ModuleExports: importlookup.Source{Manifests: manifests},
			ModuleTypes:   typelookup.Source{Manifests: manifests},
		},
	})
	if err != nil {
		return &Session{
			Diagnostics: []diag.Diagnostic{legacyDiagnostic(entryID, "check", err.Error(), diag.SeverityError)},
			manifest:    typemanifest.New(entryID),
		}
	}

	produced := diagnostics.ProduceWithConfig(checked.RootResult(), diagnostics.Config{Policy: c.diagnosticPolicy, SourceFile: entryID})
	legacy := make([]diag.Diagnostic, 0, len(produced))
	for _, d := range produced {
		legacy = append(legacy, fromAnalysisDiagnostic(entryID, d))
	}
	m := exportmanifest.FromProgramResult(entryID, checked)
	return &Session{Diagnostics: legacy, manifest: m}
}

func configuredGlobals(globalTypes map[string]typ.Type, manifests []*typemanifest.Manifest, imports map[string]*typemanifest.Manifest) []string {
	globals := globalNames(globalTypes)
	globals = append(globals, importedModuleGlobals(manifests)...)
	globals = append(globals, importedAliasNames(imports)...)
	return globals
}

func mergedGlobalTypes(base map[string]typ.Type, overlays ...map[string]typ.Type) map[string]typ.Type {
	if len(base) == 0 && len(overlays) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(base))
	for name, t := range base {
		out[name] = t
	}
	for _, overlay := range overlays {
		for name, t := range overlay {
			out[name] = t
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func (c *Checker) canonicalManifests() []*typemanifest.Manifest {
	imports := c.db.Imports()
	if len(imports) == 0 {
		return nil
	}
	out := make([]*typemanifest.Manifest, 0, len(imports))
	for path, manifest := range imports {
		if manifest == nil {
			continue
		}
		out = append(out, manifest.Rebound(path))
	}
	return out
}

func currentImportManifests(imports map[string]*typemanifest.Manifest) []*typemanifest.Manifest {
	if len(imports) == 0 {
		return nil
	}
	out := make([]*typemanifest.Manifest, 0, len(imports))
	for alias, manifest := range imports {
		if alias == "" || manifest == nil {
			continue
		}
		out = append(out, manifest.Rebound(alias))
	}
	return out
}

func (s *Session) ExportManifest(entryID string) *typemanifest.Manifest {
	if s == nil || s.manifest == nil {
		return typemanifest.New(entryID)
	}
	return s.manifest
}

func (s *Session) Release() {}

// importedModuleGlobals collects the ambient globals every imported module
// installs, so an entry that requires such a module recognizes the bare names it
// provides (a test runner's describe/it, a migration DSL's up/down) instead of
// reporting them as unknown values.
func importedModuleGlobals(manifests []*typemanifest.Manifest) []string {
	var out []string
	for _, m := range manifests {
		if m != nil {
			out = append(out, m.Globals...)
		}
	}
	return out
}

func importedAmbientGlobalTypes(manifests []*typemanifest.Manifest) map[string]typ.Type {
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

func importedAliasNames(imports map[string]*typemanifest.Manifest) []string {
	if len(imports) == 0 {
		return nil
	}
	out := make([]string, 0, len(imports))
	for alias := range imports {
		if alias != "" {
			out = append(out, alias)
		}
	}
	return out
}

func importedAliasGlobalTypes(imports map[string]*typemanifest.Manifest) map[string]typ.Type {
	if len(imports) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(imports))
	for alias, m := range imports {
		if alias == "" || m == nil || m.Export == nil {
			continue
		}
		out[alias] = m.Export
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func globalNames(globals map[string]typ.Type) []string {
	if len(globals) == 0 {
		return nil
	}
	names := make([]string, 0, len(globals))
	for name := range globals {
		names = append(names, name)
	}
	return names
}

func fromAnalysisDiagnostic(file string, d diagnostic.Diagnostic) diag.Diagnostic {
	severity := diag.SeverityError
	switch d.Severity {
	case diagnostic.SeverityWarning:
		severity = diag.SeverityWarning
	case diagnostic.SeverityHint:
		severity = diag.SeverityHint
	}
	code := diag.NamedCode(string(d.Code))
	if d.Code == diagnostic.Code("undefined") {
		code = diag.ErrUndefined
	}
	return diag.Diagnostic{
		Position: diag.Position{
			File:   firstNonEmpty(d.Position.File, file),
			Line:   d.Position.Line,
			Column: d.Position.Column,
		},
		Span: diag.Span{
			StartLine: d.Span.StartLine,
			StartCol:  d.Span.StartCol,
			EndLine:   d.Span.EndLine,
			EndCol:    d.Span.EndCol,
		},
		Code:        code,
		Message:     d.Message,
		Severity:    severity,
		Explanation: d.Explanation.String(),
		Help:        d.Help,
	}
}

func legacyDiagnostic(file, code, message string, severity diag.Severity) diag.Diagnostic {
	return diag.Diagnostic{
		Position: diag.Position{File: file, Line: 1, Column: 1},
		Span:     diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1},
		Code:     diag.NamedCode(code),
		Message:  message,
		Severity: severity,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
