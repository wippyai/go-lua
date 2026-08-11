package parserproducts

import (
	"fmt"
	goast "go/ast"
)

// deriveHelperReturns admits only the finite control shapes in parser.go.y.
// Every accepted path is a typed return; every TokenError fallback is a
// Reject, never a second successful semantic result.
func deriveHelperReturns(name string, function *goast.FuncDecl, analyzer *typedAnalyzer) error {
	switch name {
	case "parenthesizedType":
		return parenthesizedTypeReturns(function, analyzer)
	case "appendFunctionTypeParam", "setFunctionTypeVariadic":
		return terminalVariadicReturns(function, analyzer)
	case "annotatedType":
		return annotatedTypeReturns(function, analyzer)
	case "annotateDirectType":
		return directAnnotationReturns(function, analyzer)
	case "annotateFieldType":
		return fieldAnnotationReturns(function, analyzer)
	}
	returns := allReturns(function.Body)
	if len(returns) != 1 {
		return fmt.Errorf("unsupported helper return shape")
	}
	return analyzer.addReturn(1, Guard{}, returns[0])
}

func parenthesizedTypeReturns(function *goast.FuncDecl, analyzer *typedAnalyzer) error {
	branch, index := topLevelIf(function.Body)
	if branch == nil || index != 0 || branch.Else != nil || !tokenErrorIn(function.Body) {
		return fmt.Errorf("unsupported parenthesized type control")
	}
	success, err := analyzer.builder.guard(analyzer.scope, branch.Cond)
	if err != nil {
		return err
	}
	if err := analyzer.addReturn(1, success, firstReturn(branch.Body)); err != nil {
		return err
	}
	analyzer.relation.rejects = append(analyzer.relation.rejects, Reject{
		Ordinal: 1, Condition: RejectUnlessAll, Guard: success,
		Diagnostic: analyzer.builder.symbol(ActionSymbolDiagnostic, "Lexer.TokenError"),
	})
	return nil
}

func terminalVariadicReturns(function *goast.FuncDecl, analyzer *typedAnalyzer) error {
	branch, index := topLevelIf(function.Body)
	if branch == nil || index != 0 || branch.Else != nil || !tokenErrorIn(branch.Body) {
		return fmt.Errorf("unsupported variadic helper control")
	}
	failure, err := analyzer.builder.guard(analyzer.scope, branch.Cond)
	if err != nil {
		return err
	}
	success, err := negatedGuard(failure)
	if err != nil {
		return err
	}
	returns := allReturns(function.Body)
	if len(returns) != 1 {
		return fmt.Errorf("variadic helper has nonterminal return")
	}
	if err := analyzer.addReturn(1, success, returns[0]); err != nil {
		return err
	}
	analyzer.relation.rejects = append(analyzer.relation.rejects, Reject{
		Ordinal: 1, Condition: RejectUnlessAll, Guard: success,
		Diagnostic: analyzer.builder.symbol(ActionSymbolDiagnostic, "Lexer.TokenError"),
	})
	return nil
}

func annotatedTypeReturns(function *goast.FuncDecl, analyzer *typedAnalyzer) error {
	branch, index := topLevelIf(function.Body)
	if branch == nil || index != 0 || branch.Else != nil {
		return fmt.Errorf("unsupported annotated type control")
	}
	empty, err := analyzer.builder.guard(analyzer.scope, branch.Cond)
	if err != nil {
		return err
	}
	nonempty, err := negatedGuard(empty)
	if err != nil {
		return err
	}
	if err := analyzer.addReturn(1, empty, firstReturn(branch.Body)); err != nil {
		return err
	}
	returns := allReturns(function.Body)
	if len(returns) != 2 {
		return fmt.Errorf("annotated type has invalid return count")
	}
	return analyzer.addReturn(2, nonempty, returns[1])
}

func directAnnotationReturns(function *goast.FuncDecl, analyzer *typedAnalyzer) error {
	branch, index := topLevelTypeSwitch(function.Body)
	if branch == nil || index != 0 {
		return fmt.Errorf("unsupported direct annotation control")
	}
	subject, acceptedTypes, acceptedCase, defaultCase, err := annotationSwitch(branch)
	if err != nil {
		return err
	}
	accepted, err := analyzer.builder.typeGuard(analyzer.scope, subject, acceptedTypes, false)
	if err != nil {
		return err
	}
	notAccepted, err := negatedGuard(accepted)
	if err != nil {
		return err
	}
	if err := analyzer.addReturn(1, accepted, firstReturnStatements(acceptedCase.Body)); err != nil {
		return err
	}
	failureIf, finalReturn, err := defaultAnnotationTail(defaultCase)
	if err != nil {
		return err
	}
	nonempty, err := analyzer.builder.guard(analyzer.scope, failureIf.Cond)
	if err != nil {
		return err
	}
	empty, err := negatedGuard(nonempty)
	if err != nil {
		return err
	}
	acceptedDefault, err := mergeGuards(notAccepted, empty)
	if err != nil {
		return err
	}
	if err := analyzer.addReturn(2, acceptedDefault, finalReturn); err != nil {
		return err
	}
	invalid, err := mergeGuards(notAccepted, nonempty)
	if err != nil {
		return err
	}
	analyzer.relation.rejects = append(analyzer.relation.rejects, Reject{
		Ordinal: 1, Condition: RejectWhenAll, Guard: invalid,
		Diagnostic: analyzer.builder.symbol(ActionSymbolDiagnostic, "annotationError"),
	})
	return nil
}

func fieldAnnotationReturns(function *goast.FuncDecl, analyzer *typedAnalyzer) error {
	branch, index := topLevelIf(function.Body)
	if branch == nil || index != 0 || branch.Else != nil {
		return fmt.Errorf("unsupported field annotation control")
	}
	subject, assertedType, err := typeAssertFromIf(branch)
	if err != nil {
		return err
	}
	accepted, err := analyzer.builder.typeGuard(analyzer.scope, subject, []goast.Expr{assertedType}, false)
	if err != nil {
		return err
	}
	notAccepted, err := negatedGuard(accepted)
	if err != nil {
		return err
	}
	if err := analyzer.addReturn(1, accepted, firstReturn(branch.Body)); err != nil {
		return err
	}
	if index+1 >= len(function.Body.List) {
		return fmt.Errorf("field annotation lacks failure branch")
	}
	failureIf, ok := function.Body.List[index+1].(*goast.IfStmt)
	if !ok || !annotationErrorIn(failureIf.Body) {
		return fmt.Errorf("field annotation lacks annotation error branch")
	}
	nonempty, err := analyzer.builder.guard(analyzer.scope, failureIf.Cond)
	if err != nil {
		return err
	}
	empty, err := negatedGuard(nonempty)
	if err != nil {
		return err
	}
	returns := allReturns(function.Body)
	if len(returns) != 2 {
		return fmt.Errorf("field annotation has invalid return count")
	}
	acceptedDefault, err := mergeGuards(notAccepted, empty)
	if err != nil {
		return err
	}
	if err := analyzer.addReturn(2, acceptedDefault, returns[1]); err != nil {
		return err
	}
	invalid, err := mergeGuards(notAccepted, nonempty)
	if err != nil {
		return err
	}
	analyzer.relation.rejects = append(analyzer.relation.rejects, Reject{
		Ordinal: 1, Condition: RejectWhenAll, Guard: invalid,
		Diagnostic: analyzer.builder.symbol(ActionSymbolDiagnostic, "annotationError"),
	})
	return nil
}

func topLevelTypeSwitch(block *goast.BlockStmt) (*goast.TypeSwitchStmt, int) {
	for index, statement := range block.List {
		if branch, ok := statement.(*goast.TypeSwitchStmt); ok {
			return branch, index
		}
	}
	return nil, -1
}

func annotationSwitch(branch *goast.TypeSwitchStmt) (goast.Expr, []goast.Expr, *goast.CaseClause, *goast.CaseClause, error) {
	subject, err := typeSwitchSubject(branch)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var types []goast.Expr
	var accepted, fallback *goast.CaseClause
	for _, statement := range branch.Body.List {
		item, ok := statement.(*goast.CaseClause)
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("type switch has noncase clause")
		}
		if len(item.List) == 0 {
			if fallback != nil {
				return nil, nil, nil, nil, fmt.Errorf("duplicate type switch default")
			}
			fallback = item
			continue
		}
		if accepted != nil {
			return nil, nil, nil, nil, fmt.Errorf("multiple accepted type switch cases")
		}
		accepted, types = item, append([]goast.Expr(nil), item.List...)
	}
	if accepted == nil || fallback == nil || len(types) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("incomplete annotation type switch")
	}
	return subject, types, accepted, fallback, nil
}

func typeSwitchSubject(branch *goast.TypeSwitchStmt) (goast.Expr, error) {
	switch assignment := branch.Assign.(type) {
	case *goast.ExprStmt:
		assertion, ok := assignment.X.(*goast.TypeAssertExpr)
		if !ok {
			return nil, fmt.Errorf("type switch lacks assertion")
		}
		return assertion.X, nil
	case *goast.AssignStmt:
		if len(assignment.Rhs) != 1 {
			return nil, fmt.Errorf("type switch has invalid assignment")
		}
		assertion, ok := assignment.Rhs[0].(*goast.TypeAssertExpr)
		if !ok {
			return nil, fmt.Errorf("type switch lacks assertion")
		}
		return assertion.X, nil
	default:
		return nil, fmt.Errorf("unsupported type switch assignment")
	}
}

func defaultAnnotationTail(fallback *goast.CaseClause) (*goast.IfStmt, *goast.ReturnStmt, error) {
	if fallback == nil || len(fallback.Body) != 2 {
		return nil, nil, fmt.Errorf("annotation default has unsupported shape")
	}
	failure, ok := fallback.Body[0].(*goast.IfStmt)
	if !ok || !annotationErrorIn(failure.Body) {
		return nil, nil, fmt.Errorf("annotation default lacks annotation error")
	}
	returned, ok := fallback.Body[1].(*goast.ReturnStmt)
	if !ok {
		return nil, nil, fmt.Errorf("annotation default lacks return")
	}
	return failure, returned, nil
}

func annotationErrorIn(node goast.Node) bool {
	found := false
	goast.Inspect(node, func(item goast.Node) bool {
		call, ok := item.(*goast.CallExpr)
		if !ok {
			return true
		}
		name, ok := call.Fun.(*goast.Ident)
		if ok && name.Name == "annotationError" {
			found = true
			return false
		}
		return true
	})
	return found
}

func typeAssertFromIf(branch *goast.IfStmt) (goast.Expr, goast.Expr, error) {
	assignment, ok := branch.Init.(*goast.AssignStmt)
	if !ok || len(assignment.Rhs) != 1 {
		return nil, nil, fmt.Errorf("field annotation lacks type assertion initializer")
	}
	assertion, ok := assignment.Rhs[0].(*goast.TypeAssertExpr)
	if !ok {
		return nil, nil, fmt.Errorf("field annotation initializer is not a type assertion")
	}
	return assertion.X, assertion.Type, nil
}
