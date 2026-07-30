package programlower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/programlower"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
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

func TestLowerStaticArrayMapAndRecordQueries(t *testing.T) {
	const source = `type Nested = readonly {number[]}
type Dictionary<K, V> = readonly {[K]: V}
type Empty = {}
type Shape = readonly {first: string, second?: number, nested: {enabled: boolean}}`
	stmts, err := parse.ParseString(source, "fixture.lua")
	if err != nil {
		t.Fatal(err)
	}
	shapeStmt := stmts[3].(*ast.TypeDefStmt)
	shapeSyntax := shapeStmt.Type.(*ast.RecordTypeExpr)
	p, err := lowerSource(source)
	if err != nil {
		t.Fatal(err)
	}
	nested, _ := p.TypeAliasAt(0)
	dictionary, _ := p.TypeAliasAt(1)
	empty, _ := p.TypeAliasAt(2)
	shape, _ := p.TypeAliasAt(3)

	_, nestedTarget, _, _ := p.TypeAlias(nested)
	innerArray, readonly, ok := p.Array(nestedTarget)
	if !ok || !readonly {
		t.Fatalf("outer array readonly=%v ok=%v", readonly, ok)
	}
	element, readonly, ok := p.Array(innerArray)
	if !ok || readonly {
		t.Fatalf("suffix array readonly=%v ok=%v", readonly, ok)
	}
	if kind, ok := p.Primitive(element); !ok || kind != program.PrimitiveNumber {
		t.Fatalf("nested array element=%v/%v", kind, ok)
	}

	if n, ok := p.TypeAliasParamCount(dictionary); !ok || n != 2 {
		t.Fatalf("dictionary params=%d/%v", n, ok)
	}
	keyParam, _ := p.TypeAliasParamAt(dictionary, 0)
	valueParam, _ := p.TypeAliasParamAt(dictionary, 1)
	_, dictionaryTarget, _, _ := p.TypeAlias(dictionary)
	key, value, readonly, ok := p.Map(dictionaryTarget)
	if !ok || !readonly {
		t.Fatalf("map readonly=%v ok=%v", readonly, ok)
	}
	state, target, _, _, ok := p.TypeRef(key)
	if !ok || state != program.TypeRefDeclaration || target != keyParam {
		t.Fatalf("map key=%v/%v/%v", state, target, ok)
	}
	state, target, _, _, ok = p.TypeRef(value)
	if !ok || state != program.TypeRefDeclaration || target != valueParam {
		t.Fatalf("map value=%v/%v/%v", state, target, ok)
	}

	_, emptyTarget, _, _ := p.TypeAlias(empty)
	if readonly, fields, ok := p.Record(emptyTarget); !ok || readonly || fields != 0 {
		t.Fatalf("empty record=%v/%d/%v", readonly, fields, ok)
	}
	_, shapeTarget, _, _ := p.TypeAlias(shape)
	if readonly, fields, ok := p.Record(shapeTarget); !ok || !readonly || fields != 3 {
		t.Fatalf("shape record=%v/%d/%v", readonly, fields, ok)
	}
	for index, want := range shapeSyntax.Fields {
		key, typ, span, optional, ok := p.RecordField(shapeTarget, index)
		if !ok || key == 0 || optional != want.Optional {
			t.Fatalf("field[%d]=%v/%v/%v", index, key, optional, ok)
		}
		wantSpan := program.Span{
			File:      "fixture.lua",
			StartLine: want.NamePosition.Line,
			StartCol:  want.NamePosition.Column,
			EndLine:   want.NamePosition.EndLine,
			EndCol:    want.NamePosition.EndColumn,
		}
		if span != wantSpan {
			t.Fatalf("field[%d] span=%#v want=%#v", index, span, wantSpan)
		}
		if index == 2 {
			if nestedReadonly, nestedFields, ok := p.Record(typ); !ok || nestedReadonly || nestedFields != 1 {
				t.Fatalf("nested record=%v/%d/%v", nestedReadonly, nestedFields, ok)
			}
		}
	}
}

func TestLowerStaticContainerAnnotationsFailClosed(t *testing.T) {
	for _, source := range []string{
		`type A = {number} @min(0)`,
		`type R = {name: string @min_len(1)}`,
	} {
		if _, err := lowerSource(source); err == nil {
			t.Fatalf("Lower(%q) accepted unsupported static annotation", source)
		}
	}
}

func TestLowerStaticFunctionTypeAndAssertion(t *testing.T) {
	p := parseBindLower(t, `type Predicate = fun(any, value: any, ... number): (asserts value is string, number)`)
	alias, ok := p.TypeAliasAt(0)
	if !ok {
		t.Fatal("Predicate alias")
	}
	_, target, _, ok := p.TypeAlias(alias)
	if !ok {
		t.Fatal("Predicate alias query")
	}
	scope, variadic, returnsKnown, ok := p.Signature(target)
	if !ok || scope != alias || !returnsKnown {
		t.Fatalf("signature=%v/%v/%v/%v", scope, variadic, returnsKnown, ok)
	}
	if kind, ok := p.Primitive(variadic); !ok || kind != program.PrimitiveNumber {
		t.Fatalf("variadic=%v/%v", kind, ok)
	}
	if count, ok := p.SignatureParamCount(target); !ok || count != 2 {
		t.Fatalf("param count=%d/%v", count, ok)
	}
	anonymous, _, ok := p.SignatureParamAt(target, 0)
	if !ok || anonymous != 0 {
		t.Fatalf("anonymous param=%v/%v", anonymous, ok)
	}
	name, parameterType, ok := p.SignatureParamAt(target, 1)
	if !ok || name == 0 {
		t.Fatalf("named param=%v/%v", name, ok)
	}
	if kind, ok := p.Primitive(parameterType); !ok || kind != program.PrimitiveAny {
		t.Fatalf("parameter type=%v/%v", kind, ok)
	}
	if count, ok := p.SignatureReturnCount(target); !ok || count != 2 {
		t.Fatalf("return count=%d/%v", count, ok)
	}
	assertion, ok := p.SignatureReturnAt(target, 0)
	if !ok {
		t.Fatal("assertion return")
	}
	assertedName, ordinal, narrow, ok := p.Assertion(assertion)
	if !ok || assertedName != name || ordinal != 1 {
		t.Fatalf("assertion=%v/%d/%v", assertedName, ordinal, ok)
	}
	if kind, ok := p.Primitive(narrow); !ok || kind != program.PrimitiveString {
		t.Fatalf("assertion narrow=%v/%v", kind, ok)
	}
	second, ok := p.SignatureReturnAt(target, 1)
	if !ok {
		t.Fatal("second return")
	}
	if kind, ok := p.Primitive(second); !ok || kind != program.PrimitiveNumber {
		t.Fatalf("second return=%v/%v", kind, ok)
	}
}

func TestLowerStaticFunctionTypePreservesAbsentAndEmptyReturns(t *testing.T) {
	p := parseBindLower(t, "type Absent = fun()\ntype Empty = fun(): ()")
	for index, wantKnown := range []bool{false, true} {
		alias, ok := p.TypeAliasAt(index)
		if !ok {
			t.Fatalf("alias[%d]", index)
		}
		_, target, _, ok := p.TypeAlias(alias)
		if !ok {
			t.Fatalf("alias[%d] query", index)
		}
		scope, _, known, ok := p.Signature(target)
		if !ok || scope != alias || known != wantKnown {
			t.Fatalf("signature[%d]=%v/%v/%v", index, scope, known, ok)
		}
		if count, ok := p.SignatureReturnCount(target); !ok || count != 0 {
			t.Fatalf("returns[%d]=%d/%v", index, count, ok)
		}
	}
}

func TestLowerStaticFunctionTypeGenericIdentity(t *testing.T) {
	constraint := &ast.PrimitiveTypeExpr{Name: "string"}
	parameterRef := &ast.TypeRefExpr{Path: []string{"T"}}
	returnRef := &ast.TypeRefExpr{Path: []string{"T"}}
	function := &ast.FunctionTypeExpr{
		TypeParams: []ast.TypeParamExpr{{Name: "T", Constraint: constraint}},
		Params:     []ast.FunctionParamExpr{{Name: "value", Type: parameterRef}},
		Returns:    []ast.TypeExpr{returnRef},
	}
	stmts := []ast.Stmt{&ast.TypeDefStmt{Name: "Identity", Type: function}}
	p, err := programlower.Lower("fixture.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err != nil {
		t.Fatal(err)
	}
	alias, _ := p.TypeAliasAt(0)
	_, signature, _, _ := p.TypeAlias(alias)
	if count, ok := p.SignatureGenericCount(signature); !ok || count != 1 {
		t.Fatalf("generic count=%d/%v", count, ok)
	}
	generic, _ := p.SignatureGenericAt(signature, 0)
	owner, _, genericConstraint, ok := p.TypeParam(generic)
	if !ok || owner != signature {
		t.Fatalf("generic owner=%v/%v", owner, ok)
	}
	if kind, ok := p.Primitive(genericConstraint); !ok || kind != program.PrimitiveString {
		t.Fatalf("generic constraint=%v/%v", kind, ok)
	}
	_, parameterType, _ := p.SignatureParamAt(signature, 0)
	state, target, _, _, ok := p.TypeRef(parameterType)
	if !ok || state != program.TypeRefDeclaration || target != generic {
		t.Fatalf("parameter generic ref=%v/%v/%v", state, target, ok)
	}
	returnType, _ := p.SignatureReturnAt(signature, 0)
	state, target, _, _, ok = p.TypeRef(returnType)
	if !ok || state != program.TypeRefDeclaration || target != generic {
		t.Fatalf("return generic ref=%v/%v/%v", state, target, ok)
	}
}
