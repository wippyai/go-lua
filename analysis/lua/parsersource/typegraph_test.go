package parsersource

import "testing"

func TestASTTypeGraphCarriesSemanticClassesAndReferences(t *testing.T) {
	schema, err := Discover(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Types) == 0 || schema.Digest() == "" {
		t.Fatalf("type graph = %d types digest %q, want non-empty", len(schema.Types), schema.Digest())
	}
	var expression, structural bool
	for _, declaration := range schema.Types {
		if declaration.Name == "FuncCallExpr" && declaration.Class == ConstructorExpression && declaration.Semantic {
			expression = true
		}
		if declaration.Class == ConstructorStructural && !declaration.Semantic {
			structural = true
		}
	}
	if !expression || !structural {
		t.Fatal("type graph did not preserve semantic and structural class distinction")
	}
}
