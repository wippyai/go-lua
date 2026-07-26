package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func authorityNames(view authorityView) []string {
	names := make([]string, len(view.Rules))
	for index, rule := range view.Rules {
		names[index] = string(rule)
	}
	return names
}

// TestAuthorityPrecedencePinned is the review surface for every displaced
// ladder. These are exact transcriptions, not a proposed unified order.
func TestAuthorityPrecedencePinned(t *testing.T) {
	valueViews := []struct {
		view authorityView
		want []string
	}{
		{
			view: valueAtReadAuthorities,
			want: []string{
				"select-payload",
				"declared-optional-map-read",
				"heap-member",
				"assertion-narrowed",
				"proven-sequence-index",
				"typed-path",
				"reconverged",
				"declared-sequence-index",
			},
		},
		{
			view: currentValueAuthorities,
			want: []string{
				"select-payload",
				"correlation-cone-current",
				"declared-optional-map-read",
				"heap-member",
				"assertion-narrowed",
				"proven-sequence-index",
				"reconverged",
				"typed-path",
				"shape-member",
				"declared-sequence-index",
			},
		},
	}
	for _, test := range valueViews {
		t.Run(test.view.Name, func(t *testing.T) {
			if got := authorityNames(test.view); !slices.Equal(got, test.want) {
				t.Fatalf("authority order = %q, want %q", got, test.want)
			}
			seen := make(map[string]bool, len(test.view.Rules))
			for _, rule := range test.view.Rules {
				name := string(rule)
				if name == "" || seen[name] {
					t.Fatalf("incomplete or duplicate authority rule: %q", rule)
				}
				seen[name] = true
			}
		})
	}

	typeViews := []struct {
		view authorityView
		want []string
	}{
		{
			view: runtimeIndexContainerTypeAuthorities,
			want: []string{
				"runtime-value",
				"current-summary",
				"cast-target",
				"typed-path",
				"declared-map",
				"declared-array",
				"keyed-component",
			},
		},
		{
			view: iteratorElementTypeAuthorities,
			want: []string{"runtime-value", "typed-path", "declared-type", "keyed-component"},
		},
		{
			view: instantiatedFormalTypeAuthorities,
			want: []string{"typed-path", "declared-type", "keyed-component"},
		},
		{
			view: integerTermTypeAuthorities,
			want: []string{"exact-current-value", "typed-path", "declared-type"},
		},
	}
	for _, test := range typeViews {
		t.Run(test.view.Name, func(t *testing.T) {
			if got := authorityNames(test.view); !slices.Equal(got, test.want) {
				t.Fatalf("authority order = %q, want %q", got, test.want)
			}
			seen := make(map[string]bool, len(test.view.Rules))
			for _, rule := range test.view.Rules {
				name := string(rule)
				if name == "" || seen[name] {
					t.Fatalf("incomplete or duplicate authority rule: %q", rule)
				}
				seen[name] = true
			}
		})
	}

	if got, want := authorityNames(placementDescentTableAuthorities), []string{"current-table-value", "typed-path"}; !slices.Equal(got, want) {
		t.Fatalf("%s authority order = %q, want %q", placementDescentTableAuthorities.Name, got, want)
	}
	for _, rule := range placementDescentTableAuthorities.Rules {
		if rule == "" {
			t.Fatalf("incomplete predicate authority rule: %q", rule)
		}
	}
}

// TestInheritedValueAuthoritySwapPinned states the unexplained inherited
// difference explicitly: value-at-read asks typed-path before reconverged,
// while value-current asks reconverged before typed-path. Unifying the two
// orders must therefore change this test and the registry data together.
func TestInheritedValueAuthoritySwapPinned(t *testing.T) {
	atRead := authorityNames(valueAtReadAuthorities)
	current := authorityNames(currentValueAuthorities)
	atReadTyped, atReadReconverged := slices.Index(atRead, "typed-path"), slices.Index(atRead, "reconverged")
	currentTyped, currentReconverged := slices.Index(current, "typed-path"), slices.Index(current, "reconverged")
	if atReadTyped+1 != atReadReconverged {
		t.Fatalf("value-at-read swap rows = typed-path:%d reconverged:%d, want typed-path immediately first", atReadTyped, atReadReconverged)
	}
	if currentReconverged+1 != currentTyped {
		t.Fatalf("value-current swap rows = reconverged:%d typed-path:%d, want reconverged immediately first", currentReconverged, currentTyped)
	}
}

// TestCurrentValueConsumerFailsClosedOnUnknownAuthority mutates the registry
// data presented to the real current-value consumer. A derived path normally
// reaches the conservative Top fallback; an unknown rule must stop that
// fallback instead of being skipped as though the registry were complete.
func TestCurrentValueConsumerFailsClosedOnUnknownAuthority(t *testing.T) {
	term := []byte("path/box.member")
	control, err := resolveCurrentValueWithAuthorities(term, equation.Partition{}, authorityView{Name: "control"})
	if err != nil || string(control) != "scalar/top" {
		t.Fatalf("control resolution = %q, %v; want conservative Top", control, err)
	}
	mutated := authorityView{
		Name:  "mutated-value-current",
		Rules: []authorityRule{"unknown-mutation"},
	}
	value, err := resolveCurrentValueWithAuthorities(term, equation.Partition{}, mutated)
	if err == nil || value != nil || !strings.Contains(err.Error(), "unknown authority rule") {
		t.Fatalf("mutated consumer = %q, %v; want fail-closed unknown-rule error", value, err)
	}
}

// TestAuthorityLaddersStayDisplaced is the source fence. The former consumers
// may select only their declared question view; authority implementations live
// behind the registry walk, and the split declared-container readers may not
// return anywhere in production.
func TestAuthorityLaddersStayDisplaced(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	requiredView := map[string]string{
		"resolveValue":                 "valueAtReadAuthorities",
		"resolveCurrentValue":          "currentValueAuthorities",
		"typedRuntimeIndexResult":      "runtimeIndexContainerTypeAuthorities",
		"iteratorElementWitness":       "iteratorElementTypeAuthorities",
		"instantiatedFormalType":       "instantiatedFormalTypeAuthorities",
		"integerTypedTerm":             "integerTermTypeAuthorities",
		"placementDescentTargetsTable": "placementDescentTableAuthorities",
	}
	forbiddenByConsumer := map[string]map[string]bool{
		"resolveValue": {
			"selectPayloadValue": true, "declaredOptionalMapReadValue": true, "heapMemberValue": true,
			"assertionNarrowedValue": true, "provenSequenceIndexValue": true, "typedPathValue": true,
			"reconvergedValue": true, "declaredSequenceIndexValue": true,
		},
		"resolveCurrentValue": {
			"selectPayloadValue": true, "correlationConeCurrentValue": true, "declaredOptionalMapReadValue": true,
			"heapMemberValue": true, "assertionNarrowedValue": true, "provenSequenceIndexValue": true,
			"reconvergedValue": true, "typedPathValue": true, "shapeMemberValue": true,
			"declaredSequenceIndexValue": true,
		},
		"typedRuntimeIndexResult": {
			"castTargetWitness": true, "typedPathType": true, "declaredContainerType": true,
			"keyedComponentContainerType": true,
		},
		"iteratorElementWitness": {
			"typedPathType": true, "declaredTypeForTerm": true, "keyedComponentContainerType": true,
		},
		"instantiatedFormalType": {
			"typedPathType": true, "declaredTypeForTerm": true, "keyedComponentContainerType": true,
		},
		"integerTypedTerm": {
			"resolveCurrentValue": true, "typedPathType": true, "declaredTypeForTerm": true,
		},
		"placementDescentTargetsTable": {
			"resolveCurrentValue": true, "typedPathType": true,
		},
	}
	found := make(map[string]bool, len(requiredView))
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(source), "declaredMapContainerType") || strings.Contains(string(source), "declaredArrayContainerType") {
			t.Errorf("%s resurrects a split declared-container ladder", name)
		}
		file, parseErr := parser.ParseFile(fset, name, source, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			view, guarded := requiredView[function.Name.Name]
			if !guarded {
				continue
			}
			found[function.Name.Name] = true
			hasView := false
			ast.Inspect(function.Body, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if !ok {
					return true
				}
				if identifier.Name == view {
					hasView = true
				}
				if forbiddenByConsumer[function.Name.Name][identifier.Name] {
					t.Errorf("%s directly reads authority %s outside its declared table", function.Name.Name, identifier.Name)
				}
				return true
			})
			if !hasView {
				t.Errorf("%s no longer walks declared view %s", function.Name.Name, view)
			}
		}
	}
	for function, view := range requiredView {
		if !found[function] {
			t.Errorf("guarded authority consumer %s using %s was deleted or renamed", function, view)
		}
	}
}
