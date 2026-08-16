package generator

import (
	"fmt"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestStructureProjectionTypedPlanesAndJoins(t *testing.T) {
	root := t.TempDir()
	writeProjectionFixture(t, root)
	first, inv := loadStructureProjectionFixture(t, root)
	second, _ := loadStructureProjectionFixture(t, root)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("independent FileSet projections differ: first=%+v second=%+v", first, second)
	}

	if len(first.Fields) == 0 || len(first.NamedReferences) == 0 || len(first.OtherReferences) == 0 {
		t.Fatalf("typed projection planes are incomplete: %+v", first)
	}
	for _, field := range first.Fields {
		if field.FactID != structureFieldFactID(field) || field.DeclarationID == "" || field.SurfaceID == "" {
			t.Fatalf("inexact field plane row: %+v", field)
		}
	}
	for _, field := range first.Fields {
		if field.FactID == "" {
			t.Fatalf("field row has no stable identity: %+v", field)
		}
	}
	for _, declaration := range inv.Declarations {
		if declaration.Kind != "func" && declaration.Kind != "method" && declaration.Kind != "var" && declaration.Kind != "const" {
			continue
		}
		for _, row := range first.Arrays {
			if row.DeclarationID == declaration.FactID && row.SurfaceID != "" {
				t.Fatalf("declaration array reused a declaration as SurfaceID: %+v", row)
			}
		}
		for _, row := range first.Slices {
			if row.DeclarationID == declaration.FactID && row.SurfaceID != "" {
				t.Fatalf("declaration slice reused a declaration as SurfaceID: %+v", row)
			}
		}
		for _, row := range first.Maps {
			if row.DeclarationID == declaration.FactID && row.SurfaceID != "" {
				t.Fatalf("declaration map reused a declaration as SurfaceID: %+v", row)
			}
		}
		for _, row := range first.Channels {
			if row.DeclarationID == declaration.FactID && row.SurfaceID != "" {
				t.Fatalf("declaration channel reused a declaration as SurfaceID: %+v", row)
			}
		}
	}

	child := findNamedReference(t, first.NamedReferences, "example.test/program/link", "Child")
	if child.TargetDeclID == "" {
		t.Fatalf("internal named reference did not join DeclarationInfo: %+v", child)
	}
	foreign := findNamedReference(t, first.NamedReferences, "example.test/external", "Foreign")
	if foreign.TargetDeclID != "" {
		t.Fatalf("foreign named reference fabricated a declaration: %+v", foreign)
	}
	constraint := findNamedReference(t, first.NamedReferences, "example.test/program/link", "Child")
	if constraint.TargetDeclID == "" {
		t.Fatalf("generic constraint named target was not joined: %+v", constraint)
	}
	methodCount := 0
	for _, row := range first.MethodReferences {
		if row.Path == "Do" && row.Receiver == "example.test/program/link.API" {
			methodCount++
			if row.TargetDeclID == "" {
				t.Fatalf("embedded/explicit method did not join exact interface declaration: %+v", row)
			}
		}
	}
	if methodCount != 1 {
		t.Fatalf("explicit interface method was not emitted exactly once: %d rows=%+v", methodCount, first.MethodReferences)
	}

	if len(first.Arrays) == 0 || len(first.Slices) == 0 || len(first.Maps) == 0 || len(first.Channels) == 0 {
		t.Fatalf("typed container planes are incomplete: arrays=%+v slices=%+v maps=%+v channels=%+v", first.Arrays, first.Slices, first.Maps, first.Channels)
	}
	var array StructureArray
	for _, row := range first.Arrays {
		if row.SurfaceID != "" {
			array = row
			break
		}
	}
	if array.FactID == "" || array.Length < 0 || array.Element == "" {
		t.Fatalf("array length/element was not explicit: %+v", array)
	}
	var slice StructureSlice
	for _, row := range first.Slices {
		if row.Element != "" {
			slice = row
			break
		}
	}
	mapRow := first.Maps[0]
	channel := first.Channels[0]
	if slice.FactID == "" || mapRow.Key == "" || mapRow.Value == "" || channel.Element == "" || channel.Direction == "" {
		t.Fatalf("container payload was erased: slice=%+v map=%+v channel=%+v", slice, mapRow, channel)
	}
	mutatedArray := array
	mutatedArray.Element += ".changed"
	if structureArrayFactID(mutatedArray) == array.FactID {
		t.Fatal("array element mutation did not change FactID")
	}
	mutatedArray = array
	mutatedArray.Length++
	if structureArrayFactID(mutatedArray) == array.FactID {
		t.Fatal("array length mutation did not change FactID")
	}
	mutatedSlice := slice
	mutatedSlice.Element += ".changed"
	if structureSliceFactID(mutatedSlice) == slice.FactID {
		t.Fatal("slice element mutation did not change FactID")
	}
	mutatedMap := mapRow
	mutatedMap.Key += ".changed"
	if structureMapFactID(mutatedMap) == mapRow.FactID {
		t.Fatal("map key mutation did not change FactID")
	}
	mutatedMap = mapRow
	mutatedMap.Value += ".changed"
	if structureMapFactID(mutatedMap) == mapRow.FactID {
		t.Fatal("map value mutation did not change FactID")
	}
	mutatedChannel := channel
	mutatedChannel.Element += ".changed"
	if structureChannelFactID(mutatedChannel) == channel.FactID {
		t.Fatal("channel element mutation did not change FactID")
	}
	mutatedChannel = channel
	mutatedChannel.Direction += ".changed"
	if structureChannelFactID(mutatedChannel) == channel.FactID {
		t.Fatal("channel direction mutation did not change FactID")
	}

	seenSignature := false
	seenTuple := false
	for _, row := range first.OtherReferences {
		if row.Disposition == 0 || row.FactID != structureOtherReferenceFactID(row) {
			t.Fatalf("open/generic other disposition: %+v", row)
		}
		seenSignature = seenSignature || row.Disposition == OtherSignature
		seenTuple = seenTuple || row.Disposition == OtherTuple
	}
	if !seenSignature || !seenTuple {
		t.Fatalf("signature/tuple dispositions were erased: %+v", first.OtherReferences)
	}
	for _, row := range first.Cycles {
		if row.FactID != structureCycleFactID(row) || row.DeclarationID == "" {
			t.Fatalf("inexact cycle row: %+v", row)
		}
	}
}

func TestStructureProjectionPreservesExactReferenceEvidence(t *testing.T) {
	root := t.TempDir()
	writeProjectionFixture(t, root)
	projection, inv := loadStructureProjectionFixture(t, root)

	localMethods := 0
	for _, row := range projection.OtherReferences {
		if row.Type == "" || row.FactID != structureOtherReferenceFactID(row) {
			t.Fatalf("other reference erased canonical type: %+v", row)
		}
		if row.Disposition != OtherInterfaceMethod {
			continue
		}
		localMethods++
		mutated := row
		mutated.Type += ".changed"
		if structureOtherReferenceFactID(mutated) == row.FactID {
			t.Fatalf("other reference type mutation did not change FactID: %+v", row)
		}
	}
	if localMethods < 2 {
		t.Fatalf("anonymous/signature-local methods were not retained as closed evidence: %+v", projection.OtherReferences)
	}
	for _, declaration := range inv.Declarations {
		if declaration.Kind == "interface-method" && declaration.Name == "Local" {
			t.Fatalf("anonymous interface method fabricated DeclarationInfo: %+v", declaration)
		}
	}

	internal := make(map[string]StructureMethodReference)
	foreign := make(map[string]StructureMethodReference)
	var universe StructureMethodReference
	for _, row := range projection.MethodReferences {
		if row.MethodKey == "" || row.Type == "" || row.Receiver == "" || row.FactID != structureMethodReferenceFactID(row) {
			t.Fatalf("method reference erased exact evidence: %+v", row)
		}
		switch row.Receiver {
		case "example.test/program/link.API", "example.test/program/link.API2":
			if row.TargetDeclID == "" {
				t.Fatalf("named internal method did not join declaration: %+v", row)
			}
			internal[row.Receiver] = row
		case "example.test/external.ForeignAPI", "example.test/external.ForeignAPI2":
			if row.TargetDeclID != "" {
				t.Fatalf("foreign method fabricated declaration: %+v", row)
			}
			foreign[row.Receiver] = row
		case universePackagePath + ".error":
			universe = row
		}
	}
	if len(internal) != 2 || internal["example.test/program/link.API"].MethodKey == internal["example.test/program/link.API2"].MethodKey {
		t.Fatalf("named-interface receiver identity collapsed internal methods: %+v", internal)
	}
	if len(foreign) != 2 || foreign["example.test/external.ForeignAPI"].MethodKey == foreign["example.test/external.ForeignAPI2"].MethodKey {
		t.Fatalf("named-interface receiver identity collapsed foreign methods: %+v", foreign)
	}
	if universe.FactID == "" || universe.TargetPackagePath != universePackagePath || universe.TargetName != "Error" || universe.TargetDeclID != "" {
		t.Fatalf("predeclared error.Error identity was lost: universe=%+v methods=%+v", universe, projection.MethodReferences)
	}
	errorType := findNamedReference(t, projection.NamedReferences, universePackagePath, "error")
	if errorType.TargetDeclID != "" {
		t.Fatalf("predeclared error fabricated a declaration: %+v", errorType)
	}

	method := internal["example.test/program/link.API"]
	for name, mutate := range map[string]func(*StructureMethodReference){
		"key":      func(row *StructureMethodReference) { row.MethodKey += ".changed" },
		"type":     func(row *StructureMethodReference) { row.Type += ".changed" },
		"receiver": func(row *StructureMethodReference) { row.Receiver += ".changed" },
	} {
		mutated := method
		mutate(&mutated)
		if structureMethodReferenceFactID(mutated) == method.FactID {
			t.Fatalf("method %s mutation did not change FactID", name)
		}
	}
}

func TestMethodDeclarationTargetsRejectsStableKeyCollision(t *testing.T) {
	pkg := types.NewPackage("fixture", "fixture")
	signature := types.NewSignatureType(nil, nil, nil, types.NewTuple(), types.NewTuple(), false)
	firstObject := types.NewFunc(token.NoPos, pkg, "M", signature)
	secondObject := types.NewFunc(token.NoPos, pkg, "M", signature)
	first := DeclarationInfo{FactID: "first", PackagePath: "fixture", Kind: "interface-method", OwnerType: "API"}
	second := DeclarationInfo{FactID: "second", PackagePath: "fixture", Kind: "interface-method", OwnerType: "API"}
	inv := declarationInventory{byObject: map[types.Object][]string{firstObject: {first.FactID}, secondObject: {second.FactID}}}
	if _, err := methodDeclarationTargets(inv, map[string]DeclarationInfo{first.FactID: first, second.FactID: second}); err == nil {
		t.Fatal("colliding stable method identities were accepted")
	}
}

func TestStructureProjectionRejectsInvalidNonzeroProvenance(t *testing.T) {
	root := t.TempDir()
	writeProjectionFixture(t, root)
	family, shapes, inv, surfaces := loadStructureProjectionInputs(t, root)
	mutated := append([]TypeShapeInfo(nil), shapes...)
	for index := range mutated {
		if mutated[index].Name != "Link" {
			continue
		}
		facts := mutated[index].Facts
		facts.Fields = append([]FieldFact(nil), facts.Fields...)
		if len(facts.Fields) == 0 {
			t.Fatal("fixture Link has no field provenance")
		}
		facts.Fields[0].Position = token.Pos(1 << 30)
		mutated[index].Facts = facts
	}
	if _, err := structureProjection(root, family, mutated, inv, surfaces); err == nil {
		t.Fatal("invalid nonzero provenance was accepted")
	}
}

func TestStructureProjectionOwnerLookupScalesWithUnrelatedDeclarations(t *testing.T) {
	root := t.TempDir()
	writeProjectionFixture(t, root)
	family, shapes, inv, surfaces := loadStructureProjectionInputs(t, root)
	base, baseWitness, err := structureProjectionWithWitness(root, family, shapes, inv, surfaces)
	if err != nil {
		t.Fatal(err)
	}
	if baseWitness.OwnerQueries == 0 || baseWitness.PrefixVisits == 0 || baseWitness.IndexEntries == 0 {
		t.Fatalf("lookup witness did not observe work: %+v", baseWitness)
	}
	extended := inv
	extended.Declarations = append([]DeclarationInfo(nil), inv.Declarations...)
	for index := 0; index < len(inv.Declarations); index++ {
		declaration := DeclarationInfo{PackagePath: "example.test/program/link", Kind: "type", Name: fmt.Sprintf("Unrelated%04d", index), Type: "struct{}", SourceFile: "program/link/link.go", Line: 1000 + index, Column: 1}
		declaration.FactID = declarationFactID(declaration)
		extended.Declarations = append(extended.Declarations, declaration)
	}
	got, gotWitness, err := structureProjectionWithWitness(root, family, shapes, extended, surfaces)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(base, got) {
		t.Fatal("unrelated declarations changed typed projection")
	}
	if gotWitness.OwnerQueries != baseWitness.OwnerQueries || gotWitness.PrefixVisits != baseWitness.PrefixVisits || gotWitness.MaxPrefixVisits != baseWitness.MaxPrefixVisits || gotWitness.ReferenceJoins != baseWitness.ReferenceJoins {
		t.Fatalf("unrelated declarations multiplied lookup work: base=%+v got=%+v", baseWitness, gotWitness)
	}
	if gotWitness.DeclarationEntries != baseWitness.DeclarationEntries+len(inv.Declarations) {
		t.Fatalf("witness did not observe doubled declaration index: base=%+v got=%+v", baseWitness, gotWitness)
	}
}

func TestStructureOwnerIndexRejectsExactAmbiguityAndUsesLongestPrefix(t *testing.T) {
	makeDeclaration := func(path, name string) DeclarationInfo {
		declaration := DeclarationInfo{PackagePath: "fixture", Kind: "field", OwnerType: "Root", Surface: "fixture:Root", Path: path, Name: name, Type: "int", SourceFile: "root.go", Line: 1, Column: 1}
		declaration.FactID = declarationFactID(declaration)
		return declaration
	}
	short := makeDeclaration("Items", "Items")
	long := makeDeclaration("Items[].Nested", "Nested")
	exact := makeDeclaration("Items[].Nested.Value", "Value")
	index, err := buildStructureOwnerIndex(map[string]DeclarationInfo{short.FactID: short, long.FactID: long, exact.FactID: exact}, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := index.owner("fixture", "Root", "fixture:Root", "Items[].Nested.Value")
	if !ok || owner.FactID != exact.FactID {
		t.Fatalf("exact structural path was not preferred: owner=%+v ok=%v", owner, ok)
	}
	owner, ok = index.owner("fixture", "Root", "fixture:Root", "Items[].Nested.Other")
	if !ok || owner.FactID != long.FactID {
		t.Fatalf("longest structural prefix was not selected: owner=%+v ok=%v", owner, ok)
	}
	duplicate := makeDeclaration("Items", "OtherItems")
	if _, err := buildStructureOwnerIndex(map[string]DeclarationInfo{short.FactID: short, duplicate.FactID: duplicate}, nil); err == nil {
		t.Fatal("ambiguous exact structural path was accepted")
	}
}

func findNamedReference(t *testing.T, rows []StructureNamedReference, packagePath, name string) StructureNamedReference {
	t.Helper()
	for _, row := range rows {
		if row.TargetPackagePath == packagePath && row.TargetName == name {
			return row
		}
	}
	t.Fatalf("named target %s.%s was not found: %+v", packagePath, name, rows)
	return StructureNamedReference{}
}

func loadStructureProjectionFixture(t *testing.T, root string) (StructureProjection, declarationInventory) {
	t.Helper()
	family, shapes, inv, surfaces := loadStructureProjectionInputs(t, root)
	projection, err := structureProjection(root, family, shapes, inv, surfaces)
	if err != nil {
		t.Fatal(err)
	}
	return projection, inv
}

func loadStructureProjectionFamily(t *testing.T, root string) []*packages.Package {
	t.Helper()
	const familyPath = "example.test/program/link"
	loaded, err := loadWorkspace(root, familyPath)
	if err != nil {
		t.Fatal(err)
	}
	return familyPackages(indexPackages(loaded), familyPath)
}

func loadStructureProjectionInputs(t *testing.T, root string) ([]*packages.Package, []TypeShapeInfo, declarationInventory, []SurfaceInfo) {
	t.Helper()
	family := loadStructureProjectionFamily(t, root)
	shapes, err := familyTypeShapes(family)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := inventoryDeclarations(root, family, shapes)
	if err != nil {
		t.Fatal(err)
	}
	surfaces, err := surfaceProjection(root, family, shapes, inv)
	if err != nil {
		t.Fatal(err)
	}
	return family, shapes, inv, surfaces
}

func writeProjectionFixture(t *testing.T, root string) {
	t.Helper()
	write := func(name, source string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test\n\ngo 1.23\n")
	write("external/external.go", `package external

type Foreign struct { Value int }
type ForeignAPI interface { Shared(int) string }
type ForeignAPI2 interface { Shared(int) string }
`)
	write("program/link/link.go", `package link

import "example.test/external"

type Child struct { Value int }
type Link struct {
	Child *Child
	Foreign *external.Foreign
	Items []struct { Nested Child }
	Index map[*Child]Child
	Stream chan<- *Child
	Fixed [3]Child
	Local interface { Local(*Child) error }
}
type API interface { Do(*Child) *Child }
type API2 interface { Do(*Child) *Child }
type WithError interface { error }
type UsesForeign interface { external.ForeignAPI }
type UsesForeign2 interface { external.ForeignAPI2 }
func Generic[T interface { Child | ~int }](value T) T { return value }
func Signature(value [4]Child, items []Child, index map[Child]Child, stream chan<- Child) [2]Child { return [2]Child{} }
func LocalSignature(value interface { Local(*Child) error }) {}
func (Link) Do(value *Child) *Child { return value }
var Global = Link{}
const Limit = 1
`)
}

func TestStructureProjectionErrorMessagesRetainContext(t *testing.T) {
	if !strings.Contains(fmt.Sprint(ErrStructureProjection), "structure projection") {
		t.Fatalf("projection error lost context: %v", ErrStructureProjection)
	}
}
