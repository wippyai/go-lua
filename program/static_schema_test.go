package program

import (
	"math"
	"testing"
)

func staticEntry(t *testing.T) (*Builder, Term) {
	t.Helper()
	b := NewBuilder()
	entry := b.Body(Span{})
	if entry == 0 || !b.SetEntry(entry) {
		t.Fatal("entry")
	}
	return b, entry
}

func staticAlias(t *testing.T, b *Builder, owner Term, name string) Term {
	t.Helper()
	alias := b.DeclareTypeAlias(Span{}, owner, name)
	if alias == 0 || !b.SetTypeAliasGap(alias, 0) {
		t.Fatal("declare/place alias")
	}
	return alias
}

func TestStaticSchemaPredeclaredAliasAndParamTypeOf(t *testing.T) {
	b, entry := staticEntry(t)
	alias := staticAlias(t, b, entry, "Box")
	param := b.DeclareTypeParam(Span{}, alias, "T")
	operand := b.Nil(Span{}, entry)
	constraint := b.TypeOf(Span{}, param, operand)
	if alias == 0 || param == 0 || constraint == 0 || !b.FillTypeParam(param, constraint) || !b.SetTypeAliasParams(alias, []Term{param}) {
		t.Fatal("predeclare/fill parameter")
	}
	base := b.TypeRef(Span{}, "", "Box", alias)
	arg := b.TypeRef(Span{}, "", "T", param)
	generic := b.Generic(Span{}, base, []Term{arg})
	optional := b.Optional(Span{}, generic)
	boolean := b.TypeBool(Span{}, true)
	integer := b.TypeInteger(Span{}, 7)
	floating := b.TypeFloat(Span{}, math.Float64frombits(0x8000000000000000))
	nan := b.TypeFloat(Span{}, math.Float64frombits(0x7ff8000000000042))
	stringy := b.TypeString(Span{}, "ok")
	primitive := b.Primitive(Span{}, PrimitiveNumber)
	union := b.Union(Span{}, []Term{boolean, integer, floating, nan, stringy, primitive})
	target := b.Intersection(Span{}, []Term{union, optional})
	if target == 0 || !b.FillTypeAlias(alias, target) || !b.SetBody(entry) {
		t.Fatal("build static schema")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.TypeAliasParamCount(alias); !ok || got != 1 {
		t.Fatalf("parameter count = %d, %v", got, ok)
	}
	if got, ok := p.TypeAliasParamAt(alias, 0); !ok || got != param {
		t.Fatalf("parameter[0] = %v, %v", got, ok)
	}
	if owner, name, gotConstraint, ok := p.TypeParam(param); !ok || owner != alias || name == 0 || gotConstraint != constraint {
		t.Fatalf("parameter query = %v %v %v %v", owner, name, gotConstraint, ok)
	}
	if value, ok := p.Literal(floating); !ok || value.Kind != LiteralFloat || value.FloatBits != 0x8000000000000000 {
		t.Fatalf("float literal = %#v, %v", value, ok)
	}
	if value, ok := p.Literal(nan); !ok || value.Kind != LiteralFloat || value.FloatBits != 0x7ff8000000000042 {
		t.Fatalf("NaN literal = %#v, %v", value, ok)
	}
	if state, gotTarget, pkg, _, ok := p.TypeRef(base); !ok || state != TypeRefDeclaration || gotTarget != alias || pkg != 0 {
		t.Fatalf("declaration ref = %v %v %v %v", state, gotTarget, pkg, ok)
	}
	if count, ok := p.UnionLen(union); !ok || count != 6 {
		t.Fatalf("union count = %d, %v", count, ok)
	}
	if got, ok := p.UnionMember(union, 4); !ok || got != stringy {
		t.Fatalf("union[4] = %v, %v", got, ok)
	}
	if gotBase, ok := p.Generic(generic); !ok || gotBase != base {
		t.Fatalf("generic = %v %v", gotBase, ok)
	}
	if count, ok := p.GenericArgLen(generic); !ok || count != 1 {
		t.Fatalf("generic argument count = %d %v", count, ok)
	}
}

func TestStaticSchemaSignatureAndAssertion(t *testing.T) {
	b, entry := staticEntry(t)
	alias := staticAlias(t, b, entry, "Predicate")
	signatureSpan := Span{File: "signature.lua", StartLine: 1, StartCol: 18, EndLine: 1, EndCol: 48}
	assertionSpan := Span{File: "signature.lua", StartLine: 1, StartCol: 39, EndLine: 1, EndCol: 48}
	signature := b.DeclareSignature(signatureSpan, alias)
	generic := b.DeclareTypeParam(Span{File: "signature.lua", StartLine: 1, StartCol: 19, EndLine: 1, EndCol: 19}, signature, "T")
	constraint := b.Primitive(Span{}, PrimitiveString)
	if signature == 0 || generic == 0 || constraint == 0 || !b.FillTypeParam(generic, constraint) || !b.SetSignatureGenerics(signature, []Term{generic}) {
		t.Fatal("declare signature generic")
	}
	paramType := b.TypeRef(Span{}, "", "T", generic)
	variadic := b.Primitive(Span{}, PrimitiveNumber)
	narrow := b.Primitive(Span{}, PrimitiveString)
	assertion := b.Assertion(assertionSpan, "value", 0, narrow)
	name := b.TypeKey("value")
	if paramType == 0 || variadic == 0 || assertion == 0 || name == 0 || !b.FillSignature(signature, []Parameter{{Name: name, Type: paramType}}, variadic, true, []Term{assertion}) || !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, signature) || !b.SetBody(entry) {
		t.Fatal("build signature")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if gotScope, got, returnsKnown, ok := p.Signature(signature); !ok || gotScope != alias || got != variadic || !returnsKnown {
		t.Fatalf("signature = %v %v %v", got, returnsKnown, ok)
	}
	if count, ok := p.SignatureGenericCount(signature); !ok || count != 1 {
		t.Fatalf("generic count = %d, %v", count, ok)
	}
	if got, ok := p.SignatureGenericAt(signature, 0); !ok || got != generic {
		t.Fatalf("generic = %v, %v", got, ok)
	}
	if owner, _, gotConstraint, ok := p.TypeParam(generic); !ok || owner != signature || gotConstraint != constraint {
		t.Fatalf("signature generic identity = %v %v %v %v", owner, gotConstraint, generic, ok)
	}
	if count, ok := p.SignatureParamCount(signature); !ok || count != 1 {
		t.Fatalf("parameter count = %d, %v", count, ok)
	}
	if gotName, gotType, ok := p.SignatureParamAt(signature, 0); !ok || gotName != name || gotType != paramType {
		t.Fatalf("parameter = %v %v %v", gotName, gotType, ok)
	}
	if count, ok := p.SignatureReturnCount(signature); !ok || count != 1 {
		t.Fatalf("return count = %d, %v", count, ok)
	}
	if got, ok := p.SignatureReturnAt(signature, 0); !ok || got != assertion {
		t.Fatalf("return = %v, %v", got, ok)
	}
	if gotName, ordinal, gotNarrow, ok := p.Assertion(assertion); !ok || gotName != name || ordinal != 0 || gotNarrow != narrow {
		t.Fatalf("assertion = %v %d %v %v", gotName, ordinal, gotNarrow, ok)
	}
	if got, ok := p.Span(signature); !ok || got != signatureSpan {
		t.Fatalf("signature span = %#v, %v", got, ok)
	}
	if got, ok := p.Span(assertion); !ok || got != assertionSpan {
		t.Fatalf("assertion span = %#v, %v", got, ok)
	}
}

func TestStaticSchemaSignaturePreservesAbsentAndEmptyReturns(t *testing.T) {
	b, entry := staticEntry(t)
	absentAlias := staticAlias(t, b, entry, "Absent")
	emptyAlias := staticAlias(t, b, entry, "Empty")
	absent := b.DeclareSignature(Span{}, absentAlias)
	empty := b.DeclareSignature(Span{}, emptyAlias)
	if absent == 0 || empty == 0 || !b.SetSignatureGenerics(absent, nil) || !b.SetSignatureGenerics(empty, nil) || !b.FillSignature(absent, nil, 0, false, nil) || !b.FillSignature(empty, nil, 0, true, nil) || !b.SetTypeAliasParams(absentAlias, nil) || !b.SetTypeAliasParams(emptyAlias, nil) || !b.FillTypeAlias(absentAlias, absent) || !b.FillTypeAlias(emptyAlias, empty) || !b.SetBody(entry) {
		t.Fatal("build empty signatures")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, returnsKnown, ok := p.Signature(absent); !ok || returnsKnown {
		t.Fatalf("absent returns = %v, %v", returnsKnown, ok)
	}
	if _, _, returnsKnown, ok := p.Signature(empty); !ok || !returnsKnown {
		t.Fatalf("explicit empty returns = %v, %v", returnsKnown, ok)
	}
}

func TestStaticSchemaSignatureGenericTypeOfInheritsLexicalScope(t *testing.T) {
	b, entry := staticEntry(t)
	cell := b.Cell(Span{}, entry)
	bind := b.Bind(Span{}, entry, []Term{cell}, b.Values(Span{}, entry, []Term{b.Nil(Span{}, entry)}, 0))
	alias := b.DeclareTypeAlias(Span{}, entry, "Scoped")
	if bind == 0 || alias == 0 || !b.SetTypeAliasGap(alias, 1) {
		t.Fatal("declare lexical scope")
	}
	signature := b.DeclareSignature(Span{}, alias)
	param := b.DeclareTypeParam(Span{}, signature, "T")
	constraint := b.TypeOf(Span{}, param, b.Read(Span{}, entry, cell))
	ref := b.TypeRef(Span{}, "", "T", param)
	if signature == 0 || param == 0 || constraint == 0 || ref == 0 ||
		!b.FillTypeParam(param, constraint) ||
		!b.SetSignatureGenerics(signature, []Term{param}) ||
		!b.FillSignature(signature, []Parameter{{Type: ref}}, 0, true, nil) ||
		!b.SetTypeAliasParams(alias, nil) ||
		!b.FillTypeAlias(alias, signature) ||
		!b.SetBody(entry, bind) {
		t.Fatal("build scoped signature")
	}
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}

func TestStaticSchemaRejectsAssertionOutsideSignatureReturn(t *testing.T) {
	b, entry := staticEntry(t)
	alias := staticAlias(t, b, entry, "Bad")
	assertion := b.Assertion(Span{}, "value", -1, 0)
	if assertion == 0 || !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, assertion) || !b.SetBody(entry) {
		t.Fatal("build unattached assertion")
	}
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted assertion outside a signature return")
	}
}

func TestStaticSchemaRejectsDuplicateAndMissingAttachments(t *testing.T) {
	t.Run("duplicate child", func(t *testing.T) {
		b, entry := staticEntry(t)
		alias := staticAlias(t, b, entry, "T")
		child := b.Primitive(Span{}, PrimitiveString)
		if !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, b.Union(Span{}, []Term{child, child})) || !b.SetBody(entry) {
			t.Fatal("build duplicate")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("Seal accepted duplicate type child")
		}
	})
	t.Run("unattached node", func(t *testing.T) {
		b, entry := staticEntry(t)
		if b.Primitive(Span{}, PrimitiveString) == 0 || !b.SetBody(entry) {
			t.Fatal("build unattached")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("Seal accepted unattached type node")
		}
	})
	t.Run("unfilled parameter and alias range", func(t *testing.T) {
		b, entry := staticEntry(t)
		alias := staticAlias(t, b, entry, "T")
		param := b.DeclareTypeParam(Span{}, alias, "P")
		if !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, b.Primitive(Span{}, PrimitiveString)) || !b.SetBody(entry) || param == 0 {
			t.Fatal("build missing parameter")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("Seal accepted missing parameter range/fill")
		}
	})
	t.Run("typeof declaration scope cannot hide under another alias", func(t *testing.T) {
		b, entry := staticEntry(t)
		left := staticAlias(t, b, entry, "A")
		right := staticAlias(t, b, entry, "B")
		typeOf := b.TypeOf(Span{}, right, b.Nil(Span{}, entry))
		if left == 0 || right == 0 || typeOf == 0 || !b.SetTypeAliasParams(left, nil) || !b.SetTypeAliasParams(right, nil) || !b.FillTypeAlias(right, b.Primitive(Span{}, PrimitiveString)) || !b.FillTypeAlias(left, b.Optional(Span{}, typeOf)) || !b.SetBody(entry) {
			t.Fatal("build mismatched typeof host")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("Seal accepted typeof under a different declaration host")
		}
	})
}

func TestStaticSchemaTypeRefResolutionAndDeepNesting(t *testing.T) {
	b, entry := staticEntry(t)
	alias := staticAlias(t, b, entry, "T")
	if !b.SetTypeAliasParams(alias, nil) {
		t.Fatal("parameter range")
	}
	keyA, keyB := b.TypeKey("module"), b.TypeKey("Result")
	ref := b.QualifiedTypeRef(Span{}, "source", "Result", []Key{keyA, keyB})
	for i := 0; i < 12000; i++ {
		ref = b.Optional(Span{}, ref)
	}
	if ref == 0 || !b.FillTypeAlias(alias, ref) || !b.SetBody(entry) {
		t.Fatal("build deep path")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	canonical := typeRefAtBottom(t, p, ref)
	state, target, pkg, _, ok := p.TypeRef(canonical)
	count, countOK := p.TypeRefPathLen(canonical)
	if !ok || !countOK || state != TypeRefCanonicalPath || target != 0 || pkg == 0 || count != 2 {
		t.Fatalf("canonical reference = %v %v %v %d %v", state, target, pkg, count, ok)
	}
	if got, ok := p.TypeRefPathAt(canonical, 1); !ok || got != keyB {
		t.Fatalf("path[1] = %v, %v", got, ok)
	}
}

func TestStaticSchemaBareUnresolvedRef(t *testing.T) {
	b, entry := staticEntry(t)
	alias := staticAlias(t, b, entry, "T")
	ref := b.UnresolvedTypeRef(Span{}, "", "Missing")
	if ref == 0 || !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, ref) || !b.SetBody(entry) {
		t.Fatal("build unresolved reference")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	state, target, pkg, name, ok := p.TypeRef(ref)
	pathCount, pathOK := p.TypeRefPathLen(ref)
	if !ok || !pathOK || state != TypeRefUnresolved || target != 0 || pkg != 0 || name == 0 || pathCount != 0 {
		t.Fatalf("unresolved ref = %v %v %v %v %d %v", state, target, pkg, name, pathCount, ok)
	}
}

func TestStaticSchemaArrayMapRecord(t *testing.T) {
	b, entry := staticEntry(t)
	alias := staticAlias(t, b, entry, "Shape")
	stringType := b.Primitive(Span{}, PrimitiveString)
	integerType := b.Primitive(Span{}, PrimitiveInteger)
	booleanType := b.Primitive(Span{}, PrimitiveBoolean)
	array := b.Array(Span{}, stringType, true)
	mapType := b.Map(Span{}, integerType, booleanType, false)
	nameKey, valuesKey := b.TypeKey("name"), b.TypeKey("values")
	nameSpan := Span{File: "shape.lua", StartLine: 2, StartCol: 2, EndLine: 2, EndCol: 5}
	valuesSpan := Span{File: "shape.lua", StartLine: 3, StartCol: 2, EndLine: 3, EndCol: 7}
	record := b.Record(Span{File: "shape.lua"}, []RecordField{
		{Key: nameKey, Type: array, NameSpan: nameSpan},
		{Key: valuesKey, Type: mapType, NameSpan: valuesSpan, Optional: true},
	}, true)
	if record == 0 || !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, record) || !b.SetBody(entry) {
		t.Fatal("build structured static types")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if element, readonly, ok := p.Array(array); !ok || element != stringType || !readonly {
		t.Fatalf("array = %v %v %v", element, readonly, ok)
	}
	if key, value, readonly, ok := p.Map(mapType); !ok || key != integerType || value != booleanType || readonly {
		t.Fatalf("map = %v %v %v %v", key, value, readonly, ok)
	}
	if readonly, count, ok := p.Record(record); !ok || !readonly || count != 2 {
		t.Fatalf("record = %v %d %v", readonly, count, ok)
	}
	key, typ, span, optional, ok := p.RecordField(record, 1)
	if !ok || key != valuesKey || typ != mapType || !optional || span != valuesSpan {
		t.Fatalf("record field = %v %v %#v %v %v", key, typ, span, optional, ok)
	}
}

func TestStaticSchemaStructuredTypesRejectSharedChild(t *testing.T) {
	b, entry := staticEntry(t)
	alias := staticAlias(t, b, entry, "Bad")
	child := b.Primitive(Span{}, PrimitiveString)
	mapType := b.Map(Span{}, child, child, false)
	if mapType == 0 || !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, mapType) || !b.SetBody(entry) {
		t.Fatal("build shared map child")
	}
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted shared map child")
	}
}

func TestStaticSchemaQualifiedReferenceResolution(t *testing.T) {
	t.Run("qualified source resolves to declaration", func(t *testing.T) {
		b, entry := staticEntry(t)
		alias := staticAlias(t, b, entry, "T")
		ref := b.TypeRef(Span{}, "source", "T", alias)
		if ref == 0 || !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, ref) || !b.SetBody(entry) {
			t.Fatal("build qualified declaration reference")
		}
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		state, target, pkg, name, ok := p.TypeRef(ref)
		if !ok || state != TypeRefDeclaration || target != alias || pkg == 0 || name == 0 {
			t.Fatalf("qualified declaration ref = %v %v %v %v %v", state, target, pkg, name, ok)
		}
	})
	t.Run("unresolved qualified source", func(t *testing.T) {
		b, entry := staticEntry(t)
		alias := staticAlias(t, b, entry, "T")
		ref := b.UnresolvedTypeRef(Span{}, "source", "Missing")
		if ref == 0 || !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, ref) || !b.SetBody(entry) {
			t.Fatal("build unresolved qualified reference")
		}
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		state, target, pkg, name, ok := p.TypeRef(ref)
		if !ok || state != TypeRefUnresolved || target != 0 || pkg == 0 || name == 0 {
			t.Fatalf("unresolved qualified ref = %v %v %v %v %v", state, target, pkg, name, ok)
		}
	})
}

func typeRefAtBottom(t *testing.T, p *Program, term Term) Term {
	t.Helper()
	for {
		inner, ok := p.Optional(term)
		if !ok {
			return term
		}
		term = inner
	}
}
