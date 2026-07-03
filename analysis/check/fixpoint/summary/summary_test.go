package summary

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestSummaryKeyComparableAndDeterministicOrdering(t *testing.T) {
	a := ref.FuncRef{Kind: ref.KindCFG, ID: 1}
	b := ref.FuncRef{Kind: ref.KindSymbol, ID: 1}
	keys := []SummaryKey{
		{Ref: b},
		{Ref: a, Entry: EntryKey{Values: 2}},
		{Ref: a, Entry: EntryKey{Values: 1, References: 2}},
		{Ref: a},
		{Ref: a, Entry: EntryKey{Values: 1, Facts: 2}},
		{Ref: a, Entry: EntryKey{Values: 1, Facts: 1}},
	}
	seen := map[SummaryKey]string{DefaultSummaryKey(a): "default"}
	if seen[SummaryKey{Ref: a}] != "default" {
		t.Fatalf("SummaryKey is not usable as expected map key")
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	want := []SummaryKey{
		{Ref: a},
		{Ref: a, Entry: EntryKey{Values: 1, References: 2}},
		{Ref: a, Entry: EntryKey{Values: 1, Facts: 1}},
		{Ref: a, Entry: EntryKey{Values: 1, Facts: 2}},
		{Ref: a, Entry: EntryKey{Values: 2}},
		{Ref: b},
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys[%d] = %#v, want %#v", i, keys[i], want[i])
		}
	}
}

func TestSummaryKeyAxesAreDistinct(t *testing.T) {
	fn := ref.FuncRef{Kind: ref.KindCFG, ID: 1}
	values := SummaryKey{Ref: fn, Entry: EntryKey{Values: 1}}
	facts := SummaryKey{Ref: fn, Entry: EntryKey{Facts: 1}}
	references := SummaryKey{Ref: fn, Entry: EntryKey{References: 1}}

	if values == facts {
		t.Fatalf("Values and Facts keys should be distinct")
	}
	if values == references {
		t.Fatalf("Values and References keys should be distinct")
	}
	if facts == references {
		t.Fatalf("Facts and References keys should be distinct")
	}
}

func TestSnapshotExactReads(t *testing.T) {
	reg := mustRegistry(t)
	fn := ref.FuncRef{Kind: ref.KindCFG, ID: 7}
	exact := SummaryKey{Ref: fn, Entry: EntryKey{Values: 1, Facts: 2}}
	want := Summary{Returns: []product.Value{product.Top()}}
	snap := NewSnapshot(reg, EntrySummary{Key: exact, Summary: want})

	got, ok := snap.Read(exact)
	if !ok {
		t.Fatalf("Read(exact) missing")
	}
	if len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Read(exact) = %#v, want one top return", got)
	}
}

func TestSnapshotExactReadsDoNotFallbackByRef(t *testing.T) {
	reg := mustRegistry(t)
	fn := ref.FuncRef{Kind: ref.KindCFG, ID: 7}
	snap := NewSnapshot(reg, EntrySummary{
		Key:     SummaryKey{Ref: fn, Entry: EntryKey{Values: 1}},
		Summary: Summary{Returns: []product.Value{product.Top()}},
	})

	if got, ok := snap.Read(DefaultSummaryKey(fn)); ok {
		t.Fatalf("Read(default same ref) = %#v, want missing exact key", got)
	}
}

func TestSnapshotReadsNormalizedSummaries(t *testing.T) {
	reg := mustRegistry(t)
	key := DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 11})
	snap := NewSnapshot(reg, EntrySummary{
		Key: key,
		Summary: Summary{Returns: []product.Value{
			product.Top(),
			product.Bottom(reg),
			product.Bottom(reg),
		}},
	})

	got, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("Read(key) returned %d returns, want normalized 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Read(key) first return = %#v, want top", got.Returns[0])
	}
}

func TestNormalizedDomainEqualMatchesDefensiveEqualForNormalizedSummaries(t *testing.T) {
	reg := mustRegistry(t)
	p0 := path.NewPlaceholder(0).Field("ready")
	value := presentProduct(reg)
	raw := Summary{
		Returns: []product.Value{value, product.Bottom(reg)},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathRefinements: []callboundary.PathValueFact{
				{Path: p0, Value: value},
				{Path: p0.Clone(), Value: product.Bottom(reg)},
			},
			NumFloors: []callboundary.NumFloorFact{
				{Path: p0.Clone(), Floor: 1},
				{Path: p0.Clone(), Floor: 3},
			},
		},
	}
	normalized := Normalize(reg, raw)
	cloned := normalized.Clone()

	if !Equal(reg, normalized, cloned) {
		t.Fatalf("defensive Equal rejected cloned normalized summary")
	}
	if !NormalizedDomain(reg).Equal(normalized, cloned) {
		t.Fatalf("NormalizedDomain Equal rejected cloned normalized summary")
	}
}

func TestNewSnapshotOwnedNormalizedKeepsCallerNormalizedEntry(t *testing.T) {
	reg := mustRegistry(t)
	key := DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 13})
	normalized := Normalize(reg, Summary{Returns: []product.Value{
		product.Top(),
		product.Bottom(reg),
	}})
	snap := NewSnapshotOwnedNormalized(reg, EntrySummary{Key: key, Summary: normalized})

	got, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("Read(key) returned %d returns, want owned normalized 1", len(got.Returns))
	}
	got.Returns[0] = product.Bottom(reg)
	again, ok := snap.Read(key)
	if !ok {
		t.Fatalf("second Read(key) missing")
	}
	if len(again.Returns) != 1 || !product.Equal(reg, again.Returns[0], product.Top()) {
		t.Fatalf("snapshot read did not clone stored summary")
	}
}

func TestSnapshotOwnedNormalizedReadsShareStoredSummary(t *testing.T) {
	reg := mustRegistry(t)
	key := DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 14})
	normalized := Normalize(reg, Summary{Returns: []product.Value{
		product.Top(),
		product.Bottom(reg),
	}})
	snap := NewSnapshotOwnedNormalized(reg, EntrySummary{Key: key, Summary: normalized})

	got, ok := snap.ReadOwnedNormalized(key)
	if !ok {
		t.Fatalf("ReadOwnedNormalized(key) missing")
	}
	if len(got.Returns) != 1 || &got.Returns[0] != &normalized.Returns[0] {
		t.Fatalf("ReadOwnedNormalized did not return stored normalized summary")
	}

	entries := snap.EntriesOwnedNormalized()
	if len(entries) != 1 {
		t.Fatalf("EntriesOwnedNormalized length = %d, want 1", len(entries))
	}
	if len(entries[0].Summary.Returns) != 1 || &entries[0].Summary.Returns[0] != &normalized.Returns[0] {
		t.Fatalf("EntriesOwnedNormalized did not return stored normalized summary")
	}
}

func TestSnapshotNormalizesWithCustomRegistry(t *testing.T) {
	reg, err := product.RegistryWithAxes(summaryTestSpec().Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes() error = %v", err)
	}
	key := DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 12})
	value := product.Set(reg, product.Top(), summaryTestKey, summaryTestLow)
	snap := NewSnapshot(reg, EntrySummary{
		Key: key,
		Summary: Summary{Returns: []product.Value{
			value,
			product.Bottom(reg),
		}},
	})

	got, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("Read(key) returned %d returns, want normalized 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], value) {
		t.Fatalf("Read(key) first return was not preserved under custom registry")
	}
}

func TestSummaryCloneIsolatesReturns(t *testing.T) {
	reg := mustRegistry(t)
	original := Summary{Returns: []product.Value{product.Top(), product.Absent(reg)}}
	cloned := original.Clone()
	cloned.Returns[0] = product.Bottom(reg)

	if product.Equal(reg, original.Returns[0], product.Bottom(reg)) {
		t.Fatalf("mutating cloned returns changed original")
	}
	if !product.Equal(reg, original.Returns[0], product.Top()) {
		t.Fatalf("original first return changed unexpectedly")
	}
}

func TestSummaryCloneIsolatesNormalReturnParams(t *testing.T) {
	reg := mustRegistry(t)
	original := Summary{NormalReturnParams: []product.Value{product.Top(), product.Absent(reg)}}
	cloned := original.Clone()
	cloned.NormalReturnParams[0] = product.Bottom(reg)

	if product.Equal(reg, original.NormalReturnParams[0], product.Bottom(reg)) {
		t.Fatalf("mutating cloned normal return params changed original")
	}
	if !product.Equal(reg, original.NormalReturnParams[0], product.Top()) {
		t.Fatalf("original first normal return param changed unexpectedly")
	}
}

func TestSummaryCloneIsolatesHeapTableObjects(t *testing.T) {
	reg := mustRegistry(t)
	id := identity.ID{Kind: "table", Site: "summary-clone", Index: 1}
	ks := keyspace.New()
	member, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "name"}})
	if !ok {
		t.Fatal("member suffix key failed")
	}
	original := Summary{
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root:          product.Top(),
				StaticMembers: map[keyspace.Key]product.Value{member: product.Absent(reg)},
			}),
		},
	}

	cloned := original.Clone()
	object := cloned.HeapTableObjects[id]
	static := object.StaticMembers()
	static[member] = product.Top()
	cloned.HeapTableObjects[id] = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          product.Bottom(reg),
		StaticMembers: static,
	})

	got := original.HeapTableObjects[id]
	if !product.Equal(reg, got.Root(), product.Top()) {
		t.Fatalf("original heap object root changed unexpectedly")
	}
	if memberValue, ok := got.StaticMember(member); !ok || !product.Equal(reg, memberValue, product.Absent(reg)) {
		t.Fatalf("original heap object static member changed: %v/%v", memberValue, ok)
	}
}

func TestSnapshotClonesOnWriteAndRead(t *testing.T) {
	reg := mustRegistry(t)
	key := DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 9})
	input := Summary{Returns: []product.Value{product.Top()}}
	snap := NewSnapshot(reg, EntrySummary{Key: key, Summary: input})
	input.Returns[0] = product.Bottom(reg)

	first, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if !product.Equal(reg, first.Returns[0], product.Top()) {
		t.Fatalf("snapshot changed after input mutation")
	}

	first.Returns[0] = product.Bottom(reg)
	second, ok := snap.Read(key)
	if !ok {
		t.Fatalf("second Read(key) missing")
	}
	if !product.Equal(reg, second.Returns[0], product.Top()) {
		t.Fatalf("snapshot changed after read result mutation")
	}
}

func TestSummaryHeapTableObjectsNormalizeAndJoinByIdentity(t *testing.T) {
	reg := mustRegistry(t)
	id := identity.ID{Kind: "table", Site: "summary-join", Index: 1}
	left := Summary{
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
		},
	}
	right := Summary{
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Absent(reg)}),
		},
	}

	normalized := Normalize(reg, Summary{
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.BottomObject(reg),
		},
	})
	if len(normalized.HeapTableObjects) != 0 {
		t.Fatalf("Normalize kept bottom heap table object: %#v", normalized.HeapTableObjects)
	}

	joined := Join(reg, left, right)
	object, ok := joined.HeapTableObjects[id]
	if !ok {
		t.Fatalf("Join dropped shared heap object identity")
	}
	if !product.Equal(reg, object.Root(), product.Top()) {
		t.Fatalf("joined heap object root = %#v, want top", object.Root())
	}
	if !LessOrEq(reg, right, joined) || !Equal(reg, Normalize(reg, Summary{}), Summary{}) {
		t.Fatalf("heap-object summary lattice relations failed")
	}
}

func TestSummaryJoinRekeysHeapTableObjectsAcrossKeySpaces(t *testing.T) {
	reg := mustRegistry(t)
	id := identity.ID{Kind: "table", Site: "summary-join-keyspace", Index: 1}
	leftKS := keyspace.New()
	rightKS := keyspace.New()
	// Force different dense segment ids for the same structural member spelling
	// in the two keyspaces. The join must use structural rekeying, not raw ids.
	if _, ok := rightKS.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "padding"}}); !ok {
		t.Fatal("right padding suffix failed")
	}
	leftMember, ok := leftKS.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "name"}})
	if !ok {
		t.Fatal("left member suffix failed")
	}
	rightMember, ok := rightKS.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "name"}})
	if !ok {
		t.Fatal("right member suffix failed")
	}
	if leftMember.Segs == rightMember.Segs {
		t.Fatalf("test setup did not produce distinct segment ids: %v", leftMember.Segs)
	}

	left := Summary{
		HeapKeySpace: leftKS,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root:          product.Top(),
				StaticMembers: map[keyspace.Key]product.Value{leftMember: product.Top()},
			}),
		},
	}
	right := Summary{
		HeapKeySpace: rightKS,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root:          product.Top(),
				StaticMembers: map[keyspace.Key]product.Value{rightMember: product.Top()},
			}),
		},
	}

	joined := Join(reg, left, right)
	if joined.HeapKeySpace != leftKS {
		t.Fatalf("joined HeapKeySpace = %p, want left keyspace %p", joined.HeapKeySpace, leftKS)
	}
	object, ok := joined.HeapTableObjects[id]
	if !ok {
		t.Fatalf("Join dropped heap object %v", id)
	}
	for key := range object.StaticMembers() {
		if got := joined.HeapKeySpace.Format(key); got != ".name" {
			t.Fatalf("joined static member key = %q, want .name", got)
		}
	}

	target := keyspace.New()
	rekeyed := joined.RekeyHeapTableObjects(target)
	object, ok = rekeyed.HeapTableObjects[id]
	if !ok {
		t.Fatalf("RekeyHeapTableObjects dropped heap object %v", id)
	}
	if _, ok := object.StaticMember(mustRootlessSuffix(t, target, "name")); !ok {
		t.Fatalf("rekeyed heap object members = %#v, want .name in target keyspace", object.StaticMembers())
	}
}

func TestNormalizeDefensivelyCopiesHeapTableObjects(t *testing.T) {
	reg := mustRegistry(t)
	id := identity.ID{Kind: "table", Site: "summary-normalize-copy", Index: 1}
	input := Summary{
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
		},
	}

	normalized := Normalize(reg, input)
	delete(normalized.HeapTableObjects, id)

	if _, ok := input.HeapTableObjects[id]; !ok {
		t.Fatalf("Normalize returned heap map aliasing input")
	}
}

func TestNormalizeOwnedConsumesHeapTableObjectMap(t *testing.T) {
	reg := mustRegistry(t)
	id := identity.ID{Kind: "table", Site: "summary-normalize-owned", Index: 1}
	input := Summary{
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.BottomObject(reg),
		},
	}

	normalized := NormalizeOwned(reg, input)

	if len(normalized.HeapTableObjects) != 0 {
		t.Fatalf("NormalizeOwned kept bottom heap table object: %#v", normalized.HeapTableObjects)
	}
	if len(input.HeapTableObjects) != 0 {
		t.Fatalf("NormalizeOwned did not consume caller-owned heap map: %#v", input.HeapTableObjects)
	}
}

func mustRootlessSuffix(t *testing.T, ks *keyspace.KeySpace, name string) keyspace.Key {
	t.Helper()
	key, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: name}})
	if !ok {
		t.Fatalf("FromRootlessSuffix(%q) failed", name)
	}
	return key
}

func TestParamMemberReturnSlotsAreMustFacts(t *testing.T) {
	reg := standard.Registry()
	field := func(name string) segment.Segment {
		return segment.Segment{Kind: segment.SegmentField, Name: name}
	}
	fetchValue := ParamMemberReturnSlot{
		ReceiverParam:     0,
		Member:            field("fetch"),
		ReturnIndex:       0,
		MemberResultIndex: 0,
	}
	fetchError := ParamMemberReturnSlot{
		ReceiverParam:     0,
		Member:            field("fetch"),
		ReturnIndex:       1,
		MemberResultIndex: 1,
	}
	metaValue := ParamMemberReturnSlot{
		ReceiverParam:     1,
		Member:            field("meta"),
		ReturnIndex:       0,
		MemberResultIndex: 0,
	}
	left := Summary{
		Returns:                []product.Value{product.Top()},
		ParamMemberReturnSlots: []ParamMemberReturnSlot{fetchError, fetchValue, metaValue},
	}
	right := Summary{
		Returns:                []product.Value{product.Top()},
		ParamMemberReturnSlots: []ParamMemberReturnSlot{fetchValue},
	}
	withoutSlots := Summary{Returns: []product.Value{product.Top()}}

	normalized := Normalize(reg, Summary{
		Returns: []product.Value{product.Top()},
		ParamMemberReturnSlots: []ParamMemberReturnSlot{
			{ReceiverParam: -1, Member: field("ignored"), ReturnIndex: 0, MemberResultIndex: 0},
			fetchError,
			fetchValue,
			fetchValue,
		},
	})
	if len(normalized.ParamMemberReturnSlots) != 2 ||
		normalized.ParamMemberReturnSlots[0] != fetchValue ||
		normalized.ParamMemberReturnSlots[1] != fetchError {
		t.Fatalf("Normalize ParamMemberReturnSlots = %#v, want sorted unique valid fetch slots", normalized.ParamMemberReturnSlots)
	}

	joined := Join(reg, left, right)
	if len(joined.ParamMemberReturnSlots) != 1 || joined.ParamMemberReturnSlots[0] != fetchValue {
		t.Fatalf("Join ParamMemberReturnSlots = %#v, want only common must slot %#v", joined.ParamMemberReturnSlots, fetchValue)
	}
	widened := Widen(reg, left, right)
	if len(widened.ParamMemberReturnSlots) != 1 || widened.ParamMemberReturnSlots[0] != fetchValue {
		t.Fatalf("Widen ParamMemberReturnSlots = %#v, want only common must slot %#v", widened.ParamMemberReturnSlots, fetchValue)
	}
	dropped := Join(reg, left, withoutSlots)
	if len(dropped.ParamMemberReturnSlots) != 0 {
		t.Fatalf("Join kept branch-local ParamMemberReturnSlots = %#v", dropped.ParamMemberReturnSlots)
	}
	if !LessOrEq(reg, left, right) {
		t.Fatalf("summary with more must ParamMemberReturnSlots should be <= summary with fewer")
	}
	if LessOrEq(reg, right, left) {
		t.Fatalf("summary with fewer must ParamMemberReturnSlots should not be <= summary with more")
	}
	if !LessOrEq(reg, left, withoutSlots) || LessOrEq(reg, withoutSlots, left) {
		t.Fatalf("ParamMemberReturnSlots must use reverse-inclusion order")
	}
}

func TestParamMemberSummaryFactsAcceptExactBracketMembers(t *testing.T) {
	reg := standard.Registry()
	field := segment.Segment{Kind: segment.SegmentField, Name: "send"}
	stringIndex := segment.Segment{Kind: segment.SegmentIndexString, Name: "send"}
	intIndex := segment.Segment{Kind: segment.SegmentIndexInt, Index: 1}
	invalidEmptyStringIndex := segment.Segment{Kind: segment.SegmentIndexString}
	const invalidReceiverPath = "client"

	callField := ParamMemberCallObligation{ReceiverParam: 0, Member: field, ArgParam: 1, MemberParamIndex: 2}
	callString := ParamMemberCallObligation{ReceiverParam: 0, Member: stringIndex, ArgParam: 1, MemberParamIndex: 2}
	callInt := ParamMemberCallObligation{ReceiverParam: 0, Member: intIndex, ArgParam: 1, MemberParamIndex: 2}
	callNested := ParamMemberCallObligation{ReceiverParam: 0, ReceiverPath: ".client", Member: field, ArgParam: 1, MemberParamIndex: 2}
	normalizedCalls := Normalize(reg, Summary{
		ParamMemberCallObligations: []ParamMemberCallObligation{
			{ReceiverParam: -1, Member: field, ArgParam: 1, MemberParamIndex: 2},
			{ReceiverParam: 0, Member: invalidEmptyStringIndex, ArgParam: 1, MemberParamIndex: 2},
			{ReceiverParam: 0, ReceiverPath: invalidReceiverPath, Member: field, ArgParam: 1, MemberParamIndex: 2},
			callInt,
			callString,
			callField,
			callNested,
			callString,
		},
	})
	if got := normalizedCalls.ParamMemberCallObligations; len(got) != 4 ||
		got[0] != callField ||
		got[1] != callString ||
		got[2] != callInt ||
		got[3] != callNested {
		t.Fatalf("Normalize ParamMemberCallObligations = %#v, want field, string-index, int-index, nested field", got)
	}
	joinedCalls := Join(reg,
		Summary{ParamMemberCallObligations: []ParamMemberCallObligation{callField}},
		Summary{ParamMemberCallObligations: []ParamMemberCallObligation{callInt}},
	)
	if got := joinedCalls.ParamMemberCallObligations; len(got) != 2 || got[0] != callField || got[1] != callInt {
		t.Fatalf("Join ParamMemberCallObligations = %#v, want may-union of field and int-index", got)
	}

	slotField := ParamMemberReturnSlot{ReceiverParam: 0, Member: field, ReturnIndex: 0, MemberResultIndex: 0}
	slotString := ParamMemberReturnSlot{ReceiverParam: 0, Member: stringIndex, ReturnIndex: 0, MemberResultIndex: 0}
	slotInt := ParamMemberReturnSlot{ReceiverParam: 0, Member: intIndex, ReturnIndex: 0, MemberResultIndex: 0}
	normalizedSlots := Normalize(reg, Summary{
		Returns: []product.Value{product.Top()},
		ParamMemberReturnSlots: []ParamMemberReturnSlot{
			{ReceiverParam: 0, Member: invalidEmptyStringIndex, ReturnIndex: 0, MemberResultIndex: 0},
			slotInt,
			slotString,
			slotField,
			slotString,
		},
	})
	if got := normalizedSlots.ParamMemberReturnSlots; len(got) != 3 ||
		got[0] != slotField ||
		got[1] != slotString ||
		got[2] != slotInt {
		t.Fatalf("Normalize ParamMemberReturnSlots = %#v, want field, string-index, int-index", got)
	}
	joinedSlots := Join(reg,
		Summary{
			Returns:                []product.Value{product.Top()},
			ParamMemberReturnSlots: []ParamMemberReturnSlot{slotField, slotInt},
		},
		Summary{
			Returns:                []product.Value{product.Top()},
			ParamMemberReturnSlots: []ParamMemberReturnSlot{slotInt},
		},
	)
	if got := joinedSlots.ParamMemberReturnSlots; len(got) != 1 || got[0] != slotInt {
		t.Fatalf("Join ParamMemberReturnSlots = %#v, want must-intersection of int-index slot", got)
	}
}

func TestReturnParamPathAliasesAreMustFacts(t *testing.T) {
	reg := standard.Registry()
	apiSource := mustPlaceholderKey(t, path.PathKey("$0.registry"))
	backupSource := mustPlaceholderKey(t, path.PathKey("$0.registry.backup"))
	apiAlias := ReturnParamPathAlias{
		ReturnIndex: 0,
		Member:      ".api",
		Source:      apiSource,
	}
	backupAlias := ReturnParamPathAlias{
		ReturnIndex: 0,
		Member:      ".api.backup",
		Source:      backupSource,
	}
	left := Summary{
		Returns:                []product.Value{product.Top()},
		ReturnParamPathAliases: []ReturnParamPathAlias{backupAlias, apiAlias},
	}
	right := Summary{
		Returns:                []product.Value{product.Top()},
		ReturnParamPathAliases: []ReturnParamPathAlias{apiAlias},
	}
	withoutAliases := Summary{Returns: []product.Value{product.Top()}}

	normalized := Normalize(reg, Summary{
		Returns: []product.Value{product.Top()},
		ReturnParamPathAliases: []ReturnParamPathAlias{
			{ReturnIndex: -1, Member: ".ignored", Source: mustPlaceholderKey(t, path.PathKey("$0"))},
			apiAlias,
			apiAlias,
			backupAlias,
		},
	})
	if len(normalized.ReturnParamPathAliases) != 2 ||
		normalized.ReturnParamPathAliases[0] != apiAlias ||
		normalized.ReturnParamPathAliases[1] != backupAlias {
		t.Fatalf("Normalize ReturnParamPathAliases = %#v, want sorted unique aliases", normalized.ReturnParamPathAliases)
	}

	joined := Join(reg, left, right)
	if len(joined.ReturnParamPathAliases) != 1 || joined.ReturnParamPathAliases[0] != apiAlias {
		t.Fatalf("Join ReturnParamPathAliases = %#v, want only common must alias %#v", joined.ReturnParamPathAliases, apiAlias)
	}
	widened := Widen(reg, left, right)
	if len(widened.ReturnParamPathAliases) != 1 || widened.ReturnParamPathAliases[0] != apiAlias {
		t.Fatalf("Widen ReturnParamPathAliases = %#v, want only common must alias %#v", widened.ReturnParamPathAliases, apiAlias)
	}
	dropped := Join(reg, left, withoutAliases)
	if len(dropped.ReturnParamPathAliases) != 0 {
		t.Fatalf("Join kept branch-local ReturnParamPathAliases = %#v", dropped.ReturnParamPathAliases)
	}
	if !LessOrEq(reg, left, right) {
		t.Fatalf("summary with more must ReturnParamPathAliases should be <= summary with fewer")
	}
	if LessOrEq(reg, right, left) {
		t.Fatalf("summary with fewer must ReturnParamPathAliases should not be <= summary with more")
	}
	if !LessOrEq(reg, left, withoutAliases) || LessOrEq(reg, withoutAliases, left) {
		t.Fatalf("ReturnParamPathAliases must use reverse-inclusion order")
	}
}

func mustPlaceholderKey(t *testing.T, key path.PathKey) pathaddr.PlaceholderKey {
	t.Helper()
	got, ok := pathaddr.PlaceholderKeyFromPathKey(key)
	if !ok {
		t.Fatalf("PlaceholderKeyFromPathKey(%q) failed", key)
	}
	return got
}

func TestParamSinkExposuresAreRootPlaceholderMayFacts(t *testing.T) {
	reg := standard.Registry()
	source0, ok := pathaddr.RootPlaceholderKeyForIndex(0)
	if !ok {
		t.Fatal("RootPlaceholderKeyForIndex(0) failed")
	}
	source1, ok := pathaddr.RootPlaceholderKeyForIndex(1)
	if !ok {
		t.Fatal("RootPlaceholderKeyForIndex(1) failed")
	}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)
	wantContract := product.Join(reg, present, absent)

	normalized := Normalize(reg, Summary{
		ParamSinkExposures: []ParamSinkExposure{
			{Source: pathaddr.RootPlaceholderKey(path.PathKey("$2.member")), Contract: present},
			{Source: source1, Contract: product.Top()},
			{Source: source1, Contract: product.Bottom(reg)},
			{Source: source0, Contract: present},
			{Source: source0, Contract: absent},
		},
	})

	if got := normalized.ParamSinkExposures; len(got) != 1 ||
		got[0].Source != source0 ||
		!product.Equal(reg, got[0].Contract, wantContract) {
		t.Fatalf("Normalize ParamSinkExposures = %#v, want one source0 exposure with joined contract", got)
	}

	joined := Join(reg,
		Summary{ParamSinkExposures: []ParamSinkExposure{{Source: source0, Contract: present}}},
		Summary{ParamSinkExposures: []ParamSinkExposure{{Source: source1, Contract: absent}}},
	)
	if got := joined.ParamSinkExposures; len(got) != 2 || got[0].Source != source0 || got[1].Source != source1 {
		t.Fatalf("Join ParamSinkExposures = %#v, want both may exposures sorted by source", got)
	}
}

type summaryTestAxis uint8

const (
	summaryTestBottom summaryTestAxis = iota
	summaryTestLow
	summaryTestHigh
	summaryTestTop
)

var summaryTestKey = axis.NewKey[summaryTestAxis]("test.summary.axis")

func summaryTestSpec() axis.Spec[summaryTestAxis] {
	return axis.Spec[summaryTestAxis]{
		Key:    summaryTestKey,
		Bottom: func() summaryTestAxis { return summaryTestBottom },
		Top:    func() summaryTestAxis { return summaryTestTop },
		Equal:  func(a, b summaryTestAxis) bool { return a == b },
		LessOrEq: func(a, b summaryTestAxis) bool {
			return a <= b
		},
		Join: func(a, b summaryTestAxis) summaryTestAxis {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b summaryTestAxis) summaryTestAxis {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next summaryTestAxis) summaryTestAxis {
			if prev > next {
				return prev
			}
			return next
		},
		Hash: func(v summaryTestAxis) uint64 { return uint64(v) },
	}
}

func TestEqualTreatsAbsentReturnSlotAsBottom(t *testing.T) {
	reg := mustRegistry(t)
	empty := Summary{}
	explicitBottom := Summary{Returns: []product.Value{product.Bottom(reg)}}

	if !Equal(reg, empty, explicitBottom) {
		t.Fatalf("missing return slot should equal explicit bottom")
	}
	if !Equal(reg, explicitBottom, empty) {
		t.Fatalf("explicit bottom should equal missing return slot")
	}
}

func TestJoinWithMissingReturnSlot(t *testing.T) {
	reg := mustRegistry(t)
	got := Join(reg, Summary{}, Summary{Returns: []product.Value{product.Top()}})
	if len(got.Returns) != 1 {
		t.Fatalf("Join returned %d slots, want 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Join missing slot with top = %#v, want top", got.Returns[0])
	}
}

func TestJoinReturnSlotsPreservesNilRecordUnionWitness(t *testing.T) {
	reg := standard.Registry()
	record := typetable.NewRecord().Field("answer", typ.String).Build()
	recordValue := typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)

	got := Join(reg,
		Summary{Returns: []product.Value{product.Absent(reg)}},
		Summary{Returns: []product.Value{recordValue}},
	)
	if len(got.Returns) != 1 {
		t.Fatalf("Join returned %d slots, want 1", len(got.Returns))
	}
	gotType, ok := typevalue.TypeOf(reg, got.Returns[0])
	wantType := typenormalize.Optional(record)
	if !ok || !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("joined return type = %v/%v, want %v", gotType, ok, wantType)
	}
	if gotPresence := product.PresenceOf(got.Returns[0]); !presence.Equal(gotPresence, presence.Maybe()) {
		t.Fatalf("joined return presence = %s, want maybe", gotPresence)
	}
}

func TestWidenReturnSlotsPreservesNilRecordUnionWitness(t *testing.T) {
	reg := standard.Registry()
	record := typetable.NewRecord().Field("answer", typ.String).Build()
	recordValue := typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)

	got := Widen(reg,
		Summary{Returns: []product.Value{product.Absent(reg)}},
		Summary{Returns: []product.Value{recordValue}},
	)
	if len(got.Returns) != 1 {
		t.Fatalf("Widen returned %d slots, want 1", len(got.Returns))
	}
	gotType, ok := typevalue.TypeOf(reg, got.Returns[0])
	wantType := typenormalize.Optional(record)
	if !ok || !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("widened return type = %v/%v, want %v", gotType, ok, wantType)
	}
	if gotPresence := product.PresenceOf(got.Returns[0]); !presence.Equal(gotPresence, presence.Maybe()) {
		t.Fatalf("widened return presence = %s, want maybe", gotPresence)
	}
}

func TestNormalizeTrimsTrailingBottomReturnSlots(t *testing.T) {
	reg := mustRegistry(t)
	s := Summary{
		Returns: []product.Value{
			product.Top(),
			product.Bottom(reg),
			product.Bottom(reg),
		},
	}
	got := Normalize(reg, s)
	if len(got.Returns) != 1 {
		t.Fatalf("Normalize kept %d returns, want 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Normalize first return = %#v, want top", got.Returns[0])
	}

	allBottom := Normalize(reg, Summary{Returns: []product.Value{product.Bottom(reg)}})
	if len(allBottom.Returns) != 0 {
		t.Fatalf("Normalize(all bottom) kept %d returns, want 0", len(allBottom.Returns))
	}
}

func TestNormalizeTrimsTrailingBottomNormalReturnParams(t *testing.T) {
	reg := mustRegistry(t)
	s := Summary{
		NormalReturnParams: []product.Value{
			product.Top(),
			product.Bottom(reg),
			product.Bottom(reg),
		},
	}
	got := Normalize(reg, s)
	if len(got.NormalReturnParams) != 1 {
		t.Fatalf("Normalize kept %d normal return params, want 1", len(got.NormalReturnParams))
	}
	if !product.Equal(reg, got.NormalReturnParams[0], product.Top()) {
		t.Fatalf("Normalize first normal return param = %#v, want top", got.NormalReturnParams[0])
	}

	allBottom := Normalize(reg, Summary{NormalReturnParams: []product.Value{product.Bottom(reg)}})
	if len(allBottom.NormalReturnParams) != 0 {
		t.Fatalf("Normalize(all bottom) kept %d normal return params, want 0", len(allBottom.NormalReturnParams))
	}
}

func TestLessOrEqAndEqualForReturnTuples(t *testing.T) {
	reg := mustRegistry(t)
	bottom := Summary{}
	top := Summary{Returns: []product.Value{product.Top()}}
	topWithTrailingBottom := Summary{Returns: []product.Value{product.Top(), product.Bottom(reg)}}

	if !LessOrEq(reg, bottom, top) {
		t.Fatalf("bottom summary should be <= top-return summary")
	}
	if LessOrEq(reg, top, bottom) {
		t.Fatalf("top-return summary should not be <= bottom summary")
	}
	if !Equal(reg, top, topWithTrailingBottom) {
		t.Fatalf("trailing bottom slot should not affect equality")
	}
	if Equal(reg, bottom, top) {
		t.Fatalf("bottom summary should not equal top-return summary")
	}
}

func TestJoinWeakensNormalReturnParamConstraints(t *testing.T) {
	reg := mustRegistry(t)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	got := Join(reg,
		Summary{NormalReturnParams: []product.Value{present}},
		Summary{NormalReturnParams: []product.Value{product.Top()}},
	)

	if len(got.NormalReturnParams) != 1 {
		t.Fatalf("Join returned %d normal return params, want 1", len(got.NormalReturnParams))
	}
	if !product.Equal(reg, got.NormalReturnParams[0], product.Top()) {
		t.Fatalf("Join did not weaken normal return param to top: %v", got.NormalReturnParams[0])
	}
}

func TestLessOrEqAndEqualForNormalReturnParams(t *testing.T) {
	reg := mustRegistry(t)
	presentValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	bottom := Summary{}
	present := Summary{NormalReturnParams: []product.Value{presentValue}}
	presentWithTrailingBottom := Summary{NormalReturnParams: []product.Value{presentValue, product.Bottom(reg)}}

	if !LessOrEq(reg, bottom, present) {
		t.Fatalf("bottom summary should be <= present normal-return summary")
	}
	if LessOrEq(reg, present, bottom) {
		t.Fatalf("present normal-return summary should not be <= bottom summary")
	}
	if !Equal(reg, present, presentWithTrailingBottom) {
		t.Fatalf("trailing bottom normal-return slot should not affect equality")
	}
	if Equal(reg, bottom, present) {
		t.Fatalf("bottom summary should not equal present normal-return summary")
	}
}

func TestWidenWithMissingReturnSlot(t *testing.T) {
	reg := mustRegistry(t)
	got := Widen(reg, Summary{}, Summary{Returns: []product.Value{product.Top()}})
	if len(got.Returns) != 1 {
		t.Fatalf("Widen returned %d slots, want 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Widen missing slot with top = %#v, want top", got.Returns[0])
	}
}

func mustRegistry(t *testing.T) *axis.Registry {
	t.Helper()
	reg, err := product.RegistryWithAxes()
	if err != nil {
		t.Fatalf("RegistryWithAxes() error = %v", err)
	}
	return reg
}
