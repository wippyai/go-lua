package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/static"
)

func TestStaticAliasesPrimitivesAndReferencesUseStaticVocabulary(t *testing.T) {
	p := parseBindLower(t, "type A = B\ntype B = number\ntype C = A\ntype Node = C?\ntype Receiver = self")
	aliases := p.Static().Declarations().Aliases()
	if aliases.Count() != 5 {
		t.Fatalf("Static Alias count = %d, want 5", aliases.Count())
	}
	a, _ := aliases.At(0)
	b, _ := aliases.At(1)
	c, _ := aliases.At(2)
	node, _ := aliases.At(3)
	receiver, _ := aliases.At(4)
	_, aTarget, _, _, _ := aliases.Get(a)
	_, bTarget, _, _, _ := aliases.Get(b)
	_, cTarget, _, _, _ := aliases.Get(c)
	_, nodeTarget, _, _, _ := aliases.Get(node)
	_, receiverTarget, _, _, _ := aliases.Get(receiver)
	assertStaticDeclarationRef(t, p, aTarget, b)
	if primitive, ok := p.Static().Types().Primitives().Get(bTarget); !ok || primitive != static.PrimitiveNumber {
		t.Fatalf("B primitive = %v/%v, want number", primitive, ok)
	}
	assertStaticDeclarationRef(t, p, cTarget, a)
	inner, optionalOK := p.Static().Types().Optionals().Get(nodeTarget)
	if !optionalOK {
		t.Fatal("missing Optional Node target")
	}
	assertStaticDeclarationRef(t, p, inner, c)
	if primitive, ok := p.Static().Types().Primitives().Get(receiverTarget); !ok || primitive != static.PrimitiveSelf {
		t.Fatalf("Receiver primitive = %v/%v, want self", primitive, ok)
	}
}

func TestStaticCompositeRowsKeepExactChildren(t *testing.T) {
	p := parseBindLower(t, "type Box<T: typeof(subject)> = T\ntype Values = number | string | boolean | nil\ntype Nested = readonly number[][]\ntype Dictionary<K, V> = {[K]: V}\ntype Shape = { readonly name: string, optional count: number }")
	aliases := p.Static().Declarations().Aliases()
	box, _ := aliases.At(0)
	values, _ := aliases.At(1)
	nested, _ := aliases.At(2)
	dictionary, _ := aliases.At(3)
	shape, _ := aliases.At(4)
	param, paramOK := aliases.ParamAt(box, 0)
	if !paramOK {
		t.Fatal("missing Box parameter")
	}
	_, _, constraint, paramRowOK := p.Static().Declarations().TypeParams().Get(param)
	if !paramRowOK || constraint == 0 {
		t.Fatal("missing Box parameter constraint")
	}
	if _, _, ok := p.Static().Operators().TypeOfs().Get(constraint); !ok {
		t.Fatal("Box constraint is not Static TypeOf")
	}
	_, valuesTarget, _, _, _ := aliases.Get(values)
	if count, ok := p.Static().Types().Unions().MemberCount(valuesTarget); !ok || count != 4 {
		t.Fatalf("Values union members = %d/%v, want 4", count, ok)
	}
	_, nestedTarget, _, _, _ := aliases.Get(nested)
	innerArray, readonly, arrayOK := p.Static().Types().Arrays().Get(nestedTarget)
	if !arrayOK || !readonly {
		t.Fatalf("outer Array = %v/%v readonly=%v", innerArray, arrayOK, readonly)
	}
	element, innerReadonly, innerArrayOK := p.Static().Types().Arrays().Get(innerArray)
	if !innerArrayOK || innerReadonly {
		t.Fatalf("inner Array = %v/%v readonly=%v", element, innerArrayOK, innerReadonly)
	}
	if primitive, ok := p.Static().Types().Primitives().Get(element); !ok || primitive != static.PrimitiveNumber {
		t.Fatalf("Nested element = %v/%v", primitive, ok)
	}
	_, dictionaryTarget, _, _, _ := aliases.Get(dictionary)
	key, value, _, mapOK := p.Static().Types().Maps().Get(dictionaryTarget)
	if !mapOK || key == 0 || value == 0 {
		t.Fatalf("Dictionary map = key %v value %v ok %v", key, value, mapOK)
	}
	_, shapeTarget, _, _, _ := aliases.Get(shape)
	shapeReadonly, fieldCount, recordOK := p.Static().Types().Records().Get(shapeTarget)
	if !recordOK || !shapeReadonly || fieldCount != 2 {
		t.Fatalf("Shape record = readonly %v fields %d ok %v", shapeReadonly, fieldCount, recordOK)
	}
}

func TestStaticAnnotationsAndDeclaredTypesStayInStaticOwner(t *testing.T) {
	p := parseBindLower(t, "local first: number = 1\nlocal second: typeof(first) @note(7) = 2\nlocal third = 3")
	declared := p.Static().Declarations().DeclaredTypes()
	if declared.Count() != 2 {
		t.Fatalf("DeclaredType count = %d, want 2", declared.Count())
	}
	annotations := p.Static().Operands().Annotations()
	annotation, annotationOK := annotations.At(0)
	row, rowOK := annotations.Get(annotation)
	if !annotationOK || !rowOK || row.Target == 0 || row.Values == 0 {
		t.Fatalf("Annotation = %#v/%v/%v", row, annotationOK, rowOK)
	}
	if count, ok := annotations.ForCount(row.Target); !ok || count != 1 {
		t.Fatalf("Annotation target count = %d/%v", count, ok)
	}
	binds := p.Flow().Authored().Storage().Binds()
	secondBind, bindOK := binds.At(1)
	if !bindOK {
		t.Fatal("missing second Bind")
	}
	secondCell := boundCell(t, p, secondBind, 0)
	if term, ok := declared.ForCell(secondCell); !ok || term == 0 {
		t.Fatalf("second Cell declared type = %v/%v", term, ok)
	}
	if !p.Flow().Containment().Static(annotation) || !p.Flow().Containment().Static(row.Values) {
		t.Fatal("Static Annotation escaped Flow static containment")
	}
}

func TestStaticSignatureRowsKeepParametersAndReturns(t *testing.T) {
	p := parseBindLower(t, "type Handler = fun(...: number, value: any): (asserts value is string, number)")
	alias, ok := p.Static().Declarations().Aliases().At(0)
	if !ok {
		t.Fatal("missing Handler Alias")
	}
	_, signature, _, _, aliasOK := p.Static().Declarations().Aliases().Get(alias)
	if !aliasOK {
		t.Fatal("missing Handler target")
	}
	scope, variadic, _, returnsKnown, signatureOK := p.Static().Signatures().TypeFunctions().Get(signature)
	if !signatureOK || scope == 0 || variadic == 0 || !returnsKnown {
		t.Fatalf("Signature = scope %v variadic %v known %v ok %v", scope, variadic, returnsKnown, signatureOK)
	}
	if primitive, ok := p.Static().Types().Primitives().Get(variadic); !ok || primitive != static.PrimitiveNumber {
		t.Fatalf("Signature variadic = %v/%v", primitive, ok)
	}
	if count, ok := p.Static().Signatures().TypeFunctions().ParameterCount(signature); !ok || count != 1 {
		t.Fatalf("Signature parameter count = %d/%v, want one", count, ok)
	}
	if count, ok := p.Static().Signatures().TypeFunctions().ReturnCount(signature); !ok || count != 2 {
		t.Fatalf("Signature return count = %d/%v, want two", count, ok)
	}
}

var _ flow.CellKind
var _ keyspace.Term
