// Package lint adapts the single-body checking engine to a deterministic Lua
// project. Hosts may provide entries directly or use LoadDirectory for the
// small filesystem-oriented CLI shim.
package lint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/interproc"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/embedding"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/compiler/source"
)

// Entry is one independently checked Lua module. Imports are canonical module
// paths; when omitted they are discovered from static require("path") calls.
type Entry struct {
	Path       string
	ModulePath string
	Source     string
	Imports    []string
}

// ProjectInput is fully materialized host input. Manifests represent builtin
// or host-provided modules; entry manifests are added as their dependencies
// are checked.
type ProjectInput struct {
	Entries   []Entry
	Manifests []*manifest.Manifest
	// Targets limits checking to named modules plus their transitive local
	// imports. An empty list checks every discovered entry.
	Targets []string
}

// ResolvedImport is the module-boundary summary used while checking an entry.
// The lookup views are intentionally represented by their resolved result, not
// by an obsolete checker session or mutable module database.
type ResolvedImport struct {
	ModulePath string
	Manifest   *manifest.Manifest
	Export     typ.Type
}

// PhaseTimings is machine-readable wall-clock timing in nanoseconds.
type PhaseTimings struct {
	LoadResolveNS    int64 `json:"load_resolve_ns"`
	ParseBindLowerNS int64 `json:"parse_bind_lower_ns"`
	EvaluateNS       int64 `json:"evaluate_ns"`
	ProjectRenderNS  int64 `json:"project_render_ns"`
}

func (p *PhaseTimings) add(other PhaseTimings) {
	p.LoadResolveNS += other.LoadResolveNS
	p.ParseBindLowerNS += other.ParseBindLowerNS
	p.EvaluateNS += other.EvaluateNS
	p.ProjectRenderNS += other.ProjectRenderNS
}

// EntryResult keeps both the raw engine publication and the lint-facing
// projection so callers that need summaries never have to re-run an entry.
type EntryResult struct {
	Entry       Entry
	Manifest    *manifest.Manifest
	Imports     []ResolvedImport
	Engine      engine.Result
	Diagnostics []diagnostic.Diagnostic
	Timings     PhaseTimings
}

// ProjectResult aggregates sorted diagnostics and per-entry phase timing.
// InterprocCache exposes the existing cache metric shape for hosts that add
// demanded-body summaries; module manifests are already retained per entry.
type ProjectResult struct {
	Entries        []EntryResult
	Diagnostics    []diagnostic.Diagnostic
	Timings        PhaseTimings
	InterprocCache interproc.CacheMetrics
}

var requirePattern = regexp.MustCompile(`(?m)\brequire\s*\(\s*["']([^"']+)["']\s*\)`)

// LoadDirectory discovers .lua files below root. Paths and module names are
// slash/dot normalized so reports are stable across host operating systems.
func LoadDirectory(root string, entryPaths []string) (ProjectInput, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return ProjectInput{}, fmt.Errorf("lint: resolve root: %w", err)
	}
	wanted := make(map[string]bool, len(entryPaths))
	for _, path := range entryPaths {
		absolute := path
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(root, path)
		}
		absolute, err = filepath.Abs(absolute)
		if err != nil {
			return ProjectInput{}, fmt.Errorf("lint: resolve entry %q: %w", path, err)
		}
		wanted[absolute] = true
	}
	entries := make([]Entry, 0)
	targets := make([]string, 0, len(wanted))
	err = filepath.WalkDir(root, func(path string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.IsDir() || filepath.Ext(path) != ".lua" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		relative = filepath.ToSlash(relative)
		module := modulePath(relative)
		entries = append(entries, Entry{Path: relative, ModulePath: module, Source: string(content)})
		if wanted[path] {
			targets = append(targets, module)
		}
		return nil
	})
	if err != nil {
		return ProjectInput{}, fmt.Errorf("lint: load directory: %w", err)
	}
	if len(targets) != len(wanted) {
		return ProjectInput{}, fmt.Errorf("lint: one or more entry paths are not Lua files below %s", root)
	}
	return ProjectInput{Entries: entries, Targets: targets}, nil
}

func modulePath(path string) string {
	path = strings.TrimSuffix(filepath.ToSlash(path), ".lua")
	path = strings.TrimSuffix(path, "/init")
	return strings.ReplaceAll(path, "/", ".")
}

// CheckProject resolves imports against module manifests in dependency order,
// invokes engine.Check exactly once for each entry, and aggregates positional
// diagnostics. It does not pretend that the current single-body engine has a
// legacy global checker session: imported manifest summaries remain explicit.
func CheckProject(ctx context.Context, input ProjectInput) (ProjectResult, error) {
	entries := append([]Entry(nil), input.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ModulePath < entries[j].ModulePath })
	byModule := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		if entry.ModulePath == "" {
			return ProjectResult{}, fmt.Errorf("lint: entry %q has empty module path", entry.Path)
		}
		if _, duplicate := byModule[entry.ModulePath]; duplicate {
			return ProjectResult{}, fmt.Errorf("lint: duplicate module path %q", entry.ModulePath)
		}
		byModule[entry.ModulePath] = entry
	}
	external := append([]*manifest.Manifest(nil), input.Manifests...)
	resolved := make(map[string]*manifest.Manifest, len(entries))
	results := make(map[string]EntryResult, len(entries))
	visiting := make(map[string]bool, len(entries))
	var visit func(string) error
	visit = func(module string) error {
		if _, done := results[module]; done {
			return nil
		}
		if visiting[module] {
			return fmt.Errorf("lint: import cycle at module %q", module)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, ok := byModule[module]
		if !ok {
			return fmt.Errorf("lint: internal unresolved module %q", module)
		}
		visiting[module] = true
		defer delete(visiting, module)
		imports := entry.Imports
		if len(imports) == 0 {
			imports = discoverImports(entry.Source)
		}
		resolveStarted := time.Now()
		resolvedImports := make([]ResolvedImport, 0, len(imports))
		preDiagnostics := make([]diagnostic.Diagnostic, 0)
		for _, imported := range imports {
			if dependency, local := byModule[imported]; local {
				if err := visit(dependency.ModulePath); err != nil {
					return err
				}
			}
			available := append([]*manifest.Manifest(nil), external...)
			for _, dependency := range resolved {
				available = append(available, dependency)
			}
			exports := importlookup.Source{Manifests: available}
			types := typelookup.Source{Manifests: available}
			signatures := signaturelookup.Source{Manifests: available, IncludeStdlib: true}
			_ = types // Constructing all three surviving lookup views is the module seam.
			if err := signatures.Validate(); err != nil {
				return fmt.Errorf("lint: validate imported signatures: %w", err)
			}
			export, found := exports.LookupExport(imported)
			if !found {
				preDiagnostics = append(preDiagnostics, moduleDiagnostic(entry, imported))
				continue
			}
			resolvedImports = append(resolvedImports, ResolvedImport{ModulePath: imported, Manifest: manifestForPath(available, imported), Export: export})
		}
		resolveElapsed := time.Since(resolveStarted)
		result, checkErr := engine.Check(entry.Source)
		if checkErr != nil {
			return fmt.Errorf("lint: check %s: %w", entry.Path, checkErr)
		}
		diagnostics, renderElapsed := projectDiagnostics(entry, result, preDiagnostics)
		summary := manifest.New(entry.ModulePath)
		// The current engine publishes runtime facts rather than a static export
		// type. Any is an explicit, conservative module boundary until its
		// manifest exporter is promoted; it still lets importlookup resolve the
		// module without fabricating a precise signature.
		summary.SetExport(typ.Any)
		resolved[module] = summary
		entryTiming := PhaseTimings{
			LoadResolveNS:    resolveElapsed.Nanoseconds(),
			ParseBindLowerNS: result.Timings.ParseBindLower.Nanoseconds(),
			EvaluateNS:       result.Timings.Evaluate.Nanoseconds(),
			ProjectRenderNS:  renderElapsed.Nanoseconds(),
		}
		results[module] = EntryResult{Entry: entry, Manifest: summary, Imports: resolvedImports, Engine: result, Diagnostics: diagnostics, Timings: entryTiming}
		return nil
	}
	targets := append([]string(nil), input.Targets...)
	if len(targets) == 0 {
		for _, entry := range entries {
			targets = append(targets, entry.ModulePath)
		}
	}
	for _, target := range targets {
		if _, exists := byModule[target]; !exists {
			return ProjectResult{}, fmt.Errorf("lint: target module %q is not present", target)
		}
		if err := visit(target); err != nil {
			return ProjectResult{}, err
		}
	}
	out := ProjectResult{Entries: make([]EntryResult, 0, len(entries))}
	for _, entry := range entries {
		item, checked := results[entry.ModulePath]
		if !checked {
			continue
		}
		out.Entries = append(out.Entries, item)
		out.Diagnostics = append(out.Diagnostics, item.Diagnostics...)
		out.Timings.add(item.Timings)
	}
	diagnostic.Sort(out.Diagnostics)
	return out, nil
}

// RenderDiagnostic is the compact terminal projection consumed by the CLI and
// host adapters that do not need rich source frames.
func RenderDiagnostic(item diagnostic.Diagnostic) string {
	return fmt.Sprintf("%s:%d:%d: %s[%s]: %s", item.Position.File, item.Position.Line, item.Position.Column, item.Severity, item.Code, item.Message)
}

func discoverImports(content string) []string {
	matches := requirePattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 || match[1] == "" || seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		out = append(out, match[1])
	}
	return out
}

func manifestForPath(manifests []*manifest.Manifest, path string) *manifest.Manifest {
	for index := len(manifests) - 1; index >= 0; index-- {
		if item := manifests[index]; item != nil && item.Path == path {
			return item
		}
	}
	return nil
}

func projectDiagnostics(entry Entry, result engine.Result, initial []diagnostic.Diagnostic) ([]diagnostic.Diagnostic, time.Duration) {
	started := time.Now()
	out := append([]diagnostic.Diagnostic(nil), initial...)
	for _, fact := range result.Diagnostics {
		span := spanForFact(entry.Source, result.DiagnosticSpans[fact.Key])
		if fact.Key == "analysis/front" {
			if _, err := parse.ParseString(entry.Source, entry.Path); err != nil {
				if parseError, ok := err.(*parse.Error); ok {
					span = source.Span{StartLine: parseError.Pos.Line, StartCol: parseError.Pos.Column, EndLine: parseError.Pos.Line, EndCol: parseError.Pos.Column}
				}
			}
		}
		code := diagnostic.Code("lint." + strings.ReplaceAll(fact.Key, "/", "."))
		if strings.HasPrefix(fact.Key, "claim/unproven/") {
			code = "lint.claim.unproven"
		}
		out = append(out, newDiagnostic(entry, span, code, string(fact.Value)))
	}
	diagnostic.Sort(out)
	return diagnostic.CoalesceSamePrimary(out), time.Since(started)
}

func moduleDiagnostic(entry Entry, imported string) diagnostic.Diagnostic {
	match := requirePattern.FindStringSubmatchIndex(entry.Source)
	span := firstSpan(entry.Source)
	if len(match) >= 4 {
		for _, item := range requirePattern.FindAllStringSubmatchIndex(entry.Source, -1) {
			if item[2] >= 0 && entry.Source[item[2]:item[3]] == imported {
				span = spanFromOffsets(entry.Source, item[2], item[3])
				break
			}
		}
	}
	return newDiagnostic(entry, span, "lint.module.unresolved", fmt.Sprintf("module %q is not resolved", imported))
}

func spanForFact(content string, span wir.Span) source.Span {
	if span.Valid() {
		return source.Span{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol}
	}
	return firstSpan(content)
}

func firstSpan(content string) source.Span {
	if content == "" {
		return source.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	}
	return source.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
}

func spanFromOffsets(content string, start, end int) source.Span {
	startLine, startCol := lineColumn(content, start)
	endLine, endCol := lineColumn(content, end)
	return source.Span{StartLine: startLine, StartCol: startCol, EndLine: endLine, EndCol: endCol}
}

func newDiagnostic(entry Entry, span source.Span, code diagnostic.Code, message string) diagnostic.Diagnostic {
	start := byteOffset(entry.Source, span.StartLine, span.StartCol)
	end := byteOffset(entry.Source, span.EndLine, span.EndCol)
	if end < start {
		end = start
	}
	location := embedding.SourceLocation{
		Document:      embedding.FileDocument(entry.Path),
		ContentDigest: embedding.DigestBytes([]byte(entry.Source)),
		Span:          embedding.ByteSpan{StartByte: start, EndByte: end},
		StartLine:     span.StartLine,
		StartColumn:   span.StartCol,
		EndLine:       span.EndLine,
		EndColumn:     span.EndCol,
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{Location: location, File: entry.Path, Span: span, Code: code, Message: message, Severity: diagnostic.SeverityError})
}

func lineColumn(content string, offset int) (line, column int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(content) {
		offset = len(content)
	}
	line, column = 1, 1
	for index := 0; index < offset; index++ {
		if content[index] == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
	}
	return line, column
}

func byteOffset(content string, line, column int) int {
	if line < 1 || column < 1 {
		return 0
	}
	currentLine, currentColumn := 1, 1
	for index := 0; index < len(content); index++ {
		if currentLine == line && currentColumn == column {
			return index
		}
		if content[index] == '\n' {
			currentLine, currentColumn = currentLine+1, 1
		} else {
			currentColumn++
		}
	}
	return len(content)
}
