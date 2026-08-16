package grammar

import (
	"bytes"
	"fmt"
	goast "go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NamedTypeKind is the complete kind of a compiler/ast named declaration.
// Every type declaration participates, not only structs observed in parser
// composite literals.
type NamedTypeKind uint8

const (
	NamedTypeInvalid NamedTypeKind = iota
	NamedTypeStruct
	NamedTypeInterface
	NamedTypeAlias
	NamedTypeDefined
)

// TypeDeclaration is one normalized node in the compiler/ast named-type
// graph. References contains only local named-type edges; qualified external
// types are explicit normalized leaves. Semantic is the least fixed point of
// semantic marker declarations and references through every supported type
// constructor.
type TypeDeclaration struct {
	Name       string
	Kind       NamedTypeKind
	Class      ConstructorClass
	Normalized string
	References []string
	Semantic   bool
}

type parsedType struct {
	name       string
	expression goast.Expr
	kind       NamedTypeKind
	class      ConstructorClass
	normalized string
	seed       bool
}

func discoverTypeGraph(directory string) ([]TypeDeclaration, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("grammar requirements: read AST type graph: %w", err)
	}
	set := token.NewFileSet()
	parsed := make(map[string]parsedType)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, parseErr := parser.ParseFile(set, path, nil, 0)
		if parseErr != nil {
			return nil, fmt.Errorf("grammar requirements: parse type graph %s: %w", path, parseErr)
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
				if _, duplicate := parsed[typeSpec.Name.Name]; duplicate {
					return nil, fmt.Errorf("grammar requirements: duplicate AST type %s", typeSpec.Name.Name)
				}
				kind := NamedTypeDefined
				class := ConstructorStructural
				seed := false
				if typeSpec.Assign.IsValid() {
					kind = NamedTypeAlias
				} else {
					switch value := typeSpec.Type.(type) {
					case *goast.StructType:
						kind, class = NamedTypeStruct, constructorClass(value)
						seed = class != ConstructorStructural
					case *goast.InterfaceType:
						kind = NamedTypeInterface
						seed = semanticMarkerInterface(value)
					}
				}
				normalized, normalizeErr := normalizedType(set, typeSpec.Type)
				if normalizeErr != nil {
					return nil, fmt.Errorf("grammar requirements: normalize ast.%s: %w", typeSpec.Name.Name, normalizeErr)
				}
				parsed[typeSpec.Name.Name] = parsedType{name: typeSpec.Name.Name, expression: typeSpec.Type, kind: kind, class: class, normalized: normalized, seed: seed}
			}
		}
	}
	known := make(map[string]bool, len(parsed))
	for name := range parsed {
		known[name] = true
	}
	references := make(map[string][]string, len(parsed))
	for name, declaration := range parsed {
		seen := make(map[string]bool)
		if err := collectTypeReferences(declaration.expression, known, seen); err != nil {
			return nil, fmt.Errorf("grammar requirements: ast.%s type graph: %w", name, err)
		}
		for reference := range seen {
			references[name] = append(references[name], reference)
		}
		sort.Strings(references[name])
	}
	semantic := make(map[string]bool, len(parsed))
	for name, declaration := range parsed {
		semantic[name] = declaration.seed
	}
	for changed := true; changed; {
		changed = false
		for name := range parsed {
			if semantic[name] {
				continue
			}
			for _, reference := range references[name] {
				if semantic[reference] {
					semantic[name], changed = true, true
					break
				}
			}
		}
	}
	names := make([]string, 0, len(parsed))
	for name := range parsed {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]TypeDeclaration, 0, len(names))
	for _, name := range names {
		declaration := parsed[name]
		result = append(result, TypeDeclaration{Name: name, Kind: declaration.kind, Class: declaration.class, Normalized: declaration.normalized, References: references[name], Semantic: semantic[name]})
	}
	return result, nil
}

func semanticMarkerInterface(value *goast.InterfaceType) bool {
	for _, field := range value.Methods.List {
		if len(field.Names) != 1 {
			continue
		}
		function, ok := field.Type.(*goast.FuncType)
		if !ok || function.Params != nil && len(function.Params.List) != 0 || function.Results != nil && len(function.Results.List) != 0 {
			continue
		}
		if strings.HasSuffix(field.Names[0].Name, "Marker") {
			return true
		}
	}
	return false
}

func normalizedType(set *token.FileSet, expression goast.Expr) (string, error) {
	var out bytes.Buffer
	if err := format.Node(&out, set, expression); err != nil {
		return "", err
	}
	if out.Len() == 0 {
		return "", fmt.Errorf("empty normalized type")
	}
	return out.String(), nil
}

func collectTypeReferences(expression goast.Expr, known map[string]bool, result map[string]bool) error {
	switch value := expression.(type) {
	case *goast.Ident:
		if predeclaredType(value.Name) {
			return nil
		}
		if !known[value.Name] {
			return fmt.Errorf("unresolved local named type %s", value.Name)
		}
		result[value.Name] = true
		return nil
	case *goast.SelectorExpr:
		if qualifier, ok := value.X.(*goast.Ident); !ok || qualifier.Name == "" || value.Sel == nil || value.Sel.Name == "" {
			return fmt.Errorf("unsupported qualified type expression")
		}
		return nil
	case *goast.StarExpr:
		return collectTypeReferences(value.X, known, result)
	case *goast.ArrayType:
		return collectTypeReferences(value.Elt, known, result)
	case *goast.MapType:
		if err := collectTypeReferences(value.Key, known, result); err != nil {
			return err
		}
		return collectTypeReferences(value.Value, known, result)
	case *goast.ChanType:
		return collectTypeReferences(value.Value, known, result)
	case *goast.Ellipsis:
		return collectTypeReferences(value.Elt, known, result)
	case *goast.ParenExpr:
		return collectTypeReferences(value.X, known, result)
	case *goast.StructType:
		for _, field := range value.Fields.List {
			if err := collectTypeReferences(field.Type, known, result); err != nil {
				return err
			}
		}
		return nil
	case *goast.InterfaceType:
		for _, method := range value.Methods.List {
			if err := collectTypeReferences(method.Type, known, result); err != nil {
				return err
			}
		}
		return nil
	case *goast.FuncType:
		if err := collectFieldListReferences(value.Params, known, result); err != nil {
			return err
		}
		return collectFieldListReferences(value.Results, known, result)
	case *goast.IndexExpr:
		if err := collectTypeReferences(value.X, known, result); err != nil {
			return err
		}
		return collectTypeReferences(value.Index, known, result)
	case *goast.IndexListExpr:
		if err := collectTypeReferences(value.X, known, result); err != nil {
			return err
		}
		for _, index := range value.Indices {
			if err := collectTypeReferences(index, known, result); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported named type expression %T", expression)
	}
}

func collectFieldListReferences(list *goast.FieldList, known map[string]bool, result map[string]bool) error {
	if list == nil {
		return nil
	}
	for _, field := range list.List {
		if err := collectTypeReferences(field.Type, known, result); err != nil {
			return err
		}
	}
	return nil
}

func predeclaredType(name string) bool {
	switch name {
	case "any", "bool", "byte", "comparable", "complex64", "complex128", "error", "float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}
