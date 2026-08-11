package architecture

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/render"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

const architectureModule = "example.com/architecturefixture"

func TestCompileDerivesContainmentIntentFromResolverSurvey(t *testing.T) {
	root := containmentFixture(t, false)
	declaration := fixtureDeclaration("model/link_state.go", architectureModule+"/model", "model")
	intent := compileFixture(t, root, declaration)
	if err := cutplan.ValidateIntent(intent); err != nil {
		t.Fatalf("generated intent: %v", err)
	}
	if len(intent.Operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(intent.Operations))
	}
	operation := intent.Operations[0]
	if operation.Authority != (cutplan.Authority{From: "link", To: "link-state"}) {
		t.Fatalf("authority = %#v", operation.Authority)
	}
	if len(operation.Edits) != 1 || operation.Edits[0].Relocate == nil {
		t.Fatalf("relocation = %#v", operation.Edits)
	}
	relocation := operation.Edits[0].Relocate
	if relocation.Source != "model/link.go" || relocation.Destination != (cutplan.Destination{Path: "model/link_state.go", Package: "model"}) {
		t.Fatalf("relocation source/destination = %#v", relocation)
	}
	if got, want := relocation.Subjects, []cutplan.Relocation{
		{From: field("model", "Link", "A"), To: field("model", "linkState", "A")},
		{From: field("model", "Link", "B"), To: field("model", "linkState", "B")},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("subjects = %#v, want %#v", got, want)
	}
	wantContainment := &cutplan.Containment{
		Parent:  packageObject("model", "Link"),
		Child:   packageObject("model", "linkState"),
		Through: field("model", "Link", "state"),
	}
	if got := relocation.Containment; !reflect.DeepEqual(got, wantContainment) {
		t.Fatalf("containment = %#v, want %#v", got, wantContainment)
	}
	if got, want := operation.Bindings, []cutplan.Binding{
		binding("consumer/use.go", "A"),
		binding("model/link.go", "A"),
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bindings = %#v, want %#v", got, want)
	}
	if len(operation.Imports) != 0 {
		t.Fatalf("same-package containment derived imports: %#v", operation.Imports)
	}
	if got, want := operation.Footprint.Read, []string{"consumer/use.go", "model/link.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("read footprint = %#v, want %#v", got, want)
	}
	if got, want := operation.Footprint.Write, []string{"consumer/use.go", "model/link.go", "model/link_state.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("write footprint = %#v, want %#v", got, want)
	}
	if got, want := operation.Verify.Gates, []cutplan.Gate{cutplan.GateDiagnostics, cutplan.GateImportDAG, cutplan.GateResidue}; !reflect.DeepEqual(got, want) {
		t.Fatalf("derived gates = %#v, want %#v", got, want)
	}
	routes, err := cutplan.ReferenceRouteRequirements(intent)
	if err != nil || len(routes) != 2 {
		t.Fatalf("generated relocation route denominator = %#v, %v", routes, err)
	}
}

func TestCompileDerivesOnlyNecessaryContainmentImport(t *testing.T) {
	root := containmentFixture(t, false)
	declaration := fixtureDeclaration("flow/link_state.go", architectureModule+"/flow", "flow")
	declaration.Destination.Child = "LinkState"
	declaration.Destination.Through = "State"
	intent := compileFixture(t, root, declaration)
	operation := intent.Operations[0]
	wantImports := []cutplan.Import{{
		Consumer: "model/link.go",
		To: &cutplan.ImportRef{
			Path: architectureModule + "/flow",
			Name: "flow",
		},
		Symbols: []cutplan.SymbolRef{packageObject("flow", "LinkState")},
	}}
	if got := operation.Imports; !reflect.DeepEqual(got, wantImports) {
		t.Fatalf("imports = %#v, want %#v", got, wantImports)
	}
	if got, want := operation.Footprint.Read, []string{"consumer/use.go", "model/link.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("read footprint = %#v, want %#v", got, want)
	}
}

// This is deliberately still a pure preflight: architecture produces the
// reviewed Intent, render produces a virtual tree, and semantic collection
// type-checks only that virtual tree. No repository file is ever applied.
func TestCompileCrossPackageIntentRendersAndTypeChecks(t *testing.T) {
	root := containmentFixture(t, false)
	declaration := fixtureDeclaration("flow/link_state.go", architectureModule+"/flow", "flow")
	declaration.Destination.Child = "LinkState"
	declaration.Destination.Through = "State"
	intent := compileFixture(t, root, declaration)

	session := newSurveySession(t, root)
	defer session.Close()
	source, err := session.Collect(context.Background(), intent, nil)
	if err != nil {
		t.Fatalf("collect source intent: %v", err)
	}
	output, err := render.Compile(render.Input{Intent: intent, Snapshot: source})
	if err != nil {
		t.Fatalf("render compiled cross-package intent: %v", err)
	}
	if _, err := session.CollectVirtual(context.Background(), intent, nil, output.Files); err != nil {
		t.Fatalf("type-check virtual cross-package intent: %v", err)
	}
}

func TestCompileRejectsIncompleteAmbiguousAndUnroutableBoundaries(t *testing.T) {
	root := containmentFixture(t, true)
	valid := fixtureDeclaration("model/link_state.go", architectureModule+"/model", "model")

	t.Run("incomplete source denominator", func(t *testing.T) {
		incomplete := valid
		incomplete.Fields = nil
		session := newSurveySession(t, root)
		defer session.Close()
		if _, err := CollectSurvey(context.Background(), session, incomplete); err == nil || !strings.Contains(err.Error(), "exact source fields") {
			t.Fatalf("incomplete declaration accepted: %v", err)
		}
	})

	t.Run("incomplete destination identity", func(t *testing.T) {
		survey := surveyFixture(t, root, valid)
		incomplete := valid
		incomplete.Destination.Child = ""
		if _, err := Compile(incomplete, survey); err == nil || !strings.Contains(err.Error(), "does not belong") {
			t.Fatalf("incomplete destination compiled: %v", err)
		}
	})

	t.Run("missing exact law", func(t *testing.T) {
		incomplete := valid
		incomplete.Laws = nil
		survey := surveyFixture(t, root, incomplete)
		if _, err := Compile(incomplete, survey); err == nil || !strings.Contains(err.Error(), "verify requires exact named laws") {
			t.Fatalf("lawless declaration compiled: %v", err)
		}
	})

	t.Run("existing destination with a different identity", func(t *testing.T) {
		ambiguous := valid
		ambiguous.Destination.Path = "other/existing.go"
		ambiguous.Destination.ImportPath = architectureModule + "/flow"
		ambiguous.Destination.Package = "flow"
		survey := surveyFixture(t, root, ambiguous)
		if _, err := Compile(ambiguous, survey); err == nil || !strings.Contains(err.Error(), "ambiguous or has different package identity") {
			t.Fatalf("ambiguous destination compiled: %v", err)
		}
	})

	t.Run("existing child target", func(t *testing.T) {
		existingRoot := containmentFixture(t, false)
		writeArchitectureFile(t, existingRoot, "model/existing.go", "package model\n\ntype linkState struct{}\n")
		if _, err := Compile(valid, surveyFixture(t, existingRoot, valid)); err == nil || !strings.Contains(err.Error(), "child target already exists") {
			t.Fatalf("existing child target compiled: %v", err)
		}
	})

	t.Run("cross-package private child", func(t *testing.T) {
		cross := valid
		cross.Destination.Path = "flow/link_state.go"
		cross.Destination.ImportPath = architectureModule + "/flow"
		cross.Destination.Package = "flow"
		cross.Destination.Through = "State"
		if _, err := Compile(cross, surveyFixture(t, containmentFixture(t, false), cross)); err == nil || !strings.Contains(err.Error(), "child linkState must be exported") {
			t.Fatalf("private cross-package child compiled: %v", err)
		}
	})

	t.Run("cross-package private through field", func(t *testing.T) {
		cross := valid
		cross.Destination.Path = "flow/link_state.go"
		cross.Destination.ImportPath = architectureModule + "/flow"
		cross.Destination.Package = "flow"
		cross.Destination.Child = "LinkState"
		if _, err := Compile(cross, surveyFixture(t, containmentFixture(t, false), cross)); err == nil || !strings.Contains(err.Error(), "through field state must be exported") {
			t.Fatalf("private cross-package through field compiled: %v", err)
		}
	})

	t.Run("external keyed literal", func(t *testing.T) {
		unroutableRoot := containmentFixture(t, false)
		writeArchitectureFile(t, unroutableRoot, "consumer/use.go", `package consumer

import "example.com/architecturefixture/model"

func Read() model.Link { return model.Link{A: 1} }
`)
		if _, err := Compile(valid, surveyFixture(t, unroutableRoot, valid)); err == nil || !strings.Contains(err.Error(), "source parent literal") {
			t.Fatalf("external keyed literal compiled: %v", err)
		}
	})

	t.Run("external unkeyed literal", func(t *testing.T) {
		unroutableRoot := containmentFixture(t, false)
		writeArchitectureFile(t, unroutableRoot, "consumer/use.go", `package consumer

import "example.com/architecturefixture/model"

func Read() model.Link { return model.Link{1, "two", true} }
`)
		if _, err := Compile(valid, surveyFixture(t, unroutableRoot, valid)); err == nil || !strings.Contains(err.Error(), "unkeyed Link literal outside containment source") {
			t.Fatalf("external unkeyed literal compiled: %v", err)
		}
	})

	t.Run("unsafe source literal layout", func(t *testing.T) {
		unroutableRoot := containmentFixture(t, false)
		writeArchitectureFile(t, unroutableRoot, "model/link.go", `package model

type Link struct { A int; B string; Keep bool }

func Local(link Link) int { return link.A }
func New() Link { return Link{A: 1, Keep: true, B: "two"} }
`)
		if _, err := Compile(valid, surveyFixture(t, unroutableRoot, valid)); err == nil || !strings.Contains(err.Error(), "interleaved") {
			t.Fatalf("unsafe source literal compiled: %v", err)
		}
	})
}

func compileFixture(t *testing.T, root string, declaration Declaration) cutplan.Intent {
	t.Helper()
	return mustCompile(t, declaration, surveyFixture(t, root, declaration))
}

func mustCompile(t *testing.T, declaration Declaration, survey Survey) cutplan.Intent {
	t.Helper()
	intent, err := Compile(declaration, survey)
	if err != nil {
		t.Fatalf("compile boundary: %v", err)
	}
	return intent
}

func surveyFixture(t *testing.T, root string, declaration Declaration) Survey {
	t.Helper()
	session := newSurveySession(t, root)
	defer session.Close()
	survey, err := CollectSurvey(context.Background(), session, declaration)
	if err != nil {
		t.Fatalf("collect survey: %v", err)
	}
	return survey
}

func newSurveySession(t *testing.T, root string) *semantic.Session {
	t.Helper()
	session, err := semantic.NewSession(semantic.Config{Root: root, Flashrefactor: "flashrefactor-v3"})
	if err != nil {
		t.Fatalf("semantic session: %v", err)
	}
	return session
}

func fixtureDeclaration(path, importPath, packageName string) Declaration {
	return Declaration{
		Name:     "link-state-cut",
		Boundary: Boundary{ID: "link-state", From: "link", To: "link-state"},
		Parent:   packageObject("model", "Link"),
		Fields: []cutplan.SymbolRef{
			field("model", "Link", "A"),
			field("model", "Link", "B"),
		},
		Destination: ContainmentDestination{
			Path:       path,
			ImportPath: importPath,
			Package:    packageName,
			Child:      "linkState",
			Through:    "state",
		},
		Laws: []cutplan.Law{{ID: "link-state", Package: "./model", Test: "TestLinkState"}},
	}
}

func packageObject(pkg, name string) cutplan.SymbolRef {
	return cutplan.SymbolRef{Object: architectureModule + "/" + pkg + "#package:" + name}
}

func field(pkg, owner, name string) cutplan.SymbolRef {
	return cutplan.SymbolRef{Object: architectureModule + "/" + pkg + "#type:" + owner + "/field:" + name}
}

func binding(consumer, name string) cutplan.Binding {
	return cutplan.Binding{
		Consumer: consumer,
		From:     field("model", "Link", name),
		To:       field("model", "linkState", name),
		Form:     cutplan.BindingField,
		Receiver: []cutplan.ReceiverPathStep{{Kind: cutplan.ReceiverField, Object: field("model", "Link", "state")}},
	}
}

func containmentFixture(t *testing.T, existingOther bool) string {
	t.Helper()
	root := t.TempDir()
	writeArchitectureFile(t, root, "go.mod", "module "+architectureModule+"\n\ngo 1.23.0\n")
	writeArchitectureFile(t, root, "model/link.go", `package model

type Link struct {
	A    int
	B    string
	Keep bool
}

func Local(link Link) int { return link.A }
func New() Link { return Link{A: 1, B: "two"} }
`)
	writeArchitectureFile(t, root, "consumer/use.go", `package consumer

import "example.com/architecturefixture/model"

func Read(link model.Link) int { return link.A }
`)
	if existingOther {
		writeArchitectureFile(t, root, "other/existing.go", "package other\n\ntype Existing struct{}\n")
	}
	return root
}

func writeArchitectureFile(t *testing.T, root, path, source string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}
