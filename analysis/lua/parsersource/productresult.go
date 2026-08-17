package parsersource

import (
	"fmt"
	goast "go/ast"
	"sort"
)

// result reads the solved relation out as rows. Products are ordered by the
// walk that starts at the value each scope yields, so the construction a
// reduction hands back is stated before the constructions that only exist to
// fill it.
func (b *productBuilder) result() (ProductAnalysis, error) {
	analysis := ProductAnalysis{}
	for _, scope := range b.scopes {
		products, ordered, err := b.scopeProducts(scope)
		if err != nil {
			return ProductAnalysis{}, err
		}
		analysis.Products = append(analysis.Products, products...)
		analysis.Uses = append(analysis.Uses, b.scopeUses(scope, ordered)...)
		sequences, sequenceErr := b.scopeSequences(scope, ordered)
		if sequenceErr != nil {
			return ProductAnalysis{}, sequenceErr
		}
		analysis.Sequences = append(analysis.Sequences, sequences...)
	}
	analysis.Mutations = b.scopeMutations()
	if len(analysis.Products) == 0 {
		return ProductAnalysis{}, fmt.Errorf("parser products: parser.go.y constructs no semantic AST values")
	}
	return analysis, nil
}

// scopeProducts walks one scope from the values it yields. An identifier is
// followed to the expressions bound to it, so a construction reached only
// through a local name is stated in the position its consumer gives it.
func (b *productBuilder) scopeProducts(scope *actionScope) ([]ActionProduct, []siteID, error) {
	var products []ActionProduct
	var ordered []siteID
	seenLiteral := make(map[*goast.CompositeLit]bool)
	seenName := make(map[string]bool)
	// nested marks that the walk reached this expression through a coordinate of
	// a construction rather than through a value the action yields, which is the
	// difference between a construction the reduction hands back and one that
	// only exists to fill another.
	var visit func(expression goast.Expr, nested bool)
	visit = func(expression goast.Expr, nested bool) {
		switch current := expression.(type) {
		case nil:
			return
		case *goast.CompositeLit:
			if seenLiteral[current] {
				return
			}
			seenLiteral[current] = true
			if id, known := scope.sites[current]; known {
				site := b.sites[id]
				if site.semantic {
					products = append(products, b.product(scope, site, len(products)+1, !nested))
					ordered = append(ordered, id)
				}
				nested = true
			}
			for _, element := range current.Elts {
				if pair, ok := element.(*goast.KeyValueExpr); ok {
					visit(pair.Value, nested)
					continue
				}
				visit(element, nested)
			}
		case *goast.Ident:
			if seenName[current.Name] {
				return
			}
			seenName[current.Name] = true
			for _, bound := range scope.locals[current.Name] {
				visit(bound.expr, nested)
			}
			for _, written := range scope.elements[current.Name] {
				visit(written, nested)
			}
		case *goast.CallExpr:
			for _, argument := range current.Args {
				visit(argument, nested)
			}
		case *goast.SelectorExpr:
			visit(current.X, nested)
		case *goast.UnaryExpr:
			visit(current.X, nested)
		case *goast.StarExpr:
			visit(current.X, nested)
		case *goast.ParenExpr:
			visit(current.X, nested)
		case *goast.IndexExpr:
			visit(current.X, nested)
			visit(current.Index, nested)
		case *goast.SliceExpr:
			visit(current.X, nested)
			visit(current.Low, nested)
			visit(current.High, nested)
			visit(current.Max, nested)
		case *goast.TypeAssertExpr:
			visit(current.X, nested)
		case *goast.BinaryExpr:
			visit(current.X, nested)
			visit(current.Y, nested)
		case *goast.KeyValueExpr:
			visit(current.Value, nested)
		}
	}
	for _, root := range scope.roots {
		visit(root, false)
	}
	return products, ordered, nil
}

func (b *productBuilder) product(scope *actionScope, site *constructionSite, ordinal int, root bool) ActionProduct {
	product := ActionProduct{
		Owner:       scope.owner,
		Scope:       scope.kind,
		Ordinal:     ordinal,
		Root:        root,
		Constructor: site.typeName,
		Guarded:     scope.guarded[site.literal],
		Rejected:    scope.rejected[site.literal],
		Elementwise: scope.elementwise[site.literal],
	}
	for _, field := range site.fields {
		_, assigned := site.elements[field.Name]
		product.Fields = append(product.Fields, ProductField{
			Ordinal:  field.Ordinal,
			Field:    field.Name,
			Assigned: assigned,
			States:   statesOf(b.constructionValue(site, field.Name), field.Form),
		})
	}
	return product
}

func (b *productBuilder) scopeMutations() []FieldMutation {
	var result []FieldMutation
	for _, mutation := range b.mutations {
		scope := b.scopes[mutation.scope]
		receiver := b.eval(mutation.scope, mutation.target)
		for _, name := range b.receiverConstructors(receiver) {
			declaration, known := b.declarations[name]
			if !known || !b.semantic[name] {
				continue
			}
			field, exists := declaredField(declaration, mutation.field)
			if !exists {
				continue
			}
			result = append(result, FieldMutation{
				Owner:       scope.owner,
				Scope:       scope.kind,
				Constructor: name,
				Field:       mutation.field,
				States:      statesOf(b.eval(mutation.scope, mutation.value), field.Form),
			})
		}
	}
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].Constructor != result[right].Constructor {
			return result[left].Constructor < result[right].Constructor
		}
		if result[left].Field != result[right].Field {
			return result[left].Field < result[right].Field
		}
		return result[left].Owner < result[right].Owner
	})
	return result
}

func (b *productBuilder) receiverConstructors(receiver value) []string {
	seen := make(map[string]bool)
	for name := range receiver.types {
		seen[name] = true
	}
	for id := range receiver.sites {
		seen[b.sites[id].typeName] = true
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func declaredField(declaration Declaration, name string) (Field, bool) {
	for _, field := range declaration.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return Field{}, false
}

// statesOf projects one abstract value onto the state space of a declared
// field. Every modelled form states its zero state first and its other state
// second, so the projection is the same two-way reading for a pointer, a
// sequence, a boolean and a scalar alike.
func statesOf(v value, form FieldForm) []FieldState {
	states := form.States()
	if len(states) != 2 {
		return nil
	}
	var result []FieldState
	if v.zero {
		result = append(result, states[0])
	}
	if v.nonZero {
		result = append(result, states[1])
	}
	return result
}
