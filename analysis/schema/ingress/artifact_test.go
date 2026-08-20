package ingress_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
		"Snapshot": true, "Point": true, "StructuralEdge": true,
		"Region": true, "Event": true, "RulePlacement": true, "BodyTransport": true,
		"Call": true, "CallOperand": true, "CallTarget": true, "CallArgument": true,
		"HeapAllocation": true, "HeapField": true, "HeapIndex": true,
		"Values": true, "ValuesMember": true, "ValuesTail": true, "StaticTypeValue": true,
		"StaticTypeArgument":    true,
		"Occurrence":            true,
		"DiagnosticObservation": true,
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
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		var receiver bytes.Buffer
		if err := printer.Fprint(&receiver, token.NewFileSet(), fn.Recv.List[0].Type); err != nil {
			t.Fatal(err)
		}
		receiverName := strings.TrimLeft(receiver.String(), "*")
		if !watched[receiverName] {
			continue
		}
		if fn.Type.Params != nil {
			for _, field := range fn.Type.Params.List {
				var rendered bytes.Buffer
				if err := printer.Fprint(&rendered, token.NewFileSet(), field.Type); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(rendered.String(), "programartifact") {
					t.Fatalf("%s.%s parameter exposes owner type %s", receiverName, fn.Name.Name, rendered.String())
				}
			}
		}
		if fn.Type.Results != nil {
			for _, field := range fn.Type.Results.List {
				var rendered bytes.Buffer
				if err := printer.Fprint(&rendered, token.NewFileSet(), field.Type); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(rendered.String(), "programartifact") {
					t.Fatalf("%s.%s result exposes owner type %s", receiverName, fn.Name.Name, rendered.String())
				}
			}
		}
	}
}
