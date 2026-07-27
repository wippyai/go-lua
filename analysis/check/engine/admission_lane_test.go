package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// TestAdmissionLanePrecedencePinned transcribes the effective priority of the
// legacy exclusion chain before that chain is removed. The overwrite lanes are
// ordered most-specific first; the fallback lanes retain each successive
// exclusion in the old chain. The sealed-capture lane also owns the chain's
// parameter-free/capture-free closed root policy.
func TestAdmissionLanePrecedencePinned(t *testing.T) {
	want := []string{
		"gradual-logical-call",
		"declared-local-union-read",
		"declared-indexed-read",
		"static-assignment",
		"typed-channel-send",
		"declared",
		"explicit-any",
		"static-captured-return",
		"static-arithmetic",
		"static-member-read",
		"declared-formal-call",
		"imported-capture",
		"sealed-capture",
		"contextual-callback",
	}
	if len(admissionLanes) != len(want) {
		t.Fatalf("admission lane count = %d, want %d", len(admissionLanes), len(want))
	}
	seen := make(map[string]bool, len(admissionLanes))
	for index, lane := range admissionLanes {
		if lane.Name != want[index] {
			t.Fatalf("admission lane %d = %q, want %q", index, lane.Name, want[index])
		}
		if lane.Precedence != index {
			t.Fatalf("admission lane %q precedence = %d, want slice index %d", lane.Name, lane.Precedence, index)
		}
		if seen[lane.Name] {
			t.Fatalf("duplicate admission lane %q", lane.Name)
		}
		seen[lane.Name] = true
		if lane.Admit == nil || len(lane.Discharges) == 0 {
			t.Fatalf("admission lane %q has an incomplete descriptor", lane.Name)
		}
	}
}

// TestAdmissionLaneVocabularyGuard prevents the displaced withholding battery
// from returning under another lane. A descriptor may name only registered
// diagnostic-family IDs; raw prefix strings belong in the central matcher.
func TestAdmissionLaneVocabularyGuard(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	foundDescriptors := false
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if identifier, ok := node.(*ast.Ident); ok && identifier.Name == "laneWithholds" {
				t.Errorf("%s: legacy laneWithholds vocabulary returned", fset.Position(identifier.Pos()))
			}
			literal, ok := node.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			key, ok := literal.Key.(*ast.Ident)
			if !ok || key.Name != "Discharges" {
				return true
			}
			foundDescriptors = true
			call, ok := literal.Value.(*ast.CallExpr)
			if !ok {
				t.Errorf("%s: Discharges must use the diagnostic-family registry API", fset.Position(literal.Value.Pos()))
				return true
			}
			constructor, ok := call.Fun.(*ast.Ident)
			if !ok || (constructor.Name != "NewDiagnosticFamilySet" && constructor.Name != "RegisteredDiagnosticFamilies") {
				t.Errorf("%s: Discharges bypasses the diagnostic-family registry API", fset.Position(call.Fun.Pos()))
				return true
			}
			for _, argument := range call.Args {
				id, registered := argument.(*ast.Ident)
				if !registered || !strings.HasPrefix(id.Name, "DiagnosticFamily") {
					t.Errorf("%s: Discharges contains a raw or unregistered diagnostic family", fset.Position(argument.Pos()))
				}
			}
			return true
		})
	}
	if !foundDescriptors {
		t.Fatal("admission lane descriptors not found")
	}
}

// TestAdmissionHelpersStayDescriptorOwned fences the displaced free-function
// battery. Stateful evaluator methods remain legitimate owners; a new
// freestanding uncalled admission predicate would recreate the parallel
// surface this lane removed.
func TestAdmissionHelpersStayDescriptorOwned(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var parsedFiles []*ast.File
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		parsedFiles = append(parsedFiles, file)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !strings.HasPrefix(function.Name.Name, "uncalled") {
				continue
			}
			t.Errorf("%s: freestanding admission helper %s must be a descriptor query", fset.Position(function.Pos()), function.Name.Name)
		}
	}
	for _, helper := range fenceFreestandingBoolCalls(parsedFiles, "selectAdmissionLane") {
		t.Errorf("selectAdmissionLane calls freestanding boolean helper %s; admission decisions must route through descriptor callbacks", helper)
	}
}

func TestAdmissionFenceRejectsRenamedFreestandingHelper(t *testing.T) {
	file, err := fenceParseSource(`package engine
func admissionRegressionHelper() bool { return false }
func selectAdmissionLane() bool {
	_ = admissionRegressionHelper()
	return false
}`)
	if err != nil {
		t.Fatal(err)
	}
	calls := fenceFreestandingBoolCalls([]*ast.File{file}, "selectAdmissionLane")
	if len(calls) != 1 || calls[0] != "admissionRegressionHelper" {
		t.Fatalf("renamed admission helper calls = %v, want admissionRegressionHelper", calls)
	}
}

// TestAdmissionConsumerRequiresBodyIndex is the consumer mutation proof. The
// explicit-any body is admitted from its declaration-owned seed projection;
// severing that projection at the allocation consumer must fail closed rather
// than letting any descriptor recompute the boundary from Compilation.
func TestAdmissionConsumerRequiresBodyIndex(t *testing.T) {
	compilation, err := front.Compile(`
local function validate(value: any): string
    local strict: string = value
    return strict
end
return validate`)
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.Nested) != 1 {
		t.Fatalf("nested bodies = %d, want 1", len(compilation.Nested))
	}
	child := compilation.Nested[0]
	bodyIndex := indexAdmissionBody(child)
	ctx := admissionLaneContext{child: child, bodyIndex: bodyIndex}
	decision, admitted := selectAdmissionLane(&ctx)
	if !admitted || decision.Lane == nil || decision.Lane.Name != "explicit-any" {
		name := ""
		if decision.Lane != nil {
			name = decision.Lane.Name
		}
		t.Fatalf("indexed admission = (%q, %v), want explicit-any", name, admitted)
	}

	ctx.bodyIndex = admissionBodyIndex{} // mutation: disconnect the consumer from its projection.
	if decision, admitted := selectAdmissionLane(&ctx); admitted {
		name := ""
		if decision.Lane != nil {
			name = decision.Lane.Name
		}
		t.Fatalf("admission survived missing body index through lane %q", name)
	}
}
