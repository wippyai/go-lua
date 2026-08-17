package parsersource

import (
	goast "go/ast"
	"sort"
)

// scopeUses reads the consumption edges of one scope out of the constructions
// the same walk already ordered. The walk that orders products starts at the
// value the action yields and descends through the coordinates that carry other
// constructions, so the edge a use row states is the very edge that gave a
// nested construction its ordinal: the two grains are one relation read from
// its two ends.
func (b *productBuilder) scopeUses(scope *actionScope, ordered []siteID) []ActionUse {
	ordinals := make(map[siteID]int, len(ordered))
	for index, id := range ordered {
		ordinals[id] = index + 1
	}
	var result []ActionUse
	for index, id := range ordered {
		site := b.sites[id]
		for _, field := range site.fields {
			expression, assigned := site.elements[field.Name]
			if !assigned {
				continue
			}
			if _, known := b.slots[site.typeName+"."+field.Name]; !known {
				continue
			}
			origins, sources, symbols := b.origins(scope, expression, ordinals)
			result = append(result, ActionUse{
				Owner:   scope.owner,
				Scope:   scope.kind,
				Ordinal: index + 1,
				Form:    site.typeName,
				Field:   field.Name,
				Origins: origins,
				Sources: sources,
				Symbols: symbols,
			})
		}
	}
	return result
}

// origins walks one coordinate expression to the points at which its value
// enters the action. A name is followed to what the action bound to it, so an
// edge stated through a local reads the same as the one stated inline; a walk
// already in progress through a name is not re-entered, so a sequence extended
// from itself terminates at the bindings it was extended with.
func (b *productBuilder) origins(scope *actionScope, expression goast.Expr, ordinals map[siteID]int) ([]UseOrigin, []int, []int) {
	walk := &originWalk{builder: b, scope: scope, ordinals: ordinals, kinds: make(map[UseOrigin]bool, 4), sources: make(map[int]bool, 2), symbols: make(map[int]bool, 2), names: make(map[string]bool, 4)}
	walk.visit(expression)
	origins := make([]UseOrigin, 0, len(walk.kinds))
	for kind := range walk.kinds {
		origins = append(origins, kind)
	}
	sort.Slice(origins, func(left, right int) bool { return origins[left] < origins[right] })
	if len(origins) == 0 {
		origins = []UseOrigin{UseOriginOpaque}
	}
	return origins, ordinalsOf(walk.sources), ordinalsOf(walk.symbols)
}

func ordinalsOf(set map[int]bool) []int {
	if len(set) == 0 {
		return nil
	}
	result := make([]int, 0, len(set))
	for ordinal := range set {
		result = append(result, ordinal)
	}
	sort.Ints(result)
	return result
}

type originWalk struct {
	builder  *productBuilder
	scope    *actionScope
	ordinals map[siteID]int
	kinds    map[UseOrigin]bool
	sources  map[int]bool
	symbols  map[int]bool
	names    map[string]bool
}

func (w *originWalk) visit(expression goast.Expr) {
	switch current := expression.(type) {
	case nil:
		return
	case *goast.CompositeLit:
		w.visitComposite(current)
	case *goast.Ident:
		w.visitIdent(current.Name)
	case *goast.CallExpr:
		w.visitCall(current)
	case *goast.ParenExpr:
		w.visit(current.X)
	case *goast.UnaryExpr:
		w.visit(current.X)
	case *goast.StarExpr:
		w.visit(current.X)
	case *goast.TypeAssertExpr:
		w.visit(current.X)
	case *goast.SelectorExpr:
		w.visitSelector(current)
	case *goast.IndexExpr:
		w.visit(current.X)
	case *goast.SliceExpr:
		w.visit(current.X)
	case *goast.BasicLit:
		w.kinds[UseOriginConstant] = true
	default:
		w.kinds[UseOriginOpaque] = true
	}
}

// visitComposite separates the two composites a coordinate can hold: a sequence
// the action assembles, whose members are themselves origins, and a value the
// action constructs, which is one origin and, when the same walk gave it an
// ordinal, one named source.
func (w *originWalk) visitComposite(literal *goast.CompositeLit) {
	switch literal.Type.(type) {
	case *goast.ArrayType, *goast.MapType:
		w.kinds[UseOriginAssembly] = true
		for _, element := range literal.Elts {
			if pair, ok := element.(*goast.KeyValueExpr); ok {
				w.visit(pair.Value)
				continue
			}
			w.visit(element)
		}
		return
	}
	id, known := w.scope.sites[literal]
	if !known {
		w.kinds[UseOriginOpaque] = true
		return
	}
	w.kinds[UseOriginConstruction] = true
	if ordinal, ordered := w.ordinals[id]; ordered {
		w.sources[ordinal] = true
	}
}

func (w *originWalk) visitIdent(name string) {
	switch name {
	case "nil", "true", "false":
		w.kinds[UseOriginConstant] = true
		return
	}
	if w.names[name] {
		return
	}
	if bindings, bound := w.scope.locals[name]; bound {
		w.names[name] = true
		for _, binding := range bindings {
			w.visitBinding(binding)
		}
		for _, written := range w.scope.elements[name] {
			w.kinds[UseOriginAssembly] = true
			w.visit(written)
		}
		return
	}
	if written, assembled := w.scope.elements[name]; assembled {
		w.names[name] = true
		w.kinds[UseOriginAssembly] = true
		for _, element := range written {
			w.visit(element)
		}
		return
	}
	for _, formal := range w.scope.formals {
		if formal == name {
			w.kinds[UseOriginParameter] = true
			return
		}
	}
	if slot := operandSlot(name); w.scope.kind == ProductScopeProduction && slot != 0 {
		w.kinds[UseOriginSymbol] = true
		w.symbols[slot] = true
		return
	}
	if _, constant := w.builder.constants[name]; constant {
		w.kinds[UseOriginConstant] = true
		return
	}
	w.kinds[UseOriginOpaque] = true
}

func (w *originWalk) visitBinding(bound binding) {
	switch bound.kind {
	case bindingCallResult:
		w.kinds[UseOriginHelper] = true
	case bindingElement:
		w.visit(bound.expr)
	default:
		w.visit(bound.expr)
	}
}

func (w *originWalk) visitCall(call *goast.CallExpr) {
	if _, conversion := call.Fun.(*goast.ArrayType); conversion {
		for _, argument := range call.Args {
			w.visit(argument)
		}
		return
	}
	callee, ok := call.Fun.(*goast.Ident)
	if !ok {
		w.kinds[UseOriginOpaque] = true
		return
	}
	switch callee.Name {
	case "append", "make":
		w.kinds[UseOriginAssembly] = true
		for _, argument := range call.Args {
			w.visit(argument)
		}
		return
	}
	if _, helper := w.builder.helperScopes[callee.Name]; helper {
		w.kinds[UseOriginHelper] = true
		return
	}
	w.kinds[UseOriginOpaque] = true
}

// visitSelector reads a projection through the value it projects from. A
// package qualifier names a declared constant instead, which is the same
// distinction the evaluation makes, so a discriminant is never mistaken for a
// value the action received.
func (w *originWalk) visitSelector(selector *goast.SelectorExpr) {
	if qualifier, ok := selector.X.(*goast.Ident); ok && w.builder.isPackage(w.scope, qualifier.Name) {
		w.kinds[UseOriginConstant] = true
		return
	}
	w.visit(selector.X)
}
