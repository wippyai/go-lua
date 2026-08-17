package parsersource

import (
	goast "go/ast"
	"go/token"
	"strconv"
	"strings"
)

// value is the abstraction one parser expression contributes to a field. It is
// two facts and their provenance: whether the expression can leave the field in
// its declared zero state, whether it can leave the field in the other state,
// and which constructions or terminals the value came from so that a projection
// through it stays exact. A value with an origin it cannot name is opaque, and
// an opaque value admits both states rather than the convenient one.
type value struct {
	zero      bool
	nonZero   bool
	opaque    bool
	sites     map[siteID]bool
	types     map[string]bool
	terminals map[string]bool
	elem      *value
}

func zeroValue() value    { return value{zero: true} }
func nonZeroValue() value { return value{nonZero: true} }
func eitherValue() value  { return value{zero: true, nonZero: true} }
func topValue() value     { return value{zero: true, nonZero: true, opaque: true} }

func (v value) join(other value) value {
	result := value{
		zero:    v.zero || other.zero,
		nonZero: v.nonZero || other.nonZero,
		opaque:  v.opaque || other.opaque,
	}
	result.sites = joinSet(v.sites, other.sites)
	result.types = joinNames(v.types, other.types)
	result.terminals = joinNames(v.terminals, other.terminals)
	switch {
	case v.elem == nil:
		result.elem = other.elem
	case other.elem == nil:
		result.elem = v.elem
	default:
		joined := v.elem.join(*other.elem)
		joined.elem = nil
		result.elem = &joined
	}
	return result
}

func (v value) withElement(element value) value {
	element.elem = nil
	if v.elem != nil {
		element = element.join(*v.elem)
		element.elem = nil
	}
	v.elem = &element
	return v
}

func (v value) equal(other value) bool {
	if v.zero != other.zero || v.nonZero != other.nonZero || v.opaque != other.opaque {
		return false
	}
	if !sameSet(v.sites, other.sites) || !sameNames(v.types, other.types) || !sameNames(v.terminals, other.terminals) {
		return false
	}
	switch {
	case v.elem == nil && other.elem == nil:
		return true
	case v.elem == nil || other.elem == nil:
		return false
	default:
		return v.elem.equal(*other.elem)
	}
}

func joinSet(left, right map[siteID]bool) map[siteID]bool {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	result := make(map[siteID]bool, len(left)+len(right))
	for key := range left {
		result[key] = true
	}
	for key := range right {
		result[key] = true
	}
	return result
}

func joinNames(left, right map[string]bool) map[string]bool {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	result := make(map[string]bool, len(left)+len(right))
	for key := range left {
		result[key] = true
	}
	for key := range right {
		result[key] = true
	}
	return result
}

func sameSet(left, right map[siteID]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if !right[key] {
			return false
		}
	}
	return true
}

func sameNames(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if !right[key] {
			return false
		}
	}
	return true
}

// solve runs the construction relation to its least fixed point. Every query is
// a monotone function of the others over a two-state lattice with finite
// provenance, so repeating a full recomputation until nothing grows terminates
// and reaches the least solution.
func (b *productBuilder) solve() {
	keys := b.queries()
	for {
		next := make(map[queryKey]value, len(keys))
		changed := false
		for _, key := range keys {
			computed := b.compute(key)
			next[key] = computed
			if !computed.equal(b.env[key]) {
				changed = true
			}
		}
		b.env = next
		if !changed {
			return
		}
	}
}

func (b *productBuilder) queries() []queryKey {
	var keys []queryKey
	for _, scope := range b.scopes {
		for name := range scope.locals {
			keys = append(keys, queryKey{kind: queryLocal, scope: scope.index, name: name})
		}
		for name := range scope.elements {
			if _, bound := scope.locals[name]; !bound {
				keys = append(keys, queryKey{kind: queryLocal, scope: scope.index, name: name})
			}
		}
		if scope.kind == ProductScopeHelper {
			for index := 0; index < len(scope.formals); index++ {
				keys = append(keys, queryKey{kind: queryFormal, name: scope.owner, index: index})
			}
			for index := 0; index < scope.results; index++ {
				keys = append(keys, queryKey{kind: queryHelperResult, name: scope.owner, index: index})
			}
		}
	}
	for nonterminal := range b.nonterminals {
		keys = append(keys, queryKey{kind: querySymbol, name: nonterminal})
	}
	for name, declaration := range b.declarations {
		for _, field := range declaration.Fields {
			keys = append(keys, queryKey{kind: queryMutation, name: name + "." + field.Name})
			keys = append(keys, queryKey{kind: queryField, name: name + "." + field.Name})
		}
	}
	for name, fields := range b.records {
		for _, field := range fields {
			keys = append(keys, queryKey{kind: queryMutation, name: name + "." + field.Name})
			keys = append(keys, queryKey{kind: queryField, name: name + "." + field.Name})
		}
	}
	return keys
}

func (b *productBuilder) compute(key queryKey) value {
	switch key.kind {
	case queryLocal:
		return b.computeLocal(key.scope, key.name)
	case querySymbol:
		return b.computeSymbol(key.name)
	case queryHelperResult:
		return b.computeHelperResult(key.name, key.index)
	case queryFormal:
		return b.computeFormal(key.name, key.index)
	case queryField:
		return b.computeField(key.name)
	case queryMutation:
		return b.computeMutation(key.name)
	default:
		return topValue()
	}
}

func (b *productBuilder) computeLocal(index int, name string) value {
	scope := b.scopes[index]
	result := value{}
	for _, bound := range scope.locals[name] {
		switch bound.kind {
		case bindingExpr:
			if bound.expr == nil {
				result = result.join(zeroValue())
				continue
			}
			result = result.join(b.eval(index, bound.expr))
		case bindingCallResult:
			result = result.join(b.env[queryKey{kind: queryHelperResult, name: bound.helper, index: bound.index}])
		case bindingElement:
			result = result.join(b.elementOf(b.eval(index, bound.expr)))
		case bindingAssert:
			if bound.typeName == "" {
				result = result.join(topValue())
				continue
			}
			result = result.join(value{nonZero: true, types: map[string]bool{bound.typeName: true}})
		}
	}
	for _, written := range scope.elements[name] {
		result = result.withElement(b.eval(index, written))
	}
	return result
}

func (b *productBuilder) computeSymbol(nonterminal string) value {
	result := value{}
	for _, index := range b.nonterminals[nonterminal] {
		result = result.join(b.env[queryKey{kind: queryLocal, scope: index, name: resultOperand}])
	}
	return result
}

func (b *productBuilder) computeHelperResult(helper string, index int) value {
	scope, known := b.helperScopes[helper]
	if !known {
		return topValue()
	}
	result := value{}
	for _, returned := range b.scopes[scope].returns {
		if index >= len(returned) {
			continue
		}
		result = result.join(b.eval(scope, returned[index]))
	}
	return result
}

func (b *productBuilder) computeFormal(helper string, index int) value {
	result := value{}
	for _, call := range b.calls {
		if call.helper != helper || index >= len(call.actuals) {
			continue
		}
		result = result.join(b.eval(call.scope, call.actuals[index]))
	}
	return result
}

func (b *productBuilder) computeField(key string) value {
	typeName, field := splitFieldKey(key)
	result := b.env[queryKey{kind: queryMutation, name: key}]
	for _, site := range b.sites {
		if site.typeName != typeName {
			continue
		}
		result = result.join(b.constructionValue(site, field))
	}
	return result
}

func (b *productBuilder) computeMutation(key string) value {
	typeName, field := splitFieldKey(key)
	result := value{}
	for _, mutation := range b.mutations {
		if mutation.field != field {
			continue
		}
		target := b.eval(mutation.scope, mutation.target)
		if !b.targets(target, typeName) {
			continue
		}
		result = result.join(b.eval(mutation.scope, mutation.value))
	}
	return result
}

// targets reports whether a mutation receiver can be a value of the given type.
// A receiver named only by an opaque origin reaches no declared type, so an
// assignment through it is not attributed to one.
func (b *productBuilder) targets(receiver value, typeName string) bool {
	if receiver.types[typeName] {
		return true
	}
	for id := range receiver.sites {
		if b.sites[id].typeName == typeName {
			return true
		}
	}
	return false
}

func splitFieldKey(key string) (string, string) {
	cut := strings.LastIndexByte(key, '.')
	if cut < 0 {
		return key, ""
	}
	return key[:cut], key[cut+1:]
}

// constructionValue is the state one construction leaves one field in. A field
// the construction does not name keeps its declared zero state, which is
// evidence in its own right rather than missing evidence.
func (b *productBuilder) constructionValue(site *constructionSite, field string) value {
	expression, named := site.elements[field]
	if !named {
		return zeroValue()
	}
	return b.eval(site.scope, expression)
}

func (b *productBuilder) elementOf(container value) value {
	if container.elem != nil {
		return *container.elem
	}
	if container.opaque || container.nonZero {
		return topValue()
	}
	return value{}
}

// project reads one field through a value's origins. Reading through a zero
// origin yields a zero field, reading through a construction yields exactly
// what that construction assigned, and reading through an origin the analysis
// cannot name yields both states.
func (b *productBuilder) project(receiver value, field string) value {
	result := value{}
	named := false
	if receiver.zero {
		result = result.join(zeroValue())
	}
	if receiver.opaque {
		result = result.join(topValue())
		named = true
	}
	for id := range receiver.sites {
		site := b.sites[id]
		result = result.join(b.constructionValue(site, field))
		result = result.join(b.env[queryKey{kind: queryMutation, name: site.typeName + "." + field}])
		named = true
	}
	for typeName := range receiver.types {
		result = result.join(b.env[queryKey{kind: queryField, name: typeName + "." + field}])
		named = true
	}
	for terminal := range receiver.terminals {
		result = result.join(b.tokenField(terminal, field))
		named = true
	}
	if receiver.nonZero && !named {
		result = result.join(topValue())
	}
	return result
}

// tokenField reads a field of a scanned token. Both coordinates a semantic
// action takes from a token are lexer facts: the lexeme's emptiness is decided
// by whether the scanner that produced this terminal anchors on the character
// that triggered it, and the position's zero-ness by the scanner's own position
// contract.
func (b *productBuilder) tokenField(terminal, field string) value {
	switch field {
	case "Str":
		if b.terminalText[terminal] {
			return nonZeroValue()
		}
		return eitherValue()
	case "Pos":
		if b.nonZeroPos {
			return nonZeroValue()
		}
		return topValue()
	default:
		return topValue()
	}
}

func (b *productBuilder) eval(index int, expression goast.Expr) value {
	if expression == nil {
		return value{}
	}
	scope := b.scopes[index]
	switch current := expression.(type) {
	case *goast.BasicLit:
		return literalValue(current)
	case *goast.Ident:
		return b.identValue(scope, current.Name)
	case *goast.ParenExpr:
		return b.eval(index, current.X)
	case *goast.SelectorExpr:
		if qualifier, ok := current.X.(*goast.Ident); ok && b.isPackage(scope, qualifier.Name) {
			if qualifier.Name != "ast" {
				return topValue()
			}
			zero, known := b.constants[current.Sel.Name]
			if !known {
				return topValue()
			}
			if zero {
				return zeroValue()
			}
			return nonZeroValue()
		}
		return b.project(b.eval(index, current.X), current.Sel.Name)
	case *goast.UnaryExpr:
		if current.Op == token.AND {
			base := b.eval(index, current.X)
			return value{nonZero: true, sites: base.sites, types: base.types}
		}
		return topValue()
	case *goast.StarExpr:
		return topValue()
	case *goast.CompositeLit:
		return b.compositeValue(scope, current)
	case *goast.TypeAssertExpr:
		name := assertedName(current.Type)
		if name == "" {
			return b.eval(index, current.X)
		}
		return value{nonZero: true, types: map[string]bool{name: true}}
	case *goast.IndexExpr:
		return b.elementOf(b.eval(index, current.X))
	case *goast.SliceExpr:
		base := b.eval(index, current.X)
		result := eitherValue()
		if base.elem != nil {
			result = result.withElement(*base.elem)
		}
		return result
	case *goast.BinaryExpr:
		if current.Op == token.ADD {
			left := b.eval(index, current.X)
			right := b.eval(index, current.Y)
			if left.nonZero && !left.zero || right.nonZero && !right.zero {
				return nonZeroValue()
			}
		}
		return topValue()
	case *goast.CallExpr:
		return b.callValue(index, current)
	default:
		return topValue()
	}
}

// isPackage separates a package qualifier from a value the scope binds. The
// package names are the ones parser.go.y imports, read from its own import
// block, so a selector is resolved against what the parser actually imports
// rather than against a set of names stated here. A scope that binds the name
// shadows the package, so the binding wins.
func (b *productBuilder) isPackage(scope *actionScope, name string) bool {
	if _, bound := scope.locals[name]; bound {
		return false
	}
	for _, formal := range scope.formals {
		if formal == name {
			return false
		}
	}
	return b.packages[name]
}

func literalValue(literal *goast.BasicLit) value {
	switch literal.Kind {
	case token.STRING:
		text, err := strconv.Unquote(literal.Value)
		if err != nil {
			return topValue()
		}
		if text == "" {
			return zeroValue()
		}
		return nonZeroValue()
	case token.INT, token.FLOAT:
		number, err := strconv.ParseFloat(literal.Value, 64)
		if err != nil {
			return topValue()
		}
		if number == 0 {
			return zeroValue()
		}
		return nonZeroValue()
	case token.CHAR:
		return nonZeroValue()
	default:
		return topValue()
	}
}

func (b *productBuilder) identValue(scope *actionScope, name string) value {
	switch name {
	case "nil", "false":
		return zeroValue()
	case "true":
		return nonZeroValue()
	}
	if _, bound := scope.locals[name]; bound {
		return b.env[queryKey{kind: queryLocal, scope: scope.index, name: name}]
	}
	for index, formal := range scope.formals {
		if formal == name {
			return b.env[queryKey{kind: queryFormal, name: scope.owner, index: index}]
		}
	}
	if scope.kind == ProductScopeProduction {
		if slot := operandSlot(name); slot != 0 {
			return b.operandValue(scope, slot)
		}
	}
	if zero, known := b.constants[name]; known {
		if zero {
			return zeroValue()
		}
		return nonZeroValue()
	}
	return topValue()
}

// operandValue reads one positional operand of a reduction. A terminal operand
// carries whatever the scanner stamped on it and is named by its terminal so a
// later projection can consult the lexer contract; a nonterminal operand
// carries whatever its own alternatives reduced to.
func (b *productBuilder) operandValue(scope *actionScope, slot int) value {
	if slot > len(scope.symbols) {
		return topValue()
	}
	name := scope.symbols[slot-1]
	symbol, known := b.vocabulary.Symbol(name)
	// A character terminal needs no declaration in yacc, so an undeclared
	// quoted symbol is a terminal rather than an unknown one.
	if !known {
		if strings.HasPrefix(name, "'") {
			return value{nonZero: true, terminals: map[string]bool{name: true}}
		}
		return topValue()
	}
	if symbol.Kind == SymbolTerminal {
		return value{nonZero: true, terminals: map[string]bool{name: true}}
	}
	return b.env[queryKey{kind: querySymbol, name: name}]
}

func (b *productBuilder) compositeValue(scope *actionScope, literal *goast.CompositeLit) value {
	switch literal.Type.(type) {
	case *goast.ArrayType, *goast.MapType:
		result := value{zero: len(literal.Elts) == 0, nonZero: len(literal.Elts) != 0}
		for _, element := range literal.Elts {
			if pair, ok := element.(*goast.KeyValueExpr); ok {
				result = result.withElement(b.eval(scope.index, pair.Value))
				continue
			}
			result = result.withElement(b.eval(scope.index, element))
		}
		return result
	}
	id, known := scope.sites[literal]
	if !known {
		return topValue()
	}
	site := b.sites[id]
	result := value{sites: map[siteID]bool{id: true}, zero: true}
	for _, field := range site.fields {
		coordinate := b.constructionValue(site, field.Name)
		if coordinate.nonZero {
			result.nonZero = true
		}
		if !coordinate.zero {
			result.zero = false
		}
	}
	if len(site.fields) == 0 {
		result.zero = true
	}
	return result
}

func (b *productBuilder) callValue(index int, call *goast.CallExpr) value {
	// A conversion to a slice type carries its operand through unchanged, which
	// is how the parser spells an explicitly typed empty sequence.
	if _, ok := call.Fun.(*goast.ArrayType); ok {
		if len(call.Args) == 1 {
			return b.eval(index, call.Args[0])
		}
		return topValue()
	}
	callee, ok := call.Fun.(*goast.Ident)
	if !ok {
		return topValue()
	}
	switch callee.Name {
	case "append":
		return b.appendValue(index, call)
	case "make":
		return b.makeValue(index, call)
	}
	if _, known := b.helperScopes[callee.Name]; known {
		return b.env[queryKey{kind: queryHelperResult, name: callee.Name, index: 0}]
	}
	return topValue()
}

func (b *productBuilder) appendValue(index int, call *goast.CallExpr) value {
	if len(call.Args) == 0 {
		return topValue()
	}
	result := b.eval(index, call.Args[0])
	spread := call.Ellipsis.IsValid()
	for position, argument := range call.Args[1:] {
		last := position == len(call.Args)-2
		if spread && last {
			tail := b.eval(index, argument)
			result = value{
				zero:      result.zero && tail.zero,
				nonZero:   result.nonZero || tail.nonZero,
				opaque:    result.opaque || tail.opaque,
				sites:     result.sites,
				types:     result.types,
				terminals: result.terminals,
				elem:      result.elem,
			}
			if tail.elem != nil {
				result = result.withElement(*tail.elem)
			}
			continue
		}
		result.zero = false
		result.nonZero = true
		result = result.withElement(b.eval(index, argument))
	}
	return result
}

// makeValue reads a slice the parser allocates by length. Such a slice is empty
// exactly when the length it was allocated from is, and its elements start at
// the element type's zero value until an index assignment states otherwise.
func (b *productBuilder) makeValue(index int, call *goast.CallExpr) value {
	if len(call.Args) < 2 {
		return topValue()
	}
	length := call.Args[1]
	result := topValue()
	if inner, ok := length.(*goast.CallExpr); ok {
		if callee, ok := inner.Fun.(*goast.Ident); ok && callee.Name == "len" && len(inner.Args) == 1 {
			source := b.eval(index, inner.Args[0])
			result = value{zero: source.zero, nonZero: source.nonZero, opaque: source.opaque}
		}
	}
	if literal, ok := length.(*goast.BasicLit); ok {
		result = literalValue(literal)
	}
	return result.withElement(zeroValue())
}
