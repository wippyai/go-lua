package typ

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/type/annotation"
)

func TestCanonicalCodecMatchesTypeEqualsAcrossSemanticCorpus(t *testing.T) {
	leftRecursive := canonicalSelfRecord("Node")
	rightRecursive := canonicalSelfRecord("Node")
	leftMutual := canonicalMutualRecords()
	rightMutual := canonicalMutualRecords()

	leftParam := NewTypeParam("T", NewInterface("Comparable", nil))
	rightParam := NewTypeParam("T", NewInterface("Comparable", nil))
	leftGeneric := NewGeneric("Box", []*TypeParam{leftParam}, RebuildRecord(RecordParts{Fields: []Field{{Name: "value", Type: leftParam}}}))
	rightGeneric := NewGeneric("Box", []*TypeParam{rightParam}, RebuildRecord(RecordParts{Fields: []Field{{Name: "value", Type: rightParam}}}))

	leftRecord := RebuildRecord(RecordParts{
		Fields: []Field{
			{Name: "z", Type: NewArray(String), Readonly: true},
			{Name: "a", Type: MaterializeOptional(Number), Optional: true},
		},
		StaticMembers: []StaticMember{
			{Kind: StaticMemberIntIndex, Index: 7, Type: Boolean},
			{Kind: StaticMemberStringIndex, Name: "kind", Type: LiteralString("record"), Readonly: true},
		},
		MapKey: String, MapValue: Unknown, Metatable: NewMeta(String), Open: true,
	})
	rightRecord := RebuildRecord(RecordParts{
		Fields: []Field{
			{Name: "a", Type: MaterializeOptional(Number), Optional: true},
			{Name: "z", Type: NewArray(String), Readonly: true},
		},
		StaticMembers: []StaticMember{
			{Kind: StaticMemberStringIndex, Name: "kind", Type: LiteralString("record"), Readonly: true},
			{Kind: StaticMemberIntIndex, Index: 7, Type: Boolean},
		},
		MapKey: String, MapValue: Unknown, Metatable: NewMeta(String), Open: true,
	})

	leftFunction := Func().TypeParamRef(leftParam).Param("presentation-left", leftRecord).OptParam("optional-left", NewMap(String, Number)).Variadic(Integer).Returns(leftRecursive, NewReadonlyMap(String, Boolean)).Build()
	rightFunction := Func().TypeParamRef(rightParam).Param("presentation-right", rightRecord).OptParam("optional-right", NewMap(String, Number)).Variadic(Integer).Returns(rightRecursive, NewReadonlyMap(String, Boolean)).Build()

	leftUnion := MaterializeUnion([]Type{String, Integer, LiteralString("x")})
	rightUnion := MaterializeUnion([]Type{LiteralString("x"), String, Integer})
	leftIntersection := MaterializeIntersection([]Type{leftRecord, NewInterface("Named", nil)})
	rightIntersection := MaterializeIntersection([]Type{NewInterface("Named", nil), rightRecord})

	annotated := NewAnnotated(leftFunction, []annotation.Annotation{{Name: "min", Arg: annotation.Int64Arg(3)}})
	alias := NewAlias("PresentationOnly", rightFunction)

	corpus := []Type{
		nil, Nil, Boolean, Number, Integer, String, Any, Unknown, Never, Self,
		LiteralBool(true), LiteralInt(-19), LiteralNumber(2.5), LiteralString("hello\x00world"),
		NewRef("module/path", "Thing"),
		MaterializeOptional(String), NewArray(Number), NewMap(String, Integer), NewReadonlyMap(String, Integer),
		NewTuple(String, Number, MaterializeOptional(Boolean)),
		leftRecord, rightRecord, leftFunction, rightFunction,
		leftGeneric, rightGeneric, Instantiate(leftGeneric, String), Instantiate(rightGeneric, String),
		NewInterface("Reader", []Method{{Name: "read", Type: Func().Param("self", Self).Returns(String).Build()}}),
		NewMeta(leftRecord), leftUnion, rightUnion, leftIntersection, rightIntersection,
		leftRecursive, rightRecursive, leftMutual, rightMutual,
		annotated, alias,
	}

	encoded := make([][]byte, len(corpus))
	for index, value := range corpus {
		var err error
		encoded[index], err = EncodeCanonical(context.Background(), value)
		if err != nil {
			t.Fatalf("encode corpus[%d] %T: %v", index, value, err)
		}
		if len(encoded[index]) == 0 {
			t.Fatalf("encode corpus[%d] returned no bytes", index)
		}
	}
	for left := range corpus {
		for right := range corpus {
			if equal, sameBytes := TypeEquals(corpus[left], corpus[right]), bytes.Equal(encoded[left], encoded[right]); equal != sameBytes {
				t.Fatalf("TypeEquals/bytes mismatch at %d (%T) and %d (%T): equal=%v bytes=%v\n%x\n%x", left, corpus[left], right, corpus[right], equal, sameBytes, encoded[left], encoded[right])
			}
		}
	}
	if !bytes.Equal(mustCanonical(t, annotated), mustCanonical(t, leftFunction)) || !bytes.Equal(mustCanonical(t, alias), mustCanonical(t, rightFunction)) {
		t.Fatal("transparent annotation/alias changed TypeEquals canonical identity")
	}
}

func TestCanonicalCodecUsesBisimulationBeforeBinderOrdinals(t *testing.T) {
	one := NewRecursive("N", func(self Type) Type { return self })
	twoA := NewRecursivePlaceholder("N")
	twoB := NewRecursivePlaceholder("N")
	twoA.SetBody(twoB)
	twoB.SetBody(twoA)
	if !TypeEquals(one, twoA) {
		t.Fatal("fixture must be coinductively equal")
	}
	if !bytes.Equal(mustCanonical(t, one), mustCanonical(t, twoA)) {
		t.Fatal("bisimilar one-node and two-node recursive graphs encoded differently")
	}
}

func TestCanonicalCodecLongRecursiveChainStabilizesByPartition(t *testing.T) {
	left := canonicalRecursiveChain(1024)
	right := canonicalRecursiveChain(1024)
	if !TypeEquals(left, right) {
		t.Fatal("independently constructed recursive chains must be equal")
	}
	leftBytes := mustCanonical(t, left)
	if again := mustCanonical(t, left); !bytes.Equal(leftBytes, again) {
		t.Fatal("repeated encoding of one recursive chain was nondeterministic")
	}
	if rightBytes := mustCanonical(t, right); !bytes.Equal(leftBytes, rightBytes) {
		t.Fatal("equal independently constructed recursive chains encoded differently")
	}
}

func TestCanonicalCodecBisimulationCrossesSCCBoundary(t *testing.T) {
	cycle := NewRecursive("N", func(self Type) Type { return self })
	prefix := NewRecursivePlaceholder("N")
	prefix.SetBody(cycle)
	if !TypeEquals(prefix, cycle) {
		t.Fatal("acyclic prefix and its equal recursive tail must be coinductively equal")
	}
	if !bytes.Equal(mustCanonical(t, prefix), mustCanonical(t, cycle)) {
		t.Fatal("SCC boundary changed the canonical bisimulation quotient")
	}
}

func TestCanonicalCodecMixedDAGCycleIgnoresConstructionOrder(t *testing.T) {
	left := canonicalMixedDAGCycle(false)
	right := canonicalMixedDAGCycle(true)
	if !TypeEquals(left, right) {
		t.Fatal("mixed graphs built in opposite allocation order must be equal")
	}
	if !bytes.Equal(mustCanonical(t, left), mustCanonical(t, right)) {
		t.Fatal("mixed DAG/cycle encoding depended on construction order")
	}
}

func TestCanonicalCodecIndependentConstructionPairs(t *testing.T) {
	pairs := []struct {
		name        string
		left, right Type
	}{
		{name: "ref", left: NewRef("m", "T"), right: NewRef("m", "T")},
		{name: "tuple", left: NewTuple(String, NewArray(Number)), right: NewTuple(String, NewArray(Number))},
		{name: "map", left: NewMap(String, NewReadonlyMap(Integer, Boolean)), right: NewMap(String, NewReadonlyMap(Integer, Boolean))},
		{name: "interface", left: NewInterface("I", []Method{{Name: "m", Type: Func().Param("x", Number).Returns(String).Build()}}), right: NewInterface("I", []Method{{Name: "m", Type: Func().Param("renamed", Number).Returns(String).Build()}})},
		{name: "meta", left: NewMeta(NewTuple(String)), right: NewMeta(NewTuple(String))},
	}
	for _, test := range pairs {
		t.Run(test.name, func(t *testing.T) {
			if !TypeEquals(test.left, test.right) {
				t.Fatal("independent fixture is not TypeEquals")
			}
			if !bytes.Equal(mustCanonical(t, test.left), mustCanonical(t, test.right)) {
				t.Fatal("independent equal construction encoded differently")
			}
		})
	}
}

// These ordinary-domain bytes predate the iterative traversal rewrite. They
// freeze representative scalar, binder, and recursive streams so a future
// traversal optimization cannot silently redefine the canonical ABI.
func TestCanonicalCodecRepresentativeBytesAreFrozen(t *testing.T) {
	formal := NewTypeParam("T", String)
	generic := NewGeneric("Box", []*TypeParam{formal}, RebuildRecord(RecordParts{Fields: []Field{{Name: "value", Type: formal}}}))
	values := []struct {
		name  string
		value Type
		want  string
	}{
		{name: "composite", value: NewTuple(NewRef("m", "T"), NewMap(String, LiteralInt(-7))), want: "2177697070792e616e616c797369732e747970652e7479702e63616e6f6e6963616c020100021002020101050c016d015400010201120201030106000104030b030d00"},
		{name: "generic", value: Instantiate(generic, Integer), want: "2177697070792e616e616c797369732e747970652e7479702e63616e6f6e6963616c020100021701020101071603426f780101020102041801540101010301060001040e1400010576616c756500000000000100020105010500"},
		{name: "recursive", value: canonicalSelfRecord("Node"), want: "2177697070792e616e616c797369732e747970652e7479702e63616e6f6e6963616c0201000719044e6f64650101010115140002046e65787401000576616c75650000000000020102010d0100000103010600"},
	}
	for _, test := range values {
		t.Run(test.name, func(t *testing.T) {
			got := fmt.Sprintf("%x", mustCanonical(t, test.value))
			if got != test.want {
				t.Errorf("canonical bytes changed:\nwant %s\n got %s", test.want, got)
			}
		})
	}
}

func TestCanonicalCodecRejectsDuplicateRecursiveIdentity(t *testing.T) {
	left := &Recursive{ID: 77, Name: "A", Body: Number}
	right := &Recursive{ID: 77, Name: "B", Body: String}
	value := NewTuple(left, right)
	if encoded, err := EncodeCanonical(context.Background(), value); err == nil || encoded != nil {
		t.Fatalf("duplicate recursive ID encoded as %x, err=%v", encoded, err)
	}
}

func TestCanonicalCodecRejectsUnsupportedImplementation(t *testing.T) {
	value := &fakeType{id: "unsupported", hash: 1}
	if encoded, err := EncodeCanonical(context.Background(), value); err == nil || encoded != nil {
		t.Fatalf("unsupported implementation encoded as %x, err=%v", encoded, err)
	}
}

func TestCanonicalCodecEncodesDeepTransparentAliasesExactly(t *testing.T) {
	var deep Type = Number
	for range 257 {
		deep = &Alias{Name: "transparent", Target: deep}
	}
	if encoded, err := EncodeCanonical(context.Background(), deep); err != nil || len(encoded) == 0 {
		t.Fatalf("deep alias was not encoded exactly: %x, err=%v", encoded, err)
	}
	if !TypeEquals(deep, Number) {
		t.Fatal("deep transparent alias is not equal to its target")
	}

	// Numeric singleton identity is raw-IEEE-bit exact, including NaN.
	if encoded, err := EncodeCanonical(context.Background(), LiteralNumber(math.NaN())); err != nil || len(encoded) == 0 {
		t.Fatalf("NaN literal encoded as %x, err=%v", encoded, err)
	}
}

func TestCanonicalCodecSeparatesNumericLiteralIdentity(t *testing.T) {
	left, right := LiteralNumber(2.25), LiteralNumber(2.75)
	if EqualityHash(left) == EqualityHash(right) || TypeEquals(left, right) {
		t.Fatal("distinct numeric literal bits collapsed")
	}
	leftBytes, rightBytes := mustCanonical(t, left), mustCanonical(t, right)
	if bytes.Equal(leftBytes, rightBytes) {
		t.Fatal("canonical bytes collapsed unequal legacy-hash collision")
	}
	leftDigest, err := DigestCanonical(context.Background(), left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := DigestCanonical(context.Background(), right)
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest == rightDigest || leftDigest != CanonicalDigest(sha256.Sum256(leftBytes)) {
		t.Fatal("typed full digest did not cover canonical bytes")
	}
}

func TestCanonicalCodecCancellationReturnsNoAuthority(t *testing.T) {
	children := make([]Type, 4096)
	for index := range children {
		children[index] = NewTuple(LiteralInt(int64(index)), NewArray(String))
	}
	ctx := &canonicalCancelContext{remaining: 3}
	if encoded, err := EncodeCanonical(ctx, NewTuple(children...)); err != context.Canceled || encoded != nil {
		t.Fatalf("canceled encoding = %x, %v", encoded, err)
	}
	ctx = &canonicalCancelContext{remaining: 3}
	if digest, err := DigestCanonical(ctx, NewTuple(children...)); err != context.Canceled || digest != (CanonicalDigest{}) {
		t.Fatalf("canceled digest = %x, %v", digest, err)
	}
}

func TestCanonicalCodecPollsCancellationThroughTransparentAnnotations(t *testing.T) {
	var value Type = Number
	for range 4096 {
		value = NewAnnotated(value, []annotation.Annotation{{Name: "transparent"}})
	}
	ctx := &canonicalCancelContext{remaining: 2}
	if encoded, err := EncodeCanonical(ctx, value); err != context.Canceled || encoded != nil {
		t.Fatalf("canceled transparent unwrap = %x, %v", encoded, err)
	}
}

func TestCanonicalEncoderReuseIsOwnershipIsolated(t *testing.T) {
	var encoder CanonicalEncoder
	first, err := encoder.Encode(context.Background(), NewArray(String))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := append([]byte(nil), first...)
	second, err := encoder.Encode(context.Background(), NewArray(Number))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, snapshot) || bytes.Equal(first, second) {
		t.Fatal("reused encoder aliased output ownership or collapsed unequal types")
	}
}

func TestCanonicalEncoderDirectCallsClearCallerReferences(t *testing.T) {
	param := NewTypeParam("T", String)
	value := NewTuple(param, canonicalSelfRecord("Direct"))
	var encoder CanonicalEncoder
	if encoded, err := encoder.EncodeFormals(context.Background(), value, []*TypeParam{param}); err != nil || len(encoded) == 0 {
		t.Fatalf("direct scoped encode = %x, %v", encoded, err)
	}
	assertCanonicalEncoderDetached(t, &encoder)

	if encoded, err := encoder.Encode(context.Background(), &fakeType{id: "direct-error", hash: 1}); err == nil || encoded != nil {
		t.Fatalf("direct malformed encode = %x, %v", encoded, err)
	}
	assertCanonicalEncoderDetached(t, &encoder)

	if encoded, err := encoder.EncodeFormals(context.Background(), String, []*TypeParam{param, param}); err == nil || encoded != nil {
		t.Fatalf("direct duplicate-formal encode = %x, %v", encoded, err)
	}
	assertCanonicalEncoderDetached(t, &encoder)
}

func TestCanonicalEncoderCancellationDuringEmissionDoesNotPoisonReuse(t *testing.T) {
	var deep Type = Number
	for range 4096 {
		deep = &Optional{Inner: deep}
	}
	var encoder CanonicalEncoder
	ctx := &canonicalEmissionCancelContext{encoder: &encoder}
	if encoded, err := encoder.Encode(ctx, deep); err != context.Canceled || encoded != nil {
		t.Fatalf("emission cancellation = %x, %v", encoded, err)
	}
	if !ctx.seenEmission {
		t.Fatal("cancellation did not reach an active emission frame")
	}
	assertCanonicalEncoderDetached(t, &encoder)

	want := mustCanonical(t, NewArray(String))
	got, err := encoder.Encode(context.Background(), NewArray(String))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("reused encoder after emission cancellation differs:\nwant %x\n got %x", want, got)
	}
	assertCanonicalEncoderDetached(t, &encoder)
}

func assertCanonicalEncoderDetached(t *testing.T, encoder *CanonicalEncoder) {
	t.Helper()
	if len(encoder.nodes) != 0 || len(encoder.discoveryStack) != 0 || len(encoder.sccFrames) != 0 || len(encoder.emissionStack) != 0 {
		t.Fatalf("encoder retained traversal stack: nodes=%d discovery=%d scc=%d emission=%d", len(encoder.nodes), len(encoder.discoveryStack), len(encoder.sccFrames), len(encoder.emissionStack))
	}
	if len(encoder.seen) != 0 || len(encoder.transparent) != 0 || len(encoder.recursiveID) != 0 || len(encoder.formals) != 0 || len(encoder.binders) != 0 {
		t.Fatalf("encoder retained graph maps: seen=%d transparent=%d recursive=%d formals=%d binders=%d", len(encoder.seen), len(encoder.transparent), len(encoder.recursiveID), len(encoder.formals), len(encoder.binders))
	}
	for _, node := range encoder.nodes[:cap(encoder.nodes)] {
		if node.typeParam != nil {
			t.Fatal("encoder retained TypeParam pointer in node scratch")
		}
	}
	for _, frame := range encoder.discoveryStack[:cap(encoder.discoveryStack)] {
		if frame.children != nil {
			t.Fatal("encoder retained child Type references in discovery scratch")
		}
	}
}

func BenchmarkCanonicalEncoder(b *testing.B) {
	value := canonicalMutualRecords()
	b.Run("fresh", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if encoded, err := EncodeCanonical(context.Background(), value); err != nil || len(encoded) == 0 {
				b.Fatal(err)
			}
		}
	})
	b.Run("reused", func(b *testing.B) {
		var encoder CanonicalEncoder
		b.ReportAllocs()
		for range b.N {
			if encoded, err := encoder.Encode(context.Background(), value); err != nil || len(encoded) == 0 {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCanonicalEncoderRecursiveChain(b *testing.B) {
	for _, size := range []int{64, 256, 1024} {
		b.Run(fmt.Sprintf("n%d", size), func(b *testing.B) {
			value := canonicalRecursiveChain(size)
			var encoder CanonicalEncoder
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if encoded, err := encoder.Encode(context.Background(), value); err != nil || len(encoded) == 0 {
					b.Fatal(err)
				}
			}
		})
	}
}

func canonicalSelfRecord(name string) *Recursive {
	return NewRecursive(name, func(self Type) Type {
		return RebuildRecord(RecordParts{Fields: []Field{
			{Name: "next", Type: MaterializeOptional(self), Optional: true},
			{Name: "value", Type: String},
		}})
	})
}

func canonicalMutualRecords() *Recursive {
	left := NewRecursivePlaceholder("Left")
	right := NewRecursivePlaceholder("Right")
	left.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "right", Type: right}, {Name: "value", Type: Number}}}))
	right.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "left", Type: left}, {Name: "value", Type: String}}}))
	return left
}

func canonicalRecursiveChain(size int) *Recursive {
	if size <= 0 {
		panic("canonical recursive chain must be non-empty")
	}
	nodes := make([]*Recursive, size)
	for index := range nodes {
		nodes[index] = NewRecursivePlaceholder("N")
	}
	nodes[len(nodes)-1].SetBody(Number)
	for index := len(nodes) - 2; index >= 0; index-- {
		nodes[index].SetBody(nodes[index+1])
	}
	return nodes[0]
}

func canonicalMixedDAGCycle(reverse bool) Type {
	var chain *Recursive
	var cycle *Recursive
	buildChain := func() { chain = canonicalRecursiveChain(64) }
	buildCycle := func() { cycle = canonicalMutualRecords() }
	if reverse {
		buildCycle()
		buildChain()
	} else {
		buildChain()
		buildCycle()
	}
	shared := NewArray(NewTuple(String, Number))
	return NewTuple(chain, cycle, shared, shared)
}

func mustCanonical(t *testing.T, value Type) []byte {
	t.Helper()
	encoded, err := EncodeCanonical(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

type canonicalCancelContext struct{ remaining int }

func (c *canonicalCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *canonicalCancelContext) Done() <-chan struct{}       { return nil }
func (c *canonicalCancelContext) Value(any) any               { return nil }
func (c *canonicalCancelContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

type canonicalEmissionCancelContext struct {
	encoder      *CanonicalEncoder
	seenEmission bool
}

func (c *canonicalEmissionCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *canonicalEmissionCancelContext) Done() <-chan struct{}       { return nil }
func (c *canonicalEmissionCancelContext) Value(any) any               { return nil }
func (c *canonicalEmissionCancelContext) Err() error {
	if c.encoder != nil && len(c.encoder.emissionStack) != 0 {
		c.seenEmission = true
		return context.Canceled
	}
	return nil
}
