package acceptance_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// This vertical witness follows the final static and Flow owners only.  Static
// syntax is not mirrored through a root Program forwarding vocabulary.
func TestSourceTypesVerticalWitnesses(t *testing.T) {
	t.Run("compound type rows", func(t *testing.T) {
		p := parseBindLower(t, "type Item = readonly {name: string, count?: number}[] | {[string]: number}")
		alias, ok := p.Static().Declarations().Aliases().At(0)
		if !ok {
			t.Fatal("missing Alias")
		}
		_, target, _, _, aliasOK := p.Static().Declarations().Aliases().Get(alias)
		if !aliasOK {
			t.Fatal("missing Alias target")
		}
		if count, ok := p.Static().Types().Unions().MemberCount(target); !ok || count != 2 {
			t.Fatalf("union members = %d/%v, want two", count, ok)
		}
		array, _ := p.Static().Types().Unions().MemberAt(target, 0)
		element, readonly, arrayOK := p.Static().Types().Arrays().Get(array)
		if !arrayOK || readonly || element == 0 {
			t.Fatalf("array member = element %v readonly %v ok %v", element, readonly, arrayOK)
		}
		recordReadonly, fields, recordOK := p.Static().Types().Records().Get(element)
		if !recordOK || !recordReadonly || fields != 2 {
			t.Fatalf("record member = readonly %v fields %d ok %v", recordReadonly, fields, recordOK)
		}
	})

	t.Run("operators and references", func(t *testing.T) {
		p := parseBindLower(t, "type A = number\ntype Result = keyof(A) | A[\"field\"] | (A extends A ? A : A)")
		operators := p.Static().Operators()
		if operators.KeyOfs().Count() != 1 || operators.IndexAccesses().Count() != 1 || operators.Conditionals().Count() != 1 {
			t.Fatalf("static operator counts = %d/%d/%d", operators.KeyOfs().Count(), operators.IndexAccesses().Count(), operators.Conditionals().Count())
		}
		for _, family := range []keyspace.Term{
			func() keyspace.Term { term, _ := operators.KeyOfs().At(0); return term }(),
			func() keyspace.Term { term, _ := operators.IndexAccesses().At(0); return term }(),
			func() keyspace.Term { term, _ := operators.Conditionals().At(0); return term }(),
		} {
			if span, ok := p.Source().Identity().Span(family); !ok || span.StartLine == 0 {
				t.Fatalf("static operator %v has no Source span", family)
			}
		}
	})

	t.Run("runtime type value and typed call", func(t *testing.T) {
		p := parseBindLower(t, "type User = number\nlocal function id<T>(value: T): T return value end\nreturn id::<User>(User)")
		typeValue, typeValueOK := p.Flow().Authored().TypeValues().At(0)
		call, callOK := p.Flow().Authored().Calls().At(0)
		if !typeValueOK || !callOK {
			t.Fatalf("TypeValue/Call = %v/%v %v/%v", typeValue, typeValueOK, call, callOK)
		}
		target, targetOK := p.Static().Operands().TypeValues().Target(typeValue)
		if !targetOK || target == 0 {
			t.Fatalf("TypeValue target = %v/%v", target, targetOK)
		}
		if count, ok := p.Static().Contracts().Calls().TypeArgumentCount(call); !ok || count != 1 {
			t.Fatalf("Call type arguments = %d/%v, want one", count, ok)
		}
		if _, _, _, _, ok := p.Flow().Authored().Calls().Get(call); !ok {
			t.Fatal("Call row is absent")
		}
	})

	t.Run("cast claim has static target", func(t *testing.T) {
		p := parseBindLower(t, "type User = number\nlocal value = 1 as User")
		claim, claimOK := p.Flow().Authored().Claims().At(0)
		if !claimOK {
			t.Fatal("missing ValueClaim")
		}
		_, _, claimKind, rowOK := p.Flow().Authored().Claims().Get(claim)
		if !rowOK || claimKind != kind.ValueClaimTypeAs {
			t.Fatalf("ValueClaim kind = %v/%v", claimKind, rowOK)
		}
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !targetOK || target == 0 {
			t.Fatalf("ValueClaim target = %v/%v", target, targetOK)
		}
		resolution, declaration, root, refOK := p.Static().References().Get(target)
		if !refOK || resolution != staticrefs.Declaration || declaration == 0 || root != 0 {
			t.Fatalf("cast target reference = %v/%v/%v/%v", resolution, declaration, root, refOK)
		}
	})
}

// TestFunctionStaticScopesSealAtTheirDistinctHeaderPhases keeps the two
// Function-hosted static frontiers separate. A Function TypeParam is authored
// before that function's formals and retains the enclosing body; a return
// type and its annotation are authored after the formals and use the
// function's own body. Nesting makes both ownership identities observable.
func TestFunctionStaticScopesSealAtTheirDistinctHeaderPhases(t *testing.T) {
	p := parseBindLower(t, `
local outer = 1
local f = function<T: typeof(outer)>(value: T): typeof(value) | number @returns(value)
  local nested = function<U: typeof(value)>(inner: U): typeof(inner) | number @returns(inner)
    return inner
  end
  return value
end
return f
`)

	outerBind, ok := p.Flow().Authored().Storage().Binds().At(0)
	if !ok {
		t.Fatal("missing outer Bind")
	}
	outer := boundCell(t, p, outerBind, 0)
	outerFunction, ok := p.Flow().Authored().Functions().At(0)
	if !ok {
		t.Fatal("missing outer Function")
	}
	nestedFunction, ok := p.Flow().Authored().Functions().At(1)
	if !ok {
		t.Fatal("missing nested Function")
	}

	assertFunctionStaticScope := func(function, preFormalsSource keyspace.Term) {
		t.Helper()
		formal, ok := p.Source().Formals().At(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing formal", function)
		}
		generic, ok := p.Static().Contracts().Functions().TypeParamAt(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing generic", function)
		}
		_, _, constraint, ok := p.Static().Declarations().TypeParams().Get(generic)
		if !ok {
			t.Fatalf("Function(%v) generic TypeParam absent", function)
		}
		constraintScope, constraintOperand, ok := p.Static().Operators().TypeOfs().Get(constraint)
		if !ok || constraintScope != generic {
			t.Fatalf(
				"Function(%v) generic TypeOf = scope %v operand %v ok %v; want TypeParam scope",
				function, constraintScope, constraintOperand, ok,
			)
		}
		if _, source, _, ok := p.Flow().Authored().Storage().Reads().Get(constraintOperand); !ok || source != preFormalsSource {
			t.Fatalf(
				"Function(%v) generic TypeOf source = %v/%v; want pre-formals source %v",
				function, source, ok, preFormalsSource,
			)
		}

		returnType, ok := p.Static().Contracts().Functions().ReturnAt(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing return type", function)
		}
		returnTypeOf, annotationTarget := keyspace.Term(0), keyspace.Term(0)
		for index := 0; index < 2; index++ {
			member, ok := p.Static().Types().Unions().MemberAt(returnType, index)
			if !ok {
				t.Fatalf("Function(%v) return type is not a two-member Union", function)
			}
			if _, _, ok := p.Static().Operators().TypeOfs().Get(member); ok {
				returnTypeOf = member
			}
			if count, ok := p.Static().Operands().Annotations().ForCount(member); ok && count == 1 {
				annotationTarget = member
			}
		}
		if returnTypeOf == 0 || annotationTarget == 0 {
			t.Fatalf("Function(%v) return type omitted typeof or annotation target", function)
		}
		returnScope, returnOperand, ok := p.Static().Operators().TypeOfs().Get(returnTypeOf)
		if !ok || returnScope != function {
			t.Fatalf(
				"Function(%v) return TypeOf = scope %v operand %v ok %v; want Function scope",
				function, returnScope, returnOperand, ok,
			)
		}
		if _, source, _, ok := p.Flow().Authored().Storage().Reads().Get(returnOperand); !ok || source != formal {
			t.Fatalf(
				"Function(%v) return TypeOf source = %v/%v; want formal %v",
				function, source, ok, formal,
			)
		}
		if !p.Flow().Containment().Static(returnTypeOf) || !p.Flow().Containment().Static(returnOperand) {
			t.Fatalf("Function(%v) return TypeOf did not retain static containment", function)
		}

		annotation, ok := p.Static().Operands().Annotations().ForAt(annotationTarget, 0)
		if !ok {
			t.Fatalf("Function(%v) missing return Annotation", function)
		}
		annotationRow, ok := p.Static().Operands().Annotations().Get(annotation)
		if !ok || annotationRow.Scope != function {
			t.Fatalf(
				"Function(%v) Annotation = scope %v values %v ok %v; want Function scope",
				function, annotationRow.Scope, annotationRow.Values, ok,
			)
		}
		annotationValue := valueAt(t, p, annotationRow.Values, 0)
		if _, source, _, ok := p.Flow().Authored().Storage().Reads().Get(annotationValue); !ok || source != formal {
			t.Fatalf(
				"Function(%v) Annotation source = %v/%v; want formal %v",
				function, source, ok, formal,
			)
		}
		if !p.Flow().Containment().Static(annotation) || !p.Flow().Containment().Static(annotationValue) {
			t.Fatalf("Function(%v) Annotation did not retain static containment", function)
		}
	}

	assertFunctionStaticScope(outerFunction, outer)
	outerFormal, ok := p.Source().Formals().At(outerFunction, 0)
	if !ok {
		t.Fatal("missing outer Function formal")
	}
	assertFunctionStaticScope(nestedFunction, outerFormal)
}

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
	if primitive, ok := p.Static().Types().Primitives().Get(bTarget); !ok || primitive != statictypes.PrimitiveNumber {
		t.Fatalf("B primitive = %v/%v, want number", primitive, ok)
	}
	assertStaticDeclarationRef(t, p, cTarget, a)
	inner, optionalOK := p.Static().Types().Optionals().Get(nodeTarget)
	if !optionalOK {
		t.Fatal("missing Optional Node target")
	}
	assertStaticDeclarationRef(t, p, inner, c)
	if primitive, ok := p.Static().Types().Primitives().Get(receiverTarget); !ok || primitive != statictypes.PrimitiveSelf {
		t.Fatalf("Receiver primitive = %v/%v, want self", primitive, ok)
	}
}

func TestStaticCompositeRowsKeepExactChildren(t *testing.T) {
	p := parseBindLower(t, "type Box<T: typeof(subject)> = T\ntype Values = number | string | boolean | nil\ntype Nested = readonly {number[]}\ntype Dictionary<K, V> = {[K]: V}\ntype Shape = readonly {name: string, count?: number}")
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
	if primitive, ok := p.Static().Types().Primitives().Get(element); !ok || primitive != statictypes.PrimitiveNumber {
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
	p := parseBindLower(t, "local first: number @note(7) = 1\nlocal second: typeof(first) = 2\nlocal third = 3")
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
	p := parseBindLower(t, "type Handler = fun(value: any, ... number): (asserts value is string, number)")
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
	if primitive, ok := p.Static().Types().Primitives().Get(variadic); !ok || primitive != statictypes.PrimitiveNumber {
		t.Fatalf("Signature variadic = %v/%v", primitive, ok)
	}
	if count, ok := p.Static().Signatures().TypeFunctions().ParameterCount(signature); !ok || count != 1 {
		t.Fatalf("Signature parameter count = %d/%v, want one", count, ok)
	}
	if count, ok := p.Static().Signatures().TypeFunctions().ReturnCount(signature); !ok || count != 2 {
		t.Fatalf("Signature return count = %d/%v, want two", count, ok)
	}
}

var _ authored.CellKind
var _ keyspace.Term
