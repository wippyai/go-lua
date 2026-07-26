package wir

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

func TestRetiredProductionSurfacesStayRetired(t *testing.T) {
	for _, name := range []string{"print.go", "debug.go", "debug_visibility.go"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Errorf("retired production surface %s returned: %v", name, err)
		}
	}

	retiredFields := map[string]bool{
		"CallExpr":             true,
		"DirectGlobals":        true,
		"ProducerPoint":        true,
		"HasProducerPoint":     true,
		"debugPointOrdinals":   true,
		"debugPointOrder":      true,
		"debugLocalVisibility": true,
	}
	files := []string{"block.go", "check.go", "instruction.go"}
	for _, name := range files {
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch item := node.(type) {
			case *ast.FuncDecl:
				if item.Name.Name == "HasPoint" {
					t.Errorf("%s: retired HasPoint accessor returned", name)
				}
			case *ast.Field:
				for _, fieldName := range item.Names {
					if retiredFields[fieldName.Name] {
						t.Errorf("%s: retired %s field returned", name, fieldName.Name)
					}
				}
			}
			return true
		})
	}
}
