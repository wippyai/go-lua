package analysis

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestEngineProductionImportsRemainDomainBlind makes the engine/domain
// direction an executable architecture law. The root analysis package may
// compose domains with Engine; Engine itself may depend only on its generic
// implementation, the shared lattice, analysis-internal codecs, and the
// neutral ContentID/Term identity vocabulary. Keyspace carries no Program or
// Flow behavior and is the scalar ABI of mounted artifact receipts.
func TestEngineProductionImportsRemainDomainBlind(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location unavailable")
	}
	engineRoot := filepath.Join(filepath.Dir(current), "engine")
	const module = "github.com/wippyai/go-lua"
	allowed := func(path string) bool {
		return path == module+"/analysis/lattice" ||
			path == module+"/program/keyspace" ||
			strings.HasPrefix(path, module+"/analysis/engine") ||
			strings.HasPrefix(path, module+"/analysis/internal/")
	}
	err := filepath.WalkDir(engineRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imported := range parsed.Imports {
			value, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			if strings.HasPrefix(value, module+"/") && !allowed(value) {
				t.Errorf("engine production source %s imports semantic owner %s", filepath.Base(path), value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEngineProductionRemainsSemanticDiagnosticBlind keeps policy and linter
// vocabulary at the Analysis boundary. Engine may expose generic runtime
// telemetry, queries, and solve receipts; it may not learn source-diagnostic
// subjects, codes, severities, policies, reports, or rendered rule names.
func TestEngineProductionRemainsSemanticDiagnosticBlind(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location unavailable")
	}
	engineRoot := filepath.Join(filepath.Dir(current), "engine")
	forbiddenIdentifiers := map[string]struct{}{
		"DiagnosticCode":        {},
		"DiagnosticRule":        {},
		"DiagnosticPolicy":      {},
		"DiagnosticReport":      {},
		"DiagnosticObservation": {},
		"FindingSeverity":       {},
	}
	err := filepath.WalkDir(engineRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(source, []byte("advice.")) || bytes.Contains(source, []byte("lint.")) {
			t.Errorf("engine production source %s contains semantic diagnostic code text", filepath.Base(path))
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, isIdentifier := node.(*ast.Ident)
			if !isIdentifier {
				return true
			}
			if _, forbidden := forbiddenIdentifiers[identifier.Name]; forbidden {
				t.Errorf("engine production source %s contains semantic diagnostic type %s", filepath.Base(path), identifier.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestProductionDomainSchemasDoNotReopenProgramInteriors freezes the reusable
// transformer boundary. Domain construction may consume detached artifact and
// Link substitution receipts, but it may not reopen a mounted Program, Flow,
// TransformerInput, or raw authored Term after ProgramArtifact compilation.
func TestProductionDomainSchemasDoNotReopenProgramInteriors(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location unavailable")
	}
	domainRoot := filepath.Join(filepath.Dir(current), "domain")
	allowedOwnerVocabulary := map[string]map[string]struct{}{
		"github.com/wippyai/go-lua/program": {
			"AllocationForm": {}, "AllocationFormInvalid": {}, "AllocationFormEmpty": {},
			"AllocationFormClosed": {}, "AllocationFormFinalOpen": {},
			"AllocationInvalid": {}, "AllocationTable": {}, "AllocationClosure": {},
		},
		"github.com/wippyai/go-lua/program/flow": {
			"CallForm": {}, "CallFormPlain": {}, "CallFormMethod": {},
		},
	}
	forbiddenSelectors := map[string]struct{}{
		"Program":          {},
		"TransformerInput": {},
		"Flow":             {},
		"Authored":         {},
		"RootTerm":         {},
		"FieldTerm":        {},
		"SelectorTerm":     {},
	}
	err := filepath.WalkDir(domainRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if parseErr != nil {
			return parseErr
		}
		ownerImports := make(map[string]string)
		for _, imported := range parsed.Imports {
			value, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			if _, tracked := allowedOwnerVocabulary[value]; !tracked {
				continue
			}
			name := filepath.Base(value)
			if imported.Name != nil {
				name = imported.Name.Name
			}
			ownerImports[name] = value
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, isSelector := node.(*ast.SelectorExpr)
			if !isSelector {
				return true
			}
			if _, forbidden := forbiddenSelectors[selector.Sel.Name]; forbidden {
				t.Errorf("production domain source %s reopens Program interior selector %s", filepath.Base(path), selector.Sel.Name)
			}
			if owner, isOwner := selector.X.(*ast.Ident); isOwner {
				if owner.Name == "keyspace" && selector.Sel.Name == "Term" {
					t.Errorf("production domain source %s retains raw authored keyspace.Term", filepath.Base(path))
				}
				if imported, tracked := ownerImports[owner.Name]; tracked {
					if _, allowed := allowedOwnerVocabulary[imported][selector.Sel.Name]; !allowed {
						t.Errorf("production domain source %s consumes structural %s.%s instead of an artifact receipt", filepath.Base(path), owner.Name, selector.Sel.Name)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestProductionAnalysisUsesReceiptAssembly is the Flash cutover gate. Every
// production analysis package must use the sealed schema/receipt route; no
// dormant Program reconstruction or legacy engine facade may remain reachable.
func TestProductionAnalysisUsesReceiptAssembly(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location unavailable")
	}
	root := filepath.Dir(current)
	deleted := []string{
		"bodies.go",
		"canonical_solve.go",
		"composition.go",
		"coverage_production.go",
		"engine/activation_facade.go",
		"engine/activation_variant_plan.go",
		"engine/factor_edge.go",
		"engine/instance_assembly.go",
		"engine/query_instance.go",
		"engine/rule_instance.go",
		"engine/source_admission.go",
		"engine/source_assembly.go",
		"engine/source_control.go",
	}
	for _, legacy := range deleted {
		if _, statErr := os.Stat(filepath.Join(root, legacy)); !os.IsNotExist(statErr) {
			if statErr != nil {
				t.Fatalf("legacy assembly path %s: %v", legacy, statErr)
			}
			t.Errorf("deleted legacy assembly file is present: %s", legacy)
		}
	}
	// These domain owners used to reconstruct executable Program/Flow
	// coordinates. The receipt path has no production consumer for them; a
	// package returning would silently reopen the retired source boundary. Keep
	// the deletion structural rather than relying on an import check alone:
	// tests and tools can otherwise make a dead package look reachable again.
	deletedDomainPackages := []string{
		"domain/escape",
		"domain/module",
		"domain/numeric",
		"domain/ownership",
		"domain/suspension",
		"domain/transfer",
	}
	for _, legacy := range deletedDomainPackages {
		path := filepath.Join(root, legacy)
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			continue
		} else if statErr != nil {
			t.Fatalf("retired domain path %s: %v", legacy, statErr)
		}
		walkErr := filepath.WalkDir(path, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, relativeErr := filepath.Rel(root, path)
			if relativeErr != nil {
				return relativeErr
			}
			t.Errorf("retired domain package returned: %s", relative)
			return nil
		})
		if walkErr != nil {
			t.Fatalf("retired domain path %s: %v", legacy, walkErr)
		}
	}
	forbiddenName := func(identifier string) bool {
		switch {
		case identifier == "newProgramAnalysis", identifier == "SourceAssembly", identifier == "NewSourceAssembly", identifier == "ActivationPlan", identifier == "PreparedActivationPlan", identifier == "StageActivationPlan", identifier == "FinalizeActivationPlan":
			return true
		case strings.HasPrefix(identifier, "solveCanonicalBodies"), strings.HasPrefix(identifier, "compileAssembly"), strings.HasPrefix(identifier, "lowerAssembly"):
			return true
		case strings.HasPrefix(identifier, "fallbackProgram"), strings.HasPrefix(identifier, "fallbackAssembly"), strings.HasPrefix(identifier, "fallbackSolver"), strings.HasPrefix(identifier, "fallbackCanonical"), strings.HasPrefix(identifier, "fallbackAnalysis"), strings.HasPrefix(identifier, "legacyFallback"), strings.HasPrefix(identifier, "dualRun"), strings.HasPrefix(identifier, "dualSolve"):
			return true
		default:
			return false
		}
	}
	legacyText := [][]byte{
		// These cannot be represented as Go identifiers. Identifiers such as
		// fallbackProgram and dualRun are handled by forbiddenName below.
		[]byte("dual-run"),
		[]byte("dual run"),
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if filepath.Ext(path) != ".go" || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			return relativeErr
		}
		lower := bytes.ToLower(source)
		for _, forbidden := range legacyText {
			if bytes.Contains(lower, forbidden) {
				t.Errorf("production analysis %s retains forbidden legacy assembly text %q", relative, forbidden)
			}
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, isIdentifier := node.(*ast.Ident)
			if !isIdentifier {
				return true
			}
			if forbiddenName(identifier.Name) {
				t.Errorf("production analysis %s references forbidden legacy symbol %s", relative, identifier.Name)
			}
			return true
		})
		for _, declaration := range parsed.Decls {
			general, isGeneral := declaration.(*ast.GenDecl)
			if !isGeneral || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, isType := specification.(*ast.TypeSpec)
				if !isType || !typeSpec.Assign.IsValid() {
					continue
				}
				target := ""
				switch alias := typeSpec.Type.(type) {
				case *ast.Ident:
					target = alias.Name
				case *ast.SelectorExpr:
					target = alias.Sel.Name
				}
				if (typeSpec.Name.Name == "Binding" && target == "Composition") || (typeSpec.Name.Name == "Composition" && target == "Binding") {
					t.Errorf("production analysis %s aliases Binding and Composition", relative)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestArtifactSnapshotLowersOnlyIntoTheCachedProgramTemplate keeps the
// borrowed ingress view at the analysis boundary while proving that row
// translation occurs only during cache publication, never per Link receipt.
func TestArtifactSnapshotLowersOnlyIntoTheCachedProgramTemplate(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location unavailable")
	}
	root := filepath.Dir(current)
	path := filepath.Join(root, "artifact_plan.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	lowerCalls := 0
	templateLowerCalls := 0
	cacheTemplateCalls := 0
	for _, declaration := range parsed.Decls {
		function, isFunction := declaration.(*ast.FuncDecl)
		if !isFunction || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector {
				return true
			}
			owner, isOwner := selector.X.(*ast.Ident)
			if !isOwner || owner.Name != "artifactingress" || selector.Sel.Name != "Lower" {
				return true
			}
			lowerCalls++
			if function.Name.Name == "newEngineArtifactScalarTemplate" {
				templateLowerCalls++
			}
			return true
		})
		if function.Name.Name == "cachedProgramArtifact" {
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, isCall := node.(*ast.CallExpr)
				if !isCall {
					return true
				}
				identifier, isIdentifier := call.Fun.(*ast.Ident)
				if isIdentifier && identifier.Name == "newEngineArtifactScalarTemplate" {
					cacheTemplateCalls++
				}
				return true
			})
		}
	}
	if lowerCalls != 1 {
		t.Fatalf("artifact snapshot lowering calls = %d, want exactly one cache-publication call", lowerCalls)
	}
	if templateLowerCalls != 1 || cacheTemplateCalls != 1 {
		t.Fatalf("artifact template lowering/cache calls = %d/%d, want 1/1", templateLowerCalls, cacheTemplateCalls)
	}
	engineRoot := filepath.Join(root, "engine")
	err = filepath.WalkDir(engineRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		engineSource, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, forbidden := range [][]byte{
			[]byte("analysis/internal/artifactingress"),
			[]byte("map[uint8]RuleSlotCapability"),
			[]byte(".TagAt("),
			[]byte(".Tag()"),
		} {
			if bytes.Contains(engineSource, forbidden) {
				t.Errorf("engine production source %s retains producer-tag boundary %q", filepath.Base(path), forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
