package ingress

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBorrowedIngressHasNoOwnedRowStorage(t *testing.T) {
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
			// Snapshot admits exactly two fields, and the law names both: the
			// borrowed artifact pointer every row accessor indexes into, and the
			// sealed structural vocabulary those rows are projected through. A
			// third field would be ingress-owned state, which is the thing this
			// law exists to forbid.
			if typeSpec.Name.Name == "Snapshot" {
				admitted := map[string]string{"artifact": "*programartifact.Artifact", "vocabulary": "structure.Table"}
				if structType.Fields.NumFields() != len(admitted) {
					t.Fatalf("Snapshot fields = %d, want its borrowed artifact pointer and the sealed vocabulary", structType.Fields.NumFields())
				}
				for _, field := range structType.Fields.List {
					if len(field.Names) != 1 {
						t.Fatal("Snapshot declares an unnamed or grouped field")
					}
					want, admissible := admitted[field.Names[0].Name]
					if !admissible {
						t.Fatalf("Snapshot retains the field %q", field.Names[0].Name)
					}
					var rendered bytes.Buffer
					if err := printer.Fprint(&rendered, token.NewFileSet(), field.Type); err != nil {
						t.Fatal(err)
					}
					if rendered.String() != want {
						t.Fatalf("Snapshot field %q is %s, want %s", field.Names[0].Name, rendered.String(), want)
					}
				}
			}
			for _, field := range structType.Fields.List {
				if _, slice := field.Type.(*ast.ArrayType); slice {
					t.Fatalf("%s retains an ingress-owned slice field", typeSpec.Name.Name)
				}
			}
		}
	}
}

func TestLowerDoesNotAllocateASecondArtifactRepresentation(t *testing.T) {
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
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "Lower" || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if identifier, ok := call.Fun.(*ast.Ident); ok && identifier.Name == "make" {
				t.Errorf("Lower allocates ingress-owned storage")
			}
			return true
		})
		return
	}
	t.Fatal("Lower function unavailable")
}
