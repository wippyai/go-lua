package parsersource

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// moduleRoot walks up from this test source until it finds the directory that
// owns go.mod. Anchoring on the module marker keeps the proof independent of
// where the grammarproof tree sits inside the module.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate grammar schema source")
	}
	root := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("module root: no go.mod above test file")
		}
		root = parent
	}
}

func TestDiscoversParserConstructedASTSchema(t *testing.T) {
	root := moduleRoot(t)
	schema, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Constructors) == 0 {
		t.Fatal("parser requirement schema has no constructors")
	}
	declarations := make(map[string]Declaration, len(schema.Declarations))
	for _, declaration := range schema.Declarations {
		declarations[declaration.Name] = declaration
	}
	if declarations["Token"].Semantic {
		t.Fatal("diagnostic Token entered the semantic AST graph")
	}
	for _, name := range []string{"ParList", "FuncName", "AnnotationExpr", "RecordFieldExpr", "TypeParamExpr", "FunctionParamExpr", "InterfaceMember"} {
		if !declarations[name].Semantic {
			t.Fatalf("transitive structural carrier %s was not classified semantic", name)
		}
	}
	if len(schema.Types) <= len(schema.Declarations) {
		t.Fatal("complete AST type universe omitted non-struct declarations")
	}
	types := make(map[string]TypeDeclaration, len(schema.Types))
	for _, declaration := range schema.Types {
		types[declaration.Name] = declaration
	}
	for _, name := range []string{"Expr", "Stmt", "TypeExpr", "ConstExpr"} {
		if !types[name].Semantic || types[name].Kind != NamedTypeInterface {
			t.Fatalf("semantic interface %s missing from normalized type graph: %#v", name, types[name])
		}
	}
	if types["Position"].Kind != NamedTypeAlias || types["Position"].Semantic || types["AttrKeySyntax"].Kind != NamedTypeDefined || types["AttrKeySyntax"].Semantic {
		t.Fatalf("nonsemantic named type classification is not explicit: Position=%#v AttrKeySyntax=%#v", types["Position"], types["AttrKeySyntax"])
	}
	t.Logf("parser schema: constructors=%d exported-fields=%d statements=%d expressions=%d types=%d structural=%d",
		schema.ConstructorCount(), schema.FieldCount(),
		schema.ClassCount(ConstructorStatement), schema.ClassCount(ConstructorExpression),
		schema.ClassCount(ConstructorTypeExpression), schema.ClassCount(ConstructorStructural))
	for _, constructor := range schema.Constructors {
		if constructor.Name == "" || constructor.Class == 0 {
			t.Fatal("parser requirement schema has unnamed constructor")
		}
		for index, field := range constructor.Fields {
			if field.Ordinal != index || field.Name == "" || field.Type == "" || field.Form == FieldFormInvalid {
				t.Fatalf("invalid schema field for ast.%s: %#v", constructor.Name, field)
			}
		}
	}
}

func TestTypeGraphNormalizesContainersAliasesInterfacesAndCycles(t *testing.T) {
	directory := t.TempDir()
	source := `package ast
type Root interface { rootMarker() }
type A []B
type B map[string]*A
type D []E
type E struct { Next D; Value Root }
type Fixed [4]Root
type C []*Root
type Alias = C
type Embedded interface { Root }
type Method interface { Apply(Alias) Root }
`
	if err := os.WriteFile(filepath.Join(directory, "types.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	graph, err := discoverTypeGraph(directory)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]TypeDeclaration, len(graph))
	for _, declaration := range graph {
		byName[declaration.Name] = declaration
	}
	if byName["A"].Semantic || byName["B"].Semantic || byName["A"].Normalized != "[]B" || byName["B"].Normalized != "map[string]*A" {
		t.Fatalf("nonsemantic recursive container cycle classified incorrectly: A=%#v B=%#v", byName["A"], byName["B"])
	}
	for _, name := range []string{"Root", "D", "E", "Fixed", "C", "Alias", "Embedded", "Method"} {
		if !byName[name].Semantic || byName[name].Normalized == "" {
			t.Fatalf("semantic type-graph propagation failed for %s: %#v", name, byName[name])
		}
	}
	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, "broken.go"), []byte("package ast\ntype Broken Future\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverTypeGraph(broken); err == nil {
		t.Fatal("unresolved named type was accepted")
	}
	unsupported := t.TempDir()
	if err := os.WriteFile(filepath.Join(unsupported, "unsupported.go"), []byte("package ast\ntype Broken interface { ~int }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverTypeGraph(unsupported); err == nil {
		t.Fatal("unsupported named type expression was accepted")
	}
}
