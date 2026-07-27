package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// fenceFoldedString evaluates the string-only subset of Go constant
// expressions. Ownership belongs to the completed protocol spelling, so
// splitting that spelling across adjacent literals cannot move it past a
// source fence.
func fenceFoldedString(expression ast.Expr) (string, bool) {
	switch typed := expression.(type) {
	case *ast.BasicLit:
		if typed.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(typed.Value)
		return value, err == nil
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return "", false
		}
		left, leftOK := fenceFoldedString(typed.X)
		right, rightOK := fenceFoldedString(typed.Y)
		return left + right, leftOK && rightOK
	case *ast.ParenExpr:
		return fenceFoldedString(typed.X)
	default:
		return "", false
	}
}

func fenceFoldedStringContains(root ast.Node, prefixes []string) string {
	found := ""
	ast.Inspect(root, func(node ast.Node) bool {
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		value, folded := fenceFoldedString(expression)
		if !folded {
			return true
		}
		for _, prefix := range prefixes {
			if strings.Contains(value, prefix) {
				found = prefix
				return false
			}
		}
		return found == ""
	})
	return found
}

// fenceWIRInstructionLoop recognizes a complete indexed WIR walk by behavior,
// independent of the receiver name, containing function, or source filename.
// Existing lowering debt has a pinned package-wide ceiling; any new loop is a
// new pre-solve authority even when it uses generic names.
func fenceWIRInstructionLoop(loop *ast.ForStmt) bool {
	return fenceHasSelectorCall(loop.Cond, "Len") &&
		fenceHasSelectorCall(loop.Body, "Instr")
}

func fenceHasSelectorCall(root ast.Node, method string) bool {
	if root == nil {
		return false
	}
	found := false
	ast.Inspect(root, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == method {
			found = true
			return false
		}
		return true
	})
	return found
}

func fenceWIRInstructionLoopCount(file *ast.File) int {
	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		loop, ok := node.(*ast.ForStmt)
		if ok && fenceWIRInstructionLoop(loop) {
			count++
		}
		return true
	})
	return count
}

func fenceParseSource(source string) (*ast.File, error) {
	return parser.ParseFile(token.NewFileSet(), "mutation.go", source, 0)
}

func fenceFunctionReturnsBool(function *ast.FuncDecl) bool {
	if function.Type.Results == nil {
		return false
	}
	for _, field := range function.Type.Results.List {
		identifier, ok := field.Type.(*ast.Ident)
		if ok && identifier.Name == "bool" {
			return true
		}
	}
	return false
}

// fenceFreestandingBoolCalls reports package functions that can inject a
// boolean decision directly into the descriptor selector. The selector's only
// admission decision must be the registered lane callback; a free predicate is
// a parallel admission authority regardless of its name.
func fenceFreestandingBoolCalls(files []*ast.File, selectorName string) []string {
	boolFunctions := make(map[string]bool)
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && fenceFunctionReturnsBool(function) {
				boolFunctions[function.Name.Name] = true
			}
		}
	}
	var found []string
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != selectorName || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee, ok := call.Fun.(*ast.Ident)
				if ok && boolFunctions[callee.Name] {
					found = append(found, callee.Name)
				}
				return true
			})
		}
	}
	return found
}

// fenceRevokerSubsetLiteral recognizes the two typed spellings by which a
// consumer could enumerate only some declared revoker family IDs. Revocation
// vocabulary is declared in factkey and consumed generically by license.go; no
// other package position may construct its own subset.
func fenceRevokerSubsetLiteral(literal *ast.CompositeLit) bool {
	switch typed := literal.Type.(type) {
	case *ast.Ident:
		return typed.Name == "RevocationSet" || typed.Name == "FamilyID"
	case *ast.SelectorExpr:
		return typed.Sel.Name == "RevocationSet"
	case *ast.ArrayType:
		switch element := typed.Elt.(type) {
		case *ast.Ident:
			return element.Name == "FamilyID"
		case *ast.SelectorExpr:
			return element.Sel.Name == "FamilyID"
		}
	}
	return false
}

// fenceHandwrittenRevokerRange covers the family-record spelling identified by
// the ownership audit. Family lists are also used for non-temporal aggregation,
// so the revocation position is the structural combination of a literal family
// list, a family read, and an ordering comparison against a publication
// occurrence.
func fenceHandwrittenRevokerRange(loop *ast.RangeStmt) bool {
	expression := loop.X
	if parenthesized, ok := expression.(*ast.ParenExpr); ok {
		expression = parenthesized.X
	}
	literal, ok := expression.(*ast.CompositeLit)
	if !ok || !fenceFamilyRecordList(literal) ||
		!fenceHasSelectorCall(loop.Body, "FamilyValues") {
		return false
	}
	temporal := false
	ast.Inspect(loop.Body, func(node ast.Node) bool {
		comparison, ok := node.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		switch comparison.Op {
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
			if fenceHasSelector(comparison, "Occurrence") {
				temporal = true
				return false
			}
		}
		return true
	})
	return temporal
}

func fenceFamilyRecordList(literal *ast.CompositeLit) bool {
	array, ok := literal.Type.(*ast.ArrayType)
	if !ok {
		return false
	}
	switch element := array.Elt.(type) {
	case *ast.Ident:
		return element.Name == "Family"
	case *ast.SelectorExpr:
		return element.Sel.Name == "Family"
	default:
		return false
	}
}

func fenceHasSelector(root ast.Node, name string) bool {
	found := false
	ast.Inspect(root, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}
