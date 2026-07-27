package signature

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/placement"
)

func TestOperationalEffectsConsumePlacementDomainNamesDirectly(t *testing.T) {
	var event EscapeEvent
	var relation ParamRelation
	event.Kind = placement.Send
	relation.EscapeClass = placement.Borrow
	relation.PlacementConsequence = placement.Keep
}

func TestPlacementCompatibilityPlaneStaysDeleted(t *testing.T) {
	source, err := os.ReadFile("operational_effects.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "operational_effects.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			switch node := spec.(type) {
			case *ast.TypeSpec:
				if node.Name.Name == "EscapeKind" || node.Name.Name == "PlacementConsequence" {
					t.Errorf("middle-plane type spelling %s returned", node.Name.Name)
				}
			case *ast.ValueSpec:
				for _, name := range node.Names {
					switch name.Name {
					case "EscapeNone", "EscapeBorrow", "EscapeRetain", "EscapeStore",
						"EscapeSend", "EscapeExport", "EscapeOpaque",
						"PlacementConsequenceKeep", "PlacementConsequenceOwnedHeap",
						"PlacementConsequenceSharedHeap":
						t.Errorf("middle-plane constant spelling %s returned", name.Name)
					}
				}
			}
		}
	}
}
