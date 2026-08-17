package parsersource

import (
	"fmt"
	goast "go/ast"
	"strconv"
	"strings"
)

// collect reads every semantic action and every parser helper once and records
// the construction geometry a state law needs: which values each scope builds,
// which names each scope binds, which fields each scope writes after
// construction, and which calls carry a scope's values into a helper.
func (b *productBuilder) collect(root string) error {
	if err := b.collectHelpers(root); err != nil {
		return err
	}
	return b.collectProductions(root)
}

func (b *productBuilder) collectHelpers(root string) error {
	var declarations []*goast.FuncDecl
	err := VisitHelperSyntax(root, func(template HelperTemplate, function *goast.FuncDecl) error {
		if function == nil || function.Body == nil || function.Name == nil {
			return nil
		}
		scope := b.newScope(ProductScopeHelper, function.Name.Name)
		for _, parameter := range function.Type.Params.List {
			for _, name := range parameter.Names {
				scope.formals = append(scope.formals, name.Name)
			}
		}
		if function.Type.Results != nil {
			for _, result := range function.Type.Results.List {
				count := len(result.Names)
				if count == 0 {
					count = 1
				}
				scope.results += count
			}
		}
		b.helperScopes[function.Name.Name] = scope.index
		declarations = append(declarations, function)
		return nil
	})
	if err != nil {
		return fmt.Errorf("parser products: derive parser helpers: %w", err)
	}
	if len(declarations) == 0 {
		return fmt.Errorf("parser products: parser.go.y states no helpers")
	}
	b.markDiagnosticHelpers(declarations)
	for _, function := range declarations {
		scope := b.scopes[b.helperScopes[function.Name.Name]]
		b.markRejectedReturns(scope, function)
		b.walk(scope, function.Body, false)
	}
	return nil
}

func (b *productBuilder) collectProductions(root string) error {
	err := VisitActionSyntax(root, func(template ActionTemplate, body *goast.BlockStmt) error {
		symbols, symbolErr := ProductionSymbols(template.RHS)
		if symbolErr != nil {
			return fmt.Errorf("parser products: %s: %w", template.Key, symbolErr)
		}
		scope := b.newScope(ProductScopeProduction, template.Key)
		scope.symbols = symbols
		b.nonterminals[template.Nonterminal] = append(b.nonterminals[template.Nonterminal], scope.index)
		b.walk(scope, body, false)
		return nil
	})
	if err != nil {
		return fmt.Errorf("parser products: derive parser actions: %w", err)
	}
	return nil
}

func (b *productBuilder) newScope(kind ProductScope, owner string) *actionScope {
	scope := &actionScope{
		index:       len(b.scopes),
		kind:        kind,
		owner:       owner,
		locals:      make(map[string][]binding),
		elements:    make(map[string][]goast.Expr),
		guarded:     make(map[*goast.CompositeLit]bool),
		rejected:    make(map[*goast.CompositeLit]bool),
		elementwise: make(map[*goast.CompositeLit]bool),
		sites:       make(map[*goast.CompositeLit]siteID),
		rejectedAt:  make(map[goast.Node]bool),
	}
	b.scopes = append(b.scopes, scope)
	return scope
}

// markDiagnosticHelpers resolves the helpers whose whole purpose is to raise a
// parse diagnostic. A helper that yields nothing and reaches the scanner's
// error surface is one; a helper that calls such a helper is one too. The set
// is a least fixed point over the call graph rather than a list of names.
func (b *productBuilder) markDiagnosticHelpers(declarations []*goast.FuncDecl) {
	direct := make(map[string]bool, len(declarations))
	calls := make(map[string][]string, len(declarations))
	for _, function := range declarations {
		name := function.Name.Name
		if function.Type.Results != nil && len(function.Type.Results.List) != 0 {
			continue
		}
		operands := lexerOperands(function)
		goast.Inspect(function.Body, func(node goast.Node) bool {
			call, ok := node.(*goast.CallExpr)
			if !ok {
				return true
			}
			if lexerDiagnostic(call, operands) {
				direct[name] = true
			}
			if callee, ok := call.Fun.(*goast.Ident); ok {
				calls[name] = append(calls[name], callee.Name)
			}
			return true
		})
	}
	for changed := true; changed; {
		changed = false
		for name, callees := range calls {
			if direct[name] {
				continue
			}
			for _, callee := range callees {
				if direct[callee] {
					direct[name] = true
					changed = true
					break
				}
			}
		}
	}
	b.diagnostics = direct
}

// yaccLexerInterface is the interface goyacc gives every semantic action and
// every helper that needs the scanner. It is generated vocabulary, like the $$
// and $N operands, so reading it is reading the yacc contract rather than
// guessing at a name.
const yaccLexerInterface = "yyLexer"

// lexerOperands names the values in one helper that carry the scanner: the
// formals declared with the yacc lexer interface, and the locals bound from one
// by a type assertion to its concrete scanner type.
func lexerOperands(function *goast.FuncDecl) map[string]bool {
	result := make(map[string]bool)
	if function.Type.Params != nil {
		for _, parameter := range function.Type.Params.List {
			if identifier, ok := parameter.Type.(*goast.Ident); !ok || identifier.Name != yaccLexerInterface {
				continue
			}
			for _, name := range parameter.Names {
				result[name.Name] = true
			}
		}
	}
	if function.Body == nil {
		return result
	}
	for changed := true; changed; {
		changed = false
		goast.Inspect(function.Body, func(node goast.Node) bool {
			assignment, ok := node.(*goast.AssignStmt)
			if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				return true
			}
			target, ok := assignment.Lhs[0].(*goast.Ident)
			if !ok || result[target.Name] {
				return true
			}
			if !lexerRooted(assignment.Rhs[0], result) {
				return true
			}
			result[target.Name] = true
			changed = true
			return true
		})
	}
	return result
}

// lexerRooted reports whether an expression reaches a scanner operand through
// the parentheses and type assertions a helper writes around it.
func lexerRooted(expression goast.Expr, operands map[string]bool) bool {
	switch current := expression.(type) {
	case *goast.Ident:
		return operands[current.Name]
	case *goast.ParenExpr:
		return lexerRooted(current.X, operands)
	case *goast.TypeAssertExpr:
		return lexerRooted(current.X, operands)
	default:
		return false
	}
}

// lexerDiagnostic recognizes a call into the scanner's error surface: a method
// named for reporting an error, on a receiver that reaches a scanner operand.
func lexerDiagnostic(call *goast.CallExpr, operands map[string]bool) bool {
	selector, ok := call.Fun.(*goast.SelectorExpr)
	if !ok {
		return false
	}
	return lexerRooted(selector.X, operands) && strings.HasSuffix(selector.Sel.Name, "Error")
}

// markRejectedReturns finds the returns a helper states after it has already
// raised a diagnostic. Such a return is the value the parser carries away from
// a rejected source, not a value a successful parse yields, so the
// constructions inside it are recorded as rejected rather than dropped.
func (b *productBuilder) markRejectedReturns(scope *actionScope, function *goast.FuncDecl) {
	operands := lexerOperands(function)
	var mark func(list []goast.Stmt, rejected bool)
	mark = func(list []goast.Stmt, rejected bool) {
		current := rejected
		for _, statement := range list {
			if current {
				scope.rejectedAt[statement] = true
			}
			switch node := statement.(type) {
			case *goast.ExprStmt:
				if call, ok := node.X.(*goast.CallExpr); ok && b.isDiagnostic(call, operands) {
					current = true
				}
			case *goast.BlockStmt:
				mark(node.List, current)
			case *goast.IfStmt:
				mark(node.Body.List, current)
				if node.Else != nil {
					switch alternative := node.Else.(type) {
					case *goast.BlockStmt:
						mark(alternative.List, current)
					case *goast.IfStmt:
						mark([]goast.Stmt{alternative}, current)
					}
				}
			case *goast.ForStmt:
				mark(node.Body.List, current)
			case *goast.RangeStmt:
				mark(node.Body.List, current)
			case *goast.SwitchStmt:
				for _, clause := range node.Body.List {
					if caseClause, ok := clause.(*goast.CaseClause); ok {
						mark(caseClause.Body, current)
					}
				}
			case *goast.TypeSwitchStmt:
				for _, clause := range node.Body.List {
					if caseClause, ok := clause.(*goast.CaseClause); ok {
						mark(caseClause.Body, current)
					}
				}
			}
		}
	}
	mark(function.Body.List, false)
}

func (b *productBuilder) isDiagnostic(call *goast.CallExpr, operands map[string]bool) bool {
	if lexerDiagnostic(call, operands) {
		return true
	}
	callee, ok := call.Fun.(*goast.Ident)
	return ok && b.diagnostics[callee.Name]
}

// walk records one scope's construction geometry. It is deliberately
// flow-insensitive: a state law asks what an action can produce, so every
// binding an action can make counts, and the branch a binding sits in is
// recorded beside it rather than pruning it.
func (b *productBuilder) walk(scope *actionScope, node goast.Node, guarded bool) {
	switch current := node.(type) {
	case nil:
		return
	case *goast.BlockStmt:
		for _, statement := range current.List {
			b.walk(scope, statement, guarded)
		}
	case *goast.IfStmt:
		b.walk(scope, current.Init, guarded)
		b.walkExpr(scope, current.Cond, guarded)
		b.walk(scope, current.Body, true)
		b.walk(scope, current.Else, true)
	case *goast.ForStmt:
		b.walk(scope, current.Init, guarded)
		b.walkExpr(scope, current.Cond, guarded)
		b.walk(scope, current.Post, guarded)
		b.walk(scope, current.Body, true)
	case *goast.RangeStmt:
		b.walkExpr(scope, current.X, guarded)
		if value, ok := current.Value.(*goast.Ident); ok && value.Name != "_" {
			scope.locals[value.Name] = append(scope.locals[value.Name], binding{kind: bindingElement, expr: current.X})
		}
		b.walk(scope, current.Body, true)
	case *goast.SwitchStmt:
		b.walk(scope, current.Init, guarded)
		b.walkExpr(scope, current.Tag, guarded)
		for _, clause := range current.Body.List {
			if caseClause, ok := clause.(*goast.CaseClause); ok {
				for _, statement := range caseClause.Body {
					b.walk(scope, statement, true)
				}
			}
		}
	case *goast.TypeSwitchStmt:
		b.walk(scope, current.Init, guarded)
		b.walk(scope, current.Assign, guarded)
		for _, clause := range current.Body.List {
			if caseClause, ok := clause.(*goast.CaseClause); ok {
				for _, statement := range caseClause.Body {
					b.walk(scope, statement, true)
				}
			}
		}
	case *goast.ExprStmt:
		b.walkExpr(scope, current.X, guarded)
	case *goast.ReturnStmt:
		rejected := scope.rejectedAt[current]
		if len(current.Results) != 0 {
			scope.returns = append(scope.returns, current.Results)
		}
		for _, result := range current.Results {
			b.walkExpr(scope, result, guarded)
			b.markLiterals(scope, result, rejected)
			scope.roots = append(scope.roots, result)
		}
	case *goast.DeclStmt:
		general, ok := current.Decl.(*goast.GenDecl)
		if !ok {
			return
		}
		for _, specification := range general.Specs {
			valueSpec, ok := specification.(*goast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range valueSpec.Names {
				if index < len(valueSpec.Values) {
					scope.locals[name.Name] = append(scope.locals[name.Name], binding{kind: bindingExpr, expr: valueSpec.Values[index]})
					b.walkExpr(scope, valueSpec.Values[index], guarded)
					continue
				}
				scope.locals[name.Name] = append(scope.locals[name.Name], binding{kind: bindingExpr})
			}
		}
	case *goast.IncDecStmt:
		b.walkExpr(scope, current.X, guarded)
	case *goast.AssignStmt:
		b.walkAssign(scope, current, guarded)
	case goast.Stmt:
		goast.Inspect(current, func(inner goast.Node) bool {
			if expression, ok := inner.(goast.Expr); ok {
				b.walkExpr(scope, expression, guarded)
				return false
			}
			return true
		})
	}
}

func (b *productBuilder) walkAssign(scope *actionScope, assignment *goast.AssignStmt, guarded bool) {
	for _, right := range assignment.Rhs {
		b.walkExpr(scope, right, guarded)
	}
	shared := len(assignment.Lhs) > 1 && len(assignment.Rhs) == 1
	var sharedCall *goast.CallExpr
	if shared {
		if call, ok := assignment.Rhs[0].(*goast.CallExpr); ok {
			sharedCall = call
		}
		if assert, ok := assignment.Rhs[0].(*goast.TypeAssertExpr); ok {
			b.bindAssert(scope, assignment.Lhs[0], assert)
			return
		}
	}
	for index, left := range assignment.Lhs {
		if sharedCall != nil {
			if callee, ok := sharedCall.Fun.(*goast.Ident); ok {
				if identifier, ok := left.(*goast.Ident); ok && identifier.Name != "_" {
					scope.locals[identifier.Name] = append(scope.locals[identifier.Name], binding{kind: bindingCallResult, helper: callee.Name, index: index})
				}
			}
			continue
		}
		if index >= len(assignment.Rhs) {
			continue
		}
		right := assignment.Rhs[index]
		switch target := left.(type) {
		case *goast.Ident:
			if target.Name == "_" {
				continue
			}
			if assert, ok := right.(*goast.TypeAssertExpr); ok {
				b.bindAssert(scope, target, assert)
				continue
			}
			scope.locals[target.Name] = append(scope.locals[target.Name], binding{kind: bindingExpr, expr: right})
			if target.Name == resultOperand {
				scope.roots = append(scope.roots, right)
			}
		case *goast.IndexExpr:
			if root := rootName(target.X); root != "" {
				scope.elements[root] = append(scope.elements[root], right)
				markElementwise(scope, right)
			}
		case *goast.SelectorExpr:
			b.mutations = append(b.mutations, mutationSite{
				scope:  scope.index,
				target: target.X,
				field:  target.Sel.Name,
				value:  right,
			})
			scope.roots = append(scope.roots, right)
		}
	}
}

func (b *productBuilder) bindAssert(scope *actionScope, left goast.Expr, assert *goast.TypeAssertExpr) {
	identifier, ok := left.(*goast.Ident)
	if !ok || identifier.Name == "_" || assert.Type == nil {
		return
	}
	scope.locals[identifier.Name] = append(scope.locals[identifier.Name], binding{kind: bindingAssert, expr: assert.X, typeName: assertedName(assert.Type)})
}

func assertedName(expression goast.Expr) string {
	switch value := expression.(type) {
	case *goast.StarExpr:
		return assertedName(value.X)
	case *goast.SelectorExpr:
		if qualifier, ok := value.X.(*goast.Ident); ok && qualifier.Name == "ast" {
			return value.Sel.Name
		}
		return ""
	case *goast.Ident:
		return value.Name
	default:
		return ""
	}
}

func rootName(expression goast.Expr) string {
	switch value := expression.(type) {
	case *goast.Ident:
		return value.Name
	case *goast.SelectorExpr:
		return rootName(value.X)
	case *goast.IndexExpr:
		return rootName(value.X)
	case *goast.ParenExpr:
		return rootName(value.X)
	case *goast.TypeAssertExpr:
		return rootName(value.X)
	default:
		return ""
	}
}

// walkExpr registers the construction sites and helper applications an
// expression states.
func (b *productBuilder) walkExpr(scope *actionScope, expression goast.Expr, guarded bool) {
	if expression == nil {
		return
	}
	goast.Inspect(expression, func(node goast.Node) bool {
		switch current := node.(type) {
		case *goast.CompositeLit:
			b.site(scope, current, guarded)
		case *goast.CallExpr:
			if callee, ok := current.Fun.(*goast.Ident); ok {
				if _, known := b.helperScopes[callee.Name]; known {
					b.calls = append(b.calls, helperCall{scope: scope.index, helper: callee.Name, actuals: current.Args})
				}
			}
		}
		return true
	})
}

// markElementwise records that a construction reaches its destination one
// element at a time. The distinction is the shape of the write, not the shape
// of the value: an indexed destination is filled per element by construction.
func markElementwise(scope *actionScope, expression goast.Expr) {
	if expression == nil {
		return
	}
	goast.Inspect(expression, func(node goast.Node) bool {
		if literal, ok := node.(*goast.CompositeLit); ok {
			scope.elementwise[literal] = true
		}
		return true
	})
}

func (b *productBuilder) markLiterals(scope *actionScope, expression goast.Expr, rejected bool) {
	if !rejected || expression == nil {
		return
	}
	goast.Inspect(expression, func(node goast.Node) bool {
		if literal, ok := node.(*goast.CompositeLit); ok {
			scope.rejected[literal] = true
		}
		return true
	})
}

// site records one composite literal as a construction site. A literal is
// recorded once per scope no matter how often the walk reaches it, so a value
// read twice does not become two constructions.
func (b *productBuilder) site(scope *actionScope, literal *goast.CompositeLit, guarded bool) siteID {
	if existing, seen := scope.sites[literal]; seen {
		if guarded && !scope.guarded[literal] {
			scope.guarded[literal] = true
		}
		return existing
	}
	name, astType := literalTypeName(literal)
	record := &constructionSite{
		id:       siteID(len(b.sites)),
		scope:    scope.index,
		typeName: name,
		astType:  astType,
		semantic: astType && b.semantic[name],
		literal:  literal,
		elements: make(map[string]goast.Expr, len(literal.Elts)),
	}
	if declaration, known := b.declarations[name]; astType && known {
		record.fields = declaration.Fields
	} else if fields, known := b.records[name]; !astType && known {
		record.fields = fields
	}
	for _, element := range literal.Elts {
		pair, ok := element.(*goast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*goast.Ident)
		if !ok {
			continue
		}
		record.elements[key.Name] = pair.Value
	}
	b.sites = append(b.sites, record)
	scope.sites[literal] = record.id
	if guarded {
		scope.guarded[literal] = true
	}
	return record.id
}

// literalTypeName names the type a composite literal builds. The second result
// separates a compiler AST type from a parser-only record: only the first can
// carry a census carrier, while both carry assignment behaviour.
func literalTypeName(literal *goast.CompositeLit) (string, bool) {
	switch typeExpr := literal.Type.(type) {
	case *goast.SelectorExpr:
		if qualifier, ok := typeExpr.X.(*goast.Ident); ok && qualifier.Name == "ast" {
			return typeExpr.Sel.Name, true
		}
		return "", false
	case *goast.Ident:
		return typeExpr.Name, false
	default:
		return "", false
	}
}

// resultOperand is the spelling VisitActionSyntax gives the yacc $$ operand.
const resultOperand = "Result"

// operandSlot decodes the spelling VisitActionSyntax gives a positional yacc
// operand. The zero result reports that the identifier is not an operand.
func operandSlot(name string) int {
	if !strings.HasPrefix(name, "Arg") {
		return 0
	}
	slot, err := strconv.Atoi(strings.TrimPrefix(name, "Arg"))
	if err != nil || slot <= 0 {
		return 0
	}
	return slot
}
