package generator

import (
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestTypeWalkSyntheticFixture(t *testing.T) {
	pkg := checkTypeWalkFixture(t)
	holder := pkg.Scope().Lookup("Holder").Type()
	options := TypeWalkOptions{Owner: "owner:test", Surface: "surface:holder"}
	facts, err := WalkType(holder, options)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Owner != "owner:test" || facts.Surface != "surface:holder" {
		t.Fatalf("context drift: %+v", facts)
	}
	if len(facts.Fields) < 5 {
		t.Fatalf("fields = %d, want direct and pointer-hidden state fields: %+v", len(facts.Fields), facts.Fields)
	}
	if hasFieldSurface(facts.Fields, "Anon.Inner", "surface:holder") || hasFieldSurface(facts.Fields, "Items[].Inner", "surface:holder") {
		t.Fatalf("anonymous fields merged into parent surface: %+v", facts.Fields)
	}
	if !hasFieldSurface(facts.Fields, "Anon.Inner", "surface:holder#Anon") || !hasFieldSurface(facts.Fields, "Items[].Inner", "surface:holder#Items[]") {
		t.Fatalf("anonymous synthetic surfaces missing: %+v", facts.Fields)
	}
	if !hasContainerSurface(facts.Containers, "Anon.Map", "surface:holder#Anon") {
		t.Fatalf("anonymous map container surface missing: %+v", facts.Containers)
	}
	if !hasReferenceSurface(facts.References, "Anon.Map{value}", "surface:holder#Anon") || !hasReferenceSurface(facts.References, "Items[].Inner", "surface:holder#Items[]") {
		t.Fatalf("anonymous nested reference surfaces missing: %+v", facts.References)
	}
	if !hasSurfaceRoot(facts.SurfaceRoots, "surface:holder#Anon", "surface:holder", "Anon") || !hasSurfaceRoot(facts.SurfaceRoots, "surface:holder#Items[]", "surface:holder", "Items[]") || !hasSurfaceRoot(facts.SurfaceRoots, "surface:holder#Empty", "surface:holder", "Empty") {
		t.Fatalf("anonymous surface roots missing: %+v", facts.SurfaceRoots)
	}
	for _, root := range facts.SurfaceRoots {
		if root.Kind != "anonymous-state" || root.Position <= 0 || root.Type == "" {
			t.Fatalf("inexact anonymous surface root: %+v", root)
		}
	}
	if hasField(facts.Fields, "State.Hidden") {
		t.Fatalf("named child state leaked into parent surface: %+v", facts.Fields)
	}
	if hasField(facts.Fields, "Foreign.Secret") {
		t.Fatalf("foreign state root expanded into owner fields: %+v", facts.Fields)
	}
	for _, container := range facts.Containers {
		if strings.HasPrefix(container.Path, "Foreign") {
			t.Fatalf("foreign named internals became containers: %+v", facts.Containers)
		}
	}
	for _, reference := range facts.References {
		if strings.HasPrefix(reference.Path, "Foreign.*{") || strings.Contains(reference.Path, "Foreign.*.") {
			t.Fatalf("foreign named internals became references: %+v", facts.References)
		}
	}
	if hasField(facts.Fields, "Rows[].Value") || hasField(facts.Fields, "Rows[].Next") {
		t.Fatalf("row members inflated authority fields: %+v", facts.Fields)
	}
	if !hasContainer(facts.Containers, "map", "Rows") {
		t.Fatalf("map container missing: %+v", facts.Containers)
	}
	mapFact := findContainer(facts.Containers, "map", "Rows")
	if mapFact.Key == "" || mapFact.Value == "" {
		t.Fatalf("map key/value discovery is incomplete: %+v", mapFact)
	}
	if !hasContainer(facts.Containers, "slice", "Items") {
		t.Fatalf("slice container missing: %+v", facts.Containers)
	}
	if !hasReferenceKind(facts.References, "method") {
		apiFacts, apiErr := WalkType(pkg.Scope().Lookup("API").Type(), TypeWalkOptions{Owner: "owner:test", Surface: "surface:api", Mode: WalkModeReference, OpenNamedRoot: true})
		if apiErr != nil || !hasReferenceKind(apiFacts.References, "method") {
			t.Fatalf("interface/signature references missing: %+v", facts.References)
		}
	}
	genericFacts, err := WalkType(pkg.Scope().Lookup("Generic").Type(), TypeWalkOptions{Owner: "owner:test", Surface: "surface:generic", Mode: WalkModeReference})
	if err != nil {
		t.Fatal(err)
	}
	numberFacts, err := WalkType(pkg.Scope().Lookup("Number").Type(), TypeWalkOptions{Owner: "owner:test", Surface: "surface:number", Mode: WalkModeReference, OpenNamedRoot: true})
	if err != nil {
		t.Fatal(err)
	}
	if !hasReferenceKindPrefix(numberFacts.References, "union-") {
		t.Fatalf("type-parameter/union references missing: %+v", genericFacts.References)
	}
	nodeFacts, err := WalkType(pkg.Scope().Lookup("Node").Type(), TypeWalkOptions{Owner: "owner:test", Surface: "surface:node"})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodeFacts.Cycles) == 0 {
		t.Fatalf("recursive generic Node did not produce a cycle fact")
	}
	for _, fact := range facts.Fields {
		if fact.Owner != "owner:test" || fact.Type == "" {
			t.Fatalf("non-canonical field fact: %+v", fact)
		}
	}
	seenNamedTarget := false
	for _, fact := range facts.References {
		if fact.Kind == "named" && fact.NamedName == "Base" {
			seenNamedTarget = true
			if fact.NamedPackagePath != "fixture" || fact.NamedOriginPackagePath != "fixture" || fact.NamedOriginName != "Base" {
				t.Fatalf("named target identity is not canonical: %+v", fact)
			}
		}
	}
	if !seenNamedTarget {
		t.Fatalf("named target identity was not emitted: %+v", facts.References)
	}
	// Repeating the walk must produce byte-for-byte deterministic facts.
	again, err := WalkType(holder, options)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := typeWalkSummary(again), typeWalkSummary(facts); got != want {
		t.Fatalf("nondeterministic facts:\n%s\nwant:\n%s", got, want)
	}
}

func TestTypeWalkRejectsUnknownShape(t *testing.T) {
	_, err := WalkType(unknownType{}, TypeWalkOptions{Owner: "owner", Surface: "surface", Mode: WalkModeReference})
	if err == nil || !errors.Is(err, ErrTypeWalkUnknown) {
		t.Fatalf("unknown type error = %v", err)
	}
	_, err = WalkType(nil, TypeWalkOptions{Owner: "owner", Surface: "surface"})
	if err == nil || !strings.Contains(err.Error(), ErrTypeWalkMalformed.Error()) {
		t.Fatalf("nil type error = %v", err)
	}
}

func TestTypeWalkAnonymousInterfaceMethodsUseStableSourceIdentity(t *testing.T) {
	pkg := checkTypeWalkFixture(t)
	facts, err := WalkType(pkg.Scope().Lookup("LocalInterfaces").Type(), TypeWalkOptions{Owner: "owner:ref", Surface: "surface:locals", Mode: WalkModeReference})
	if err != nil {
		t.Fatal(err)
	}
	keys := make(map[string]struct{})
	count := 0
	for _, fact := range facts.References {
		if fact.Kind != "interface-method-local" {
			continue
		}
		count++
		if fact.MethodKey == "" || fact.MethodSource == "" || fact.MethodReceiver != "" || fact.Type == "" {
			t.Fatalf("anonymous method lacks exact source evidence: %+v", fact)
		}
		if _, duplicate := keys[fact.MethodKey]; duplicate {
			t.Fatalf("anonymous methods collapsed onto one stable key: %+v", facts.References)
		}
		keys[fact.MethodKey] = struct{}{}
	}
	if count != 2 {
		t.Fatalf("anonymous method facts = %d, want 2: %+v", count, facts.References)
	}
}

func TestMethodReceiverEvidencePreservesDeclaredForeignInterface(t *testing.T) {
	pkg := types.NewPackage("example.test/external", "external")
	gcImporterReceiver := types.NewInterfaceType(nil, nil)
	gcImporterReceiver.Complete()
	signature := types.NewSignatureType(
		types.NewVar(token.NoPos, pkg, "", gcImporterReceiver),
		nil,
		nil,
		types.NewTuple(types.NewVar(token.NoPos, pkg, "value", types.Typ[types.Int])),
		types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.String])),
		false,
	)
	method := types.NewFunc(token.NoPos, pkg, "Shared", signature)
	first := methodReceiverEvidence(method, "example.test/external.ForeignAPI")
	second := methodReceiverEvidence(method, "example.test/external.ForeignAPI2")
	if first != "example.test/external.ForeignAPI" || second != "example.test/external.ForeignAPI2" {
		t.Fatalf("declared receiver was overwritten by imported signature receiver: first=%q second=%q", first, second)
	}
	firstKey := methodObjectKey(method, first, "")
	secondKey := methodObjectKey(method, second, "")
	if firstKey == "" || secondKey == "" || firstKey == secondKey {
		t.Fatalf("same-signature foreign methods collapsed: first=%q second=%q", firstKey, secondKey)
	}
}

func TestTypeWalkReferenceModeDoesNotInflateStateSurfaces(t *testing.T) {
	pkg := checkTypeWalkFixture(t)
	for _, test := range []struct {
		name string
		typ  types.Type
		open bool
	}{
		{"function", pkg.Scope().Lookup("Hostile").Type(), false},
		{"interface", pkg.Scope().Lookup("API").Type(), true},
		{"constraint", pkg.Scope().Lookup("Number").Type(), true},
		{"foreign-generic", pkg.Scope().Lookup("Use").Type(), false},
	} {
		facts, err := WalkType(test.typ, TypeWalkOptions{Owner: "owner:ref", Surface: "surface:" + test.name, Mode: WalkModeReference, OpenNamedRoot: test.open})
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if len(facts.Fields) != 0 {
			t.Fatalf("%s reference walk inflated state fields: %+v", test.name, facts.Fields)
		}
		if test.name == "function" {
			if len(facts.Containers) == 0 {
				t.Fatalf("reference signature lost typed containers: %+v", facts)
			}
			for _, container := range facts.Containers {
				if container.Kind == "array" && container.ArrayLen < 0 {
					t.Fatalf("reference array length is not canonical: %+v", container)
				}
			}
		}
		if len(facts.References) == 0 {
			t.Fatalf("%s reference evidence is empty", test.name)
		}
		if test.name == "interface" {
			if got := countReferenceKind(facts.References, "method"); got != 1 || countReferenceKind(facts.References, "method-reference") != 1 {
				t.Fatalf("embedded interface declarations were republished or lost: %+v", facts.References)
			}
			for _, ref := range facts.References {
				if ref.Kind != "method" && ref.Kind != "method-reference" {
					continue
				}
				if ref.Position != token.NoPos {
					t.Fatalf("foreign method retained owner-format-dependent token.Pos: %+v", ref)
				}
				if ref.MethodPackagePath != "fixture" || ref.MethodKey == "" || ref.MethodReceiver == "" || ref.Type == "" {
					t.Fatalf("interface method identity is incomplete: %+v", ref)
				}
			}
		}
		if len(facts.SurfaceRoots) != 0 {
			t.Fatalf("%s reference walk emitted surface roots: %+v", test.name, facts.SurfaceRoots)
		}
		for _, ref := range facts.References {
			if ref.Surface != "surface:"+test.name {
				t.Fatalf("%s reference acquired synthetic surface: %+v", test.name, ref)
			}
		}
	}
}

func checkTypeWalkFixture(t *testing.T) *types.Package {
	t.Helper()
	const source = `package fixture

type Base struct { Hidden int }
type Foreign struct { Secret int; Rows map[string]func(*Base) }
type Alias = *Base
type Node[T any] struct {
	Value T
	Next *Node[T]
	Rows map[*Node[T]]*Node[T]
}
type Embedded interface { Embedded(*Base) error }
type API interface { Embedded; Do(func(*Base), ...*Base) (*Base, error) }
type Number interface { ~int | ~string }
func Generic[T Number](value T) T { return value }
type Holder struct {
	State Alias
	Foreign *Foreign
	Root *Node[int]
	Rows map[*Base]*Node[int]
	Items []struct { Inner *Base }
	Anon struct { Inner *Base; Map map[string]*Base }
	Empty struct{}
	API API
}
func Hostile(fn func(struct { X int }, []struct { Y int })) {}
func LocalInterfaces(a interface { Same() }, b interface { Same() }) {}
type ForeignGeneric[T any] struct { Secret T }
type Use struct { Foreign ForeignGeneric[struct { Z int }] }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("fixture", fset, []*ast.File{file}, &types.Info{})
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

type unknownType struct{}

func (unknownType) String() string         { return "unknown" }
func (unknownType) Underlying() types.Type { return unknownType{} }

func hasField(fields []FieldFact, path string) bool {
	for _, field := range fields {
		if field.Path == path {
			return true
		}
	}
	return false
}

func hasFieldSurface(fields []FieldFact, path, surface string) bool {
	for _, field := range fields {
		if field.Path == path && field.Surface == surface {
			return true
		}
	}
	return false
}

func hasSurfaceRoot(roots []SurfaceRootFact, surface, parent, path string) bool {
	for _, root := range roots {
		if root.Surface == surface && root.ParentSurface == parent && root.Path == path {
			return true
		}
	}
	return false
}

func hasContainer(containers []ContainerFact, kind, path string) bool {
	for _, container := range containers {
		if container.Kind == kind && container.Path == path {
			return true
		}
	}
	return false
}

func hasContainerSurface(containers []ContainerFact, path, surface string) bool {
	for _, container := range containers {
		if container.Path == path && container.Surface == surface {
			return true
		}
	}
	return false
}

func findContainer(containers []ContainerFact, kind, path string) ContainerFact {
	for _, container := range containers {
		if container.Kind == kind && container.Path == path {
			return container
		}
	}
	return ContainerFact{}
}

func hasReferenceKind(references []ReferenceFact, kind string) bool {
	for _, reference := range references {
		if reference.Kind == kind {
			return true
		}
	}
	return false
}

func countReferenceKind(references []ReferenceFact, kind string) int {
	count := 0
	for _, reference := range references {
		if reference.Kind == kind {
			count++
		}
	}
	return count
}

func hasReferenceKindPrefix(references []ReferenceFact, prefix string) bool {
	for _, reference := range references {
		if strings.HasPrefix(reference.Kind, prefix) {
			return true
		}
	}
	return false
}

func hasReferenceSurface(references []ReferenceFact, path, surface string) bool {
	for _, reference := range references {
		if reference.Path == path && reference.Surface == surface {
			return true
		}
	}
	return false
}

func typeWalkSummary(facts TypeWalkFacts) string {
	return strings.TrimSpace(strings.Join([]string{
		fieldSummary(facts.Fields), containerSummary(facts.Containers), referenceSummary(facts.References), cycleSummary(facts.Cycles),
	}, "\n"))
}

func fieldSummary(fields []FieldFact) string {
	var out []string
	for _, field := range fields {
		out = append(out, field.Path+"="+field.Type)
	}
	return strings.Join(out, ";")
}
func containerSummary(containers []ContainerFact) string {
	var out []string
	for _, fact := range containers {
		out = append(out, fact.Path+"="+fact.Kind+":"+fact.Type+":"+fact.Element+":"+fact.Key+":"+fact.Value+":"+fact.Direction+":"+fmt.Sprint(fact.ArrayLen))
	}
	return strings.Join(out, ";")
}
func referenceSummary(references []ReferenceFact) string {
	var out []string
	for _, fact := range references {
		out = append(out, fact.Path+"="+fact.Kind+":"+fact.Type)
	}
	return strings.Join(out, ";")
}
func cycleSummary(cycles []CycleFact) string {
	var out []string
	for _, fact := range cycles {
		out = append(out, fact.Path+"="+fact.Type)
	}
	return strings.Join(out, ";")
}
