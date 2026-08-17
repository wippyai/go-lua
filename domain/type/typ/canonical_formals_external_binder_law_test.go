package typ

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// A scoped canonical root is one cut of a declaration graph. Cutting a generic
// declaration at its body installs that declaration's own parameters as the
// external formal scope, and the body reaches the declaration again through
// every self reference. The binder found there owns the installed scope rather
// than introducing fresh parameters, and the scoped codec must represent that.
func TestCanonicalFormalsBindDeclarationReenteredFromItsOwnBody(t *testing.T) {
	formal := NewTypeParam("T", nil)
	declaration := NewGeneric("Container", []*TypeParam{formal}, nil)
	method := Func().Param("self", Instantiate(declaration, formal)).Returns(formal).Build()
	body := RebuildRecord(RecordParts{Fields: []Field{
		{Name: "_value", Type: formal},
		{Name: "get", Type: method},
	}})
	declaration.SetBody(body)

	encoded, err := EncodeCanonicalFormals(context.Background(), body, []*TypeParam{formal})
	if err != nil {
		t.Fatalf("EncodeCanonicalFormals: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("EncodeCanonicalFormals produced no bytes")
	}
	if err := ValidateCanonicalFormals(encoded, 1); err != nil {
		t.Fatalf("ValidateCanonicalFormals: %v", err)
	}

	receiver := NewTypeParam("R", nil)
	decoded, err := DecodeCanonicalFormals(context.Background(), encoded, []*TypeParam{receiver})
	if err != nil {
		t.Fatalf("DecodeCanonicalFormals: %v", err)
	}
	record, ok := decoded.(*Record)
	if !ok || record == nil || len(record.Fields) != 2 {
		t.Fatalf("decoded = %T", decoded)
	}
	value := record.GetField("_value")
	if value == nil || value.Type != Type(receiver) {
		t.Fatalf("_value field = %v; want the receiver formal", value)
	}
	get := record.GetField("get")
	if get == nil {
		t.Fatal("decoded record has no get field")
	}
	function, functionOK := get.Type.(*Function)
	if !functionOK || len(function.Params) != 1 || len(function.Returns) != 1 {
		t.Fatalf("get type = %T", get.Type)
	}
	if function.Returns[0] != Type(receiver) {
		t.Fatalf("get return = %v; want the receiver formal", function.Returns[0])
	}
	self, selfOK := function.Params[0].Type.(*Instantiated)
	if !selfOK || self.Generic == nil {
		t.Fatalf("self parameter = %T", function.Params[0].Type)
	}
	if len(self.Generic.TypeParams) != 1 || self.Generic.TypeParams[0] != receiver {
		t.Fatalf("re-entered declaration binds %v; want the receiver formal", self.Generic.TypeParams)
	}
	if len(self.TypeArgs) != 1 || self.TypeArgs[0] != Type(receiver) {
		t.Fatalf("re-entered application argument = %v; want the receiver formal", self.TypeArgs)
	}

	roundTrip, err := EncodeCanonicalFormals(context.Background(), decoded, []*TypeParam{receiver})
	if err != nil {
		t.Fatalf("EncodeCanonicalFormals(decoded): %v", err)
	}
	if !bytes.Equal(roundTrip, encoded) {
		t.Fatal("scoped re-encoding of the decoded graph changed canonical bytes")
	}
}

// An inner declaration cut inside an outer one installs both binders in the
// external scope. The inner binder re-enters only its own parameters, so an
// external-scope binder must be representable at a nonzero external ordinal.
func TestCanonicalFormalsBindInnerDeclarationAtNonZeroExternalOrdinal(t *testing.T) {
	outer := NewTypeParam("X", nil)
	inner := NewTypeParam("Y", nil)
	declaration := NewGeneric("Inner", []*TypeParam{inner}, nil)
	body := RebuildRecord(RecordParts{Fields: []Field{
		{Name: "outer", Type: outer},
		{Name: "inner", Type: inner},
		{Name: "next", Type: MaterializeOptional(Instantiate(declaration, inner))},
	}})
	declaration.SetBody(body)

	scope := []*TypeParam{outer, inner}
	encoded, err := EncodeCanonicalFormals(context.Background(), body, scope)
	if err != nil {
		t.Fatalf("EncodeCanonicalFormals: %v", err)
	}
	if err := ValidateCanonicalFormals(encoded, len(scope)); err != nil {
		t.Fatalf("ValidateCanonicalFormals: %v", err)
	}

	receivers := []*TypeParam{NewTypeParam("P", nil), NewTypeParam("Q", nil)}
	decoded, err := DecodeCanonicalFormals(context.Background(), encoded, receivers)
	if err != nil {
		t.Fatalf("DecodeCanonicalFormals: %v", err)
	}
	record, ok := decoded.(*Record)
	if !ok || record == nil {
		t.Fatalf("decoded = %T", decoded)
	}
	next := record.GetField("next")
	if next == nil {
		t.Fatal("decoded record has no next field")
	}
	optional, optionalOK := next.Type.(*Optional)
	if !optionalOK || optional == nil {
		t.Fatalf("next type = %T", next.Type)
	}
	application, applicationOK := optional.Inner.(*Instantiated)
	if !applicationOK || application.Generic == nil {
		t.Fatalf("next inner = %T", optional.Inner)
	}
	if len(application.Generic.TypeParams) != 1 || application.Generic.TypeParams[0] != receivers[1] {
		t.Fatalf("re-entered declaration binds %v; want external ordinal 1", application.Generic.TypeParams)
	}

	roundTrip, err := EncodeCanonicalFormals(context.Background(), decoded, receivers)
	if err != nil {
		t.Fatalf("EncodeCanonicalFormals(decoded): %v", err)
	}
	if !bytes.Equal(roundTrip, encoded) {
		t.Fatal("scoped re-encoding of the decoded graph changed canonical bytes")
	}
}

// A binder is one frame. It either owns the installed external scope or
// introduces every parameter itself; a list drawn from both leaves a parameter
// without a single owner and has no canonical identity.
func TestCanonicalFormalsRejectBinderMixingExternalAndLocalParameters(t *testing.T) {
	external := NewTypeParam("T", nil)
	local := NewTypeParam("U", nil)
	declaration := NewGeneric("Mixed", []*TypeParam{external, local}, nil)
	declaration.SetBody(RebuildRecord(RecordParts{Fields: []Field{
		{Name: "left", Type: external},
		{Name: "right", Type: local},
	}}))
	root := NewTuple(external, declaration)

	encoded, err := EncodeCanonicalFormals(context.Background(), root, []*TypeParam{external})
	if err == nil || encoded != nil {
		t.Fatalf("EncodeCanonicalFormals = %d bytes, %v; want a rejected mixed binder", len(encoded), err)
	}
}

// A frame is classified only once every one of its parameters is a parameter.
// An absent one is reported as itself: it is not evidence that the frame draws
// from both the external scope and its own binder.
func TestCanonicalFormalsReportAbsentBinderParameterBeforeMixture(t *testing.T) {
	external := NewTypeParam("T", nil)
	declaration := &Generic{Name: "Absent", TypeParams: []*TypeParam{external, nil}, Body: external}
	root := NewTuple(external, declaration)

	encoded, err := EncodeCanonicalFormals(context.Background(), root, []*TypeParam{external})
	if err == nil || encoded != nil {
		t.Fatalf("EncodeCanonicalFormals = %d bytes, %v; want a rejected binder frame", len(encoded), err)
	}
	if !strings.Contains(err.Error(), "nil local canonical formal") {
		t.Fatalf("EncodeCanonicalFormals = %v; want the absent formal reported", err)
	}
}

// Identity is load-bearing: canonical bytes key interning and digests. A
// declaration that introduces its own parameters keeps binder-local identity,
// so its formals decode to fresh parameters rather than to receiver formals,
// and its bytes reproduce exactly.
func TestCanonicalFormalsKeepLocalBinderIdentityForIntroducedParameters(t *testing.T) {
	source := NewTypeParam("T", nil)
	receiver := NewTypeParam("R", nil)

	boxFormal := NewTypeParam("U", nil)
	box := NewGeneric("Box", []*TypeParam{boxFormal}, RebuildRecord(RecordParts{Fields: []Field{
		{Name: "value", Type: boxFormal},
		{Name: "outer", Type: source},
	}}))

	functionFormal := NewTypeParam("V", nil)
	function := Func().TypeParamRef(functionFormal).Param("v", functionFormal).Returns(source).Build()

	node := NewRecursive("Node", func(self Type) Type {
		return RebuildRecord(RecordParts{Fields: []Field{
			{Name: "tag", Type: source},
			{Name: "next", Type: MaterializeOptional(self)},
		}})
	})

	tests := []struct {
		name  string
		value Type
	}{
		{name: "tuple", value: NewTuple(source, NewArray(source))},
		{name: "generic", value: box},
		{name: "function", value: function},
		{name: "recursive", value: node},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := EncodeCanonicalFormals(context.Background(), test.value, []*TypeParam{source})
			if err != nil {
				t.Fatalf("EncodeCanonicalFormals: %v", err)
			}
			decoded, err := DecodeCanonicalFormals(context.Background(), encoded, []*TypeParam{receiver})
			if err != nil {
				t.Fatalf("DecodeCanonicalFormals: %v", err)
			}
			for _, formal := range introducedFormals(decoded) {
				if formal == receiver {
					t.Fatal("an introduced binder parameter decoded to the receiver formal")
				}
			}
			roundTrip, err := EncodeCanonicalFormals(context.Background(), decoded, []*TypeParam{receiver})
			if err != nil {
				t.Fatalf("EncodeCanonicalFormals(decoded): %v", err)
			}
			if !bytes.Equal(roundTrip, encoded) {
				t.Fatal("scoped re-encoding of the decoded graph changed canonical bytes")
			}
		})
	}
}

// introducedFormals collects every parameter listed by a binder reachable from
// root.
func introducedFormals(root Type) []*TypeParam {
	var out []*TypeParam
	seen := make(map[Type]struct{})
	stack := []Type{root}
	for len(stack) != 0 {
		value := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if value == nil {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		switch binder := value.(type) {
		case *Generic:
			out = append(out, binder.TypeParams...)
		case *Function:
			out = append(out, binder.TypeParams...)
		}
		WalkChildren(value, func(child Type) bool {
			stack = append(stack, child)
			return false
		})
	}
	return out
}
