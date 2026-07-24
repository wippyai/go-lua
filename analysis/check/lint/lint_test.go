package lint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCheckProjectChecksResolvedModuleTreeAndRendersPositions(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{
		{
			Path:       "app/provider.lua",
			ModulePath: "app.provider",
			Source:     "return {}\n",
		},
		{
			Path:       "app/consumer.lua",
			ModulePath: "app.consumer",
			Source: "local provider = require(\"app.provider\")\n" +
				"local text: string = 1\n",
		},
	}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("checked entries = %d, want 2", len(result.Entries))
	}
	consumer := result.Entries[0]
	if consumer.Entry.ModulePath != "app.consumer" {
		consumer = result.Entries[1]
	}
	if len(consumer.Imports) != 1 || consumer.Imports[0].ModulePath != "app.provider" {
		t.Fatalf("consumer imports = %#v", consumer.Imports)
	}
	if consumer.Imports[0].Manifest == nil || consumer.Imports[0].Export == typ.Any {
		t.Fatalf("module summary was not resolved through importlookup: %#v", consumer.Imports[0])
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one assignment diagnostic", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != "type.assignment" || diag.Position.File != "app/consumer.lua" || diag.Position.Line != 2 || diag.Position.Column != 7 {
		t.Fatalf("positional diagnostic = %#v", diag)
	}
	if got, want := RenderDiagnostic(diag), "app/consumer.lua:2:7: error[type.assignment]: cannot assign text because it is number, not string"; got != want {
		t.Fatalf("RenderDiagnostic = %q, want %q", got, want)
	}
	evidence := diag.Explanation.Evidence()
	if len(evidence) != 2 || evidence[0].Kind.String() != "abstract fact" || evidence[0].Trust.String() != "proven" || !strings.Contains(evidence[0].Message, "text has literal value 1") || evidence[1].Kind.String() != "user assertion" || evidence[1].Trust.String() != "claimed" || !strings.Contains(evidence[1].Message, "text is declared as string") {
		t.Fatalf("assignment explanation = %#v", evidence)
	}
	if len(diag.Labels) != 2 || !strings.Contains(diag.Help, "change the target type") {
		t.Fatalf("assignment labels/help = %#v / %q", diag.Labels, diag.Help)
	}
	if consumer.Timings.ParseBindLowerNS <= 0 || consumer.Timings.EvaluateNS <= 0 || result.Timings.ProjectRenderNS <= 0 {
		t.Fatalf("structured timings missing: entry=%#v project=%#v", consumer.Timings, result.Timings)
	}
}

func TestCheckProjectRetainsDistinctSameCodeDiagnostics(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{{
		Path:       "main.lua",
		ModulePath: "main",
		Source: `local first: string = 1
local second: string = 2
`,
	}}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want one assignment diagnostic at each source site", result.Diagnostics)
	}
	for index, item := range result.Diagnostics {
		if item.Code != "type.assignment" || item.Position.File != "main.lua" || item.Position.Line != index+1 {
			t.Fatalf("diagnostics = %#v, want source-ordered distinct assignment diagnostics", result.Diagnostics)
		}
	}
}

func TestCheckProjectPublishesImportedOwnershipStorePlacement(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{{
		Path:       "storage.lua",
		ModulePath: "storage",
		Source: `type Item = { child: { route: string } }
type Box = { items: {[string]: Item} }
local M = {}
function M.store_item(item: Item, box: Box)
  ownership.store(item, box)
end
return M`,
	}, {
		Path:       "main.lua",
		ModulePath: "main",
		Imports:    []string{"storage"},
		Source: `local storage = require("storage")
local box: {items: {[string]: {child: {route: string}}}} = { items = {} }
local item: {child: {route: string}} = { child = { route = "owned" } }
storage.store_item(item, box)`,
	}}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 6 {
		t.Fatalf("placement = %#v, want six module and consumer allocations", result.Placement)
	}
	owned, blocked := 0, 0
	for _, item := range result.Placement.Allocations {
		if item.Placement.String() == "owned-heap" && item.OwnerIdentity {
			owned++
		}
		blocked += len(item.Blockers)
	}
	if owned < 4 || blocked != 0 {
		t.Fatalf("placement = %#v, want four imported owned allocations and no opaque blockers", result.Placement)
	}
}

func TestCheckProjectOmitsFrameLocalClosureCapabilityBesideDataSite(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{{
		Path:       "main.lua",
		ModulePath: "main",
		Source: `local scratch = { value = 1 }
local callback = function(): integer
    return scratch.value
end`,
	}}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 1 {
		t.Fatalf("placement = %#v, want only the materialized scratch table", result.Placement)
	}
	item := result.Placement.Allocations[0]
	if item.Kind != "lua.table" || !item.FrameLocal {
		t.Fatalf("allocation = %#v, want the materialized table without its closure capability", item)
	}
}

func TestCheckProjectPublishesClosedScalarReturnPlacementWitness(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{{
		Path:       "main.lua",
		ModulePath: "main",
		Source: `local function count(): integer
    return 1
end
return count`,
	}}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if result.Placement == nil {
		t.Fatal("placement = nil, want a closed scalar-return witness")
	}
	for _, allocation := range result.Placement.Allocations {
		if allocation.Identity == "return-scalar/main" && allocation.Kind == "lua.scalar" && allocation.Placement.String() == "stack" && allocation.FrameLocal {
			return
		}
	}
	t.Fatalf("placement = %#v, want a closed scalar-return stack witness", result.Placement)
}

func TestCheckProjectPublishesClosedScalarMemberReturnPlacementWitness(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{{
		Path:       "main.lua",
		ModulePath: "main",
		Source: `local M = {}
function M.count(): integer
    return 1
end
return M`,
	}}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if result.Placement == nil {
		t.Fatal("placement = nil, want a closed scalar-member-return witness")
	}
	for _, allocation := range result.Placement.Allocations {
		if allocation.Identity == "return-scalar/main/count" && allocation.Kind == "lua.scalar" && allocation.Placement.String() == "stack" && allocation.FrameLocal {
			return
		}
	}
	t.Fatalf("placement = %#v, want a closed scalar-member-return stack witness", result.Placement)
}

func TestCheckProjectResolvesAProviderExportInsteadOfAny(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{
		{Path: "provider.lua", ModulePath: "provider", Source: `return { answer = 42 }`},
		{Path: "main.lua", ModulePath: "main", Source: `local provider = require("provider")`},
	}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	var consumer EntryResult
	for _, entry := range result.Entries {
		if entry.Entry.ModulePath == "main" {
			consumer = entry
		}
	}
	if len(consumer.Imports) != 1 || consumer.Imports[0].Export == typ.Any {
		t.Fatalf("consumer imports = %#v, want typed provider export", consumer.Imports)
	}
	record, ok := consumer.Imports[0].Export.(*typ.Record)
	if !ok || record.GetField("answer") == nil || record.GetField("answer").Type.String() != "42" {
		t.Fatalf("provider export = %T %[1]v, want answer: 42", consumer.Imports[0].Export)
	}
}

func TestCheckProjectSeedsResolvedExportIntoRequireResult(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{
		{Path: "provider.lua", ModulePath: "provider", Source: `return { answer = 42 }`},
		{Path: "main.lua", ModulePath: "main", Source: `local provider = require("provider")
local answer: string = provider.answer
`},
	}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one imported-member assignment diagnostic", result.Diagnostics)
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.Code != "type.assignment" || diagnostic.Position.File != "main.lua" || diagnostic.Position.Line != 2 || !strings.Contains(diagnostic.Message, "42") || !strings.Contains(diagnostic.Message, "string") {
		t.Fatalf("seeded require diagnostic = %#v", diagnostic)
	}
}

func TestCheckProjectSeedsPublishedCallableMemberResult(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{
		{Path: "provider.lua", ModulePath: "provider", Source: `local M = {}
function M.answer(): number return 42 end
return M`},
		{Path: "main.lua", ModulePath: "main", Source: `local provider = require("provider")
local answer: string = provider.answer()
`},
	}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "type.assignment" || !strings.Contains(result.Diagnostics[0].Message, "number") || !strings.Contains(result.Diagnostics[0].Message, "string") {
		t.Fatalf("published callable import diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckProjectRehydratesImportedQualifiedTypeDefinitions(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{
		{Path: "provider.lua", ModulePath: "provider", Source: `
type User = { id: string }
local M = {}
M.User = User
function M.make(): User return { id = "ok" } end
return M
`},
		{Path: "main.lua", ModulePath: "main", Source: `
local provider = require("provider")
local user: provider.User = provider.make()
local wrong: number = user.id
`},
	}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "type.assignment" || !strings.Contains(result.Diagnostics[0].Message, "string") || !strings.Contains(result.Diagnostics[0].Message, "number") {
		t.Fatalf("diagnostics = %#v, want imported qualified-field assignment diagnostic", result.Diagnostics)
	}
}

func TestCheckProjectPublishesRecursiveTypesFromTheirDefiningResolver(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{
		{Path: "store.lua", ModulePath: "store", Source: `
type Store = {
    entries: {[string]: string},
    put: (self: Store, key: string, value: string) -> (),
}
local Store = {}
function Store:put(key: string, value: string)
    self.entries[key] = value
end
local M = {}
function M.new(): Store
    return { entries = {}, put = Store.put }
end
return M
`},
		{Path: "main.lua", ModulePath: "main", Source: `
local store = require("store")
local value: store.Store = store.new()
value:put("key", "value")
`},
	}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
	var provider EntryResult
	for _, entry := range result.Entries {
		if entry.Entry.ModulePath == "store" {
			provider = entry
			break
		}
	}
	if provider.Manifest == nil || provider.Manifest.Types["Store"] == nil {
		t.Fatalf("provider manifest = %#v, want published Store definition", provider.Manifest)
	}
	if provider.Engine.TypeDefinitions["Store"] != provider.Manifest.Types["Store"] {
		t.Fatal("manifest Store must retain the provider resolver's declaration graph")
	}
	var consumer EntryResult
	for _, entry := range result.Entries {
		if entry.Entry.ModulePath == "main" {
			consumer = entry
			break
		}
	}
	if len(consumer.Imports) != 1 || consumer.Imports[0].Export == nil {
		t.Fatalf("consumer imports = %#v, want scoped store export", consumer.Imports)
	}
	export, ok := consumer.Imports[0].Export.(*typ.Record)
	if !ok {
		t.Fatalf("scoped export = %T/%v, want record", consumer.Imports[0].Export, consumer.Imports[0].Export)
	}
	newMember := export.GetField("new")
	newFn, ok := newMember.Type.(*typ.Function)
	if !ok || len(newFn.Returns) != 1 || newFn.Returns[0] != provider.Manifest.Types["Store"] {
		t.Fatalf("scoped new = %#v, want provider Store identity %v", newMember, provider.Manifest.Types["Store"])
	}
}

func TestDiagnosticPolicyConfiguresOptionalHintsAndSeverity(t *testing.T) {
	input := []diagnostic.Diagnostic{
		{Code: "lint.condition.redundant", Severity: diagnostic.SeverityError},
		{Code: "type.assignment", Severity: diagnostic.SeverityError},
	}
	policy := diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		"lint.condition.redundant": diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
		"type.assignment":          diagnostic.Disable(),
	}}
	got := applyDiagnosticPolicy(input, nil, policy)
	if len(got) != 1 {
		t.Fatalf("policy diagnostics = %#v, want one", got)
	}
	if got[0].Code != "lint.condition.redundant" || got[0].Severity != diagnostic.SeverityHint {
		t.Fatalf("configured optional hint = %#v, want redundant hint", got[0])
	}
	if input[0].Severity != diagnostic.SeverityError {
		t.Fatalf("policy mutated input diagnostic: %#v", input[0])
	}
}

func TestDiagnosticRulesCompatibilityOptInRemainsSupported(t *testing.T) {
	input := []diagnostic.Diagnostic{{Code: "send.isolation", Severity: diagnostic.SeverityHint}}
	if got := applyDiagnosticPolicy(input, nil, diagnostic.Policy{}); len(got) != 0 {
		t.Fatalf("unconfigured optional hint = %#v, want suppressed", got)
	}
	got := applyDiagnosticPolicy(input, map[diagnostic.Code]bool{"send.isolation": true}, diagnostic.Policy{})
	if len(got) != 1 || got[0].Code != "send.isolation" {
		t.Fatalf("compatibility opt-in = %#v, want send-isolation hint", got)
	}
}

func TestCheckProjectReportsUnresolvedRequireAtRequirePosition(t *testing.T) {
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{{
		Path:       "main.lua",
		ModulePath: "main",
		Source:     "local missing = require(\"does.not.exist\")\n",
	}}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != "lint.module.unresolved" || diag.Position.File != "main.lua" || diag.Position.Line != 1 || diag.Position.Column < 1 {
		t.Fatalf("unresolved module diagnostic = %#v", diag)
	}
}

func TestLoadDirectoryEndToEndMultiModuleTree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "provider.lua"), []byte("return {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("local provider = require('pkg.provider')\nlocal n: number = 'bad'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := LoadDirectory(root, nil)
	if err != nil {
		t.Fatalf("LoadDirectory: %v", err)
	}
	result, err := CheckProject(context.Background(), project)
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Entries) != 2 || len(result.Diagnostics) != 1 {
		t.Fatalf("end-to-end result = %#v", result)
	}
	if !strings.HasPrefix(RenderDiagnostic(result.Diagnostics[0]), "main.lua:2:7:") {
		t.Fatalf("rendered position = %q", RenderDiagnostic(result.Diagnostics[0]))
	}
}

func TestLoadDirectoryExpandsSelectedEntryToItsLocalImportClosure(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "provider.lua"), []byte("return {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("local provider = require('pkg.provider')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := LoadDirectory(root, []string{"main.lua"})
	if err != nil {
		t.Fatalf("LoadDirectory: %v", err)
	}
	if len(project.Entries) != 2 || len(project.Targets) != 1 || project.Targets[0] != "main" {
		t.Fatalf("selected project = %#v", project)
	}
	result, err := CheckProject(context.Background(), project)
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Entries) != 2 || len(result.Diagnostics) != 0 {
		t.Fatalf("selected closure result = %#v", result)
	}
}
