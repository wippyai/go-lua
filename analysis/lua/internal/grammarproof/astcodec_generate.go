package grammarproof

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

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
)

const astCodecRelativePath = "lua/internal/grammarproof/astcodec/codec_gen.go"

type astCodecFieldKind uint8

const (
	astCodecFieldInvalid astCodecFieldKind = iota
	astCodecFieldPointer
	astCodecFieldInterface
	astCodecFieldSlice
	astCodecFieldArray
	astCodecFieldMap
	astCodecFieldBool
	astCodecFieldString
	astCodecFieldSigned
	astCodecFieldUnsigned
	astCodecFieldStruct
)

type astCodecField struct {
	Name        string
	Source      string
	Qualified   string
	Kind        astCodecFieldKind
	StructName  string
	ContainsAST bool
}

type astCodecStruct struct {
	Name               string
	Fields             []astCodecField
	AllFields          []astCodecField
	HasPosition        bool
	HasPrivatePosition bool
}

type astCodecNamed struct {
	Name       string
	Expression goast.Expr
	Struct     bool
	Interface  bool
}

type astCodecModel struct {
	Structs []astCodecStruct
	Named   map[string]astCodecNamed
	Slices  []string
	Arrays  []string
	Maps    []string
}

func astCodecModelForRoot(root string) (astCodecModel, error) {
	directory := filepath.Join(root, "compiler", "ast")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return astCodecModel{}, fmt.Errorf("grammarproof AST codec: read AST directory: %w", err)
	}
	set := token.NewFileSet()
	model := astCodecModel{Named: make(map[string]astCodecNamed)}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, parseErr := parser.ParseFile(set, path, nil, 0)
		if parseErr != nil {
			return astCodecModel{}, fmt.Errorf("grammarproof AST codec: parse %s: %w", path, parseErr)
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
				if _, exists := model.Named[typeSpec.Name.Name]; exists {
					return astCodecModel{}, fmt.Errorf("grammarproof AST codec: duplicate AST type %s", typeSpec.Name.Name)
				}
				model.Named[typeSpec.Name.Name] = astCodecNamed{Name: typeSpec.Name.Name, Expression: typeSpec.Type}
				if structType, ok := typeSpec.Type.(*goast.StructType); ok {
					item := model.Named[typeSpec.Name.Name]
					item.Struct = true
					model.Named[typeSpec.Name.Name] = item
					fields, allFields, fieldErr := astCodecFields(structType)
					if fieldErr != nil {
						return astCodecModel{}, fmt.Errorf("grammarproof AST codec: ast.%s: %w", typeSpec.Name.Name, fieldErr)
					}
					hasPrivate := false
					for _, field := range allFields {
						if !token.IsExported(field.Name) {
							hasPrivate = true
							break
						}
					}
					model.Structs = append(model.Structs, astCodecStruct{
						Name: typeSpec.Name.Name, Fields: fields, AllFields: allFields,
						HasPosition: false, HasPrivatePosition: hasPrivate,
					})
				} else if _, ok := typeSpec.Type.(*goast.InterfaceType); ok {
					item := model.Named[typeSpec.Name.Name]
					item.Interface = true
					model.Named[typeSpec.Name.Name] = item
				}
			}
		}
	}
	for index := range model.Structs {
		if item := model.Named[model.Structs[index].Name]; item.Struct {
			if structType, ok := item.Expression.(*goast.StructType); ok {
				model.Structs[index].HasPosition = astCodecHasPosition(item.Name, structType, model.Named)
			}
		}
	}
	// Resolve field forms after every local named type is known. This makes a
	// newly-added AST field fail closed instead of silently falling back to a
	// generic observer.
	for index := range model.Structs {
		for fieldIndex := range model.Structs[index].AllFields {
			field := &model.Structs[index].AllFields[fieldIndex]
			kind, structName, classifyErr := astCodecClassify(field.Source, model.Named)
			if classifyErr != nil {
				return astCodecModel{}, fmt.Errorf("grammarproof AST codec: ast.%s.%s: %w", model.Structs[index].Name, field.Name, classifyErr)
			}
			field.Kind, field.StructName = kind, structName
			field.Qualified, classifyErr = astCodecQualifiedType(field.Source, model.Named)
			if classifyErr != nil {
				return astCodecModel{}, fmt.Errorf("grammarproof AST codec: qualify ast.%s.%s: %w", model.Structs[index].Name, field.Name, classifyErr)
			}
			field.ContainsAST = astCodecContainsAST(field.Source, model.Named, make(map[string]bool))
			switch kind {
			case astCodecFieldSlice:
				model.Slices = append(model.Slices, field.Qualified)
			case astCodecFieldArray:
				model.Arrays = append(model.Arrays, field.Qualified)
			case astCodecFieldMap:
				model.Maps = append(model.Maps, field.Qualified)
			}
		}
		resolved := make(map[string]astCodecField, len(model.Structs[index].AllFields))
		for _, field := range model.Structs[index].AllFields {
			resolved[field.Name] = field
		}
		for fieldIndex := range model.Structs[index].Fields {
			model.Structs[index].Fields[fieldIndex] = resolved[model.Structs[index].Fields[fieldIndex].Name]
		}
	}
	model.Slices = uniqueStrings(append([]string{"[]ast.Stmt"}, model.Slices...))
	model.Arrays = uniqueStrings(model.Arrays)
	model.Maps = uniqueStrings(model.Maps)
	sort.Slice(model.Structs, func(left, right int) bool { return model.Structs[left].Name < model.Structs[right].Name })
	if err := validateASTCodecModel(model); err != nil {
		return astCodecModel{}, err
	}
	return model, nil
}

func astCodecFields(structType *goast.StructType) ([]astCodecField, []astCodecField, error) {
	var exported, all []astCodecField
	for _, declaration := range structType.Fields.List {
		source, err := astCodecSourceExpr(declaration.Type)
		if err != nil {
			return nil, nil, err
		}
		if len(declaration.Names) == 0 {
			name := astCodecEmbeddedName(declaration.Type)
			if name == "" {
				return nil, nil, fmt.Errorf("unsupported anonymous field type %s", source)
			}
			field := astCodecField{Name: name, Source: source}
			all = append(all, field)
			if token.IsExported(name) {
				exported = append(exported, field)
			}
			continue
		}
		for _, identifier := range declaration.Names {
			field := astCodecField{Name: identifier.Name, Source: source}
			all = append(all, field)
			if identifier.IsExported() {
				exported = append(exported, field)
			}
		}
	}
	return exported, all, nil
}

func astCodecSourceExpr(expression goast.Expr) (string, error) {
	var output bytes.Buffer
	if err := format.Node(&output, token.NewFileSet(), expression); err != nil {
		return "", err
	}
	if output.Len() == 0 {
		return "", fmt.Errorf("empty field type")
	}
	return output.String(), nil
}

func astCodecEmbeddedName(expression goast.Expr) string {
	switch value := expression.(type) {
	case *goast.Ident:
		return value.Name
	case *goast.StarExpr:
		return astCodecEmbeddedName(value.X)
	case *goast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

func astCodecHasPosition(name string, structType *goast.StructType, named map[string]astCodecNamed) bool {
	// The former field-by-name observer found no embedded Node field on
	// ast.Node itself. Its own occurrence therefore has a zero span even
	// though Node exposes position methods; only types embedding a Node
	// (directly or through one of the AST bases) carry occurrence spans.
	for _, declaration := range structType.Fields.List {
		if len(declaration.Names) != 0 {
			continue
		}
		embedded := astCodecEmbeddedName(declaration.Type)
		if embedded == "Node" || embedded == "ExprBase" || embedded == "ConstExprBase" || embedded == "StmtBase" || embedded == "TypeExprBase" {
			return true
		}
		if item, ok := named[embedded]; ok {
			if nested, ok := item.Expression.(*goast.StructType); ok && astCodecHasPosition(embedded, nested, named) {
				return true
			}
		}
	}
	return false
}

func astCodecClassify(source string, named map[string]astCodecNamed) (astCodecFieldKind, string, error) {
	expression, err := parser.ParseExpr(source)
	if err != nil {
		return astCodecFieldInvalid, "", err
	}
	return astCodecClassifyExpr(expression, named, make(map[string]bool))
}

func astCodecClassifyExpr(expression goast.Expr, named map[string]astCodecNamed, stack map[string]bool) (astCodecFieldKind, string, error) {
	switch value := expression.(type) {
	case *goast.StarExpr:
		return astCodecFieldPointer, "", nil
	case *goast.ArrayType:
		if value.Len == nil {
			return astCodecFieldSlice, "", nil
		}
		return astCodecFieldArray, "", nil
	case *goast.MapType:
		return astCodecFieldMap, "", nil
	case *goast.InterfaceType:
		return astCodecFieldInterface, "", nil
	case *goast.SelectorExpr:
		if value.Sel == nil || value.Sel.Name == "" {
			return astCodecFieldInvalid, "", fmt.Errorf("selector field type has no name")
		}
		return astCodecFieldStruct, value.Sel.Name, nil
	case *goast.Ident:
		switch value.Name {
		case "bool":
			return astCodecFieldBool, "", nil
		case "string":
			return astCodecFieldString, "", nil
		case "int", "int8", "int16", "int32", "int64":
			return astCodecFieldSigned, "", nil
		case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
			return astCodecFieldUnsigned, "", nil
		case "any":
			return astCodecFieldInterface, "", nil
		}
		item, ok := named[value.Name]
		if !ok {
			return astCodecFieldInvalid, "", fmt.Errorf("unresolved field type %s", value.Name)
		}
		if stack[value.Name] {
			return astCodecFieldInvalid, "", fmt.Errorf("recursive field type %s", value.Name)
		}
		if item.Struct {
			return astCodecFieldStruct, value.Name, nil
		}
		if item.Interface {
			return astCodecFieldInterface, "", nil
		}
		stack[value.Name] = true
		kind, target, err := astCodecClassifyExpr(item.Expression, named, stack)
		delete(stack, value.Name)
		if kind == astCodecFieldStruct && target == "" {
			target = value.Name
		}
		return kind, target, err
	default:
		return astCodecFieldInvalid, "", fmt.Errorf("unsupported field type %T", expression)
	}
}

func astCodecQualifiedType(source string, named map[string]astCodecNamed) (string, error) {
	expression, err := parser.ParseExpr(source)
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	var write func(goast.Expr) error
	write = func(value goast.Expr) error {
		switch item := value.(type) {
		case *goast.Ident:
			if _, exists := named[item.Name]; exists {
				output.WriteString("ast.")
			}
			output.WriteString(item.Name)
		case *goast.SelectorExpr:
			if err := write(item.X); err != nil {
				return err
			}
			output.WriteByte('.')
			output.WriteString(item.Sel.Name)
		case *goast.StarExpr:
			output.WriteByte('*')
			return write(item.X)
		case *goast.ArrayType:
			output.WriteByte('[')
			if item.Len != nil {
				if err := write(item.Len); err != nil {
					return err
				}
			}
			output.WriteByte(']')
			return write(item.Elt)
		case *goast.MapType:
			output.WriteString("map[")
			if err := write(item.Key); err != nil {
				return err
			}
			output.WriteByte(']')
			return write(item.Value)
		case *goast.InterfaceType:
			output.WriteString("any")
		case *goast.BasicLit:
			output.WriteString(item.Value)
		default:
			var nested bytes.Buffer
			if err := format.Node(&nested, token.NewFileSet(), value); err != nil {
				return err
			}
			output.Write(nested.Bytes())
		}
		return nil
	}
	if err := write(expression); err != nil {
		return "", err
	}
	return output.String(), nil
}

func astCodecContainsAST(source string, named map[string]astCodecNamed, stack map[string]bool) bool {
	expression, err := parser.ParseExpr(source)
	if err != nil {
		return false
	}
	return astCodecContainsASTExpr(expression, named, stack)
}

func astCodecContainsASTExpr(expression goast.Expr, named map[string]astCodecNamed, stack map[string]bool) bool {
	switch value := expression.(type) {
	case *goast.StarExpr:
		return astCodecContainsASTExpr(value.X, named, stack)
	case *goast.ArrayType:
		return astCodecContainsASTExpr(value.Elt, named, stack)
	case *goast.MapType:
		return astCodecContainsASTExpr(value.Key, named, stack) || astCodecContainsASTExpr(value.Value, named, stack)
	case *goast.InterfaceType:
		return true
	case *goast.SelectorExpr:
		return false
	case *goast.Ident:
		item, ok := named[value.Name]
		if !ok {
			return false
		}
		if item.Struct || item.Interface {
			return true
		}
		if stack[value.Name] {
			return false
		}
		stack[value.Name] = true
		result := astCodecContainsASTExpr(item.Expression, named, stack)
		delete(stack, value.Name)
		return result
	default:
		return false
	}
}

func validateASTCodecModel(model astCodecModel) error {
	if len(model.Structs) == 0 {
		return fmt.Errorf("grammarproof AST codec: no AST struct declarations")
	}
	seenTypes := make(map[string]bool, len(model.Structs))
	for _, item := range model.Structs {
		if item.Name == "" || seenTypes[item.Name] {
			return fmt.Errorf("grammarproof AST codec: duplicate product type %s", item.Name)
		}
		seenTypes[item.Name] = true
		seenFields := make(map[string]bool, len(item.Fields))
		for _, field := range item.Fields {
			if field.Name == "" || seenFields[field.Name] {
				return fmt.Errorf("grammarproof AST codec: duplicate exported field %s.%s", item.Name, field.Name)
			}
			if field.Kind == astCodecFieldInvalid {
				return fmt.Errorf("grammarproof AST codec: unsupported exported field %s.%s", item.Name, field.Name)
			}
			seenFields[field.Name] = true
		}
		all := make(map[string]bool, len(item.AllFields))
		for _, field := range item.AllFields {
			if field.Name == "" || all[field.Name] {
				return fmt.Errorf("grammarproof AST codec: duplicate declaration field %s.%s", item.Name, field.Name)
			}
			all[field.Name] = true
		}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func renderASTCodec(root string) ([]byte, error) {
	model, err := astCodecModelForRoot(root)
	if err != nil {
		return nil, err
	}
	var output strings.Builder
	output.WriteString("// Code generated by grammarproof AST declarations; DO NOT EDIT.\n\npackage astcodec\n\nimport \"github.com/wippyai/go-lua/compiler/ast\"\n\n")
	output.WriteString(astCodecSupportSource)
	for _, item := range model.Structs {
		output.WriteString("\n")
		output.WriteString(astCodecRenderStateHelper(item))
	}
	for _, name := range []string{"Position", "Span"} {
		if _, exists := model.Named[name]; exists {
			fmt.Fprintf(&output, "\nfunc astcodecIsZero%s(value ast.%s) bool { return value == (ast.%s{}) }\nfunc astcodecFieldState%s(value ast.%s) FieldState { if astcodecIsZero%s(value) { return astcodecFieldZero }; return astcodecFieldNonZero }\n", name, name, name, name, name, name)
		}
	}
	for _, item := range model.Structs {
		fmt.Fprintf(&output, "\nfunc astcodecEncode%s(value ast.%s) (Occurrence, bool) {\n", item.Name, item.Name)
		fmt.Fprintf(&output, "\toccurrence := Occurrence{Type: %q}\n", item.Name)
		if item.HasPosition {
			output.WriteString("\toccurrence.StartLine = value.Line()\n\toccurrence.StartCol = value.Column()\n\toccurrence.EndLine = value.LastLine()\n\toccurrence.EndCol = value.LastColumn()\n")
		}
		if len(item.Fields) != 0 {
			fmt.Fprintf(&output, "\toccurrence.Fields = make([]Field, 0, %d)\n", len(item.Fields))
		}
		for _, field := range item.Fields {
			state := astCodecStateExpression(field)
			valueExpression := astCodecValueExpression(field)
			fmt.Fprintf(&output, "\toccurrence.Fields = append(occurrence.Fields, Field{Name: %q, State: %s, Value: %s})\n", field.Name, state, valueExpression)
		}
		output.WriteString("\treturn occurrence, true\n}\n")
	}
	output.WriteString("\n// Encode returns one direct typed occurrence. It never walks child fields.\nfunc Encode(value any) (Occurrence, bool) {\n\tswitch typed := value.(type) {\n")
	for _, item := range model.Structs {
		fmt.Fprintf(&output, "\tcase ast.%s:\n\t\treturn astcodecEncode%s(typed)\n\tcase *ast.%s:\n\t\tif typed == nil { return Occurrence{}, false }\n\t\treturn astcodecEncode%s(*typed)\n", item.Name, item.Name, item.Name, item.Name)
	}
	output.WriteString("\tdefault:\n\t\treturn Occurrence{}, false\n\t}\n}\n")

	for _, item := range model.Structs {
		fmt.Fprintf(&output, "\nfunc astcodecPush%s(value ast.%s, stack *[]any) {\n", item.Name, item.Name)
		for index := len(item.Fields) - 1; index >= 0; index-- {
			field := item.Fields[index]
			if astCodecVisitField(field) {
				fmt.Fprintf(&output, "\t*stack = append(*stack, value.%s)\n", field.Name)
			}
		}
		output.WriteString("}\n")
	}
	output.WriteString("\n// Observe iteratively visits typed AST values without reflection or a depth\n// cap. Pointer identity is retained so recursive/shared AST graphs have the\n// same cycle boundary as the former cold observer.\nfunc Observe(value any) []Occurrence {\n\tvar result []Occurrence\n\tseen := make(map[any]struct{})\n\tstack := []any{value}\n\tfor len(stack) != 0 {\n\t\tcurrent := stack[len(stack)-1]\n\t\tstack = stack[:len(stack)-1]\n\t\tswitch typed := current.(type) {\n")
	for _, item := range model.Structs {
		fmt.Fprintf(&output, "\t\tcase ast.%s:\n\t\t\tif occurrence, ok := astcodecEncode%s(typed); ok { result = append(result, occurrence) }\n\t\t\tastcodecPush%s(typed, &stack)\n\t\tcase *ast.%s:\n\t\t\tif typed == nil { continue }\n\t\t\tif _, ok := seen[typed]; ok { continue }\n\t\t\tseen[typed] = struct{}{}\n\t\t\tif occurrence, ok := astcodecEncode%s(*typed); ok { result = append(result, occurrence) }\n\t\t\tastcodecPush%s(*typed, &stack)\n", item.Name, item.Name, item.Name, item.Name, item.Name, item.Name)
	}
	for _, collection := range model.Slices {
		fmt.Fprintf(&output, "\t\tcase %s:\n\t\t\tfor index := len(typed) - 1; index >= 0; index-- { stack = append(stack, typed[index]) }\n", collection)
	}
	for _, collection := range model.Arrays {
		fmt.Fprintf(&output, "\t\tcase %s:\n\t\t\tfor index := len(typed) - 1; index >= 0; index-- { stack = append(stack, typed[index]) }\n", collection)
	}
	for _, collection := range model.Maps {
		fmt.Fprintf(&output, "\t\tcase %s:\n\t\t\titems := make([]any, 0, len(typed))\n\t\t\tfor _, item := range typed { items = append(items, item) }\n\t\t\tfor index := len(items) - 1; index >= 0; index-- { stack = append(stack, items[index]) }\n", collection)
	}
	output.WriteString("\t\t}\n\t}\n\treturn result\n}\n")

	output.WriteString("\n// Product describes the generated type/field bijection. It lets the\n// generator fail closed when this checked-in codec differs from compiler/ast.\ntype Product struct { Type string; Fields []string }\n\nfunc Schema() []Product {\n\treturn []Product{\n")
	for _, item := range model.Structs {
		fmt.Fprintf(&output, "\t\t{Type: %q, Fields: []string{", item.Name)
		for _, field := range item.Fields {
			fmt.Fprintf(&output, "%q,", field.Name)
		}
		output.WriteString("}},\n")
	}
	output.WriteString("\t}\n}\n")
	formatted, err := format.Source([]byte(output.String()))
	if err != nil {
		return nil, fmt.Errorf("grammarproof AST codec: format generated source: %w", err)
	}
	return formatted, nil
}

func astCodecVisitField(field astCodecField) bool {
	return field.ContainsAST
}

func astCodecStateExpression(field astCodecField) string {
	expression := "value." + field.Name
	switch field.Kind {
	case astCodecFieldPointer:
		return "astcodecFieldStatePointer(" + expression + ")"
	case astCodecFieldInterface:
		return "astcodecFieldStateInterface(" + expression + ")"
	case astCodecFieldSlice:
		return "astcodecFieldStateSlice(" + expression + ")"
	case astCodecFieldArray:
		return "astcodecFieldStateArrayValue(len(" + expression + "))"
	case astCodecFieldMap:
		return "astcodecFieldStateMap(" + expression + ")"
	case astCodecFieldBool:
		return "astcodecFieldStateBool(" + expression + ")"
	case astCodecFieldString:
		return "astcodecFieldStateString(" + expression + ")"
	case astCodecFieldSigned:
		return "astcodecFieldStateSigned(" + expression + ")"
	case astCodecFieldUnsigned:
		return "astcodecFieldStateUnsigned(" + expression + ")"
	case astCodecFieldStruct:
		return "astcodecFieldState" + field.StructName + "(" + expression + ")"
	default:
		return "FieldStateInvalid"
	}
}

func astCodecValueExpression(field astCodecField) string {
	expression := "value." + field.Name
	switch field.Kind {
	case astCodecFieldSigned:
		return "astcodecFieldValueSigned(" + expression + ")"
	case astCodecFieldUnsigned:
		return "astcodecFieldValueUnsigned(" + expression + ")"
	default:
		return "0"
	}
}

func astCodecRenderStateHelper(item astCodecStruct) string {
	var output strings.Builder
	fmt.Fprintf(&output, "func astcodecIsZero%s(value ast.%s) bool {\n", item.Name, item.Name)
	conditions := make([]string, 0, len(item.Fields)+1)
	if item.HasPosition || item.HasPrivatePosition {
		conditions = append(conditions, "value.Line() == 0", "value.Column() == 0", "value.LastLine() == 0", "value.LastColumn() == 0")
	}
	for _, field := range item.Fields {
		conditions = append(conditions, astCodecZeroExpression(field))
	}
	if len(conditions) == 0 {
		output.WriteString("\treturn true\n")
	} else {
		output.WriteString("\treturn ")
		output.WriteString(strings.Join(conditions, " && "))
		output.WriteString("\n")
	}
	output.WriteString("}\n")
	fmt.Fprintf(&output, "\nfunc astcodecFieldState%s(value ast.%s) FieldState {\n\tif astcodecIsZero%s(value) { return astcodecFieldZero }\n\treturn astcodecFieldNonZero\n}\n", item.Name, item.Name, item.Name)
	return output.String()
}

func astCodecZeroExpression(field astCodecField) string {
	expression := "value." + field.Name
	switch field.Kind {
	case astCodecFieldPointer, astCodecFieldInterface, astCodecFieldMap, astCodecFieldSlice:
		return expression + " == nil"
	case astCodecFieldArray:
		return "len(" + expression + ") == 0"
	case astCodecFieldBool:
		return "!" + expression
	case astCodecFieldString:
		return expression + " == \"\""
	case astCodecFieldSigned, astCodecFieldUnsigned:
		return expression + " == 0"
	case astCodecFieldStruct:
		return "astcodecIsZero" + field.StructName + "(" + expression + ")"
	default:
		return "false"
	}
}

const astCodecSupportSource = `const (
	astcodecFieldAbsent = FieldStateAbsent
	astcodecFieldPresent = FieldStatePresent
	astcodecFieldEmpty = FieldStateEmpty
	astcodecFieldNonEmpty = FieldStateNonEmpty
	astcodecFieldFalse = FieldStateFalse
	astcodecFieldTrue = FieldStateTrue
	astcodecFieldZero = FieldStateZero
	astcodecFieldNonZero = FieldStateNonZero
)

func astcodecFieldStatePointer[T any](value *T) FieldState {
	if value == nil { return astcodecFieldAbsent }
	return astcodecFieldPresent
}

func astcodecFieldStateInterface(value any) FieldState {
	if value == nil { return astcodecFieldAbsent }
	return astcodecFieldPresent
}

func astcodecFieldStateSlice[T any](value []T) FieldState {
	if len(value) == 0 { return astcodecFieldEmpty }
	return astcodecFieldNonEmpty
}

func astcodecFieldStateArrayValue(length int) FieldState {
	if length == 0 { return astcodecFieldEmpty }
	return astcodecFieldNonEmpty
}

func astcodecFieldStateMap[K comparable, V any](value map[K]V) FieldState {
	if value == nil { return astcodecFieldAbsent }
	return astcodecFieldPresent
}

func astcodecFieldStateBool(value bool) FieldState {
	if value { return astcodecFieldTrue }
	return astcodecFieldFalse
}

func astcodecFieldStateString(value string) FieldState {
	if value == "" { return astcodecFieldEmpty }
	return astcodecFieldNonEmpty
}

func astcodecFieldStateSigned[T ~int | ~int8 | ~int16 | ~int32 | ~int64](value T) FieldState {
	if value == 0 { return astcodecFieldZero }
	return astcodecFieldNonZero
}

func astcodecFieldValueSigned[T ~int | ~int8 | ~int16 | ~int32 | ~int64](value T) uint64 {
	if value < 0 { return 0 }
	return uint64(value)
}

func astcodecFieldStateUnsigned[T ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr](value T) FieldState {
	if value == 0 { return astcodecFieldZero }
	return astcodecFieldNonZero
}

func astcodecFieldValueUnsigned[T ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr](value T) uint64 {
	return uint64(value)
}
`

// ValidateGeneratedASTCodec compares the checked-in generated schema to the
// declarations that would be rendered today. A missing or extra type/field
// fails closed before any cold parser trace is trusted.
func ValidateGeneratedASTCodec(root string) error {
	model, err := astCodecModelForRoot(root)
	if err != nil {
		return err
	}
	actual := astcodec.Schema()
	if len(actual) != len(model.Structs) {
		return fmt.Errorf("grammarproof AST codec has %d products, declarations have %d", len(actual), len(model.Structs))
	}
	for index, item := range model.Structs {
		if actual[index].Type != item.Name || !sameStrings(actual[index].Fields, astCodecFieldNames(item.Fields)) {
			return fmt.Errorf("grammarproof AST codec product %s differs from compiler/ast declaration", item.Name)
		}
	}
	return nil
}

func astCodecFieldNames(fields []astCodecField) []string {
	result := make([]string, len(fields))
	for index, field := range fields {
		result[index] = field.Name
	}
	return result
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
