package ingress

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestIngressPublicTypesDoNotEmbedProgramArtifact(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("ingress source location unavailable")
	}
	path := filepath.Join(filepath.Dir(current), "artifact.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	watched := map[string]bool{
		"Snapshot": true, "Point": true, "StructuralEdge": true, "LocalTransfer": true,
		"Region": true, "Event": true, "RulePlacement": true, "BodyTransport": true, "FunctionBoundary": true,
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok || !watched[typeSpec.Name.Name] {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("%s is not a struct", typeSpec.Name.Name)
			}
			for _, field := range structType.Fields.List {
				var rendered bytes.Buffer
				if err := printer.Fprint(&rendered, token.NewFileSet(), field.Type); err != nil {
					t.Fatal(err)
				}
				spelling := rendered.String()
				if strings.Contains(spelling, "programartifact") {
					t.Fatalf("%s exposes owner type %s", typeSpec.Name.Name, spelling)
				}
			}
		}
	}
}

func TestLowerProjectsSealedColumnsWithoutRetainingTheOwner(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "ingress-lower.lua", Text: []byte(`
local function identity(value)
  return value
end
return identity(1)
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("ingress compilation failed: %s", failure.Error())
	}
	vocabulary, vocabularyOK := composite.StructureVocabulary()
	snapshot, lowered := Lower(artifact, vocabulary)
	if !vocabularyOK || !lowered || snapshot == nil || !snapshot.Available() {
		t.Fatal("Lower refused a sealed artifact")
	}
	if snapshot.ArtifactID() != artifact.ID() || snapshot.ProgramID() != artifact.CompileKey().ProgramID() || snapshot.SchemaID() != artifact.CompileKey().SchemaDigest() {
		t.Fatal("Lower lost the sealed artifact identity")
	}
	if snapshot.PointCount() != artifact.PointCount() || snapshot.StructuralEdgeCount() != artifact.EnvironmentEdgeCount() ||
		snapshot.LocalTransferCount() != artifact.LocalTransferCount() || snapshot.RegionCount() != artifact.RegionCount() ||
		snapshot.EventCount() != artifact.WTOEventCount() || snapshot.RulePlacementCount() != artifact.RulePlacementCount() ||
		snapshot.BodyTransportCount() != artifact.BodyCount() || snapshot.FunctionBoundaryCount() != artifact.FunctionBoundaryCount() {
		t.Fatalf("Lower column counts drifted from the sealed artifact")
	}
	for index := 0; index < snapshot.PointCount(); index++ {
		got, gotOK := snapshot.PointAt(index)
		want, wantOK := artifact.PointAt(index)
		if !gotOK || !wantOK || got.ID() != want.ID() {
			t.Fatalf("point %d drifted from the sealed artifact", index)
		}
	}
	if snapshot.BodyTransportCount() == 0 {
		t.Fatal("fixture issued no body transports")
	}
	body, bodyOK := snapshot.BodyTransportAt(0)
	if !bodyOK || !body.BodyID().Available() {
		t.Fatal("body transport")
	}
	if body.ExitCount() == 0 {
		t.Fatal("body transport issued no accepted exits")
	}
	points := make(map[identity.ContentID]struct{}, snapshot.PointCount())
	for index := 0; index < snapshot.PointCount(); index++ {
		point, ok := snapshot.PointAt(index)
		if !ok {
			t.Fatal("point column")
		}
		points[point.ID()] = struct{}{}
	}
	for index := 0; index < body.ExitCount(); index++ {
		exit, ok := body.ExitAt(index)
		if !ok || !exit.Available() {
			t.Fatalf("exit %d", index)
		}
		if _, known := points[exit]; !known {
			t.Fatalf("exit %d is not a sealed point", index)
		}
	}
	snapshotType := reflect.TypeOf(*snapshot)
	for index := 0; index < snapshotType.NumField(); index++ {
		if strings.Contains(snapshotType.Field(index).Type.String(), "programartifact") {
			t.Fatalf("Snapshot retained owner field %s", snapshotType.Field(index).Name)
		}
	}
	bodyType := reflect.TypeOf(body)
	for index := 0; index < bodyType.NumField(); index++ {
		if strings.Contains(bodyType.Field(index).Type.String(), "programartifact") {
			t.Fatalf("BodyTransport retained owner field %s", bodyType.Field(index).Name)
		}
	}
}
