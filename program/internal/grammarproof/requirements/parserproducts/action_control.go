package parserproducts

import (
	"fmt"
	goast "go/ast"
	"sort"

	"github.com/wippyai/go-lua/program/internal/grammarproof"
)

// deriveControlledAction owns the parser actions whose semantic relation
// depends on a branch, range, or parser diagnostic. Generic action indexing
// intentionally never descends into those paths: each case below names the
// finite source shape and emits the corresponding guarded rows.
func deriveControlledAction(template grammarproof.ActionTemplate, block *goast.BlockStmt, analyzer *typedAnalyzer) (bool, error) {
	switch template.Key {
	case "chunk#1", "chunk#2", "chunk#3":
		return true, deriveLexerStatementSnapshotControl(block, analyzer)
	case "stat#2":
		return true, deriveFuncCallStatementControl(block, analyzer)
	case "stat#6":
		return true, deriveIfChainControl(block, analyzer, false)
	case "stat#7":
		return true, deriveIfChainControl(block, analyzer, true)
	case "prefixexp#4":
		return true, deriveCommaAdjustmentControl(block, analyzer)
	case "typeexpr#2":
		return true, deriveTypeFoldControl(block, analyzer, "UnionTypeExpr")
	case "typeexpr#3":
		return true, deriveTypeFoldControl(block, analyzer, "IntersectionTypeExpr")
	case "primarytypeexpr#5":
		return true, deriveNumberLiteralControl(block, analyzer)
	case "interfacemethod#1", "interfacemethod#2":
		return true, deriveInterfaceMethodControl(block, analyzer)
	case "stat#17", "stat#18":
		return true, deriveIdentifierDiagnosticControl(block, analyzer)
	case "args#1", "args#2":
		return true, deriveNewlineDiagnosticControl(block, analyzer)
	default:
		return false, nil
	}
}

func prepareControlledAction(block *goast.BlockStmt, analyzer *typedAnalyzer) error {
	if err := bindBlockLocals(analyzer.scope, block); err != nil {
		return err
	}
	for name, form := range assertedLocalFormsBlock(block) {
		analyzer.forms[name] = form
	}
	return nil
}

func (a *typedAnalyzer) visitGuarded(guard Guard, expression goast.Expr) error {
	prior := a.guard
	a.guard = guard
	a.visit(expression)
	a.guard = prior
	return a.err
}

type actionTypeAssertion struct {
	subject         goast.Expr
	typeExpression  goast.Expr
	valueName       string
	successWhenTrue bool
}

func actionIfAssertion(branch *goast.IfStmt) (actionTypeAssertion, error) {
	if branch == nil {
		return actionTypeAssertion{}, fmt.Errorf("missing type assertion branch")
	}
	assignment, ok := branch.Init.(*goast.AssignStmt)
	if !ok || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
		return actionTypeAssertion{}, fmt.Errorf("control branch has malformed type assertion")
	}
	value, ok := assignment.Lhs[0].(*goast.Ident)
	if !ok || value.Name == "" {
		return actionTypeAssertion{}, fmt.Errorf("control branch has unnamed asserted value")
	}
	okName, ok := assignment.Lhs[1].(*goast.Ident)
	if !ok || okName.Name == "" {
		return actionTypeAssertion{}, fmt.Errorf("control branch has unnamed assertion predicate")
	}
	assertion, ok := assignment.Rhs[0].(*goast.TypeAssertExpr)
	if !ok || assertion.Type == nil {
		return actionTypeAssertion{}, fmt.Errorf("control branch initializer is not a typed assertion")
	}
	positive, conditionErr := assertionCondition(branch.Cond, okName.Name)
	if conditionErr != nil {
		return actionTypeAssertion{}, conditionErr
	}
	return actionTypeAssertion{
		subject:         assertion.X,
		typeExpression:  assertion.Type,
		valueName:       value.Name,
		successWhenTrue: positive,
	}, nil
}

func assertionCondition(expression goast.Expr, name string) (bool, error) {
	if identifier, ok := expression.(*goast.Ident); ok && identifier.Name == name {
		return true, nil
	}
	if unary, ok := expression.(*goast.UnaryExpr); ok {
		if identifier, ok := unary.X.(*goast.Ident); ok && identifier.Name == name {
			return false, nil
		}
	}
	return false, fmt.Errorf("control branch does not test its assertion predicate")
}

func directResultExpression(block *goast.BlockStmt) (goast.Expr, error) {
	var result goast.Expr
	for _, statement := range block.List {
		assignment, ok := statement.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		name, ok := assignment.Lhs[0].(*goast.Ident)
		if !ok || name.Name != "Result" {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("control block has multiple Result assignments")
		}
		result = assignment.Rhs[0]
	}
	if result == nil {
		return nil, fmt.Errorf("control block lacks a Result assignment")
	}
	return result, nil
}

func directIf(block *goast.BlockStmt) (*goast.IfStmt, int, error) {
	var branch *goast.IfStmt
	index := -1
	for statementIndex, statement := range block.List {
		item, ok := statement.(*goast.IfStmt)
		if !ok {
			continue
		}
		if branch != nil {
			return nil, -1, fmt.Errorf("action has multiple top-level if branches")
		}
		branch, index = item, statementIndex
	}
	if branch == nil {
		return nil, -1, fmt.Errorf("action lacks a top-level if branch")
	}
	return branch, index, nil
}

func deriveFuncCallStatementControl(block *goast.BlockStmt, analyzer *typedAnalyzer) error {
	if err := prepareControlledAction(block, analyzer); err != nil {
		return err
	}
	branch, _, err := directIf(block)
	if err != nil {
		return err
	}
	assertion, err := actionIfAssertion(branch)
	if err != nil {
		return err
	}
	if assertion.valueName != "_" || assertion.successWhenTrue || branch.Else == nil {
		return fmt.Errorf("function-call statement control has unsupported branch polarity")
	}
	success, err := analyzer.builder.typeGuard(analyzer.scope, assertion.subject, []goast.Expr{assertion.typeExpression}, false)
	if err != nil {
		return err
	}
	failure, err := negatedGuard(success)
	if err != nil {
		return err
	}
	elseBlock, ok := branch.Else.(*goast.BlockStmt)
	if !ok {
		return fmt.Errorf("function-call statement control lacks an else block")
	}
	result, err := directResultExpression(elseBlock)
	if err != nil {
		return err
	}
	if err := analyzer.visitGuarded(success, result); err != nil {
		return err
	}
	analyzer.relation.rejects = append(analyzer.relation.rejects, Reject{
		Ordinal:    1,
		Condition:  RejectWhenAll,
		Guard:      failure,
		Diagnostic: analyzer.builder.symbol(ActionSymbolDiagnostic, "Lexer.Error"),
	})
	return nil
}

func deriveIfChainControl(block *goast.BlockStmt, analyzer *typedAnalyzer, withElse bool) error {
	if err := prepareControlledAction(block, analyzer); err != nil {
		return err
	}
	seedExpression, err := directResultExpression(block)
	if err != nil {
		return err
	}
	if err := analyzer.visitGuarded(Guard{}, seedExpression); err != nil {
		return err
	}
	seed, err := analyzer.builder.expression(analyzer.scope, seedExpression)
	if err != nil {
		return err
	}
	rangeIndex, loop, err := directRange(block)
	if err != nil {
		return err
	}
	input, err := analyzer.builder.expression(analyzer.scope, loop.X)
	if err != nil {
		return err
	}
	receiver, linkField, err := chainLink(loop)
	if err != nil {
		return err
	}
	if linkField != "Else" {
		return fmt.Errorf("if chain uses %q instead of Else", linkField)
	}
	tailStart := uint32(len(analyzer.builder.terms.ChainTails))
	if withElse {
		tails, tailErr := chainTails(block.List[rangeIndex+1:], analyzer, receiver)
		if tailErr != nil {
			return tailErr
		}
		analyzer.builder.terms.ChainTails = append(analyzer.builder.terms.ChainTails, tails...)
	} else if len(chainAssignments(block.List[rangeIndex+1:], receiver)) != 0 {
		return fmt.Errorf("if chain without else has final-node assignments")
	}
	tailCount := uint16(len(analyzer.builder.terms.ChainTails) - int(tailStart))
	analyzer.relation.chains = append(analyzer.relation.chains, ChainLaw{
		Scope:     analyzer.scope.id,
		Input:     input,
		Seed:      seed,
		LinkField: analyzer.builder.symbol(ActionSymbolField, linkField),
		TailStart: tailStart,
		TailCount: tailCount,
	})
	return nil
}

func directRange(block *goast.BlockStmt) (int, *goast.RangeStmt, error) {
	var loop *goast.RangeStmt
	index := -1
	for statementIndex, statement := range block.List {
		item, ok := statement.(*goast.RangeStmt)
		if !ok {
			continue
		}
		if loop != nil {
			return -1, nil, fmt.Errorf("action has multiple top-level ranges")
		}
		loop, index = item, statementIndex
	}
	if loop == nil {
		return -1, nil, fmt.Errorf("action lacks a top-level range")
	}
	return index, loop, nil
}

func chainLink(loop *goast.RangeStmt) (string, string, error) {
	item, ok := loop.Value.(*goast.Ident)
	if !ok || item.Name == "" {
		return "", "", fmt.Errorf("chain range has no item binding")
	}
	var receiver, field string
	advanced := false
	for _, statement := range loop.Body.List {
		assignment, ok := statement.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		if name, ok := assignment.Lhs[0].(*goast.Ident); ok && name.Name != "" {
			value, valueOK := assignment.Rhs[0].(*goast.Ident)
			if name.Name != receiver && receiver != "" {
				continue
			}
			if valueOK && value.Name == item.Name {
				advanced = true
			}
			continue
		}
		name, selector, selected := chainSelector(assignment.Lhs[0])
		if !selected {
			continue
		}
		if receiver != "" {
			return "", "", fmt.Errorf("chain range has multiple link writes")
		}
		if !singletonSequenceIdentifier(assignment.Rhs[0], item.Name) {
			return "", "", fmt.Errorf("chain link does not install the range item")
		}
		receiver, field = name, selector
	}
	if receiver == "" || !advanced {
		return "", "", fmt.Errorf("chain range lacks an ordered link and advance")
	}
	return receiver, field, nil
}

func chainSelector(expression goast.Expr) (string, string, bool) {
	selector, ok := expression.(*goast.SelectorExpr)
	if !ok || selector.Sel == nil || selector.Sel.Name == "" {
		return "", "", false
	}
	assertion, ok := selector.X.(*goast.TypeAssertExpr)
	if !ok {
		return "", "", false
	}
	name, ok := assertion.X.(*goast.Ident)
	if !ok || name.Name == "" {
		return "", "", false
	}
	return name.Name, selector.Sel.Name, true
}

func singletonSequenceIdentifier(expression goast.Expr, name string) bool {
	literal, ok := expression.(*goast.CompositeLit)
	if !ok || len(literal.Elts) != 1 {
		return false
	}
	if _, ok := literal.Type.(*goast.ArrayType); !ok {
		return false
	}
	identifier, ok := literal.Elts[0].(*goast.Ident)
	return ok && identifier.Name == name
}

func chainAssignments(statements []goast.Stmt, receiver string) []*goast.AssignStmt {
	var result []*goast.AssignStmt
	for _, statement := range statements {
		assignment, ok := statement.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		name, _, selected := chainSelector(assignment.Lhs[0])
		if selected && name == receiver {
			result = append(result, assignment)
		}
	}
	return result
}

func chainTails(statements []goast.Stmt, analyzer *typedAnalyzer, receiver string) ([]ChainTail, error) {
	assignments := chainAssignments(statements, receiver)
	if len(assignments) != 2 {
		return nil, fmt.Errorf("if-else chain has %d final-node assignments, want two", len(assignments))
	}
	tails := make([]ChainTail, 0, len(assignments))
	for _, assignment := range assignments {
		_, field, ok := chainSelector(assignment.Lhs[0])
		if !ok {
			return nil, fmt.Errorf("malformed chain tail")
		}
		value, err := analyzer.builder.expression(analyzer.scope, assignment.Rhs[0])
		if err != nil {
			return nil, err
		}
		tails = append(tails, ChainTail{
			Field: analyzer.builder.symbol(ActionSymbolField, field),
			Value: value,
		})
	}
	sort.Slice(tails, func(left, right int) bool {
		return analyzer.builder.symbolValue(tails[left].Field).Text < analyzer.builder.symbolValue(tails[right].Field).Text
	})
	if tails[0].Field == tails[1].Field || analyzer.builder.symbolValue(tails[0].Field).Text != "Else" || analyzer.builder.symbolValue(tails[1].Field).Text != "HasElse" {
		return nil, fmt.Errorf("if-else chain tails are not Else and HasElse")
	}
	return tails, nil
}

func deriveCommaAdjustmentControl(block *goast.BlockStmt, analyzer *typedAnalyzer) error {
	if err := prepareControlledAction(block, analyzer); err != nil {
		return err
	}
	branch, _, err := directIf(block)
	if err != nil {
		return err
	}
	assertion, err := actionIfAssertion(branch)
	if err != nil {
		return err
	}
	if !assertion.successWhenTrue || assertion.valueName == "_" || branch.Else != nil {
		return fmt.Errorf("comma adjustment has unsupported branch polarity")
	}
	guard, err := analyzer.builder.typeGuard(analyzer.scope, assertion.subject, []goast.Expr{assertion.typeExpression}, false)
	if err != nil {
		return err
	}
	target, value, err := directFieldAssignment(branch.Body, assertion.valueName, "AdjustRet")
	if err != nil {
		return err
	}
	place, err := analyzer.builder.place(analyzer.scope, target)
	if err != nil {
		return err
	}
	term, err := analyzer.builder.expression(analyzer.scope, value)
	if err != nil {
		return err
	}
	analyzer.relation.mutations = append(analyzer.relation.mutations, Edit{Kind: EditAssign, Guard: guard, Place: place, Value: term})
	return nil
}

func directFieldAssignment(block *goast.BlockStmt, receiver, field string) (goast.Expr, goast.Expr, error) {
	var target, value goast.Expr
	for _, statement := range block.List {
		assignment, ok := statement.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		selector, ok := assignment.Lhs[0].(*goast.SelectorExpr)
		if !ok || selector.Sel == nil || selector.Sel.Name != field {
			continue
		}
		name, ok := selector.X.(*goast.Ident)
		if !ok || name.Name != receiver {
			continue
		}
		if target != nil {
			return nil, nil, fmt.Errorf("multiple writes to %s.%s", receiver, field)
		}
		target, value = assignment.Lhs[0], assignment.Rhs[0]
	}
	if target == nil {
		return nil, nil, fmt.Errorf("missing write to %s.%s", receiver, field)
	}
	return target, value, nil
}

func deriveTypeFoldControl(block *goast.BlockStmt, analyzer *typedAnalyzer, form string) error {
	if err := prepareControlledAction(block, analyzer); err != nil {
		return err
	}
	branch, _, err := directIf(block)
	if err != nil {
		return err
	}
	assertion, err := actionIfAssertion(branch)
	if err != nil {
		return err
	}
	if !assertion.successWhenTrue || assertion.valueName == "_" {
		return fmt.Errorf("type fold has unsupported branch polarity")
	}
	guard, err := analyzer.builder.typeGuard(analyzer.scope, assertion.subject, []goast.Expr{assertion.typeExpression}, false)
	if err != nil {
		return err
	}
	target, value, err := directFieldAssignment(branch.Body, assertion.valueName, "Types")
	if err != nil {
		return err
	}
	place, err := analyzer.builder.place(analyzer.scope, target)
	if err != nil {
		return err
	}
	term, err := analyzer.builder.expression(analyzer.scope, value)
	if err != nil {
		return err
	}
	analyzer.relation.mutations = append(analyzer.relation.mutations, Edit{Kind: EditAppend, Guard: guard, Place: place, Value: term})
	fallback, err := negatedGuard(guard)
	if err != nil {
		return err
	}
	elseBlock, ok := branch.Else.(*goast.BlockStmt)
	if !ok {
		return fmt.Errorf("type fold lacks an else construction")
	}
	result, err := directResultExpression(elseBlock)
	if err != nil {
		return err
	}
	if err := analyzer.visitGuarded(fallback, result); err != nil {
		return err
	}
	if len(analyzer.relation.products) != 1 || analyzer.relation.products[0].Constructor != form {
		return fmt.Errorf("type fold does not construct ast.%s in its fallback", form)
	}
	return nil
}

type numberParseBranch struct {
	value string
	input goast.Expr
}

func deriveNumberLiteralControl(block *goast.BlockStmt, analyzer *typedAnalyzer) error {
	if err := prepareControlledAction(block, analyzer); err != nil {
		return err
	}
	integerBranch, _, err := directIf(block)
	if err != nil {
		return err
	}
	integer, err := numberParseIf(integerBranch, "ParseIntegerLiteral")
	if err != nil {
		return err
	}
	floatBranch, ok := integerBranch.Else.(*goast.IfStmt)
	if !ok {
		return fmt.Errorf("number literal control lacks float branch")
	}
	floating, err := numberParseIf(floatBranch, "ParseFloatLiteral")
	if err != nil {
		return err
	}
	invalidBlock, ok := floatBranch.Else.(*goast.BlockStmt)
	if !ok {
		return fmt.Errorf("number literal control lacks invalid branch")
	}
	integerGuard, err := analyzer.builder.numberParseGuard(analyzer.scope, integer.input, NumberParseClassInteger)
	if err != nil {
		return err
	}
	floatGuard, err := analyzer.builder.numberParseGuard(analyzer.scope, floating.input, NumberParseClassFloat)
	if err != nil {
		return err
	}
	invalidGuard, err := analyzer.builder.numberParseGuard(analyzer.scope, integer.input, NumberParseClassInvalid)
	if err != nil {
		return err
	}
	integerResult, err := directResultExpression(integerBranch.Body)
	if err != nil {
		return err
	}
	floatResult, err := directResultExpression(floatBranch.Body)
	if err != nil {
		return err
	}
	invalidResult, err := directResultExpression(invalidBlock)
	if err != nil {
		return err
	}
	if err := analyzer.visitGuarded(integerGuard, integerResult); err != nil {
		return err
	}
	if err := analyzer.visitGuarded(floatGuard, floatResult); err != nil {
		return err
	}
	if err := analyzer.visitGuarded(invalidGuard, invalidResult); err != nil {
		return err
	}
	return nil
}

func numberParseIf(branch *goast.IfStmt, function string) (numberParseBranch, error) {
	if branch == nil {
		return numberParseBranch{}, fmt.Errorf("missing %s branch", function)
	}
	assignment, ok := branch.Init.(*goast.AssignStmt)
	if !ok || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
		return numberParseBranch{}, fmt.Errorf("%s branch has malformed initializer", function)
	}
	value, ok := assignment.Lhs[0].(*goast.Ident)
	if !ok || value.Name == "" {
		return numberParseBranch{}, fmt.Errorf("%s branch has unnamed parsed value", function)
	}
	okName, ok := assignment.Lhs[1].(*goast.Ident)
	if !ok || okName.Name == "" {
		return numberParseBranch{}, fmt.Errorf("%s branch has unnamed success predicate", function)
	}
	if positive, err := assertionCondition(branch.Cond, okName.Name); err != nil || !positive {
		return numberParseBranch{}, fmt.Errorf("%s branch does not use a positive success predicate", function)
	}
	call, ok := assignment.Rhs[0].(*goast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return numberParseBranch{}, fmt.Errorf("%s branch has malformed parser call", function)
	}
	selector, ok := call.Fun.(*goast.SelectorExpr)
	if !ok || selector.Sel == nil || selector.Sel.Name != function {
		return numberParseBranch{}, fmt.Errorf("number control does not call numparse.%s", function)
	}
	packageName, ok := selector.X.(*goast.Ident)
	if !ok || packageName.Name != "numparse" {
		return numberParseBranch{}, fmt.Errorf("number control parser callable is not numparse.%s", function)
	}
	return numberParseBranch{value: value.Name, input: call.Args[0]}, nil
}

func deriveInterfaceMethodControl(block *goast.BlockStmt, analyzer *typedAnalyzer) error {
	if err := analyzer.indexActionTopLevel(block); err != nil {
		return err
	}
	analyzer.visitName("Result")
	if analyzer.err != nil {
		return analyzer.err
	}
	branch, err := interfaceReturnNormalization(block)
	if err != nil {
		return err
	}
	guard, err := analyzer.builder.guard(analyzer.scope, branch.Cond)
	if err != nil {
		return err
	}
	assignment, receiver, value, err := singleLocalAssignment(branch.Body)
	if err != nil {
		return err
	}
	if receiver == "" || assignment == nil {
		return fmt.Errorf("interface method control has malformed returns assignment")
	}
	place, err := analyzer.builder.place(analyzer.scope, assignment.Lhs[0])
	if err != nil {
		return err
	}
	term, err := analyzer.builder.expression(analyzer.scope, value)
	if err != nil {
		return err
	}
	analyzer.relation.edits = append(analyzer.relation.edits, Edit{Kind: EditAssign, Guard: guard, Place: place, Value: term})
	return nil
}

func deriveLexerStatementSnapshotControl(block *goast.BlockStmt, analyzer *typedAnalyzer) error {
	if err := analyzer.indexActionTopLevel(block); err != nil {
		return err
	}
	analyzer.visitName("Result")
	if analyzer.err != nil {
		return analyzer.err
	}
	branch, _, err := directIf(block)
	if err != nil {
		return err
	}
	if !isLexerStatementSnapshot(branch) {
		return fmt.Errorf("chunk action has unsupported lexer statement snapshot")
	}
	return nil
}

func isLexerStatementSnapshot(branch *goast.IfStmt) bool {
	assertion, err := actionIfAssertion(branch)
	if err != nil || !assertion.successWhenTrue || assertion.valueName == "_" || branch.Else != nil {
		return false
	}
	for _, statement := range branch.Body.List {
		assignment, ok := statement.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		selector, ok := assignment.Lhs[0].(*goast.SelectorExpr)
		if !ok || selector.Sel == nil || selector.Sel.Name != "Stmts" {
			continue
		}
		receiver, ok := selector.X.(*goast.Ident)
		value, valueOK := assignment.Rhs[0].(*goast.Ident)
		if ok && valueOK && receiver.Name == assertion.valueName && value.Name == "Result" {
			return true
		}
	}
	return false
}

func interfaceReturnNormalization(block *goast.BlockStmt) (*goast.IfStmt, error) {
	var branch *goast.IfStmt
	for _, statement := range block.List {
		candidate, ok := statement.(*goast.IfStmt)
		if !ok || !isKnownNilCondition(candidate.Cond) {
			continue
		}
		if branch != nil {
			return nil, fmt.Errorf("interface method has multiple return-normalization branches")
		}
		branch = candidate
	}
	if branch == nil {
		return nil, fmt.Errorf("interface method lacks known-nil return normalization")
	}
	return branch, nil
}

func isKnownNilCondition(expression goast.Expr) bool {
	binary, ok := expression.(*goast.BinaryExpr)
	if !ok || binary.Op.String() != "&&" {
		return false
	}
	left, leftOK := binary.X.(*goast.SelectorExpr)
	right, rightOK := binary.Y.(*goast.BinaryExpr)
	if !leftOK || !rightOK || left.Sel == nil || left.Sel.Name != "known" || right.Op.String() != "==" {
		return false
	}
	_, localOK := right.X.(*goast.Ident)
	nilValue, nilOK := right.Y.(*goast.Ident)
	return localOK && nilOK && nilValue.Name == "nil"
}

func singleLocalAssignment(block *goast.BlockStmt) (*goast.AssignStmt, string, goast.Expr, error) {
	var result *goast.AssignStmt
	var receiver string
	var value goast.Expr
	for _, statement := range block.List {
		assignment, ok := statement.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		name, ok := assignment.Lhs[0].(*goast.Ident)
		if !ok || name.Name == "" {
			continue
		}
		if result != nil {
			return nil, "", nil, fmt.Errorf("control branch has multiple local assignments")
		}
		result, receiver, value = assignment, name.Name, assignment.Rhs[0]
	}
	if result == nil {
		return nil, "", nil, fmt.Errorf("control branch has no local assignment")
	}
	return result, receiver, value, nil
}

func deriveIdentifierDiagnosticControl(block *goast.BlockStmt, analyzer *typedAnalyzer) error {
	if err := analyzer.indexActionTopLevel(block); err != nil {
		return err
	}
	analyzer.visitName("Result")
	if analyzer.err != nil {
		return analyzer.err
	}
	branch, _, err := directIf(block)
	if err != nil {
		return err
	}
	guard, err := analyzer.builder.guard(analyzer.scope, branch.Cond)
	if err != nil {
		return err
	}
	if !containsLexerDiagnostic(branch.Body, "Error") {
		return fmt.Errorf("identifier diagnostic branch does not call Lexer.Error")
	}
	analyzer.relation.rejects = append(analyzer.relation.rejects, Reject{
		Ordinal:    1,
		Condition:  RejectWhenAll,
		Guard:      guard,
		Diagnostic: analyzer.builder.symbol(ActionSymbolDiagnostic, "Lexer.Error"),
	})
	return nil
}

func deriveNewlineDiagnosticControl(block *goast.BlockStmt, analyzer *typedAnalyzer) error {
	if err := analyzer.indexActionTopLevel(block); err != nil {
		return err
	}
	analyzer.visitName("Result")
	if analyzer.err != nil {
		return analyzer.err
	}
	branch, _, err := directIf(block)
	if err != nil {
		return err
	}
	if !isLexerNewline(branch.Cond) || !containsLexerDiagnostic(branch.Body, "TokenError") {
		return fmt.Errorf("newline diagnostic control has unsupported shape")
	}
	state := analyzer.builder.control(analyzer.scope, "Lexer.PNewLine")
	guard := Guard{Atoms: []GuardAtom{{
		Kind:     GuardAtomEqConst,
		Term:     state,
		Constant: analyzer.builder.symbol(ActionSymbolConstant, "true"),
	}}}
	analyzer.relation.rejects = append(analyzer.relation.rejects, Reject{
		Ordinal:    1,
		Condition:  RejectWhenAll,
		Guard:      guard,
		Diagnostic: analyzer.builder.symbol(ActionSymbolDiagnostic, "Lexer.TokenError"),
	})
	return nil
}

func isLexerNewline(expression goast.Expr) bool {
	selector, ok := expression.(*goast.SelectorExpr)
	if !ok || selector.Sel == nil || selector.Sel.Name != "PNewLine" {
		return false
	}
	assertion, ok := selector.X.(*goast.TypeAssertExpr)
	if !ok {
		return false
	}
	name, ok := assertion.X.(*goast.Ident)
	return ok && name.Name == "yylex"
}

func containsLexerDiagnostic(block *goast.BlockStmt, method string) bool {
	for _, statement := range block.List {
		expression, ok := statement.(*goast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := expression.X.(*goast.CallExpr)
		if !ok {
			continue
		}
		selector, ok := call.Fun.(*goast.SelectorExpr)
		if !ok || selector.Sel == nil || selector.Sel.Name != method {
			continue
		}
		if assertion, ok := selector.X.(*goast.TypeAssertExpr); ok {
			if name, ok := assertion.X.(*goast.Ident); ok && name.Name == "yylex" {
				return true
			}
		}
	}
	return false
}

func hasActionFlow(block *goast.BlockStmt) bool {
	for _, statement := range block.List {
		switch statement.(type) {
		case *goast.IfStmt, *goast.RangeStmt, *goast.ForStmt, *goast.SwitchStmt, *goast.TypeSwitchStmt, *goast.SelectStmt:
			return true
		}
	}
	return false
}
