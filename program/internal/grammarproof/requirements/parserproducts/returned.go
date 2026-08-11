package parserproducts

import (
	"fmt"
	goast "go/ast"
	"go/token"
	"sort"
	"strconv"

	"github.com/wippyai/go-lua/program/internal/grammarproof"
	"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/grammar"
)

// typedRelation is private cold extraction state. Every expression is turned
// into a term before it enters a public row.
type typedRelation struct {
	products  []ConstructorProduct
	helpers   []HelperApplication
	edits     []Edit
	mutations []Edit
	returns   []GuardedReturn
	rejects   []Reject
	chains    []ChainLaw
}

type indexedWrite struct {
	assignment *goast.AssignStmt
	target     *goast.IndexExpr
	value      goast.Expr
}

type indexedMutation struct {
	assignment *goast.AssignStmt
	target     goast.Expr
	value      goast.Expr
}

type indexedAssembly struct {
	assignment *goast.AssignStmt
	target     goast.Expr
	value      goast.Expr
}

// typedAnalyzer first indexes parser-local definitions and then walks only
// values reachable from Result/return. This keeps an action's construction
// relation exact without serializing any source expression.
type typedAnalyzer struct {
	builder      *actionTermBuilder
	scope        *actionTermScope
	products     map[string]constructorFields
	declarations map[string]grammar.Declaration
	types        map[string]grammar.TypeDeclaration
	helpers      map[string]bool
	forms        map[string]string

	definitions  map[string][]goast.Expr
	aliases      map[string][]string
	writes       map[string][]indexedWrite
	mutations    map[string][]indexedMutation
	assemblies   map[string][]indexedAssembly
	callResults  map[*goast.CallExpr][]goast.Expr
	literalOwner map[*goast.CompositeLit]goast.Expr

	seenNames      map[string]bool
	seenExprs      map[goast.Expr]bool
	seenProducts   map[*goast.CompositeLit]bool
	seenHelpers    map[*goast.CallExpr]bool
	seenWrites     map[*goast.AssignStmt]bool
	seenMutation   map[*goast.AssignStmt]bool
	seenAssembly   map[*goast.AssignStmt]bool
	guard          Guard
	suppressRanges bool
	relation       typedRelation
	err            error
}

func newTypedAnalyzer(builder *actionTermBuilder, scope *actionTermScope, schema grammar.Schema, helpers map[string]bool, forms map[string]string) *typedAnalyzer {
	item := &typedAnalyzer{
		builder: builder, scope: scope, products: schemaConstructorFields(schema), helpers: helpers,
		declarations: make(map[string]grammar.Declaration, len(schema.Declarations)),
		types:        make(map[string]grammar.TypeDeclaration, len(schema.Types)), forms: forms,
		definitions: make(map[string][]goast.Expr), aliases: make(map[string][]string), writes: make(map[string][]indexedWrite),
		mutations: make(map[string][]indexedMutation), assemblies: make(map[string][]indexedAssembly),
		callResults: make(map[*goast.CallExpr][]goast.Expr), literalOwner: make(map[*goast.CompositeLit]goast.Expr),
		seenNames: make(map[string]bool), seenExprs: make(map[goast.Expr]bool), seenProducts: make(map[*goast.CompositeLit]bool),
		seenHelpers: make(map[*goast.CallExpr]bool), seenWrites: make(map[*goast.AssignStmt]bool),
		seenMutation: make(map[*goast.AssignStmt]bool), seenAssembly: make(map[*goast.AssignStmt]bool),
	}
	for _, declaration := range schema.Declarations {
		item.declarations[declaration.Name] = declaration
	}
	for _, declaration := range schema.Types {
		item.types[declaration.Name] = declaration
	}
	return item
}

func (a *typedAnalyzer) index(block *goast.BlockStmt) error {
	if err := bindBlockLocals(a.scope, block); err != nil {
		return err
	}
	for name, form := range assertedLocalFormsBlock(block) {
		a.forms[name] = form
	}
	goast.Inspect(block, func(node goast.Node) bool {
		if a.err != nil {
			return false
		}
		switch statement := node.(type) {
		case *goast.RangeStmt:
			if a.suppressRanges {
				return false
			}
		case *goast.AssignStmt:
			a.indexAssignment(statement)
		case *goast.ValueSpec:
			for index, name := range statement.Names {
				if index < len(statement.Values) {
					a.definitions[name.Name] = append(a.definitions[name.Name], statement.Values[index])
				}
			}
		}
		return true
	})
	return a.err
}

// indexActionTopLevel deliberately stops at the action block boundary. Parser
// branch semantics are emitted by deriveControlledAction, so indexing nested
// statements here would flatten mutually exclusive paths into one relation.
func (a *typedAnalyzer) indexActionTopLevel(block *goast.BlockStmt) error {
	if err := bindBlockLocals(a.scope, block); err != nil {
		return err
	}
	for name, form := range assertedLocalFormsBlock(block) {
		a.forms[name] = form
	}
	for _, statement := range block.List {
		switch row := statement.(type) {
		case *goast.AssignStmt:
			a.indexAssignment(row)
		case *goast.DeclStmt:
			declaration, ok := row.Decl.(*goast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range declaration.Specs {
				values, ok := specification.(*goast.ValueSpec)
				if !ok {
					continue
				}
				for index, name := range values.Names {
					if index < len(values.Values) {
						a.definitions[name.Name] = append(a.definitions[name.Name], values.Values[index])
					}
				}
			}
		}
	}
	return a.err
}

func bindBlockLocals(scope *actionTermScope, block *goast.BlockStmt) error {
	if block == nil {
		return nil
	}
	for _, statement := range block.List {
		if err := bindLocalStatement(scope, statement); err != nil {
			return err
		}
	}
	return nil
}

func bindLocalStatement(scope *actionTermScope, statement goast.Stmt) error {
	if statement == nil {
		return nil
	}
	switch row := statement.(type) {
	case *goast.AssignStmt:
		if row.Tok != token.DEFINE {
			return nil
		}
		for _, left := range row.Lhs {
			if name, ok := left.(*goast.Ident); ok {
				if err := scope.bindLocal(name.Name); err != nil {
					return err
				}
			}
		}
	case *goast.DeclStmt:
		declaration, ok := row.Decl.(*goast.GenDecl)
		if !ok {
			return nil
		}
		for _, specification := range declaration.Specs {
			values, ok := specification.(*goast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range values.Names {
				if err := scope.bindLocal(name.Name); err != nil {
					return err
				}
			}
		}
	case *goast.IfStmt:
		if err := bindLocalStatement(scope, row.Init); err != nil {
			return err
		}
		if err := bindBlockLocals(scope, row.Body); err != nil {
			return err
		}
		if elseBlock, ok := row.Else.(*goast.BlockStmt); ok {
			return bindBlockLocals(scope, elseBlock)
		}
		if elseIf, ok := row.Else.(*goast.IfStmt); ok {
			return bindLocalStatement(scope, elseIf)
		}
	case *goast.RangeStmt:
		for _, expression := range []goast.Expr{row.Key, row.Value} {
			if name, ok := expression.(*goast.Ident); ok {
				if err := scope.bindLocal(name.Name); err != nil {
					return err
				}
			}
		}
		return bindBlockLocals(scope, row.Body)
	case *goast.ForStmt:
		if err := bindLocalStatement(scope, row.Init); err != nil {
			return err
		}
		return bindBlockLocals(scope, row.Body)
	case *goast.SwitchStmt:
		if err := bindLocalStatement(scope, row.Init); err != nil {
			return err
		}
		for _, statement := range row.Body.List {
			caseClause, ok := statement.(*goast.CaseClause)
			if !ok {
				continue
			}
			for _, body := range caseClause.Body {
				if err := bindLocalStatement(scope, body); err != nil {
					return err
				}
			}
		}
	case *goast.TypeSwitchStmt:
		if err := bindLocalStatement(scope, row.Init); err != nil {
			return err
		}
		if err := bindLocalStatement(scope, row.Assign); err != nil {
			return err
		}
		for _, statement := range row.Body.List {
			caseClause, ok := statement.(*goast.CaseClause)
			if !ok {
				continue
			}
			for _, body := range caseClause.Body {
				if err := bindLocalStatement(scope, body); err != nil {
					return err
				}
			}
		}
	case *goast.LabeledStmt:
		return bindLocalStatement(scope, row.Stmt)
	case *goast.BlockStmt:
		return bindBlockLocals(scope, row)
	}
	return nil
}

func assertedLocalFormsBlock(block *goast.BlockStmt) map[string]string {
	forms := make(map[string]string)
	collectAssertedForms(block, forms)
	return forms
}

func collectAssertedForms(block *goast.BlockStmt, forms map[string]string) {
	if block == nil {
		return
	}
	for _, statement := range block.List {
		collectAssertedFormsStatement(statement, forms)
	}
}

func collectAssertedFormsStatement(statement goast.Stmt, forms map[string]string) {
	if statement == nil {
		return
	}
	if assignment, ok := statement.(*goast.AssignStmt); ok {
		for index, right := range assignment.Rhs {
			if index >= len(assignment.Lhs) {
				break
			}
			left, leftOK := assignment.Lhs[index].(*goast.Ident)
			assertion, assertionOK := right.(*goast.TypeAssertExpr)
			if !leftOK || !assertionOK {
				continue
			}
			star, starOK := assertion.Type.(*goast.StarExpr)
			if !starOK {
				continue
			}
			selector, selectorOK := star.X.(*goast.SelectorExpr)
			if !selectorOK {
				continue
			}
			packageName, packageOK := selector.X.(*goast.Ident)
			if packageOK && packageName.Name == "ast" {
				forms[left.Name] = selector.Sel.Name
			}
		}
	}
	switch row := statement.(type) {
	case *goast.IfStmt:
		collectAssertedFormsStatement(row.Init, forms)
		collectAssertedForms(row.Body, forms)
		if elseBlock, ok := row.Else.(*goast.BlockStmt); ok {
			collectAssertedForms(elseBlock, forms)
		}
		if elseIf, ok := row.Else.(*goast.IfStmt); ok {
			collectAssertedFormsStatement(elseIf, forms)
		}
	case *goast.RangeStmt:
		collectAssertedForms(row.Body, forms)
	case *goast.ForStmt:
		collectAssertedFormsStatement(row.Init, forms)
		collectAssertedForms(row.Body, forms)
	case *goast.LabeledStmt:
		collectAssertedFormsStatement(row.Stmt, forms)
	case *goast.BlockStmt:
		collectAssertedForms(row, forms)
	}
}

func helperParameterForms(function *goast.FuncDecl) map[string]string {
	forms := make(map[string]string)
	if function == nil || function.Type.Params == nil {
		return forms
	}
	for _, parameter := range function.Type.Params.List {
		star, ok := parameter.Type.(*goast.StarExpr)
		if !ok {
			continue
		}
		selector, ok := star.X.(*goast.SelectorExpr)
		if !ok {
			continue
		}
		packageName, ok := selector.X.(*goast.Ident)
		if !ok || packageName.Name != "ast" {
			continue
		}
		for _, name := range parameter.Names {
			forms[name.Name] = selector.Sel.Name
		}
	}
	return forms
}

func (a *typedAnalyzer) indexAssignment(assignment *goast.AssignStmt) {
	var sharedCall *goast.CallExpr
	if len(assignment.Rhs) == 1 {
		sharedCall, _ = assignment.Rhs[0].(*goast.CallExpr)
	}
	if sharedCall != nil {
		a.callResults[sharedCall] = append([]goast.Expr(nil), assignment.Lhs...)
	}
	for index, left := range assignment.Lhs {
		rightIndex := index
		if sharedCall != nil {
			rightIndex = 0
		}
		if rightIndex >= len(assignment.Rhs) {
			continue
		}
		right := assignment.Rhs[rightIndex]
		if literal := rootCompositeLiteral(right); literal != nil {
			a.literalOwner[literal] = left
		}
		if call, ok := right.(*goast.CallExpr); ok && sharedCall == nil {
			a.callResults[call] = []goast.Expr{left}
		}
		if identifier, ok := left.(*goast.Ident); ok {
			a.definitions[identifier.Name] = append(a.definitions[identifier.Name], right)
			if source := aliasIdentifier(right); source != "" && source != identifier.Name {
				a.aliases[identifier.Name] = append(a.aliases[identifier.Name], source)
				a.aliases[source] = append(a.aliases[source], identifier.Name)
			}
			if literal := rootCompositeLiteral(right); literal != nil {
				name, semantic, err := a.literalClass(literal)
				if err != nil {
					a.err = err
					return
				}
				if semantic {
					a.forms[identifier.Name] = name
				}
			}
			continue
		}
		if indexed, ok := left.(*goast.IndexExpr); ok {
			if name := rootIdentifier(indexed.X); name != "" {
				a.writes[name] = append(a.writes[name], indexedWrite{assignment: assignment, target: indexed, value: right})
			}
			continue
		}
		if receiver, _, ok := mutationReceiverTyped(left, a.forms); ok {
			a.mutations[receiver] = append(a.mutations[receiver], indexedMutation{assignment: assignment, target: left, value: right})
			continue
		}
		if _, ok := left.(*goast.SelectorExpr); ok {
			if root := rootIdentifier(left); root != "" {
				a.assemblies[root] = append(a.assemblies[root], indexedAssembly{assignment: assignment, target: left, value: right})
			}
		}
	}
}

func mutationReceiverTyped(left goast.Expr, forms map[string]string) (string, string, bool) {
	selector, ok := left.(*goast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	if receiver, ok := selector.X.(*goast.Ident); ok && forms[receiver.Name] != "" {
		return receiver.Name, forms[receiver.Name], true
	}
	assertion, ok := selector.X.(*goast.TypeAssertExpr)
	if !ok {
		return "", "", false
	}
	receiver, ok := assertion.X.(*goast.Ident)
	if !ok {
		return "", "", false
	}
	star, ok := assertion.Type.(*goast.StarExpr)
	if !ok {
		return "", "", false
	}
	qualified, ok := star.X.(*goast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	packageName, ok := qualified.X.(*goast.Ident)
	if !ok || packageName.Name != "ast" {
		return "", "", false
	}
	return receiver.Name, qualified.Sel.Name, true
}

func (a *typedAnalyzer) visitName(name string) {
	if name == "" || a.seenNames[name] || a.err != nil {
		return
	}
	a.seenNames[name] = true
	for _, alias := range a.aliases[name] {
		a.visitName(alias)
	}
	for _, definition := range a.definitions[name] {
		a.visit(definition)
	}
	for _, write := range a.writes[name] {
		a.visitWrite(write)
	}
	for _, mutation := range a.mutations[name] {
		a.visitMutation(mutation)
	}
	for _, assembly := range a.assemblies[name] {
		a.visitAssembly(assembly)
	}
}

func (a *typedAnalyzer) visitWrite(write indexedWrite) {
	if a.seenWrites[write.assignment] || a.err != nil {
		return
	}
	a.seenWrites[write.assignment] = true
	place, err := a.builder.place(a.scope, write.target)
	if err != nil {
		a.err = err
		return
	}
	value, err := a.builder.expression(a.scope, write.value)
	if err != nil {
		a.err = err
		return
	}
	a.relation.edits = append(a.relation.edits, Edit{Kind: editKind(write.value), Guard: a.guard, Place: place, Value: value})
	a.visit(write.target.Index)
	a.visit(write.value)
}

func (a *typedAnalyzer) visitMutation(mutation indexedMutation) {
	if a.seenMutation[mutation.assignment] || a.err != nil {
		return
	}
	a.seenMutation[mutation.assignment] = true
	place, err := a.builder.place(a.scope, mutation.target)
	if err != nil {
		a.err = err
		return
	}
	value, err := a.builder.expression(a.scope, mutation.value)
	if err != nil {
		a.err = err
		return
	}
	edit := Edit{Kind: editKind(mutation.value), Guard: a.guard, Place: place, Value: value}
	if a.builder.terms.Scopes[a.scope.index].Kind == ActionScopeHelper {
		a.relation.edits = append(a.relation.edits, edit)
	} else {
		a.relation.mutations = append(a.relation.mutations, edit)
	}
	a.visit(mutation.value)
}

func (a *typedAnalyzer) visitAssembly(assembly indexedAssembly) {
	if a.seenAssembly[assembly.assignment] || a.err != nil {
		return
	}
	a.seenAssembly[assembly.assignment] = true
	place, err := a.builder.place(a.scope, assembly.target)
	if err != nil {
		a.err = err
		return
	}
	value, err := a.builder.expression(a.scope, assembly.value)
	if err != nil {
		a.err = err
		return
	}
	a.relation.edits = append(a.relation.edits, Edit{Kind: editKind(assembly.value), Guard: a.guard, Place: place, Value: value})
	a.visit(assembly.value)
}

func editKind(value goast.Expr) EditKind {
	call, ok := value.(*goast.CallExpr)
	if !ok {
		return EditAssign
	}
	name, ok := call.Fun.(*goast.Ident)
	if ok && name.Name == "append" {
		return EditAppend
	}
	return EditAssign
}

func (a *typedAnalyzer) visit(expression goast.Expr) {
	if expression == nil || a.seenExprs[expression] || a.err != nil {
		return
	}
	a.seenExprs[expression] = true
	switch node := expression.(type) {
	case *goast.Ident:
		a.visitName(node.Name)
	case *goast.CompositeLit:
		a.visitLiteral(node)
	case *goast.UnaryExpr:
		a.visit(node.X)
	case *goast.ParenExpr:
		a.visit(node.X)
	case *goast.CallExpr:
		a.visitCall(node)
	case *goast.SelectorExpr:
		a.visit(node.X)
	case *goast.IndexExpr:
		a.visit(node.X)
		a.visit(node.Index)
	case *goast.SliceExpr:
		a.visit(node.X)
		a.visit(node.Low)
		a.visit(node.High)
		a.visit(node.Max)
	case *goast.TypeAssertExpr:
		a.visit(node.X)
	case *goast.BinaryExpr:
		a.visit(node.X)
		a.visit(node.Y)
	case *goast.KeyValueExpr:
		a.visit(node.Value)
	case *goast.StarExpr:
		a.visit(node.X)
	}
}

func (a *typedAnalyzer) visitLiteral(literal *goast.CompositeLit) {
	name, semantic, err := a.literalClass(literal)
	if err != nil {
		a.err = err
		return
	}
	if semantic && !a.seenProducts[literal] {
		product, ok, productErr := productFromTypedLiteral(a.builder, a.scope, literal, a.products)
		if productErr != nil {
			a.err = productErr
			return
		}
		if !ok {
			a.err = fmt.Errorf("semantic ast.%s is absent from parser constructor schema", name)
			return
		}
		a.seenProducts[literal] = true
		product.Guard = a.guard
		a.relation.products = append(a.relation.products, product)
	}
	// Parser-only records carry scanner positions and other assembly state. They
	// are intentionally not converted into semantic edits: the typed evidence
	// owns AST products, controlled mutations, map summaries, and diagnostics,
	// not span bookkeeping.
	for _, element := range literal.Elts {
		if pair, ok := element.(*goast.KeyValueExpr); ok {
			a.visit(pair.Value)
		} else {
			a.visit(element)
		}
	}
}

func (a *typedAnalyzer) literalClass(literal *goast.CompositeLit) (string, bool, error) {
	selector, ok := literal.Type.(*goast.SelectorExpr)
	if !ok {
		return "", false, nil
	}
	packageName, ok := selector.X.(*goast.Ident)
	if !ok || packageName.Name != "ast" {
		return "", false, nil
	}
	declaration, exists := a.declarations[selector.Sel.Name]
	if !exists {
		if _, known := a.types[selector.Sel.Name]; known {
			return selector.Sel.Name, false, nil
		}
		return "", false, fmt.Errorf("unknown compiler AST declaration ast.%s", selector.Sel.Name)
	}
	return declaration.Name, declaration.Semantic, nil
}

func (a *typedAnalyzer) visitCall(call *goast.CallExpr) {
	if name, ok := call.Fun.(*goast.Ident); ok && a.helpers[name.Name] && !a.seenHelpers[call] {
		actuals := make([]ActionTermID, len(call.Args))
		for index, argument := range call.Args {
			term, err := a.builder.expression(a.scope, argument)
			if err != nil {
				a.err = err
				return
			}
			actuals[index] = term
		}
		results := make([]Place, len(a.callResults[call]))
		for index, destination := range a.callResults[call] {
			place, err := a.builder.place(a.scope, destination)
			if err != nil {
				a.err = err
				return
			}
			results[index] = place
		}
		a.relation.helpers = append(a.relation.helpers, HelperApplication{Helper: a.builder.symbol(ActionSymbolCallable, name.Name), Scope: a.scope.id, Guard: a.guard, Actuals: actuals, Results: results})
		a.seenHelpers[call] = true
	}
	for _, argument := range call.Args {
		a.visit(argument)
	}
}

func (a *typedAnalyzer) addReturn(ordinal int, guard Guard, returned *goast.ReturnStmt) error {
	if returned == nil {
		return fmt.Errorf("missing helper return")
	}
	priorGuard := a.guard
	a.guard = guard
	defer func() { a.guard = priorGuard }()
	values := make([]ActionTermID, len(returned.Results))
	for index, expression := range returned.Results {
		term, err := a.builder.expression(a.scope, expression)
		if err != nil {
			return err
		}
		values[index] = term
		a.visit(expression)
	}
	a.relation.returns = append(a.relation.returns, GuardedReturn{Ordinal: ordinal, Guard: guard, Values: values})
	return a.err
}

func (a *typedAnalyzer) canonicalize() error {
	for index := range a.relation.products {
		a.relation.products[index].Ordinal = index + 1
	}
	sort.Slice(a.relation.helpers, func(left, right int) bool {
		return applicationLess(a.relation.helpers[left], a.relation.helpers[right])
	})
	sort.Slice(a.relation.edits, func(left, right int) bool { return editLess(a.relation.edits[left], a.relation.edits[right]) })
	sort.Slice(a.relation.mutations, func(left, right int) bool { return editLess(a.relation.mutations[left], a.relation.mutations[right]) })
	sort.Slice(a.relation.chains, func(left, right int) bool {
		first, second := a.relation.chains[left], a.relation.chains[right]
		if first.Input != second.Input {
			return first.Input < second.Input
		}
		if first.Seed != second.Seed {
			return first.Seed < second.Seed
		}
		return first.LinkField < second.LinkField
	})
	return a.err
}

func productFromTypedLiteral(builder *actionTermBuilder, scope *actionTermScope, literal *goast.CompositeLit, schema map[string]constructorFields) (ConstructorProduct, bool, error) {
	selector, ok := literal.Type.(*goast.SelectorExpr)
	if !ok {
		return ConstructorProduct{}, false, nil
	}
	packageName, ok := selector.X.(*goast.Ident)
	if !ok || packageName.Name != "ast" {
		return ConstructorProduct{}, false, nil
	}
	constructor, known := schema[selector.Sel.Name]
	if !known {
		return ConstructorProduct{}, false, nil
	}
	values := make(map[string]goast.Expr, len(literal.Elts))
	for _, element := range literal.Elts {
		pair, ok := element.(*goast.KeyValueExpr)
		if !ok {
			return ConstructorProduct{}, false, fmt.Errorf("unkeyed parser AST field")
		}
		name, ok := pair.Key.(*goast.Ident)
		if !ok {
			return ConstructorProduct{}, false, fmt.Errorf("nonidentifier parser AST field key")
		}
		if _, duplicate := values[name.Name]; duplicate {
			return ConstructorProduct{}, false, fmt.Errorf("duplicate parser AST field %s", name.Name)
		}
		values[name.Name] = pair.Value
	}
	fields := make([]ProductField, len(constructor.fields))
	for index, name := range constructor.fields {
		value, exists := values[name]
		if !exists {
			fields[index] = ProductField{Field: name, Kind: ActionValueZero}
			continue
		}
		term, err := builder.expression(scope, value)
		if err != nil {
			return ConstructorProduct{}, false, err
		}
		fields[index] = ProductField{Field: name, Kind: ActionValueTerm, Term: term}
		delete(values, name)
	}
	if len(values) != 0 {
		return ConstructorProduct{}, false, fmt.Errorf("parser AST constructor has unknown field")
	}
	return ConstructorProduct{Constructor: selector.Sel.Name, Fields: fields}, true, nil
}

func rootCompositeLiteral(expression goast.Expr) *goast.CompositeLit {
	switch node := expression.(type) {
	case *goast.CompositeLit:
		return node
	case *goast.UnaryExpr:
		if node.Op == token.AND {
			return rootCompositeLiteral(node.X)
		}
	case *goast.ParenExpr:
		return rootCompositeLiteral(node.X)
	}
	return nil
}

func keyedLiteral(literal *goast.CompositeLit) bool {
	if len(literal.Elts) == 0 {
		return false
	}
	for _, element := range literal.Elts {
		if _, ok := element.(*goast.KeyValueExpr); !ok {
			return false
		}
	}
	return true
}

func rootIdentifier(expression goast.Expr) string {
	switch node := expression.(type) {
	case *goast.Ident:
		return node.Name
	case *goast.SelectorExpr:
		return rootIdentifier(node.X)
	case *goast.IndexExpr:
		return rootIdentifier(node.X)
	case *goast.ParenExpr:
		return rootIdentifier(node.X)
	case *goast.TypeAssertExpr:
		return rootIdentifier(node.X)
	case *goast.StarExpr:
		return rootIdentifier(node.X)
	default:
		return ""
	}
}

func aliasIdentifier(expression goast.Expr) string {
	switch node := expression.(type) {
	case *goast.Ident:
		return node.Name
	case *goast.ParenExpr:
		return aliasIdentifier(node.X)
	case *goast.TypeAssertExpr:
		return aliasIdentifier(node.X)
	default:
		return ""
	}
}

func applicationLess(left, right HelperApplication) bool {
	if left.Helper != right.Helper {
		return left.Helper < right.Helper
	}
	if left.Scope != right.Scope {
		return left.Scope < right.Scope
	}
	if comparison := compareGuards(left.Guard, right.Guard); comparison != 0 {
		return comparison < 0
	}
	for index := 0; index < len(left.Actuals) && index < len(right.Actuals); index++ {
		if left.Actuals[index] != right.Actuals[index] {
			return left.Actuals[index] < right.Actuals[index]
		}
	}
	if len(left.Actuals) != len(right.Actuals) {
		return len(left.Actuals) < len(right.Actuals)
	}
	for index := 0; index < len(left.Results) && index < len(right.Results); index++ {
		if placeLess(left.Results[index], right.Results[index]) {
			return true
		}
		if placeLess(right.Results[index], left.Results[index]) {
			return false
		}
	}
	return len(left.Results) < len(right.Results)
}

func placeLess(left, right Place) bool {
	if left.Scope != right.Scope {
		return left.Scope < right.Scope
	}
	if left.Root != right.Root {
		return left.Root < right.Root
	}
	if left.Slot != right.Slot {
		return left.Slot < right.Slot
	}
	if left.StepStart != right.StepStart {
		return left.StepStart < right.StepStart
	}
	return left.StepCount < right.StepCount
}

func editLess(left, right Edit) bool {
	if comparison := compareGuards(left.Guard, right.Guard); comparison != 0 {
		return comparison < 0
	}
	if placeLess(left.Place, right.Place) {
		return true
	}
	if placeLess(right.Place, left.Place) {
		return false
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.Value < right.Value
}

func compareGuards(left, right Guard) int {
	for index := 0; index < len(left.Atoms) && index < len(right.Atoms); index++ {
		first, second := left.Atoms[index], right.Atoms[index]
		if first == second {
			continue
		}
		if guardAtomLess(first, second) {
			return -1
		}
		return 1
	}
	if len(left.Atoms) < len(right.Atoms) {
		return -1
	}
	if len(left.Atoms) > len(right.Atoms) {
		return 1
	}
	return 0
}

func helperFormalNames(function *goast.FuncDecl) ([]string, error) {
	if function == nil || function.Type.Params == nil {
		return nil, nil
	}
	var result []string
	for _, parameter := range function.Type.Params.List {
		if len(parameter.Names) == 0 {
			return nil, fmt.Errorf("helper has unnamed formal")
		}
		for _, name := range parameter.Names {
			if name == nil || name.Name == "" {
				return nil, fmt.Errorf("helper has unnamed formal")
			}
			result = append(result, name.Name)
		}
	}
	return result, nil
}

func helperReturnArity(function *goast.FuncDecl) int {
	maximum := 0
	goast.Inspect(function.Body, func(node goast.Node) bool {
		if returned, ok := node.(*goast.ReturnStmt); ok && len(returned.Results) > maximum {
			maximum = len(returned.Results)
		}
		return true
	})
	return maximum
}

func allReturns(block *goast.BlockStmt) []*goast.ReturnStmt {
	var result []*goast.ReturnStmt
	goast.Inspect(block, func(node goast.Node) bool {
		if returned, ok := node.(*goast.ReturnStmt); ok {
			result = append(result, returned)
		}
		return true
	})
	return result
}

func firstReturn(block *goast.BlockStmt) *goast.ReturnStmt {
	rows := allReturns(block)
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func firstReturnStatements(statements []goast.Stmt) *goast.ReturnStmt {
	for _, statement := range statements {
		if returned, ok := statement.(*goast.ReturnStmt); ok {
			return returned
		}
	}
	return nil
}

func topLevelIf(block *goast.BlockStmt) (*goast.IfStmt, int) {
	for index, statement := range block.List {
		if row, ok := statement.(*goast.IfStmt); ok {
			return row, index
		}
	}
	return nil, -1
}

func tokenErrorIn(node goast.Node) bool {
	found := false
	goast.Inspect(node, func(item goast.Node) bool {
		call, ok := item.(*goast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*goast.SelectorExpr)
		if ok && selector.Sel.Name == "TokenError" {
			found = true
			return false
		}
		return true
	})
	return found
}

func mergeGuards(left, right Guard) (Guard, error) {
	atoms := append(append([]GuardAtom(nil), left.Atoms...), right.Atoms...)
	sort.Slice(atoms, func(first, second int) bool { return guardAtomLess(atoms[first], atoms[second]) })
	for index := 1; index < len(atoms); index++ {
		if !guardAtomLess(atoms[index-1], atoms[index]) {
			return Guard{}, fmt.Errorf("duplicate guard atom")
		}
	}
	return Guard{Atoms: atoms}, nil
}

func negatedGuard(guard Guard) (Guard, error) {
	if len(guard.Atoms) != 1 {
		return Guard{}, fmt.Errorf("only one-atom guard can be negated directly")
	}
	atom := guard.Atoms[0]
	atom.Negated = !atom.Negated
	return Guard{Atoms: []GuardAtom{atom}}, nil
}

// deriveTypedRelations consumes grammarproof's ephemeral syntax visitors and
// emits all parser action rows into one ActionTerms arena.
func deriveTypedRelations(root string, schema grammar.Schema) ([]ProductLaw, []HelperLaw, []SequenceLaw, []FieldMutation, ActionTerms, error) {
	templates, err := grammarproof.HelperTemplates(root)
	if err != nil {
		return nil, nil, nil, nil, ActionTerms{}, err
	}
	if len(templates) != 19 {
		return nil, nil, nil, nil, ActionTerms{}, fmt.Errorf("parser products: helper denominator = %d, want 19", len(templates))
	}
	helperNames := make(map[string]bool, len(templates))
	for _, template := range templates {
		if helperNames[template.Name] {
			return nil, nil, nil, nil, ActionTerms{}, fmt.Errorf("parser products: duplicate helper %s", template.Name)
		}
		helperNames[template.Name] = true
	}
	builder := newActionTermBuilder(helperNames)
	helperSyntax := make(map[string]*goast.FuncDecl, len(templates))
	helperLaws := make([]HelperLaw, 0, len(templates))
	semanticHelpers := 0
	metadataHelpers := 0
	diagnosticHelpers := 0
	acceptedReturns := 0
	rejectRows := 0
	if err := grammarproof.VisitHelperSyntax(root, func(template grammarproof.HelperTemplate, function *goast.FuncDecl) error {
		helperSyntax[template.Name] = function
		formals, formErr := helperFormalNames(function)
		if formErr != nil {
			return fmt.Errorf("helper %s: %w", template.Name, formErr)
		}
		scope := builder.scope(ActionScopeHelper, template.Name, 0, uint16(helperReturnArity(function)), formals)
		law := HelperLaw{Scope: scope.id, Digest: template.Digest, Disposition: HelperDispositionSemantic}
		if helperMetadataOnly(template.Name) {
			law.Disposition = HelperDispositionMetadata
			metadataHelpers++
			builder.closeScope(scope)
			helperLaws = append(helperLaws, law)
			return nil
		}
		if helperDiagnosticOnly(template.Name) {
			law.Disposition = HelperDispositionDiagnostic
			diagnosticHelpers++
			builder.closeScope(scope)
			helperLaws = append(helperLaws, law)
			return nil
		}
		semanticHelpers++
		analyzer := newTypedAnalyzer(builder, &scope, schema, helperNames, helperParameterForms(function))
		analyzer.suppressRanges = helperMapOnly(template.Name)
		if indexErr := analyzer.index(function.Body); indexErr != nil {
			return fmt.Errorf("helper %s index: %w", template.Name, indexErr)
		}
		if returnErr := deriveHelperReturns(template.Name, function, analyzer); returnErr != nil {
			return fmt.Errorf("helper %s control: %w", template.Name, returnErr)
		}
		summary, summaryErr := deriveHelperSummary(template.Name, function, builder, &scope)
		if summaryErr != nil {
			return fmt.Errorf("helper %s summary: %w", template.Name, summaryErr)
		}
		if canonicalErr := analyzer.canonicalize(); canonicalErr != nil {
			return canonicalErr
		}
		law.Returns, law.Rejects, law.Products, law.Helpers, law.Edits, law.Summary = analyzer.relation.returns, analyzer.relation.rejects, analyzer.relation.products, analyzer.relation.helpers, analyzer.relation.edits, summary
		acceptedReturns += len(law.Returns)
		rejectRows += len(law.Rejects)
		builder.closeScope(scope)
		helperLaws = append(helperLaws, law)
		return nil
	}); err != nil {
		return nil, nil, nil, nil, ActionTerms{}, err
	}
	if semanticHelpers != 15 || metadataHelpers != 3 || diagnosticHelpers != 1 {
		return nil, nil, nil, nil, ActionTerms{}, fmt.Errorf("parser products: helper dispositions semantic=%d metadata=%d diagnostic=%d, want 15/3/1", semanticHelpers, metadataHelpers, diagnosticHelpers)
	}
	if acceptedReturns != 18 || rejectRows != 5 {
		return nil, nil, nil, nil, ActionTerms{}, fmt.Errorf("parser products: helper control rows returns=%d rejects=%d, want 18/5", acceptedReturns, rejectRows)
	}
	sort.Slice(helperLaws, func(left, right int) bool { return helperLaws[left].Scope < helperLaws[right].Scope })

	carriers, err := grammarproof.SequenceCarriers(root)
	if err != nil {
		return nil, nil, nil, nil, ActionTerms{}, err
	}
	byTag := make(map[string][]grammarproof.SequenceCarrier)
	for _, carrier := range carriers {
		byTag[carrier.Tag] = append(byTag[carrier.Tag], carrier)
	}
	var productLaws []ProductLaw
	var sequences []SequenceLaw
	var mutations []FieldMutation
	if err := grammarproof.VisitActionSyntax(root, func(template grammarproof.ActionTemplate, block *goast.BlockStmt) error {
		scope := builder.scope(ActionScopeProduction, template.Key, uint16(len(template.RHS)), 1, nil)
		analyzer := newTypedAnalyzer(builder, &scope, schema, helperNames, make(map[string]string))
		handled, controlErr := deriveControlledAction(template, block, analyzer)
		if controlErr != nil {
			return fmt.Errorf("%s control: %w", template.Key, controlErr)
		}
		if !handled {
			if hasActionFlow(block) {
				return fmt.Errorf("%s has unmodeled action control", template.Key)
			}
			if indexErr := analyzer.indexActionTopLevel(block); indexErr != nil {
				return fmt.Errorf("%s index: %w", template.Key, indexErr)
			}
			analyzer.visitName("Result")
		}
		if analyzer.err != nil {
			return fmt.Errorf("%s relation: %w", template.Key, analyzer.err)
		}
		if canonicalErr := analyzer.canonicalize(); canonicalErr != nil {
			return canonicalErr
		}
		form, forward := classifyActionForm(block, analyzer.relation.products)
		if form == ActionFormInvalid {
			return fmt.Errorf("%s has unsupported action shape", template.Key)
		}
		law := ProductLaw{Production: template.Key, Nonterminal: template.Nonterminal, RHS: append([]string(nil), template.RHS...), ActionDigest: template.ActionDigest, Scope: scope.id, Form: form, Forward: forward, Products: analyzer.relation.products, Helpers: analyzer.relation.helpers, Edits: analyzer.relation.edits, Rejects: analyzer.relation.rejects, Chains: analyzer.relation.chains}
		productLaws = append(productLaws, law)
		for _, edit := range analyzer.relation.mutations {
			mutations = append(mutations, FieldMutation{Production: template.Key, Edit: edit})
		}
		if rows, sequenceErr := deriveActionSequences(template, block, builder, &scope, byTag[template.ResultTag], helperSyntax); sequenceErr != nil {
			return fmt.Errorf("%s sequences: %w", template.Key, sequenceErr)
		} else {
			sequences = append(sequences, rows...)
		}
		builder.closeScope(scope)
		return nil
	}); err != nil {
		return nil, nil, nil, nil, ActionTerms{}, err
	}
	sortProductLaws(productLaws)
	if err := validateProductLawOrder(productLaws); err != nil {
		return nil, nil, nil, nil, ActionTerms{}, err
	}
	sort.Slice(sequences, func(left, right int) bool { return sequenceLess(sequences[left], sequences[right]) })
	sort.Slice(mutations, func(left, right int) bool {
		if mutations[left].Production != mutations[right].Production {
			return mutations[left].Production < mutations[right].Production
		}
		return editLess(mutations[left].Edit, mutations[right].Edit)
	})
	remap := builder.finalize()
	remapActionSymbols(productLaws, helperLaws, mutations, remap)
	if err := builder.terms.Validate(); err != nil {
		return nil, nil, nil, nil, ActionTerms{}, err
	}
	return productLaws, helperLaws, sequences, mutations, builder.terms, nil
}

func helperMetadataOnly(name string) bool {
	return name == "positionAtEnd" || name == "annotationToken" || name == "TokenName"
}
func helperDiagnosticOnly(name string) bool { return name == "annotationError" }
func helperMapOnly(name string) bool {
	return name == "splitNameList" || name == "splitTypedNames" || name == "toFuncParams"
}

func classifyActionForm(block *goast.BlockStmt, products []ConstructorProduct) (ActionForm, int) {
	if len(products) != 0 {
		return ActionFormDirectConstruct, 0
	}
	if len(block.List) != 1 {
		return ActionFormAssembly, 0
	}
	assignment, ok := block.List[0].(*goast.AssignStmt)
	if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return ActionFormAssembly, 0
	}
	left, ok := assignment.Lhs[0].(*goast.Ident)
	if !ok || left.Name != "Result" {
		return ActionFormAssembly, 0
	}
	right, ok := assignment.Rhs[0].(*goast.Ident)
	if !ok {
		return ActionFormAssembly, 0
	}
	if len(right.Name) < 4 || right.Name[:3] != "Arg" {
		return ActionFormAssembly, 0
	}
	index, err := strconv.Atoi(right.Name[3:])
	if err != nil || index <= 0 {
		return ActionFormAssembly, 0
	}
	return ActionFormForward, index
}

func remapActionSymbols(products []ProductLaw, helpers []HelperLaw, mutations []FieldMutation, remap []ActionSymbolID) {
	remapID := func(id ActionSymbolID) ActionSymbolID {
		if id == 0 {
			return 0
		}
		return remap[id]
	}
	remappedGuards := make(map[*GuardAtom]bool)
	remapGuard := func(guard *Guard) {
		if len(guard.Atoms) == 0 {
			return
		}
		key := &guard.Atoms[0]
		if remappedGuards[key] {
			return
		}
		remappedGuards[key] = true
		for index := range guard.Atoms {
			if guard.Atoms[index].Constant != 0 {
				guard.Atoms[index].Constant = remapID(guard.Atoms[index].Constant)
			}
		}
	}
	remapApplications := func(rows []HelperApplication) {
		for index := range rows {
			rows[index].Helper = remapID(rows[index].Helper)
			remapGuard(&rows[index].Guard)
		}
	}
	remapProducts := func(rows []ConstructorProduct) {
		for index := range rows {
			remapGuard(&rows[index].Guard)
		}
	}
	remapEdits := func(rows []Edit) {
		for index := range rows {
			remapGuard(&rows[index].Guard)
		}
	}
	remapRejects := func(rows []Reject) {
		for index := range rows {
			remapGuard(&rows[index].Guard)
			rows[index].Diagnostic = remapID(rows[index].Diagnostic)
		}
	}
	remapChains := func(rows []ChainLaw) {
		for index := range rows {
			remapGuard(&rows[index].Guard)
			rows[index].LinkField = remapID(rows[index].LinkField)
		}
	}
	for index := range products {
		remapProducts(products[index].Products)
		remapApplications(products[index].Helpers)
		remapEdits(products[index].Edits)
		remapRejects(products[index].Rejects)
		remapChains(products[index].Chains)
	}
	for index := range helpers {
		remapProducts(helpers[index].Products)
		remapApplications(helpers[index].Helpers)
		remapEdits(helpers[index].Edits)
		for returned := range helpers[index].Returns {
			remapGuard(&helpers[index].Returns[returned].Guard)
		}
		remapRejects(helpers[index].Rejects)
		for mapIndex := range helpers[index].Summary.Presence {
			helpers[index].Summary.Presence[mapIndex].ItemField = remapID(helpers[index].Summary.Presence[mapIndex].ItemField)
		}
	}
	for index := range mutations {
		remapGuard(&mutations[index].Edit.Guard)
	}
}
