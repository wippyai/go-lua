package parserproducts

import (
	"fmt"
	goast "go/ast"
	"go/token"
	"sort"
	"strconv"
)

// actionTermBuilder exists only during cold source derivation. The published
// result is the numeric ActionTerms arena; syntax names and Go ASTs do not
// cross the Evidence boundary.
type actionTermBuilder struct {
	terms       ActionTerms
	callables   map[string]bool
	guardRanges []actionGuardRange
}

type actionGuardRange struct {
	start uint32
	count uint16
}

type actionTermScope struct {
	id        ActionScopeID
	index     int
	formals   map[string]uint16
	locals    map[string]uint16
	inputs    map[string]uint16
	results   map[string]uint16
	nextLocal uint16
}

func newActionTermBuilder(callables map[string]bool) *actionTermBuilder {
	known := make(map[string]bool, len(callables)+5)
	for name := range callables {
		known[name] = true
	}
	for _, name := range []string{"append", "len", "make", "string", "byte"} {
		known[name] = true
	}
	return &actionTermBuilder{callables: known}
}

func (b *actionTermBuilder) symbol(kind ActionSymbolKind, text string) ActionSymbolID {
	for index, existing := range b.terms.Symbols {
		if existing.Kind == kind && existing.Text == text {
			return ActionSymbolID(index + 1)
		}
	}
	b.terms.Symbols = append(b.terms.Symbols, ActionSymbol{Kind: kind, Text: text})
	return ActionSymbolID(len(b.terms.Symbols))
}

func (b *actionTermBuilder) scope(kind ActionScopeKind, owner string, inputs, results uint16, formals []string) actionTermScope {
	ownerKind := ActionSymbolOwner
	if kind == ActionScopeHelper || kind == ActionScopeMapItem {
		ownerKind = ActionSymbolCallable
	}
	b.terms.Scopes = append(b.terms.Scopes, ActionScope{Kind: kind, Owner: b.symbol(ownerKind, owner), Inputs: inputs, Formals: uint16(len(formals)), Results: results})
	scope := actionTermScope{
		id: ActionScopeID(len(b.terms.Scopes)), index: len(b.terms.Scopes) - 1,
		formals: make(map[string]uint16, len(formals)), locals: make(map[string]uint16),
		inputs: make(map[string]uint16), results: make(map[string]uint16),
	}
	for index, name := range formals {
		scope.formals[name] = uint16(index)
	}
	if kind == ActionScopeProduction {
		for index := uint16(0); index < inputs; index++ {
			scope.inputs["Arg"+strconv.Itoa(int(index+1))] = index
		}
		scope.results["Result"] = 0
	}
	if kind == ActionScopeMapItem {
		scope.inputs["item"] = 0
	}
	return scope
}

func (b *actionTermBuilder) mapItemScope(owner string, item string) actionTermScope {
	scope := b.scope(ActionScopeMapItem, owner, 1, 0, nil)
	delete(scope.inputs, "item")
	scope.inputs[item] = 0
	return scope
}

func (b *actionTermBuilder) closeScope(scope actionTermScope) {
	b.terms.Scopes[scope.index].Locals = scope.nextLocal
}

func (scope *actionTermScope) bindLocal(name string) error {
	if name == "" || name == "_" {
		return nil
	}
	if _, exists := scope.formals[name]; exists {
		return fmt.Errorf("parser products: local %s shadows a formal", name)
	}
	if _, exists := scope.inputs[name]; exists {
		return fmt.Errorf("parser products: local %s shadows an input", name)
	}
	if _, exists := scope.results[name]; exists {
		return fmt.Errorf("parser products: local %s shadows a result", name)
	}
	if _, exists := scope.locals[name]; exists {
		return nil
	}
	scope.locals[name] = scope.nextLocal
	scope.nextLocal++
	return nil
}

func (scope *actionTermScope) local(name string) (uint16, bool) {
	slot, ok := scope.locals[name]
	return slot, ok
}

func sameActionTerm(left, right ActionTerm) bool {
	return left.Scope == right.Scope && left.Kind == right.Kind && left.Slot == right.Slot && left.Symbol == right.Symbol && left.EdgeCount == right.EdgeCount
}

func (b *actionTermBuilder) intern(term ActionTerm, edges ...ActionEdge) ActionTermID {
	term.EdgeStart = uint32(len(b.terms.Edges))
	term.EdgeCount = uint16(len(edges))
	for index, existing := range b.terms.Terms {
		if !sameActionTerm(existing, term) {
			continue
		}
		matched := true
		for offset, edge := range edges {
			if b.terms.Edges[existing.EdgeStart+uint32(offset)] != edge {
				matched = false
				break
			}
		}
		if matched {
			return ActionTermID(index + 1)
		}
	}
	b.terms.Edges = append(b.terms.Edges, edges...)
	b.terms.Terms = append(b.terms.Terms, term)
	return ActionTermID(len(b.terms.Terms))
}

func (b *actionTermBuilder) expression(scope *actionTermScope, expression goast.Expr) (ActionTermID, error) {
	switch node := expression.(type) {
	case *goast.ParenExpr:
		return b.expression(scope, node.X)
	case *goast.Ident:
		if node.Name == "nil" {
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermNil}), nil
		}
		if node.Name == "true" || node.Name == "false" {
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermBool, Symbol: b.symbol(ActionSymbolConstant, node.Name)}), nil
		}
		if slot, ok := scope.inputs[node.Name]; ok {
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermInput, Slot: slot}), nil
		}
		if slot, ok := scope.results[node.Name]; ok {
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermResult, Slot: slot}), nil
		}
		if slot, ok := scope.formals[node.Name]; ok {
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermFormal, Slot: slot}), nil
		}
		if slot, ok := scope.local(node.Name); ok {
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermLocal, Slot: slot}), nil
		}
		if node.Name == "yylex" && b.terms.Scopes[scope.index].Kind == ActionScopeProduction {
			return b.control(scope, "Lexer"), nil
		}
		return 0, fmt.Errorf("unbound action identifier %q", node.Name)
	case *goast.BasicLit:
		switch node.Kind {
		case token.STRING:
			value, err := strconv.Unquote(node.Value)
			if err != nil {
				return 0, err
			}
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermString, Symbol: b.symbol(ActionSymbolConstant, value)}), nil
		case token.INT:
			value, err := strconv.ParseInt(node.Value, 0, 64)
			if err != nil {
				return 0, err
			}
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermInt, Symbol: b.symbol(ActionSymbolConstant, strconv.FormatInt(value, 10))}), nil
		case token.CHAR:
			value, err := strconv.Unquote(node.Value)
			if err != nil {
				return 0, err
			}
			runes := []rune(value)
			if len(runes) != 1 {
				return 0, fmt.Errorf("action character literal is not one rune")
			}
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermInt, Symbol: b.symbol(ActionSymbolConstant, strconv.FormatInt(int64(runes[0]), 10))}), nil
		default:
			return 0, fmt.Errorf("unsupported action literal %s", node.Kind)
		}
	case *goast.SelectorExpr:
		if base, ok := node.X.(*goast.Ident); ok && base.Name == "ast" {
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermEnum, Symbol: b.symbol(ActionSymbolEnum, "ast."+node.Sel.Name)}), nil
		}
		base, err := b.expression(scope, node.X)
		if err != nil {
			return 0, err
		}
		return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermProject, Symbol: b.symbol(ActionSymbolField, node.Sel.Name)}, ActionEdge{Term: base}), nil
	case *goast.IndexExpr:
		base, err := b.expression(scope, node.X)
		if err != nil {
			return 0, err
		}
		index, err := b.expression(scope, node.Index)
		if err != nil {
			return 0, err
		}
		return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermIndex}, ActionEdge{Term: base}, ActionEdge{Term: index}), nil
	case *goast.UnaryExpr:
		if node.Op != token.AND {
			return 0, fmt.Errorf("unsupported action unary %s", node.Op)
		}
		child, err := b.expression(scope, node.X)
		if err != nil {
			return 0, err
		}
		return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermAddress}, ActionEdge{Term: child}), nil
	case *goast.CompositeLit:
		return b.composite(scope, node)
	case *goast.CallExpr:
		return b.call(scope, node)
	default:
		return 0, fmt.Errorf("unsupported action expression %T", expression)
	}
}

// control is a closed parser-state observation used only by diagnostics.
// It prevents action rejects from smuggling raw lexer identifiers into the
// evidence while keeping the state scoped to the owning action.
func (b *actionTermBuilder) control(scope *actionTermScope, name string) ActionTermID {
	return b.intern(ActionTerm{
		Scope:  scope.id,
		Kind:   ActionTermControl,
		Symbol: b.symbol(ActionSymbolControl, name),
	})
}

func (b *actionTermBuilder) numberParseGuard(scope *actionTermScope, expression goast.Expr, class NumberParseClass) (Guard, error) {
	if class != NumberParseClassInteger && class != NumberParseClassFloat && class != NumberParseClassInvalid {
		return Guard{}, fmt.Errorf("invalid number parse class")
	}
	term, err := b.expression(scope, expression)
	if err != nil {
		return Guard{}, err
	}
	return Guard{Atoms: []GuardAtom{{
		Kind:       GuardAtomNumberParseClass,
		Term:       term,
		ParseClass: class,
	}}}, nil
}

func (b *actionTermBuilder) composite(scope *actionTermScope, literal *goast.CompositeLit) (ActionTermID, error) {
	typeText := ""
	if literal.Type != nil {
		var err error
		typeText, err = actionTypeSymbol(literal.Type)
		if err != nil {
			return 0, err
		}
	}
	record := false
	if len(literal.Elts) != 0 {
		_, record = literal.Elts[0].(*goast.KeyValueExpr)
	}
	edges := make([]ActionEdge, 0, len(literal.Elts))
	for _, element := range literal.Elts {
		if pair, keyed := element.(*goast.KeyValueExpr); keyed {
			if !record {
				return 0, fmt.Errorf("mixed keyed and positional action literal")
			}
			field, ok := pair.Key.(*goast.Ident)
			if !ok {
				return 0, fmt.Errorf("unsupported action record key")
			}
			value, err := b.expression(scope, pair.Value)
			if err != nil {
				return 0, err
			}
			edges = append(edges, ActionEdge{Term: value, Label: b.symbol(ActionSymbolField, field.Name)})
			continue
		}
		if record {
			return 0, fmt.Errorf("mixed keyed and positional action literal")
		}
		value, err := b.expression(scope, element)
		if err != nil {
			return 0, err
		}
		edges = append(edges, ActionEdge{Term: value})
	}
	if record || typeText == "" {
		if typeText == "struct{}" {
			return 0, fmt.Errorf("untyped record cannot use a fake struct type")
		}
		sort.Slice(edges, func(left, right int) bool {
			return b.symbolValue(edges[left].Label).Text < b.symbolValue(edges[right].Label).Text
		})
		for index := 1; index < len(edges); index++ {
			if edges[index-1].Label == edges[index].Label {
				return 0, fmt.Errorf("duplicate action record field")
			}
		}
		symbol := ActionSymbolID(0)
		if typeText != "" {
			symbol = b.symbol(ActionSymbolType, typeText)
		}
		return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermRecord, Symbol: symbol}, edges...), nil
	}
	if len(literal.Elts) == 0 {
		if _, sequence := literal.Type.(*goast.ArrayType); !sequence {
			return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermRecord, Symbol: b.symbol(ActionSymbolType, typeText)}), nil
		}
	}
	if _, ok := literal.Type.(*goast.ArrayType); !ok {
		return 0, fmt.Errorf("positional action literal is not a sequence")
	}
	return b.intern(ActionTerm{Scope: scope.id, Kind: ActionTermSequence, Symbol: b.symbol(ActionSymbolType, typeText)}, edges...), nil
}

func (b *actionTermBuilder) call(scope *actionTermScope, call *goast.CallExpr) (ActionTermID, error) {
	name, ok := call.Fun.(*goast.Ident)
	if !ok || !b.callables[name.Name] {
		return 0, fmt.Errorf("unsupported action callable")
	}
	if call.Ellipsis.IsValid() && len(call.Args) == 0 {
		return 0, fmt.Errorf("empty spread call")
	}
	edges := make([]ActionEdge, 0, len(call.Args))
	for index, argument := range call.Args {
		term, err := b.expression(scope, argument)
		if err != nil {
			return 0, err
		}
		edge := ActionEdge{Term: term}
		if call.Ellipsis.IsValid() && index == len(call.Args)-1 {
			edge.Flags = ActionEdgeSpread
		}
		edges = append(edges, edge)
	}
	kind := ActionTermCall
	if name.Name == "append" {
		if len(edges) < 2 {
			return 0, fmt.Errorf("append has fewer than two operands")
		}
		kind = ActionTermAppend
	}
	return b.intern(ActionTerm{Scope: scope.id, Kind: kind, Symbol: b.symbol(ActionSymbolCallable, name.Name)}, edges...), nil
}

func (b *actionTermBuilder) symbolValue(id ActionSymbolID) ActionSymbol { return b.terms.Symbols[id-1] }

func (b *actionTermBuilder) place(scope *actionTermScope, expression goast.Expr) (Place, error) {
	start := uint32(len(b.terms.PlaceSteps))
	root, slot, err := b.placePath(scope, expression)
	if err != nil {
		return Place{}, err
	}
	return Place{Scope: scope.id, Root: root, Slot: slot, StepStart: start, StepCount: uint16(uint32(len(b.terms.PlaceSteps)) - start)}, nil
}

func (b *actionTermBuilder) placePath(scope *actionTermScope, expression goast.Expr) (PlaceRoot, uint16, error) {
	switch node := expression.(type) {
	case *goast.ParenExpr:
		return b.placePath(scope, node.X)
	case *goast.Ident:
		if slot, ok := scope.results[node.Name]; ok {
			return PlaceRootResult, slot, nil
		}
		if slot, ok := scope.formals[node.Name]; ok {
			return PlaceRootFormal, slot, nil
		}
		if slot, ok := scope.local(node.Name); ok {
			return PlaceRootLocal, slot, nil
		}
		return PlaceRootInvalid, 0, fmt.Errorf("unbound place identifier %q", node.Name)
	case *goast.SelectorExpr:
		root, slot, err := b.placePath(scope, node.X)
		if err != nil {
			return PlaceRootInvalid, 0, err
		}
		b.terms.PlaceSteps = append(b.terms.PlaceSteps, PlaceStep{Kind: PlaceStepField, Field: b.symbol(ActionSymbolField, node.Sel.Name)})
		return root, slot, nil
	case *goast.IndexExpr:
		root, slot, err := b.placePath(scope, node.X)
		if err != nil {
			return PlaceRootInvalid, 0, err
		}
		index, err := b.expression(scope, node.Index)
		if err != nil {
			return PlaceRootInvalid, 0, err
		}
		b.terms.PlaceSteps = append(b.terms.PlaceSteps, PlaceStep{Kind: PlaceStepIndex, Index: index})
		return root, slot, nil
	default:
		return PlaceRootInvalid, 0, fmt.Errorf("unsupported place %T", expression)
	}
}

func (b *actionTermBuilder) guard(scope *actionTermScope, expression goast.Expr) (Guard, error) {
	var atoms []GuardAtom
	if err := b.appendGuardAtoms(scope, expression, false, &atoms); err != nil {
		return Guard{}, err
	}
	sort.Slice(atoms, func(left, right int) bool { return guardAtomLess(atoms[left], atoms[right]) })
	for index := 1; index < len(atoms); index++ {
		if !guardAtomLess(atoms[index-1], atoms[index]) {
			return Guard{}, fmt.Errorf("duplicate guard atom")
		}
	}
	return Guard{Atoms: atoms}, nil
}

func (b *actionTermBuilder) appendGuardAtoms(scope *actionTermScope, expression goast.Expr, negated bool, atoms *[]GuardAtom) error {
	if paren, ok := expression.(*goast.ParenExpr); ok {
		return b.appendGuardAtoms(scope, paren.X, negated, atoms)
	}
	if unary, ok := expression.(*goast.UnaryExpr); ok && unary.Op == token.NOT {
		return b.appendGuardAtoms(scope, unary.X, !negated, atoms)
	}
	binary, ok := expression.(*goast.BinaryExpr)
	if !ok {
		term, err := b.expression(scope, expression)
		if err != nil {
			return fmt.Errorf("unsupported control guard %T: %w", expression, err)
		}
		*atoms = append(*atoms, GuardAtom{
			Kind:     GuardAtomEqConst,
			Negated:  negated,
			Term:     term,
			Constant: b.symbol(ActionSymbolConstant, "true"),
		})
		return nil
	}
	if binary.Op == token.LAND {
		if negated {
			return fmt.Errorf("negated conjunction is not a finite guard")
		}
		if err := b.appendGuardAtoms(scope, binary.X, false, atoms); err != nil {
			return err
		}
		return b.appendGuardAtoms(scope, binary.Y, false, atoms)
	}
	if binary.Op != token.EQL && binary.Op != token.NEQ {
		return fmt.Errorf("unsupported control comparison %s", binary.Op)
	}
	negated = negated != (binary.Op == token.NEQ)
	left, right := binary.X, binary.Y
	if isNil(right) || isNil(left) {
		termExpr := left
		if isNil(left) {
			termExpr = right
		}
		term, err := b.expression(scope, termExpr)
		if err != nil {
			return err
		}
		*atoms = append(*atoms, GuardAtom{Kind: GuardAtomNil, Negated: negated, Term: term})
		return nil
	}
	if length, count, ok := lengthComparison(left, right); ok {
		term, err := b.expression(scope, length)
		if err != nil {
			return err
		}
		*atoms = append(*atoms, GuardAtom{Kind: GuardAtomLenEq, Negated: negated, Term: term, Constant: b.symbol(ActionSymbolConstant, strconv.Itoa(count))})
		return nil
	}
	if length, count, ok := lengthComparison(right, left); ok {
		term, err := b.expression(scope, length)
		if err != nil {
			return err
		}
		*atoms = append(*atoms, GuardAtom{Kind: GuardAtomLenEq, Negated: negated, Term: term, Constant: b.symbol(ActionSymbolConstant, strconv.Itoa(count))})
		return nil
	}
	termExpr, constantExpr := left, right
	if !isGuardConstant(right) && isGuardConstant(left) {
		termExpr, constantExpr = right, left
	}
	if !isGuardConstant(constantExpr) {
		return fmt.Errorf("control equality has no finite constant")
	}
	term, err := b.expression(scope, termExpr)
	if err != nil {
		return err
	}
	constant, err := b.guardConstant(constantExpr)
	if err != nil {
		return err
	}
	*atoms = append(*atoms, GuardAtom{Kind: GuardAtomEqConst, Negated: negated, Term: term, Constant: constant})
	return nil
}

func isNil(expression goast.Expr) bool {
	ident, ok := expression.(*goast.Ident)
	return ok && ident.Name == "nil"
}

func lengthComparison(lengthSide, countSide goast.Expr) (goast.Expr, int, bool) {
	call, ok := lengthSide.(*goast.CallExpr)
	if !ok {
		return nil, 0, false
	}
	name, ok := call.Fun.(*goast.Ident)
	if !ok || name.Name != "len" || len(call.Args) != 1 {
		return nil, 0, false
	}
	literal, ok := countSide.(*goast.BasicLit)
	if !ok || literal.Kind != token.INT {
		return nil, 0, false
	}
	count, err := strconv.Atoi(literal.Value)
	if err != nil || (count != 0 && count != 1) {
		return nil, 0, false
	}
	return call.Args[0], count, true
}

func isGuardConstant(expression goast.Expr) bool {
	switch node := expression.(type) {
	case *goast.BasicLit:
		return node.Kind == token.STRING || node.Kind == token.INT
	case *goast.Ident:
		return node.Name == "true" || node.Name == "false"
	default:
		return false
	}
}

func (b *actionTermBuilder) guardConstant(expression goast.Expr) (ActionSymbolID, error) {
	switch node := expression.(type) {
	case *goast.BasicLit:
		if node.Kind == token.STRING {
			value, err := strconv.Unquote(node.Value)
			if err != nil {
				return 0, err
			}
			return b.symbol(ActionSymbolConstant, value), nil
		}
		if node.Kind == token.INT {
			value, err := strconv.ParseInt(node.Value, 0, 64)
			if err != nil {
				return 0, err
			}
			return b.symbol(ActionSymbolConstant, strconv.FormatInt(value, 10)), nil
		}
	case *goast.Ident:
		if node.Name == "true" || node.Name == "false" {
			return b.symbol(ActionSymbolConstant, node.Name), nil
		}
	}
	return 0, fmt.Errorf("unsupported guard constant")
}

func (b *actionTermBuilder) typeGuard(scope *actionTermScope, expression goast.Expr, cases []goast.Expr, negated bool) (Guard, error) {
	term, err := b.expression(scope, expression)
	if err != nil {
		return Guard{}, err
	}
	if len(cases) == 0 {
		return Guard{}, fmt.Errorf("empty type guard")
	}
	set := make([]ActionSymbolID, len(cases))
	for index, item := range cases {
		name, typeErr := actionTypeSymbol(item)
		if typeErr != nil {
			return Guard{}, typeErr
		}
		set[index] = b.symbol(ActionSymbolType, name)
	}
	sort.Slice(set, func(left, right int) bool { return symbolLess(b.symbolValue(set[left]), b.symbolValue(set[right])) })
	for index := 1; index < len(set); index++ {
		if set[index-1] == set[index] {
			return Guard{}, fmt.Errorf("duplicate type guard case")
		}
	}
	start := uint32(len(b.terms.GuardSymbols))
	b.terms.GuardSymbols = append(b.terms.GuardSymbols, set...)
	b.guardRanges = append(b.guardRanges, actionGuardRange{start: start, count: uint16(len(set))})
	return Guard{Atoms: []GuardAtom{{Kind: GuardAtomTypeIn, Negated: negated, Term: term, SetStart: start, SetCount: uint16(len(set))}}}, nil
}

// actionTypeSymbol is intentionally a closed AST renderer, not go/format.
// It names only type syntax admitted by parser actions/helpers.
func actionTypeSymbol(expression goast.Expr) (string, error) {
	switch node := expression.(type) {
	case *goast.Ident:
		if node.Name == "" {
			return "", fmt.Errorf("empty action type")
		}
		return node.Name, nil
	case *goast.SelectorExpr:
		base, ok := node.X.(*goast.Ident)
		if !ok || base.Name != "ast" || node.Sel == nil {
			return "", fmt.Errorf("unsupported qualified action type")
		}
		return "ast." + node.Sel.Name, nil
	case *goast.StarExpr:
		child, err := actionTypeSymbol(node.X)
		if err != nil {
			return "", err
		}
		return "*" + child, nil
	case *goast.ArrayType:
		if node.Len != nil {
			return "", fmt.Errorf("array action type is unsupported")
		}
		child, err := actionTypeSymbol(node.Elt)
		if err != nil {
			return "", err
		}
		return "[]" + child, nil
	case *goast.InterfaceType:
		if node.Methods != nil && len(node.Methods.List) != 0 {
			return "", fmt.Errorf("nonempty interface action type is unsupported")
		}
		return "interface{}", nil
	default:
		return "", fmt.Errorf("unsupported action type %T", expression)
	}
}

// finalize canonicalizes symbols and returns their old-to-new mapping so the
// enclosing Evidence rows can be remapped before publication.
func (b *actionTermBuilder) finalize() []ActionSymbolID {
	order := make([]int, len(b.terms.Symbols))
	for index := range order {
		order[index] = index
	}
	sort.Slice(order, func(left, right int) bool {
		return symbolLess(b.terms.Symbols[order[left]], b.terms.Symbols[order[right]])
	})
	remap := make([]ActionSymbolID, len(order)+1)
	symbols := make([]ActionSymbol, len(order))
	for target, source := range order {
		symbols[target] = b.terms.Symbols[source]
		remap[source+1] = ActionSymbolID(target + 1)
	}
	b.terms.Symbols = symbols
	for index := range b.terms.Terms {
		if id := b.terms.Terms[index].Symbol; id != 0 {
			b.terms.Terms[index].Symbol = remap[id]
		}
	}
	for index := range b.terms.Edges {
		if id := b.terms.Edges[index].Label; id != 0 {
			b.terms.Edges[index].Label = remap[id]
		}
	}
	for index := range b.terms.ChainTails {
		b.terms.ChainTails[index].Field = remap[b.terms.ChainTails[index].Field]
	}
	for index := range b.terms.Scopes {
		b.terms.Scopes[index].Owner = remap[b.terms.Scopes[index].Owner]
	}
	for index := range b.terms.PlaceSteps {
		if id := b.terms.PlaceSteps[index].Field; id != 0 {
			b.terms.PlaceSteps[index].Field = remap[id]
		}
	}
	for index := range b.terms.GuardSymbols {
		b.terms.GuardSymbols[index] = remap[b.terms.GuardSymbols[index]]
	}
	for _, item := range b.guardRanges {
		set := b.terms.GuardSymbols[item.start : item.start+uint32(item.count)]
		sort.Slice(set, func(left, right int) bool { return set[left] < set[right] })
	}
	for index, term := range b.terms.Terms {
		if term.Kind != ActionTermRecord {
			continue
		}
		edges := b.terms.Edges[term.EdgeStart : term.EdgeStart+uint32(term.EdgeCount)]
		sort.Slice(edges, func(left, right int) bool { return edges[left].Label < edges[right].Label })
		b.terms.Terms[index] = term
	}
	return remap
}
