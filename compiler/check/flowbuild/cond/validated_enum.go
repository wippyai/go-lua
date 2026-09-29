package cond

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

// validatedEnumGuards recognizes a common proof of membership: a false flag
// becomes true only after a value equals an item in a fixed literal list, and
// the false path exits. The surviving edge can safely narrow the value to the
// list's literals. Keep the recognized statement shape restrictive so a later
// write to the flag, list, or tested value cannot invalidate the proof.
type enumProof struct {
	subject ast.Expr
	values  typ.Type
}

func validatedEnumGuards(fc *core.FlowContext) map[ast.Expr]enumProof {
	if fc == nil || fc.Graph == nil || fc.Derived == nil || fc.Derived.Synth == nil || fc.Graph.Func() == nil || fc.Graph.Bindings() == nil {
		return nil
	}
	graph := fc.Graph
	fn := graph.Func()
	var proofs map[ast.Expr]enumProof
	for i := 0; i+3 < len(fn.Stmts); i++ {
		listDecl, ok := fn.Stmts[i].(*ast.LocalAssignStmt)
		if !ok || len(listDecl.Names) != 1 || len(listDecl.Exprs) != 1 {
			continue
		}
		list, ok := listDecl.Exprs[0].(*ast.TableExpr)
		if !ok || len(list.Fields) == 0 {
			continue
		}
		members := make([]typ.Type, 0, len(list.Fields))
		for _, field := range list.Fields {
			if field.Key != nil {
				members = nil
				break
			}
			literal, ok := field.Value.(*ast.StringExpr)
			if !ok {
				members = nil
				break
			}
			members = append(members, typ.LiteralString(literal.Value))
		}
		if len(members) == 0 {
			continue
		}
		flagDecl, ok := fn.Stmts[i+1].(*ast.LocalAssignStmt)
		if !ok || len(flagDecl.Names) != 1 || len(flagDecl.Exprs) != 1 {
			continue
		}
		if _, ok := flagDecl.Exprs[0].(*ast.FalseExpr); !ok {
			continue
		}
		flagName := flagDecl.Names[0]
		loop, ok := fn.Stmts[i+2].(*ast.GenericForStmt)
		if !ok || len(loop.Names) != 2 || len(loop.Exprs) != 1 || len(loop.Stmts) != 1 {
			continue
		}
		iterCall, ok := loop.Exprs[0].(*ast.FuncCallExpr)
		if !ok || iterCall.Method != "" || len(iterCall.Args) != 1 || !namedIdent(iterCall.Func, "ipairs") || !namedIdent(iterCall.Args[0], listDecl.Names[0]) {
			continue
		}
		if !hasIndexedIteratorContract(fc, loop, iterCall) {
			continue
		}
		match, ok := loop.Stmts[0].(*ast.IfStmt)
		if !ok || len(match.Else) != 0 || len(match.Then) != 2 {
			continue
		}
		comparison, ok := match.Condition.(*ast.RelationalOpExpr)
		if !ok || comparison.Operator != "==" {
			continue
		}
		var subject ast.Expr
		if namedIdent(comparison.Lhs, loop.Names[1]) {
			subject = comparison.Rhs
		} else if namedIdent(comparison.Rhs, loop.Names[1]) {
			subject = comparison.Lhs
		} else {
			continue
		}
		subjectIdent, ok := subject.(*ast.IdentExpr)
		if !ok || subjectIdent.Value == flagName || subjectIdent.Value == listDecl.Names[0] || subjectIdent.Value == loop.Names[0] || subjectIdent.Value == loop.Names[1] {
			continue
		}
		write, ok := match.Then[0].(*ast.AssignStmt)
		if !ok || len(write.Lhs) != 1 || len(write.Rhs) != 1 || !namedIdent(write.Lhs[0], flagName) {
			continue
		}
		if _, ok := write.Rhs[0].(*ast.TrueExpr); !ok {
			continue
		}
		if _, ok := match.Then[1].(*ast.BreakStmt); !ok {
			continue
		}
		guard, ok := fn.Stmts[i+3].(*ast.IfStmt)
		if !ok || len(guard.Else) != 0 || len(guard.Then) == 0 {
			continue
		}
		negated, ok := guard.Condition.(*ast.UnaryNotOpExpr)
		if !ok || !namedIdent(negated.Expr, flagName) {
			continue
		}
		if _, ok := guard.Then[len(guard.Then)-1].(*ast.ReturnStmt); !ok {
			continue
		}
		if proofs == nil {
			proofs = make(map[ast.Expr]enumProof)
		}
		proofs[guard.Condition] = enumProof{subject: subject, values: typ.NewUnion(members...)}
	}
	return proofs
}

func hasIndexedIteratorContract(fc *core.FlowContext, loop *ast.GenericForStmt, call *ast.FuncCallExpr) bool {
	proved := false
	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if proved || info == nil || info.Stmt != loop {
			return
		}
		spec := contract.ExtractSpec(fc.Derived.Synth(call.Func, p))
		if spec != nil {
			iter := spec.GetIterator()
			proved = iter != nil && iter.Kind == effect.IterateIndexed
		}
	})
	return proved
}

func namedIdent(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.IdentExpr)
	return ok && ident.Value == name
}
