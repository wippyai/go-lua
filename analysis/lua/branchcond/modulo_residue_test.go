package branchcond

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/compiler/ast"
)

func modulo(lhs ast.Expr, modulus string) *ast.ArithmeticOpExpr {
	return &ast.ArithmeticOpExpr{Operator: "%", Lhs: lhs, Rhs: number(modulus)}
}

func relational(operator string, lhs, rhs ast.Expr) *ast.RelationalOpExpr {
	return &ast.RelationalOpExpr{Operator: operator, Lhs: lhs, Rhs: rhs}
}

func TestNormalizeModuloResidue(t *testing.T) {
	tests := []struct {
		name        string
		expr        func(*ast.IdentExpr) ast.Expr
		wantKind    CheckKind
		wantModulus int64
		wantResidue int64
		wantNegated bool
	}{
		{
			name:        "residue equality",
			expr:        func(root *ast.IdentExpr) ast.Expr { return relational("==", modulo(root, "2"), number("1")) },
			wantKind:    CheckModResidue,
			wantModulus: 2,
			wantResidue: 1,
		},
		{
			name:        "flipped operand order",
			expr:        func(root *ast.IdentExpr) ast.Expr { return relational("==", number("0"), modulo(root, "4")) },
			wantKind:    CheckModResidue,
			wantModulus: 4,
			wantResidue: 0,
		},
		{
			name:        "inequality holds on the false edge",
			expr:        func(root *ast.IdentExpr) ast.Expr { return relational("~=", modulo(root, "3"), number("2")) },
			wantKind:    CheckModResidue,
			wantModulus: 3,
			wantResidue: 2,
			wantNegated: true,
		},
		{
			name:     "negative modulus is not a residue window",
			expr:     func(root *ast.IdentExpr) ast.Expr { return relational("==", modulo(root, "-3"), number("0")) },
			wantKind: CheckNone,
		},
		{
			name:     "zero modulus names no class",
			expr:     func(root *ast.IdentExpr) ast.Expr { return relational("==", modulo(root, "0"), number("0")) },
			wantKind: CheckNone,
		},
		{
			name:     "float modulus is outside the integer window",
			expr:     func(root *ast.IdentExpr) ast.Expr { return relational("==", modulo(root, "2.5"), number("1")) },
			wantKind: CheckNone,
		},
		{
			name:     "residue at the modulus is unreachable",
			expr:     func(root *ast.IdentExpr) ast.Expr { return relational("==", modulo(root, "2"), number("2")) },
			wantKind: CheckNone,
		},
		{
			name:     "negative residue is unreachable",
			expr:     func(root *ast.IdentExpr) ast.Expr { return relational("==", modulo(root, "2"), number("-1")) },
			wantKind: CheckNone,
		},
		{
			name:     "ordered comparison is not a residue guard",
			expr:     func(root *ast.IdentExpr) ast.Expr { return relational("<", modulo(root, "2"), number("1")) },
			wantKind: CheckNone,
		},
		{
			name: "another arithmetic operator is not a residue guard",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return relational("==", &ast.ArithmeticOpExpr{Operator: "+", Lhs: root, Rhs: number("2")}, number("1"))
			},
			wantKind: CheckNone,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := ident("value")
			expr := test.expr(root)
			bindings := bindReturn(expr)
			symbolID := mustIdentSymbol(t, bindings, root)
			got := Normalize(expr, bindings)
			if test.wantKind == CheckNone {
				assertCheckNone(t, got)
				return
			}
			assertCheck(t, got, test.wantKind, path.NewPath(symbolID, "value"), "")
			if got.Modulus != test.wantModulus || got.Residue != test.wantResidue {
				t.Fatalf("modulus/residue = %d/%d, want %d/%d", got.Modulus, got.Residue, test.wantModulus, test.wantResidue)
			}
			if got.Negated != test.wantNegated {
				t.Fatalf("negated = %v, want %v", got.Negated, test.wantNegated)
			}
		})
	}
}

func TestNormalizeModuloResidueOnMemberPath(t *testing.T) {
	root := ident("obj")
	expr := relational("==", modulo(dot(root, "count"), "8"), number("3"))
	bindings := bindReturn(expr)
	symbolID := mustIdentSymbol(t, bindings, root)
	got := Normalize(expr, bindings)
	assertCheck(t, got, CheckModResidue, path.NewPath(symbolID, "obj").Field("count"), "")
	if got.Modulus != 8 || got.Residue != 3 {
		t.Fatalf("modulus/residue = %d/%d, want 8/3", got.Modulus, got.Residue)
	}
}
