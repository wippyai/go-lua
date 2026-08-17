package parsersource

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"
)

// SymbolKind separates the two halves of the grammar vocabulary. A terminal
// carries whatever the scanner stamped on it, so a fact about its value is a
// lexer fact; a nonterminal carries whatever its own alternatives reduced to,
// so a fact about its value is a fact about those alternatives.
type SymbolKind uint8

const (
	SymbolInvalid SymbolKind = iota
	SymbolTerminal
	SymbolNonterminal
)

// GrammarSymbol is one declared parser.go.y symbol together with the Go type
// its semantic value has. Tag is the %union member parser.go.y declares for the
// symbol and Type is that member's declared Go type; a symbol declared without
// a tag carries no semantic value and states both as empty rather than
// defaulting to one.
type GrammarSymbol struct {
	Name string
	Kind SymbolKind
	Tag  string
	Type string
}

// GrammarVocabulary is the complete declared symbol universe of parser.go.y.
// It is stated as one value because a semantic action indexes its right-hand
// side positionally: a reader that resolved only nonterminals would silently
// mistype every token operand.
type GrammarVocabulary struct {
	Symbols []GrammarSymbol
}

// Symbol resolves one declared symbol by its parser.go.y spelling.
func (v GrammarVocabulary) Symbol(name string) (GrammarSymbol, bool) {
	for _, symbol := range v.Symbols {
		if symbol.Name == name {
			return symbol, true
		}
	}
	return GrammarSymbol{}, false
}

var yaccToken = regexp.MustCompile(`(?m)^\s*%token(?:<([^>]+)>)?\s+(.+)$`)

// DiscoverVocabulary derives the declared grammar symbol universe from
// parser.go.y alone: the %token declarations name the terminals, the %type
// declarations name the nonterminals, and the %union declaration gives both
// their semantic Go type. Nothing here reads the generated parser or observes
// a parse.
func DiscoverVocabulary(path string) (GrammarVocabulary, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return GrammarVocabulary{}, err
	}
	source := string(contents)
	union, err := declaredUnion(path, source)
	if err != nil {
		return GrammarVocabulary{}, err
	}
	tags, err := DeclaredResultTags(path)
	if err != nil {
		return GrammarVocabulary{}, fmt.Errorf("parser grammar %s: %w", path, err)
	}
	symbols := make(map[string]GrammarSymbol)
	declare := func(name string, kind SymbolKind, tag string) error {
		if existing, seen := symbols[name]; seen && existing.Kind != kind {
			return fmt.Errorf("parser grammar %s: symbol %s is declared both terminal and nonterminal", path, name)
		}
		typeName := ""
		if tag != "" {
			declared, known := union[tag]
			if !known {
				return fmt.Errorf("parser grammar %s: symbol %s uses %%union member %s which is not declared", path, name, tag)
			}
			typeName = sourceExpr(declared)
		}
		symbols[name] = GrammarSymbol{Name: name, Kind: kind, Tag: tag, Type: typeName}
		return nil
	}
	masked, err := maskQuotedAndComments(source)
	if err != nil {
		return GrammarVocabulary{}, err
	}
	for _, match := range yaccToken.FindAllStringSubmatchIndex(masked, -1) {
		tag := ""
		if match[2] >= 0 {
			tag = source[match[2]:match[3]]
		}
		for _, name := range grammarSymbols(source[match[4]:match[5]]) {
			if err := declare(name, SymbolTerminal, tag); err != nil {
				return GrammarVocabulary{}, err
			}
		}
	}
	for name, tag := range tags {
		if err := declare(name, SymbolNonterminal, tag); err != nil {
			return GrammarVocabulary{}, err
		}
	}
	if len(symbols) == 0 {
		return GrammarVocabulary{}, fmt.Errorf("parser grammar %s: declares no symbols", path)
	}
	result := GrammarVocabulary{Symbols: make([]GrammarSymbol, 0, len(symbols))}
	for _, symbol := range symbols {
		result.Symbols = append(result.Symbols, symbol)
	}
	sort.Slice(result.Symbols, func(left, right int) bool { return result.Symbols[left].Name < result.Symbols[right].Name })
	return result, nil
}

// declaredUnion reads the %union declaration as the Go struct it is. yacc
// copies the member list into the generated parser verbatim, so parsing it as
// Go source is reading it in its own language rather than re-lexing it. Each
// arm is kept as the declared type expression, so a caller that needs the
// member's form reads the declaration rather than a rendering of it.
func declaredUnion(path, source string) (map[string]goast.Expr, error) {
	masked, err := maskQuotedAndComments(source)
	if err != nil {
		return nil, err
	}
	start := strings.Index(masked, "%union")
	if start < 0 {
		return nil, fmt.Errorf("parser grammar %s: has no %%union declaration", path)
	}
	open := strings.IndexByte(masked[start:], '{')
	if open < 0 {
		return nil, fmt.Errorf("parser grammar %s: %%union declaration has no body", path)
	}
	end, err := scanGoAction(source, start+open)
	if err != nil {
		return nil, fmt.Errorf("parser grammar %s: %%union declaration: %w", path, err)
	}
	body := source[start+open : end]
	file, parseErr := parser.ParseFile(token.NewFileSet(), "parser-union.go", "package parserunion\ntype union struct "+body, 0)
	if parseErr != nil {
		return nil, fmt.Errorf("parser grammar %s: parse %%union declaration: %w", path, parseErr)
	}
	result := make(map[string]goast.Expr)
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
				continue
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					if _, duplicate := result[name.Name]; duplicate {
						return nil, fmt.Errorf("parser grammar %s: %%union declares member %s twice", path, name.Name)
					}
					result[name.Name] = field.Type
				}
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("parser grammar %s: %%union declares no members", path)
	}
	return result, nil
}

// ProductionSymbols is the right-hand symbol sequence a semantic action indexes
// with $1, $2 and so on. A trailing %prec directive is a precedence override
// rather than a grammar symbol, so it is removed here: leaving it in would let
// a positional operand resolve against a precedence token.
func ProductionSymbols(rhs []string) ([]string, error) {
	for index, symbol := range rhs {
		if symbol != "%prec" {
			continue
		}
		if index != len(rhs)-2 {
			return nil, fmt.Errorf("parser grammar: %%prec directive is not the final right-hand element")
		}
		return append([]string(nil), rhs[:index]...), nil
	}
	return append([]string(nil), rhs...), nil
}
