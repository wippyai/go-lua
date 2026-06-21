package transferfacts

import (
	"testing"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestTypeRefsSkipNilTypeExprs(t *testing.T) {
	first := primitiveType("string")
	second := primitiveType("number")
	l := lowerer{types: make(map[any]factflow.TypeRef)}

	got := l.typeRefs([]ast.TypeExpr{first, nil, second})

	if len(got) != 2 {
		t.Fatalf("type refs = %#v, want two non-nil refs", got)
	}
	if got[0] == 0 || got[1] == 0 || got[0] == got[1] {
		t.Fatalf("type refs = %#v, want distinct non-zero refs", got)
	}
}

func TestTypeRefsReuseIdentityForSameTypeExpr(t *testing.T) {
	typ := primitiveType("string")
	l := lowerer{types: make(map[any]factflow.TypeRef)}

	got := l.typeRefs([]ast.TypeExpr{typ, typ})

	if len(got) != 2 {
		t.Fatalf("type refs = %#v, want two refs", got)
	}
	if got[0] == 0 || got[0] != got[1] {
		t.Fatalf("type refs = %#v, want repeated node to reuse identity", got)
	}
}
