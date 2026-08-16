package census

import (
	"fmt"
	goast "go/ast"
	"sort"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/grammar"
)

// Build derives the complete census from parser.go.y semantic actions and the
// compiler/ast declaration graph. It deliberately does not consume the
// generated parser, fixture corpus, or any Program package.
func Build(root string) (Census, error) {
	productions, helperConstructors, err := actionTemplates(root)
	if err != nil {
		return Census{}, err
	}
	schema, err := grammar.Discover(root)
	if err != nil {
		return Census{}, fmt.Errorf("parser census: discover AST declarations: %w", err)
	}
	productions, err = filterProductionConstructors(productions, schema)
	if err != nil {
		return Census{}, err
	}
	constructors, err := constructorsFromActions(productions, helperConstructors, schema)
	if err != nil {
		return Census{}, err
	}
	grammarDigest, err := grammarproof.ParserSourceDigest(root)
	if err != nil {
		return Census{}, fmt.Errorf("parser census: digest parser.go.y: %w", err)
	}
	astOnly := grammar.Schema{Declarations: schema.Declarations, Types: schema.Types}
	result := Census{
		GrammarSourceDigest: grammarDigest,
		ASTDigest:           astOnly.Digest(),
		Productions:         productions,
		Constructors:        constructors,
	}
	result.Digest = digest(result)
	return result, nil
}

func filterProductionConstructors(productions []grammarproof.ActionTemplate, schema grammar.Schema) ([]grammarproof.ActionTemplate, error) {
	declarations := make(map[string]bool, len(schema.Declarations))
	for _, declaration := range schema.Declarations {
		declarations[declaration.Name] = true
	}
	types := make(map[string]bool, len(schema.Types))
	for _, declaration := range schema.Types {
		types[declaration.Name] = true
	}
	result := make([]grammarproof.ActionTemplate, len(productions))
	for index, production := range productions {
		result[index] = production
		result[index].Constructors = nil
		for _, name := range production.Constructors {
			switch {
			case declarations[name]:
				result[index].Constructors = append(result[index].Constructors, name)
			case types[name]:
				// A parser action may construct a source-coordinate alias or
				// use a named interface as an operand. Neither is an AST
				// constructor row.
			default:
				return nil, fmt.Errorf("parser census: parser action cites ast.%s without an AST declaration", name)
			}
		}
	}
	return result, nil
}

func actionTemplates(root string) ([]grammarproof.ActionTemplate, []string, error) {
	var result []grammarproof.ActionTemplate
	var helperConstructors []string
	err := grammarproof.VisitActionSyntax(root, func(template grammarproof.ActionTemplate, block *goast.BlockStmt) error {
		// The existing ActionTemplate index is intentionally lexical and can
		// also see ast constants. The census needs concrete constructor
		// literals only, so derive this narrow projection from the parsed Go
		// action while retaining ActionTemplate as the row vocabulary.
		template.Constructors = compositeConstructors(block)
		result = append(result, template)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("parser census: derive parser.go.y productions: %w", err)
	}
	err = grammarproof.VisitHelperSyntax(root, func(_ grammarproof.HelperTemplate, function *goast.FuncDecl) error {
		if function != nil && function.Body != nil {
			helperConstructors = append(helperConstructors, compositeConstructors(function.Body)...)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("parser census: derive parser.go.y helpers: %w", err)
	}
	if len(result) == 0 {
		return nil, nil, fmt.Errorf("parser census: parser.go.y has no semantic productions")
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Key < result[right].Key })
	sort.Strings(helperConstructors)
	return result, helperConstructors, nil
}

func compositeConstructors(root goast.Node) []string {
	seen := make(map[string]bool)
	goast.Inspect(root, func(node goast.Node) bool {
		literal, ok := node.(*goast.CompositeLit)
		if !ok {
			return true
		}
		selector, ok := literal.Type.(*goast.SelectorExpr)
		if !ok || selector.Sel == nil {
			return true
		}
		qualifier, ok := selector.X.(*goast.Ident)
		if !ok || qualifier.Name != "ast" {
			return true
		}
		seen[selector.Sel.Name] = true
		return true
	})
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func constructorsFromActions(productions []grammarproof.ActionTemplate, helpers []string, schema grammar.Schema) ([]grammar.Constructor, error) {
	declarations := make(map[string]grammar.Declaration, len(schema.Declarations))
	for _, declaration := range schema.Declarations {
		declarations[declaration.Name] = declaration
	}
	types := make(map[string]grammar.TypeDeclaration, len(schema.Types))
	for _, declaration := range schema.Types {
		types[declaration.Name] = declaration
	}
	seen := make(map[string]bool)
	for _, production := range productions {
		for _, name := range production.Constructors {
			if seen[name] {
				continue
			}
			seen[name] = true
			if _, ok := declarations[name]; ok {
				continue
			}
			if _, ok := types[name]; ok {
				// Aliases and interfaces can be parser action operands but
				// are not concrete AST constructor rows.
				continue
			}
			return nil, fmt.Errorf("parser census: parser action constructs ast.%s without an AST declaration", name)
		}
	}
	for _, name := range helpers {
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, ok := declarations[name]; ok {
			continue
		}
		if _, ok := types[name]; ok {
			continue
		}
		return nil, fmt.Errorf("parser census: parser helper constructs ast.%s without an AST declaration", name)
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		if _, ok := declarations[name]; ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	result := make([]grammar.Constructor, 0, len(names))
	for _, name := range names {
		declaration := declarations[name]
		result = append(result, grammar.Constructor{
			Name: declaration.Name, Class: declaration.Class, Semantic: declaration.Semantic,
			Fields: append([]grammar.Field(nil), declaration.Fields...),
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("parser census: parser.go.y constructs no concrete AST declarations")
	}
	return result, nil
}
