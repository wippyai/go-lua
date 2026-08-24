package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/internal/cutovermanifest/inventory"
	"github.com/wippyai/go-lua/internal/cutovermanifest/residue"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("could not locate module root from %s: %v", wd, err)
	}
	return root
}

func TestBuildAndRenderFixtureEndToEnd(t *testing.T) {
	root := repoRoot(t)
	domainPkg := "internal/cutovermanifest/render/testdata/fixture"

	m, err := Build(root, domainPkg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	out := Render(m)

	wantContains := []string{
		"CUTOVER MANIFEST: " + domainPkg,
		"(1) FAMILY-OWNED CUT SUMMARY",
		"axis=fixture rule=fixture-rule",
		"fold FixtureReducer (key fixture/reducer)",
		"(2) CANONICAL KEYS INVENTORY",
		"reducer    FixtureReducer",
		"fixture/reducer",
		"(3) LEGACY FILES TO REMOVE",
		"legacy_hot_rule.go",
		"HotRule",
		"BindHot",
		"(4) VISIBLE MISMATCHES",
		"[confirmed]",
		"FixtureMissing",
		"(5) REQUIRED LAWS CHECKLIST",
		"member-definition key law",
		"once-per-invocation law",
		"seal law",
		"single-slot/single-family law",
		"refusal cases",
	}
	for _, want := range wantContains {
		if !strings.Contains(out, want) {
			t.Errorf("rendered manifest missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderMissingMemberDefinition(t *testing.T) {
	m := Manifest{
		DomainPkg: "domain/nowhere",
		Package: inventory.Package{
			ImportPath:             "domain/nowhere",
			MemberDefinitionRelDir: "domain/nowhere/memberdefinition",
			Present:                false,
		},
	}
	out := Render(m)
	if !strings.Contains(out, "no memberdefinition child package") {
		t.Errorf("expected an explicit absent-memberdefinition note, got:\n%s", out)
	}
	if !strings.Contains(out, "(3) LEGACY FILES TO REMOVE") || !strings.Contains(out, "none - no file") {
		t.Errorf("expected an empty legacy-files section even with no declarations, got:\n%s", out)
	}
	if !strings.Contains(out, "(4) VISIBLE MISMATCHES") || !strings.Contains(out, "none found") {
		t.Errorf("expected an empty mismatches section, got:\n%s", out)
	}
}

func TestRenderRequiresSolveMismatchIsLabeled(t *testing.T) {
	m := Manifest{
		DomainPkg: "domain/example",
		Package: inventory.Package{
			ImportPath: "domain/example",
			Present:    true,
			Declarations: []inventory.Declaration{{
				Axis: inventory.Field{Value: "example", Literal: true},
				Rule: inventory.Field{Value: "example-rule", Literal: true},
			}},
		},
		Mismatches: []inventory.Mismatch{
			{Row: "Relation R", Field: "CandidateProvider", Detail: "member key \"x\" not found", Confirmed: false},
			{Row: "Reducer F", Field: "Implementation", Detail: "no such function", Confirmed: true},
		},
	}
	out := Render(m)
	if !strings.Contains(out, "[requires solve] ") {
		t.Errorf("expected a requires-solve row, got:\n%s", out)
	}
	if !strings.Contains(out, "[confirmed] ") {
		t.Errorf("expected a confirmed row, got:\n%s", out)
	}
}

func TestRenderLegacyFilesSectionListsHits(t *testing.T) {
	m := Manifest{
		DomainPkg: "domain/example",
		RepoRoot:  "/repo",
		Package:   inventory.Package{Present: true},
		LegacyFiles: []residue.File{{
			Path: "/repo/domain/example/hot_rule.go",
			Hits: []residue.Hit{{File: "/repo/domain/example/hot_rule.go", Line: 5, Text: "type HotRule struct{}"}},
		}},
	}
	out := Render(m)
	if !strings.Contains(out, "domain/example/hot_rule.go") {
		t.Errorf("expected the legacy file path relative to repo root, got:\n%s", out)
	}
	if !strings.Contains(out, ":5: type HotRule struct{}") {
		t.Errorf("expected the residue line to be listed, got:\n%s", out)
	}
}
