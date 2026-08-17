// Package parsersource reads the sources the parser is generated from. Two
// source authorities live here: compiler/parse/parser.go.y, which states the
// grammar alternatives, their yacc semantic actions and the parser-only
// helper functions those actions may call; and the compiler/ast declaration
// graph, which states the constructor and field schema those actions build.
//
// Everything derived here is source authority. The package never runs a
// fixture, parses Lua, binds source, lowers a Program, or observes parser
// output, so its rows describe what the parser can construct rather than what
// any particular program did construct.
package parsersource

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FieldForm is the structural representation of one exported AST field. It
// intentionally describes construction shape, not a semantic value domain:
// those domains belong to the parser-alternative and projection obligations.
type FieldForm uint8

const (
	FieldFormInvalid FieldForm = iota
	FieldFormScalar
	FieldFormBool
	FieldFormString
	FieldFormOptional
	FieldFormSequence
	FieldFormMapping
	FieldFormInterface
	FieldFormNamed
)

// ConstructorClass distinguishes an AST value which crosses a semantic
// boundary from parser-only structural material. The classification is read
// from AST marker embedding, not inferred from fixture output.
type ConstructorClass uint8

const (
	ConstructorStructural ConstructorClass = iota + 1
	ConstructorStatement
	ConstructorExpression
	ConstructorTypeExpression
)

// Field is one exact exported semantic field in source order. Position and
// marker embeddings are intentionally absent: they carry source coordinates
// or Go interface method sets, not a parser-to-Program semantic value.
// Name and Type are compiler-source symbols, not a semantic-law vocabulary.
type Field struct {
	Ordinal int
	Name    string
	Type    string
	Form    FieldForm
}

// Constructor is one AST struct the shipped parser constructs. Fields are the
// complete compiler-owned exported-field schema for that struct.
type Constructor struct {
	Name     string
	Class    ConstructorClass
	Semantic bool
	Fields   []Field
}

// Declaration is one compiler/ast struct declaration, including parser
// infrastructure which is not itself a semantic value. Semantic is derived
// transitively from marker classes and semantic child fields; it is never a
// hand-maintained name list.
type Declaration struct {
	Name     string
	Class    ConstructorClass
	Semantic bool
	Fields   []Field
}

// Schema is parser authority only. It is deliberately separate from binder
// transitions and Program projection requirements.
type Schema struct {
	Constructors []Constructor
	Declarations []Declaration
	Types        []TypeDeclaration
}

// Digest is the opaque semantic shape fingerprint of parser-constructed AST
// declarations. Together with grammarproof.GrammarActionDigest it lets a
// frozen language profile fail closed when a parser action or its exported AST
// result gains a new semantic carrier.
func (s Schema) Digest() string {
	hash := sha256.New()
	for _, constructor := range s.Constructors {
		fmt.Fprintf(hash, "%s\x00%d\x00%t\n", constructor.Name, constructor.Class, constructor.Semantic)
		for _, field := range constructor.Fields {
			fmt.Fprintf(hash, "%d\x00%s\x00%s\x00%d\n", field.Ordinal, field.Name, field.Type, field.Form)
		}
	}
	for _, declaration := range s.Declarations {
		fmt.Fprintf(hash, "decl\x00%s\x00%d\x00%t\n", declaration.Name, declaration.Class, declaration.Semantic)
		for _, field := range declaration.Fields {
			fmt.Fprintf(hash, "decl-field\x00%s\x00%d\x00%s\x00%s\x00%d\n", declaration.Name, field.Ordinal, field.Name, field.Type, field.Form)
		}
	}
	for _, declaration := range s.Types {
		fmt.Fprintf(hash, "type\x00%s\x00%d\x00%d\x00%s\x00%t", declaration.Name, declaration.Kind, declaration.Class, declaration.Normalized, declaration.Semantic)
		for _, reference := range declaration.References {
			fmt.Fprintf(hash, "\x00%s", reference)
		}
		fmt.Fprintln(hash)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s Schema) ConstructorCount() int { return len(s.Constructors) }

// FieldCount is the parser-owned exported-field denominator. It is not a
// semantic-witness count: projection and recursive obligations must still
// account for each structural occurrence.
func (s Schema) FieldCount() int {
	count := 0
	for _, constructor := range s.Constructors {
		count += len(constructor.Fields)
	}
	return count
}

func (s Schema) ClassCount(class ConstructorClass) int {
	count := 0
	for _, constructor := range s.Constructors {
		if constructor.Class == class {
			count++
		}
	}
	return count
}

// Discover derives the complete parser-constructed AST schema from the
// checked-in generated parser and the AST declarations. It uses neither the
// fixture corpus nor observed parser output, so those sources cannot reduce
// the requirement set. The existing grammarproof generator separately proves
// that parser.go is the pinned parser.go.y generation.
func Discover(root string) (Schema, error) {
	astDirectory := filepath.Join(root, "compiler", "ast")
	declarations, aliases, err := astDeclarations(astDirectory)
	if err != nil {
		return Schema{}, err
	}
	types, err := discoverTypeGraph(astDirectory)
	if err != nil {
		return Schema{}, err
	}
	constructed, err := parserConstructors(filepath.Join(root, "compiler", "parse", "parser.go"))
	if err != nil {
		return Schema{}, err
	}
	if len(constructed) == 0 {
		return Schema{}, fmt.Errorf("grammar requirements: parser constructs no compiler AST values")
	}
	semantic := make(map[string]bool, len(types))
	for _, declaration := range types {
		semantic[declaration.Name] = declaration.Semantic
	}
	result := Schema{Constructors: make([]Constructor, 0, len(constructed)), Declarations: make([]Declaration, 0, len(declarations)), Types: types}
	declarationNames := make([]string, 0, len(declarations))
	for name := range declarations {
		declarationNames = append(declarationNames, name)
	}
	sort.Strings(declarationNames)
	for _, name := range declarationNames {
		declaration := declarations[name]
		result.Declarations = append(result.Declarations, Declaration{Name: name, Class: declaration.class, Semantic: semantic[name], Fields: append([]Field(nil), declaration.fields...)})
	}
	for _, name := range constructed {
		fields, exists := declarations[name]
		if !exists {
			if aliases[name] {
				// Position and Span are source-coordinate aliases. They are
				// operands of a parser action, not AST constructors with a
				// lowering obligation of their own.
				continue
			}
			return Schema{}, fmt.Errorf("grammar requirements: parser constructs ast.%s without an AST struct declaration", name)
		}
		result.Constructors = append(result.Constructors, Constructor{Name: name, Class: fields.class, Semantic: semantic[name], Fields: fields.fields})
	}
	return result, nil
}

type astDeclaration struct {
	class  ConstructorClass
	fields []Field
}

func astDeclarations(directory string) (map[string]astDeclaration, map[string]bool, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, nil, fmt.Errorf("grammar requirements: read AST directory: %w", err)
	}
	result := make(map[string]astDeclaration)
	aliases := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("grammar requirements: parse %s: %w", path, parseErr)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*goast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, ok := specification.(*goast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*goast.StructType)
				if !ok {
					if typeSpec.Assign.IsValid() {
						aliases[typeSpec.Name.Name] = true
					}
					continue
				}
				if _, exists := result[typeSpec.Name.Name]; exists {
					return nil, nil, fmt.Errorf("grammar requirements: duplicate AST struct %s", typeSpec.Name.Name)
				}
				fields, fieldErr := exportedFields(structType)
				if fieldErr != nil {
					return nil, nil, fmt.Errorf("grammar requirements: ast.%s: %w", typeSpec.Name.Name, fieldErr)
				}
				result[typeSpec.Name.Name] = astDeclaration{class: constructorClass(structType), fields: fields}
			}
		}
	}
	return result, aliases, nil
}

func constructorClass(structType *goast.StructType) ConstructorClass {
	for _, field := range structType.Fields.List {
		if len(field.Names) != 0 {
			continue
		}
		if class := EmbeddedBaseClass(embeddedName(field.Type)); class != ConstructorStructural {
			return class
		}
	}
	return ConstructorStructural
}

// EmbeddedBaseClass is the class an AST base embedding gives the struct that
// embeds it. It is the single authority for what makes a declaration a
// statement, an expression, or a type expression, so a reader that has a base
// name rather than a struct reaches the same answer.
func EmbeddedBaseClass(name string) ConstructorClass {
	switch name {
	case "StmtBase":
		return ConstructorStatement
	case "ExprBase", "ConstExprBase":
		return ConstructorExpression
	case "TypeExprBase":
		return ConstructorTypeExpression
	default:
		return ConstructorStructural
	}
}

func exportedFields(structType *goast.StructType) ([]Field, error) {
	var result []Field
	for _, declaration := range structType.Fields.List {
		form, err := fieldForm(declaration.Type)
		if err != nil {
			return nil, err
		}
		typeName := sourceExpr(declaration.Type)
		if len(declaration.Names) == 0 {
			name := embeddedName(declaration.Type)
			if StructuralEmbedding(name) {
				continue
			}
			if name == "" || !token.IsExported(name) {
				continue
			}
			result = append(result, Field{Ordinal: len(result), Name: name, Type: typeName, Form: form})
			continue
		}
		for _, name := range declaration.Names {
			if !name.IsExported() {
				continue
			}
			result = append(result, Field{Ordinal: len(result), Name: name.Name, Type: typeName, Form: form})
		}
	}
	return result, nil
}

// StructuralEmbedding identifies parser-AST infrastructure, not language
// payload. These embeddings provide PositionHolder and marker behavior. A
// source occurrence cannot vary their semantic state, and no Program Term may
// truthfully be said to lower one, so including them in the occurrence
// denominator would manufacture unreachable requirements.
func StructuralEmbedding(name string) bool {
	switch name {
	case "Node", "ExprBase", "ConstExprBase", "StmtBase", "TypeExprBase":
		return true
	default:
		return false
	}
}

func fieldForm(expr goast.Expr) (FieldForm, error) {
	switch value := expr.(type) {
	case *goast.StarExpr:
		return FieldFormOptional, nil
	case *goast.ArrayType:
		return FieldFormSequence, nil
	case *goast.MapType:
		return FieldFormMapping, nil
	case *goast.InterfaceType:
		return FieldFormInterface, nil
	case *goast.Ident:
		switch value.Name {
		case "bool":
			return FieldFormBool, nil
		case "string":
			return FieldFormString, nil
		case "any", "Expr", "Stmt", "TypeExpr", "PositionHolder":
			return FieldFormInterface, nil
		}
		if token.IsExported(value.Name) {
			return FieldFormNamed, nil
		}
		return FieldFormScalar, nil
	case *goast.SelectorExpr:
		return FieldFormNamed, nil
	default:
		return FieldFormInvalid, fmt.Errorf("unsupported exported field type %T", expr)
	}
}

func embeddedName(expr goast.Expr) string {
	switch value := expr.(type) {
	case *goast.Ident:
		return value.Name
	case *goast.StarExpr:
		return embeddedName(value.X)
	case *goast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

func sourceExpr(expr goast.Expr) string {
	switch value := expr.(type) {
	case *goast.Ident:
		return value.Name
	case *goast.StarExpr:
		return "*" + sourceExpr(value.X)
	case *goast.ArrayType:
		return "[]" + sourceExpr(value.Elt)
	case *goast.MapType:
		return "map[" + sourceExpr(value.Key) + "]" + sourceExpr(value.Value)
	case *goast.InterfaceType:
		return "interface{}"
	case *goast.SelectorExpr:
		return sourceExpr(value.X) + "." + value.Sel.Name
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func parserConstructors(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("grammar requirements: parse generated parser: %w", err)
	}
	seen := make(map[string]struct{})
	goast.Inspect(file, func(node goast.Node) bool {
		literal, ok := node.(*goast.CompositeLit)
		if !ok {
			return true
		}
		selector, ok := literal.Type.(*goast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*goast.Ident)
		if !ok || qualifier.Name != "ast" {
			return true
		}
		seen[selector.Sel.Name] = struct{}{}
		return true
	})
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}
