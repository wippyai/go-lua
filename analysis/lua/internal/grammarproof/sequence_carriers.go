package grammarproof

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// SequenceCarrier is a yacc-result slice location. A blank Field denotes the
// result value itself; a nonblank Field is a slice owned by a parser-private
// result wrapper. It is derived from %union and the parser preamble together,
// never from a handwritten list of wrappers.
type SequenceCarrier struct {
	Tag   string
	Field string
}

// SequenceCarriers exposes the complete parser sequence denominator. Every
// %type tag must resolve to exactly one %union arm. Slice arms are root
// carriers; struct-valued arms resolve their declared slice fields from the
// yacc preamble. Unsupported declarations fail rather than becoming an
// implicit "not a sequence" exemption.
func SequenceCarriers(root string) ([]SequenceCarrier, error) {
	path := filepath.Join(root, "compiler", "parse", "parser.go.y")
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	source := string(contents)
	tags, err := parsersource.DeclaredResultTags(path)
	if err != nil {
		return nil, err
	}
	union, err := yaccUnion(source)
	if err != nil {
		return nil, fmt.Errorf("parser grammar %s: %w", path, err)
	}
	preamble, err := yaccPreamble(source)
	if err != nil {
		return nil, fmt.Errorf("parser grammar %s: %w", path, err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path+":preamble", preamble, 0)
	if err != nil {
		return nil, fmt.Errorf("parser grammar %s: parse preamble: %w", path, err)
	}
	structs := make(map[string]*goast.StructType)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*goast.GenDecl)
		if !ok || general.Tok.String() != "type" {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*goast.TypeSpec)
			if !ok {
				continue
			}
			if body, ok := typeSpec.Type.(*goast.StructType); ok {
				structs[typeSpec.Name.Name] = body
			}
		}
	}

	rows := make([]SequenceCarrier, 0)
	seenTags := make(map[string]bool, len(tags))
	for nonterminal, tag := range tags {
		arm, ok := union[tag]
		if !ok {
			return nil, fmt.Errorf("%%type<%s> %s has no %%union arm", tag, nonterminal)
		}
		if seenTags[tag] {
			continue
		}
		seenTags[tag] = true
		if _, ok := arm.(*goast.ArrayType); ok {
			rows = append(rows, SequenceCarrier{Tag: tag})
			continue
		}
		name, ok := arm.(*goast.Ident)
		if !ok {
			// A scalar arm has a deliberate non-sequence disposition. Its
			// exact type remains checked above rather than default-ignored.
			continue
		}
		body, private := structs[name.Name]
		if !private {
			continue
		}
		for _, field := range body.Fields.List {
			if _, ok := field.Type.(*goast.ArrayType); !ok {
				continue
			}
			if len(field.Names) != 1 || field.Names[0] == nil || field.Names[0].Name == "" {
				return nil, fmt.Errorf("private result wrapper %s has an unnamed or grouped slice field", name.Name)
			}
			rows = append(rows, SequenceCarrier{Tag: tag, Field: field.Names[0].Name})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Tag < rows[j].Tag || rows[i].Tag == rows[j].Tag && rows[i].Field < rows[j].Field
	})
	for index, row := range rows {
		if row.Tag == "" || index > 0 && rows[index-1] == row {
			return nil, fmt.Errorf("sequence carriers are not a canonical relation")
		}
	}
	return rows, nil
}

func yaccPreamble(source string) (string, error) {
	start := strings.Index(source, "%{")
	if start < 0 {
		return "", fmt.Errorf("parser preamble start is missing")
	}
	end := strings.Index(source[start+2:], "%}")
	if end < 0 {
		return "", fmt.Errorf("parser preamble end is missing")
	}
	return source[start+2 : start+2+end], nil
}

func yaccUnion(source string) (map[string]goast.Expr, error) {
	start := strings.Index(source, "%union")
	if start < 0 {
		return nil, fmt.Errorf("%%union is missing")
	}
	open := strings.Index(source[start:], "{")
	if open < 0 {
		return nil, fmt.Errorf("%%union body is missing")
	}
	open += start
	close := strings.Index(source[open+1:], "}")
	if close < 0 {
		return nil, fmt.Errorf("%%union body is unterminated")
	}
	body := source[open+1 : open+1+close]
	file, err := parser.ParseFile(token.NewFileSet(), "parser-union.go", "package parse\ntype parserUnion struct {\n"+body+"\n}\n", 0)
	if err != nil {
		return nil, fmt.Errorf("parse %%union: %w", err)
	}
	declaration, ok := file.Decls[0].(*goast.GenDecl)
	if !ok || len(declaration.Specs) != 1 {
		return nil, fmt.Errorf("parse %%union declaration")
	}
	typeSpec, ok := declaration.Specs[0].(*goast.TypeSpec)
	if !ok {
		return nil, fmt.Errorf("parse %%union type")
	}
	bodyType, ok := typeSpec.Type.(*goast.StructType)
	if !ok {
		return nil, fmt.Errorf("parse %%union body")
	}
	result := make(map[string]goast.Expr, len(bodyType.Fields.List))
	for _, field := range bodyType.Fields.List {
		if len(field.Names) != 1 || field.Names[0] == nil || field.Names[0].Name == "" {
			return nil, fmt.Errorf("%%union has an unnamed or grouped arm")
		}
		name := field.Names[0].Name
		if result[name] != nil {
			return nil, fmt.Errorf("duplicate %%union arm %s", name)
		}
		result[name] = field.Type
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%%union has no arms")
	}
	return result, nil
}
