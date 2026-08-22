package typ

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"
)

func TestCanonicalCodecFormalsExternalAlphaInvariant(t *testing.T) {
	leftFormal := NewTypeParam("T", String)
	rightFormal := NewTypeParam("Value", String)
	left := NewTuple(leftFormal, NewArray(leftFormal))
	right := NewTuple(rightFormal, NewArray(rightFormal))

	leftBytes, err := EncodeCanonicalFormals(context.Background(), left, []*TypeParam{leftFormal})
	if err != nil {
		t.Fatal(err)
	}
	rightBytes, err := EncodeCanonicalFormals(context.Background(), right, []*TypeParam{rightFormal})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftBytes.Bytes(), rightBytes.Bytes()) {
		t.Fatalf("alpha-renamed external formals differ:\n%x\n%x", leftBytes.Bytes(), rightBytes.Bytes())
	}
	leftDigest, err := digestCanonicalFormalsTest(context.Background(), left, []*TypeParam{leftFormal})
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != CanonicalDigest(sha256.Sum256(leftBytes.Bytes())) {
		t.Fatal("scoped formal digest did not cover scoped bytes")
	}
}

func TestCanonicalCodecFormalsOrderIsSemantic(t *testing.T) {
	first := NewTypeParam("First", nil)
	second := NewTypeParam("Second", nil)
	value := NewTuple(first, second)
	forward, err := EncodeCanonicalFormals(context.Background(), value, []*TypeParam{first, second})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := EncodeCanonicalFormals(context.Background(), value, []*TypeParam{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(forward.Bytes(), reversed.Bytes()) {
		t.Fatal("external formal order was not encoded")
	}
}

func TestCanonicalCodecFormalsRejectInvalidScope(t *testing.T) {
	formal := NewTypeParam("T", nil)
	if encoded, err := EncodeCanonicalFormals(context.Background(), String, []*TypeParam{formal, formal}); err == nil || encoded.Valid() {
		t.Fatalf("duplicate external formals encoded as %x, %v", encoded.Bytes(), err)
	}
	if encoded, err := EncodeCanonicalFormals(context.Background(), String, []*TypeParam{nil}); err == nil || encoded.Valid() {
		t.Fatalf("nil external formal encoded as %x, %v", encoded.Bytes(), err)
	}
	foreign := NewTypeParam("Foreign", nil)
	if encoded, err := EncodeCanonicalFormals(context.Background(), foreign, nil); err == nil || encoded.Valid() {
		t.Fatalf("foreign TypeParam encoded as %x, %v", encoded.Bytes(), err)
	}

	shared := NewTypeParam("Shared", nil)
	first := Func().TypeParamRef(shared).Returns(shared).Build()
	second := Func().TypeParamRef(shared).Returns(shared).Build()
	if encoded, err := EncodeCanonicalFormals(context.Background(), NewTuple(first, second), nil); err == nil || encoded.Valid() {
		t.Fatalf("multi-owner TypeParam encoded as %x, %v", encoded.Bytes(), err)
	}

	var typedNil *Optional
	if encoded, err := encodeCanonicalTest(context.Background(), typedNil); err == nil || encoded != nil {
		t.Fatalf("typed nil encoded as %x, %v", encoded, err)
	}
}

func TestCanonicalCodecFormalsNestedBindersAreAlphaInvariant(t *testing.T) {
	left := canonicalScopedNested("Outer", "Inner")
	right := canonicalScopedNested("Element", "Result")
	leftBytes, err := EncodeCanonicalFormals(context.Background(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	rightBytes, err := EncodeCanonicalFormals(context.Background(), right, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftBytes.Bytes(), rightBytes.Bytes()) {
		t.Fatalf("nested alpha-renamed binders differ:\n%x\n%x", leftBytes.Bytes(), rightBytes.Bytes())
	}

	channelParam := NewTypeParam("Item", nil)
	channel := NewGeneric("Channel", []*TypeParam{channelParam}, NewArray(channelParam))
	instantiated := Instantiate(channel, String)
	if encoded, err := EncodeCanonicalFormals(context.Background(), instantiated, nil); err != nil || !encoded.Valid() {
		t.Fatalf("Generic/Instantiated Channel<T> = %x, %v", encoded.Bytes(), err)
	}

	outerReference := canonicalScopedReference(false)
	innerReference := canonicalScopedReference(true)
	outerBytes, err := EncodeCanonicalFormals(context.Background(), outerReference, nil)
	if err != nil {
		t.Fatal(err)
	}
	innerBytes, err := EncodeCanonicalFormals(context.Background(), innerReference, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(outerBytes.Bytes(), innerBytes.Bytes()) {
		t.Fatal("nested outer and inner binder references collapsed")
	}
}

func TestCanonicalCodecFormalsAdmitsNestedInstantiationAfterCanonicalRoundTrip(t *testing.T) {
	formal := NewTypeParam("T", nil)
	wrapper := NewGeneric("Wrapper", []*TypeParam{formal}, RebuildRecord(RecordParts{Fields: []Field{{Name: "inner", Type: formal}}}))
	value := Instantiate(wrapper, Instantiate(wrapper, String))
	receipt, err := EncodeCanonicalFormals(context.Background(), value, nil)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalFormals(context.Background(), receipt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EncodeCanonicalFormals(context.Background(), decoded, nil); err != nil {
		t.Fatalf("closed nested instantiation became open after canonical round trip: %v", err)
	}
}

func TestCanonicalCodecFormalsRecursiveNamesAreNotSemantic(t *testing.T) {
	left := NewRecursive("LeftName", func(self Type) Type {
		return NewArray(self)
	})
	right := NewRecursive("RightName", func(self Type) Type {
		return NewArray(self)
	})
	leftBytes, err := EncodeCanonicalFormals(context.Background(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	rightBytes, err := EncodeCanonicalFormals(context.Background(), right, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftBytes.Bytes(), rightBytes.Bytes()) {
		t.Fatalf("recursive alpha-renaming differs:\n%x\n%x", leftBytes.Bytes(), rightBytes.Bytes())
	}
}

func TestCanonicalFormalsReceiptCannotCrossAuthenticateBisimilarSourceGraphs(t *testing.T) {
	validFormal := NewTypeParam("T", nil)
	valid := NewGeneric("G", []*TypeParam{validFormal}, nil)
	valid.SetBody(NewArray(valid))

	leftFormal := NewTypeParam("T", nil)
	rightFormal := NewTypeParam("T", nil)
	left := NewGeneric("G", []*TypeParam{leftFormal}, nil)
	right := NewGeneric("G", []*TypeParam{rightFormal}, nil)
	left.SetBody(NewArray(right))
	right.SetBody(NewArray(left))

	if receipt, err := EncodeCanonicalFormals(context.Background(), valid, nil); err != nil || !receipt.Valid() {
		t.Fatalf("productive source graph rejected: valid=%t err=%v", receipt.Valid(), err)
	}
	if receipt, err := EncodeCanonicalFormals(context.Background(), left, nil); err == nil || receipt.Valid() {
		t.Fatalf("valid receipt authenticated a distinct invalid source graph: valid=%t err=%v", receipt.Valid(), err)
	}
}

func TestCanonicalCodecDeepGraphsUseNoGoStack(t *testing.T) {
	const depth = 100_001

	// This is both a discovery-depth and emission-depth assertion: every
	// Optional has a distinct final class because the chain terminates in a
	// primitive leaf, so the emitted definition stream is over depth nodes.
	var structural Type = Number
	for range depth {
		structural = &Optional{Inner: structural}
	}
	if encoded, err := encodeCanonicalTest(context.Background(), structural); err != nil || len(encoded) < depth {
		t.Fatalf("deep structural/emission graph = %d bytes, %v", len(encoded), err)
	}

	// A single 100k-node SCC exercises iterative Tarjan independently of the
	// acyclic emission chain above. Equal labels intentionally collapse in the
	// final quotient; traversal must still visit every original graph node.
	cycle := make([]*Recursive, depth)
	for index := range cycle {
		cycle[index] = &Recursive{ID: uint64(index + 1), Name: "Cycle"}
	}
	for index := range cycle {
		cycle[index].Body = cycle[(index+1)%len(cycle)]
	}
	if encoded, err := encodeCanonicalTest(context.Background(), cycle[0]); err != nil || len(encoded) == 0 {
		t.Fatalf("deep cyclic SCC = %d bytes, %v", len(encoded), err)
	}
}

func canonicalScopedNested(outerName, innerName string) *Generic {
	outer := NewTypeParam(outerName, nil)
	inner := NewTypeParam(innerName, nil)
	body := Func().TypeParamRef(inner).Param("outer", outer).Returns(inner, outer).Build()
	return NewGeneric("Channel", []*TypeParam{outer}, body)
}

func canonicalScopedReference(returnInner bool) *Generic {
	outer := NewTypeParam("T", nil)
	inner := NewTypeParam("U", nil)
	result := Type(outer)
	if returnInner {
		result = inner
	}
	body := Func().TypeParamRef(inner).Param("outer", outer).Returns(result).Build()
	return NewGeneric("Box", []*TypeParam{outer}, body)
}
