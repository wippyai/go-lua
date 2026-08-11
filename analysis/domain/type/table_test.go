package typedomain

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestTablePreloadsStaticTypesInStableSelectorOrderAndReusesAuthoredOrigin(t *testing.T) {
	static := staticAuthority(t, `
type NilLabel = nil
type Shape = { name: string }
type SameShape = { name: string }
`)
	first, err := NewTable(static)
	if err != nil {
		t.Fatal(err)
	}
	shape := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "name", Type: typ.String}}})
	handle, err := first.DeriveClosed(shape)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := first.EncodeOrigin(handle)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := decodeOrigin(encoded)
	if err != nil || origin.kind != coldOriginAuthored {
		t.Fatalf("equal derived shape did not reuse first authored static origin: %#v / %v", origin, err)
	}
	if got, err := first.DeriveClosed(typ.Nil); err != nil || got != first.Nil() {
		t.Fatalf("static nil did not retain first authored label: %v / %v", got, err)
	}
	projected, err := first.Project(handle)
	if err != nil {
		t.Fatal(err)
	}
	alias, ok := projected.(*typ.Alias)
	if !ok {
		t.Fatalf("authored projection=%T", projected)
	}
	alias.Target = typ.Boolean
	fresh, err := first.Project(handle)
	if err != nil || !typ.TypeEquals(fresh, shape) {
		t.Fatalf("authored projection leaked authority ownership: %v / %v", fresh, err)
	}

	second, err := NewTable(static)
	if err != nil {
		t.Fatal(err)
	}
	again, err := second.DeriveClosed(shape)
	if err != nil {
		t.Fatal(err)
	}
	againBytes, err := second.EncodeOrigin(again)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(againBytes) {
		t.Fatalf("static preload order drifted\nfirst=%x\nsecond=%x", encoded, againBytes)
	}
}

func TestTableTypeTopHasDomainOnlyOriginAndStaticPolicyRemainsAuthored(t *testing.T) {
	static := staticAuthority(t, `
type DeclaredAny = any
type DeclaredUnknown = unknown
`)
	table, err := NewTable(static)
	if err != nil {
		t.Fatal(err)
	}
	anyHandle, err := table.DeriveClosed(typ.Any)
	if err != nil {
		t.Fatal(err)
	}
	unknownHandle, err := table.DeriveClosed(typ.Unknown)
	if err != nil {
		t.Fatal(err)
	}
	if anyHandle != table.TypeTop() || unknownHandle != table.TypeTop() {
		t.Fatalf("top-level gradual values did not collapse: any=%v unknown=%v top=%v", anyHandle, unknownHandle, table.TypeTop())
	}
	topOrigin, err := table.EncodeOrigin(table.TypeTop())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeOrigin(topOrigin)
	if err != nil || decoded.kind != coldOriginDomainTop || decoded.owner.Available() || decoded.root != 0 || len(decoded.canonical) != 0 {
		t.Fatalf("TypeTop fabricated a concrete type origin: %#v / %v", decoded, err)
	}
	if _, err := table.Project(table.TypeTop()); !errors.Is(err, ErrNoTypeProjection) {
		t.Fatalf("TypeTop projection=%v", err)
	}
	second, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	if restored, err := second.DecodeOrigin(topOrigin); err != nil || restored != second.TypeTop() {
		t.Fatalf("domain-top origin round trip=%v / %v", restored, err)
	}
	nested, err := table.DeriveClosed(typ.NewArray(typ.Any))
	if err != nil {
		t.Fatal(err)
	}
	if nested == table.TypeTop() {
		t.Fatal("nested any collapsed into top-level TypeTop")
	}
	if projected, err := table.Project(nested); err != nil || !typ.TypeEquals(projected, typ.NewArray(typ.Any)) {
		t.Fatalf("nested gradual projection=%v / %v", projected, err)
	}

	declared := make(map[string]typeauthority.StaticTypeRef)
	for index := 0; index < static.Count(); index++ {
		selector, ok := static.At(index)
		if !ok {
			t.Fatalf("static selector %d missing", index)
		}
		ref, ok := static.Ref(selector)
		if !ok {
			t.Fatalf("static ref %d missing", index)
		}
		if round, ok := static.Find(ref.Owner(), ref.Root()); !ok || round != selector {
			t.Fatalf("static policy ref did not round trip: %v / %v", round, ok)
		}
		value, ok := static.Resolve(ref)
		if !ok {
			t.Fatalf("static policy ref %v did not resolve", ref)
		}
		if alias, ok := value.(*typ.Alias); ok {
			declared[alias.Name] = ref
		}
	}
	for name, want := range map[string]typ.Type{"DeclaredAny": typ.Any, "DeclaredUnknown": typ.Unknown} {
		ref, ok := declared[name]
		if !ok {
			t.Fatalf("static declaration %q was not retained", name)
		}
		value, ok := static.Resolve(ref)
		if !ok || !typ.TypeEquals(value, want) {
			t.Fatalf("static declaration %q policy=%v / %v", name, value, ok)
		}
		if raw := encodeAuthoredOrigin(t, ref, originVersion); raw == nil {
			t.Fatal("test authored origin encoding failed")
		} else if _, err := table.DecodeOrigin(raw); !errors.Is(err, ErrInvalidOrigin) {
			t.Fatalf("authored top origin was accepted: %v", err)
		}
	}
	unknownBytes, err := typ.EncodeCanonical(context.Background(), typ.Unknown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := table.DecodeOrigin(encodeDerivedOrigin(t, unknownBytes, originVersion)); !errors.Is(err, ErrInvalidOrigin) {
		t.Fatalf("derived Unknown origin was accepted: %v", err)
	}
	if _, err := table.DecodeOrigin(encodeDerivedOrigin(t, unknownBytes, originVersion-1)); !errors.Is(err, ErrInvalidOrigin) {
		t.Fatalf("pre-domain-top artifact version was accepted: %v", err)
	}
	if _, err := table.DecodeOrigin(encodeMalformedTopOrigin(t)); !errors.Is(err, ErrInvalidOrigin) {
		t.Fatalf("domain-top record with invented payload was accepted: %v", err)
	}
}

func TestTableAdmitsEveryRawIEEEFloatLiteralBeforeSeal(t *testing.T) {
	table, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[uint64]Handle)
	for _, bits := range []uint64{
		0x0000000000000000,
		0x8000000000000000,
		0x7ff0000000000000,
		0xfff0000000000000,
		0x7ff8000000000001,
		0x7ff8000000000002,
		0x7ff0000000000001,
	} {
		handle, err := table.DeriveClosed(typ.LiteralNumber(math.Float64frombits(bits)))
		if err != nil {
			t.Fatalf("admit %#x: %v", bits, err)
		}
		seen[bits] = handle
	}
	if seen[0] == seen[0x8000000000000000] || seen[0x7ff8000000000001] == seen[0x7ff8000000000002] {
		t.Fatal("raw IEEE literal identities collapsed during cold admission")
	}
	table.Seal()
	for bits, handle := range seen {
		value, err := table.Project(handle)
		if err != nil {
			t.Fatalf("project %#x: %v", bits, err)
		}
		literal, ok := value.(*typ.Literal)
		if !ok || math.Float64bits(literal.Value.(float64)) != bits {
			t.Fatalf("project %#x = %#v", bits, value)
		}
	}
}

func TestTableRetainsNominalRecursiveAndGenericAuthoredOrigins(t *testing.T) {
	static := staticAuthority(t, `
type Node = Node?
type Box<T: string> = { value: T }
`)
	table, err := NewTable(static)
	if err != nil {
		t.Fatal(err)
	}
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type { return typeexpr.Optional(self) })
	nodeHandle, err := table.DeriveClosed(node)
	if err != nil {
		t.Fatal(err)
	}
	nodeOrigin, err := table.EncodeOrigin(nodeHandle)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := decodeOrigin(nodeOrigin); err != nil || decoded.kind != coldOriginAuthored {
		t.Fatalf("recursive type did not retain authored origin: %#v / %v", decoded, err)
	}
	projectedNode, err := table.Project(nodeHandle)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := projectedNode.(*typ.Recursive); !ok || value.Name != "Node" {
		t.Fatalf("recursive projection=%T/%v", projectedNode, projectedNode)
	}

	formal := typ.NewTypeParam("T", typ.String)
	box := typ.NewGeneric("Box", []*typ.TypeParam{formal}, typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "value", Type: formal}}}))
	derivedGeneric, err := table.DeriveClosed(box)
	if err != nil {
		t.Fatal(err)
	}
	if !table.Valid(derivedGeneric) {
		t.Fatal("closed generic was not retained")
	}
	genericOrigin, err := table.EncodeOrigin(derivedGeneric)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := decodeOrigin(genericOrigin); err != nil || decoded.kind != coldOriginAuthored {
		t.Fatalf("generic type did not retain authored origin: %#v / %v", decoded, err)
	}
	projectedGeneric, err := table.Project(derivedGeneric)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := projectedGeneric.(*typ.Generic); !ok {
		t.Fatalf("generic projection=%T", projectedGeneric)
	}
}

func TestTableManifestOriginReinternsWithoutLocalVocabulary(t *testing.T) {
	source, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := source.DeriveClosed(typ.NewArray(typ.String))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := source.EncodeOrigin(handle)
	if err != nil {
		t.Fatal(err)
	}
	source.Seal()

	target, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	reinterned, err := target.DecodeOrigin(encoded)
	if err != nil {
		t.Fatal(err)
	}
	target.Seal()
	projected, err := target.Project(reinterned)
	if err != nil || !typ.TypeEquals(projected, typ.NewArray(typ.String)) {
		t.Fatalf("reinterned projection=%v / %v", projected, err)
	}
	roundTrip, err := target.EncodeOrigin(reinterned)
	if err != nil || string(roundTrip) != string(encoded) {
		t.Fatalf("manifest chain changed origin: %x / %x / %v", encoded, roundTrip, err)
	}
	if again, err := target.DecodeOrigin(encoded); err != nil || again != reinterned {
		t.Fatalf("sealed re-intern changed local handle: %v / %v", again, err)
	}
}

func TestTableRejectsForeignOriginsOpenDerivedStateAndAlternateBytes(t *testing.T) {
	leftStatic := staticAuthority(t, `type Value = string`)
	left, err := NewTable(leftStatic)
	if err != nil {
		t.Fatal(err)
	}
	value, err := left.DeriveClosed(typ.String)
	if err != nil {
		t.Fatal(err)
	}
	authored, err := left.EncodeOrigin(value)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewTable(staticAuthority(t, `type Other = number`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := right.DecodeOrigin(authored); !errors.Is(err, ErrInvalidOrigin) {
		t.Fatalf("foreign authored origin=%v", err)
	}
	if right.Valid(left.Nil()) {
		t.Fatal("foreign handle accepted")
	}
	if _, err := right.Project(left.Nil()); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("foreign projection=%v", err)
	}

	formal := typ.NewTypeParam("T", nil)
	if _, err := right.DeriveClosed(typ.NewArray(formal)); !errors.Is(err, ErrOpenType) {
		t.Fatalf("open derived value=%v", err)
	}
	if _, err := right.DeriveClosed(&hostileType{label: "custom"}); !errors.Is(err, ErrOpenType) {
		t.Fatalf("custom derived value=%v", err)
	}

	derived, err := right.DeriveClosed(typ.NewArray(typ.Number))
	if err != nil {
		t.Fatal(err)
	}
	portable, err := right.EncodeOrigin(derived)
	if err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), portable...)
	bad[len(bad)-1] ^= 1
	receiver, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receiver.DecodeOrigin(bad); !errors.Is(err, ErrInvalidOrigin) && !errors.Is(err, ErrOpenType) {
		t.Fatalf("alternate origin bytes=%v", err)
	}
}

func TestTableOwnsMutableInputsAndPublicProjections(t *testing.T) {
	table, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	source := typ.NewArray(typ.String)
	handle, err := table.DeriveClosed(source)
	if err != nil {
		t.Fatal(err)
	}
	source.Element = typ.Number
	first, err := table.Project(handle)
	if err != nil {
		t.Fatal(err)
	}
	first.(*typ.Array).Element = typ.Boolean
	second, err := table.Project(handle)
	if err != nil || !typ.TypeEquals(second, typ.NewArray(typ.String)) {
		t.Fatalf("mutable input/projection leaked: %v / %v", second, err)
	}
}

func TestTableAdmitsClosedSelfRecursiveGenericOwnership(t *testing.T) {
	table, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	parameter := typ.NewTypeParam("T", typ.String)
	generic := typ.NewGeneric("Self", []*typ.TypeParam{parameter}, nil)
	generic.SetBody(typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{
		{Name: "value", Type: parameter},
		{Name: "next", Type: generic},
	}}))
	handle, err := table.DeriveClosed(generic)
	if err != nil {
		t.Fatalf("closed self-recursive generic rejected: %v", err)
	}
	origin, err := table.EncodeOrigin(handle)
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := receiver.DecodeOrigin(origin)
	if err != nil {
		t.Fatalf("self-recursive generic origin rejected: %v", err)
	}
	projected, err := receiver.Project(restored)
	if err != nil || !typ.TypeEquals(projected, generic) {
		t.Fatalf("self-recursive generic round trip=%v / %v", projected, err)
	}

	external := typ.NewTypeParam("External", nil)
	sibling := typ.NewGeneric("Sibling", []*typ.TypeParam{typ.NewTypeParam("U", nil)}, typ.NewTuple(external))
	if _, err := table.DeriveClosed(sibling); !errors.Is(err, ErrOpenType) {
		t.Fatalf("free formal admitted as closed generic: %v", err)
	}
}

func encodeAuthoredOrigin(t testing.TB, ref typeauthority.StaticTypeRef, version uint64) []byte {
	t.Helper()
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), originDomain, version); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(originAuthored); err != nil {
		t.Fatal(err)
	}
	owner := ref.Owner()
	if err := writer.Bytes(owner[:]); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(uint64(ref.Root())); err != nil {
		t.Fatal(err)
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func encodeDerivedOrigin(t testing.TB, value []byte, version uint64) []byte {
	t.Helper()
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), originDomain, version); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(originDerived); err != nil {
		t.Fatal(err)
	}
	if err := writer.Bytes(value); err != nil {
		t.Fatal(err)
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func encodeMalformedTopOrigin(t testing.TB) []byte {
	t.Helper()
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), originDomain, originVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(originTop); err != nil {
		t.Fatal(err)
	}
	if err := writer.Bytes([]byte{1}); err != nil {
		t.Fatal(err)
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestTableDeepRecursiveGenericAdmissionAndConcurrency(t *testing.T) {
	table, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	var deep typ.Type = typ.String
	for range 20_000 {
		deep = typ.NewArray(deep)
	}
	deepHandle, err := table.DeriveClosed(deep)
	if err != nil {
		t.Fatalf("deep closed derivation: %v", err)
	}
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type { return typ.NewArray(self) })
	recursiveHandle, err := table.DeriveClosed(recursive)
	if err != nil {
		t.Fatalf("recursive closed derivation: %v", err)
	}
	table.Seal()

	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 64 {
				if !table.Valid(deepHandle) || table.Hash(recursiveHandle) == 0 {
					t.Error("sealed table rejected own handle")
					return
				}
				if _, err := table.Project(recursiveHandle); err != nil {
					t.Errorf("concurrent cold projection: %v", err)
					return
				}
			}
		}()
	}
	group.Wait()
}

func TestTableSealPublishesHotHandlesSafely(t *testing.T) {
	table, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	value, err := table.DeriveClosed(typ.NewArray(typ.String))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			for range 256 {
				if !table.Valid(value) || !table.Equal(value, value) || table.Hash(value) == 0 {
					t.Error("published handle operation failed")
					return
				}
			}
		}()
	}
	close(start)
	table.Seal()
	group.Wait()
	if !table.Sealed() {
		t.Fatal("table did not publish its sealed view")
	}
	if _, err := table.DeriveClosed(typ.Boolean); !errors.Is(err, ErrSealed) {
		t.Fatalf("post-publication mutation=%v", err)
	}
}

func TestTableHandlesAreOneWordAndHotOperationsDoNoCodecWork(t *testing.T) {
	if got, want := unsafe.Sizeof(Handle{}), unsafe.Sizeof(uint64(0)); got != want {
		t.Fatalf("handle size=%d, want one word=%d", got, want)
	}
	table, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	table.Seal()
	value := table.Nil()
	if allocations := testing.AllocsPerRun(1000, func() {
		if !table.Valid(value) || !table.Equal(value, value) {
			t.Fatal("hot label operation failed")
		}
		_ = table.Hash(value)
	}); allocations != 0 {
		t.Fatalf("hot table labels allocated: %g", allocations)
	}
}

func BenchmarkTableSealedHotHandles(b *testing.B) {
	table, err := NewTable(nil)
	if err != nil {
		b.Fatal(err)
	}
	value, err := table.DeriveClosed(typ.NewArray(typ.String))
	if err != nil {
		b.Fatal(err)
	}
	table.Seal()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			if !table.Valid(value) || !table.Equal(value, value) || table.Hash(value) == 0 {
				b.Fatal("sealed hot label operation failed")
			}
		}
	})
}

func staticAuthority(t testing.TB, source string) *typeauthority.Authority {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "table.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "table", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	authority, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("static type authority did not seal")
	}
	return authority
}

type hostileType struct{ label string }

func (*hostileType) Kind() kind.Kind  { return kind.Record }
func (h *hostileType) String() string { return h.label }
func (*hostileType) Hash() uint64     { return 1 }
func (h *hostileType) Equals(other typ.Type) bool {
	otherHostile, ok := other.(*hostileType)
	return ok && otherHostile.label == h.label
}
