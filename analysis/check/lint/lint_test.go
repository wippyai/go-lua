package lint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if consumer.Imports[0].Manifest == nil || consumer.Imports[0].Export != typ.Any {
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
