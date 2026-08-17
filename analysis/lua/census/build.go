// Package census owns the parser/AST construct census: the denominator of
// every parser alternative, every AST form those alternatives construct,
// every exported carrier of those forms, every representation state those
// carriers admit, every whole-constructor field vector an action builds, and
// every typed slot at which one of those values is consumed. It is cold source
// evidence only. Generation reads parser.go.y and the compiler/ast
// declarations, and never runs a fixture or constructs a Program.
//
// The last two grains are one relation read from its two ends: a product row
// states what a construction builds, and a use row states where each built
// value lands. Together they close it - a construction the parser performs is
// either handed back by its action or consumed at exactly one typed slot.
//
// Laws stated over those rows live here too, so a law whose premises are rows
// stays with the rows: the structural induction premises are one such law.
//
// The census is consumed by the sealed catalog's grammar disposition join,
// which accounts for each row. It lives outside the internal tree deliberately:
// the account is stated at the seal, so a denominator fenced to the frontend
// could not reach it.
//
//go:generate go run ./cmd/generate -root ../../../ -out census_gen.go
package census

import (
	"fmt"
	goast "go/ast"
	"sort"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// Build derives the complete census from parser.go.y semantic actions and the
// compiler/ast declaration graph. It deliberately does not consume the
// generated parser, fixture corpus, or any Program package.
func Build(root string) (Census, error) {
	productions, helperConstructors, err := actionTemplates(root)
	if err != nil {
		return Census{}, err
	}
	schema, err := parsersource.Discover(root)
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
	grammarDigest, err := parsersource.ParserSourceDigest(root)
	if err != nil {
		return Census{}, fmt.Errorf("parser census: digest parser.go.y: %w", err)
	}
	analysis, err := parsersource.DiscoverProducts(root)
	if err != nil {
		return Census{}, err
	}
	// The slot denominator is stated over the census's own form rows, so every
	// slot it publishes belongs to a carrier row the same census states.
	slots, err := parsersource.UseSlots(parsersource.Schema{Constructors: constructors, Types: schema.Types})
	if err != nil {
		return Census{}, err
	}
	astOnly := parsersource.Schema{Declarations: schema.Declarations, Types: schema.Types}
	result := Census{
		GrammarSourceDigest: grammarDigest,
		ASTDigest:           astOnly.Digest(),
		Productions:         productions,
		Constructors:        constructors,
		Products:            analysis.Products,
		Mutations:           analysis.Mutations,
		Slots:               slots,
		Uses:                analysis.Uses,
	}
	result.Digest = digest(result)
	return result, nil
}

func filterProductionConstructors(productions []parsersource.ActionTemplate, schema parsersource.Schema) ([]parsersource.ActionTemplate, error) {
	declarations := make(map[string]bool, len(schema.Declarations))
	for _, declaration := range schema.Declarations {
		declarations[declaration.Name] = true
	}
	types := make(map[string]bool, len(schema.Types))
	for _, declaration := range schema.Types {
		types[declaration.Name] = true
	}
	result := make([]parsersource.ActionTemplate, len(productions))
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

func actionTemplates(root string) ([]parsersource.ActionTemplate, []string, error) {
	helpers, helperConstructors, err := helperClosure(root)
	if err != nil {
		return nil, nil, err
	}
	var result []parsersource.ActionTemplate
	err = parsersource.VisitActionSyntax(root, func(template parsersource.ActionTemplate, block *goast.BlockStmt) error {
		// The existing ActionTemplate index is intentionally lexical and can
		// also see ast constants. The census needs concrete constructor
		// literals only, so derive this narrow projection from the parsed Go
		// action while retaining ActionTemplate as the row vocabulary.
		//
		// A production which builds its result through a parser.go.y helper
		// constructs that helper's AST values just as directly as an inline
		// composite literal does. Attribution therefore follows the helper
		// call, so a production row states the forms its reduction builds and
		// an empty column means the reduction builds none.
		template.Constructors = union(compositeConstructors(block), calledConstructors(block, helpers))
		result = append(result, template)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("parser census: derive parser.go.y productions: %w", err)
	}
	if len(result) == 0 {
		return nil, nil, fmt.Errorf("parser census: parser.go.y has no semantic productions")
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Key < result[right].Key })
	return result, helperConstructors, nil
}

// helper is one parser.go.y helper's own construction facts: the AST values its
// body builds directly and the helpers it delegates to.
type helper struct {
	constructors []string
	calls        []string
}

// helperClosure resolves every parser.go.y helper to the complete set of AST
// values a call to it constructs, following helper-to-helper delegation to a
// fixpoint. The second result is the sorted union across all helpers, which is
// the constructor universe a helper contributes even when no action reaches it.
func helperClosure(root string) (map[string][]string, []string, error) {
	direct := make(map[string]helper)
	err := parsersource.VisitHelperSyntax(root, func(template parsersource.HelperTemplate, function *goast.FuncDecl) error {
		if function == nil || function.Body == nil || function.Name == nil {
			return nil
		}
		direct[function.Name.Name] = helper{
			constructors: compositeConstructors(function.Body),
			calls:        calledNames(function.Body),
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("parser census: derive parser.go.y helpers: %w", err)
	}
	closure := make(map[string][]string, len(direct))
	for name, facts := range direct {
		closure[name] = append([]string(nil), facts.constructors...)
	}
	// Delegation is a finite graph over a finite helper set, so unioning callee
	// sets into callers until nothing grows terminates. Recursive helpers are
	// admitted by the same fixpoint rather than by a depth guard.
	for changed := true; changed; {
		changed = false
		for name, facts := range direct {
			for _, callee := range facts.calls {
				reached, known := closure[callee]
				if !known {
					continue
				}
				merged := union(closure[name], reached)
				if len(merged) != len(closure[name]) {
					closure[name] = merged
					changed = true
				}
			}
		}
	}
	var all []string
	for _, constructors := range closure {
		all = union(all, constructors)
	}
	return closure, all, nil
}

// calledConstructors is the union of the closures of every helper the node
// calls.
func calledConstructors(node goast.Node, helpers map[string][]string) []string {
	var result []string
	for _, name := range calledNames(node) {
		if reached, known := helpers[name]; known {
			result = union(result, reached)
		}
	}
	return result
}

// calledNames is every plain function name the node calls. Selector calls are
// method or package calls and are never parser.go.y helpers.
func calledNames(node goast.Node) []string {
	seen := make(map[string]bool)
	goast.Inspect(node, func(current goast.Node) bool {
		call, ok := current.(*goast.CallExpr)
		if !ok {
			return true
		}
		if callee, ok := call.Fun.(*goast.Ident); ok {
			seen[callee.Name] = true
		}
		return true
	})
	return sorted(seen)
}

func union(left, right []string) []string {
	seen := make(map[string]bool, len(left)+len(right))
	for _, name := range left {
		seen[name] = true
	}
	for _, name := range right {
		seen[name] = true
	}
	return sorted(seen)
}

func sorted(seen map[string]bool) []string {
	if len(seen) == 0 {
		return nil
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
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
	return sorted(seen)
}

func constructorsFromActions(productions []parsersource.ActionTemplate, helpers []string, schema parsersource.Schema) ([]parsersource.Constructor, error) {
	declarations := make(map[string]parsersource.Declaration, len(schema.Declarations))
	for _, declaration := range schema.Declarations {
		declarations[declaration.Name] = declaration
	}
	types := make(map[string]parsersource.TypeDeclaration, len(schema.Types))
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
	result := make([]parsersource.Constructor, 0, len(names))
	for _, name := range names {
		declaration := declarations[name]
		result = append(result, parsersource.Constructor{
			Name: declaration.Name, Class: declaration.Class, Semantic: declaration.Semantic,
			Fields: append([]parsersource.Field(nil), declaration.Fields...),
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("parser census: parser.go.y constructs no concrete AST declarations")
	}
	return result, nil
}
