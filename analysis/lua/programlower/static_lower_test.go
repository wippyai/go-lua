package programlower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
)

func TestLowerCoreStaticAliasesAndRefs(t *testing.T) {
	p := parseBindLower(t, "type A = B\ntype B = number\ntype C = Missing\ntype Node = Node?")
	if p.TypeAliasCount() != 4 {
		t.Fatalf("aliases=%d", p.TypeAliasCount())
	}
	a, _ := p.TypeAliasAt(0)
	b, _ := p.TypeAliasAt(1)
	c, _ := p.TypeAliasAt(2)
	node, _ := p.TypeAliasAt(3)
	_, aTarget, _, _ := p.TypeAlias(a)
	_, bTarget, _, _ := p.TypeAlias(b)
	_, cTarget, _, _ := p.TypeAlias(c)
	_, nodeTarget, _, _ := p.TypeAlias(node)
	state, target, _, _, ok := p.TypeRef(aTarget)
	if !ok || state != program.TypeRefDeclaration || target != b {
		t.Fatalf("forward ref=%v/%v", state, target)
	}
	if kind, ok := p.Primitive(bTarget); !ok || kind != program.PrimitiveNumber {
		t.Fatalf("primitive=%v/%v", kind, ok)
	}
	state, target, _, _, ok = p.TypeRef(cTarget)
	if !ok || state != program.TypeRefUnresolved || target != 0 {
		t.Fatal("unresolved ref")
	}
	inner, ok := p.Optional(nodeTarget)
	if !ok {
		t.Fatal("self optional")
	}
	state, target, _, _, ok = p.TypeRef(inner)
	if !ok || state != program.TypeRefDeclaration || target != node {
		t.Fatal("self ref")
	}
	entry, _ := p.Entry()
	for _, root := range bodyRoots(t, p, entry) {
		if root == a || root == b || root == c || root == node {
			t.Fatal("typedef root")
		}
	}
}

func TestLowerCoreStaticCompositesAndTypeOf(t *testing.T) {
	p := parseBindLower(t, "local x = 1\ntype Box<T: typeof(x)> = T\ntype V = true | 1 | 1.5 | \"x\"\ntype G = Box<number>")
	box, _ := p.TypeAliasAt(0)
	v, _ := p.TypeAliasAt(1)
	g, _ := p.TypeAliasAt(2)
	if n, ok := p.TypeAliasParamCount(box); !ok || n != 1 {
		t.Fatal("params")
	}
	param, _ := p.TypeAliasParamAt(box, 0)
	_, _, constraint, ok := p.TypeParam(param)
	if !ok {
		t.Fatal("param")
	}
	if _, _, ok := p.TypeOf(constraint); !ok {
		t.Fatal("typeof constraint")
	}
	_, vTarget, _, _ := p.TypeAlias(v)
	if n, ok := p.UnionLen(vTarget); !ok || n != 4 {
		t.Fatal("literal union")
	}
	_, gTarget, _, _ := p.TypeAlias(g)
	base, ok := p.Generic(gTarget)
	if !ok {
		t.Fatal("generic")
	}
	state, target, _, _, ok := p.TypeRef(base)
	if !ok || state != program.TypeRefDeclaration || target != box {
		t.Fatal("generic base")
	}
	if n, ok := p.GenericArgLen(gTarget); !ok || n != 1 {
		t.Fatal("generic args")
	}
}
