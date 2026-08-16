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
	if !bytes.Equal(leftBytes, rightBytes) {
		t.Fatalf("alpha-renamed external formals differ:\n%x\n%x", leftBytes, rightBytes)
	}
	if bytes.Equal(leftBytes, mustCanonical(t, left)) {
		t.Fatal("scoped formal bytes reused ordinary canonical framing")
	}
	leftDigest, err := DigestCanonicalFormals(context.Background(), left, []*TypeParam{leftFormal})
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != CanonicalDigest(sha256.Sum256(leftBytes)) {
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
	if bytes.Equal(forward, reversed) {
		t.Fatal("external formal order was not encoded")
	}
}

func TestCanonicalCodecFormalsRejectInvalidScope(t *testing.T) {
	formal := NewTypeParam("T", nil)
	if encoded, err := EncodeCanonicalFormals(context.Background(), String, []*TypeParam{formal, formal}); err == nil || encoded != nil {
		t.Fatalf("duplicate external formals encoded as %x, %v", encoded, err)
	}
	if encoded, err := EncodeCanonicalFormals(context.Background(), String, []*TypeParam{nil}); err == nil || encoded != nil {
		t.Fatalf("nil external formal encoded as %x, %v", encoded, err)
	}
	foreign := NewTypeParam("Foreign", nil)
	if encoded, err := EncodeCanonicalFormals(context.Background(), foreign, nil); err == nil || encoded != nil {
		t.Fatalf("foreign TypeParam encoded as %x, %v", encoded, err)
	}

	shared := NewTypeParam("Shared", nil)
	first := Func().TypeParamRef(shared).Returns(shared).Build()
	second := Func().TypeParamRef(shared).Returns(shared).Build()
	if encoded, err := EncodeCanonicalFormals(context.Background(), NewTuple(first, second), nil); err == nil || encoded != nil {
		t.Fatalf("multi-owner TypeParam encoded as %x, %v", encoded, err)
	}

	var typedNil *Optional
	if encoded, err := EncodeCanonical(context.Background(), typedNil); err == nil || encoded != nil {
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
	if !bytes.Equal(leftBytes, rightBytes) {
		t.Fatalf("nested alpha-renamed binders differ:\n%x\n%x", leftBytes, rightBytes)
	}

	channelParam := NewTypeParam("Item", nil)
	channel := NewGeneric("Channel", []*TypeParam{channelParam}, NewArray(channelParam))
	instantiated := Instantiate(channel, String)
	if encoded, err := EncodeCanonicalFormals(context.Background(), instantiated, nil); err != nil || len(encoded) == 0 {
		t.Fatalf("Generic/Instantiated Channel<T> = %x, %v", encoded, err)
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
	if bytes.Equal(outerBytes, innerBytes) {
		t.Fatal("nested outer and inner binder references collapsed")
	}
}

func TestCanonicalCodecFormalsAdmitsNestedInstantiationAfterCanonicalRoundTrip(t *testing.T) {
	formal := NewTypeParam("T", nil)
	wrapper := NewGeneric("Wrapper", []*TypeParam{formal}, RebuildRecord(RecordParts{Fields: []Field{{Name: "inner", Type: formal}}}))
	value := Instantiate(wrapper, Instantiate(wrapper, String))
	encoded, err := EncodeCanonical(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalStructural(context.Background(), encoded)
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
	if !bytes.Equal(leftBytes, rightBytes) {
		t.Fatalf("recursive alpha-renaming differs:\n%x\n%x", leftBytes, rightBytes)
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
	if encoded, err := EncodeCanonical(context.Background(), structural); err != nil || len(encoded) < depth {
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
	if encoded, err := EncodeCanonical(context.Background(), cycle[0]); err != nil || len(encoded) == 0 {
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
