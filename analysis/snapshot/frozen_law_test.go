package snapshot

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// A Frozen is content produced once and shared by every consumer that mounts
// it. These laws state the properties that make sharing one published value
// safe: it admits no derivation, so its generation is final; it carries no
// mount or query facts, so nothing in it belongs to one mount or one solve;
// and it answers exactly what an equivalent hot publication answers.

func frozenLawSchema(t *testing.T) identity.ContentID {
	t.Helper()
	id, derived := identity.DeriveContentID("snapshot/frozen-law/v1", nil)
	if !derived {
		t.Fatal("frozen law schema")
	}
	return id
}

func frozenLawAxis(t *testing.T) Axis[uint64, uint64] {
	t.Helper()
	return Axis[uint64, uint64]{SchemaID: frozenLawSchema(t), Slot: 0}
}

func frozenLawDenominator(t *testing.T) identity.ContentID {
	t.Helper()
	id, derived := identity.DeriveContentID("snapshot/frozen-law/denominator", nil)
	if !derived {
		t.Fatal("frozen law denominator")
	}
	return id
}

func frozenLawContent(t *testing.T) Content[uint64, uint64] {
	t.Helper()
	return Content[uint64, uint64]{
		Rows:        map[uint64]uint64{1: 7, 2: 9},
		Denominator: frozenLawDenominator(t),
		Members:     []uint64{1, 2, 3},
	}
}

func sealFrozenLaw(t *testing.T, store identity.StoreID) Frozen {
	t.Helper()
	builder := NewFrozen(frozenLawSchema(t), store)
	if err := PutFrozenColumn(&builder, frozenLawAxis(t), frozenLawContent(t)); err != nil {
		t.Fatalf("put cold column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal frozen: %v", err)
	}
	return sealed
}

// The generation of a frozen store is final because nothing can advance it.
// No exported entry point in this package accepts a Frozen and returns a
// builder, so the property is enforced by the type rather than by a refusal
// a caller could forget to check. This law pins the surface that statement
// depends on: a Frozen is readers only, and it publishes the one revision.
func TestFrozenAdmitsNoDerivation(t *testing.T) {
	frozen := sealFrozenLaw(t, identity.StoreID(41))

	if frozen.Generation() != coldGeneration {
		t.Fatalf("frozen published at generation %d, want %d", frozen.Generation(), coldGeneration)
	}

	frozenType := reflect.TypeOf(Frozen{})
	for index := 0; index < frozenType.NumField(); index++ {
		if frozenType.Field(index).IsExported() {
			t.Errorf("Frozen exposes field %s", frozenType.Field(index).Name)
		}
	}
	readers := map[string]bool{
		"Schema": true, "Store": true, "Generation": true,
		"Published": true, "Columns": true, "Denominators": true,
	}
	pointer := reflect.PointerTo(frozenType)
	for index := 0; index < pointer.NumMethod(); index++ {
		if name := pointer.Method(index).Name; !readers[name] {
			t.Errorf("Frozen exposes non-reader method %s", name)
		}
	}
	if pointer.NumMethod() != len(readers) {
		t.Fatalf("Frozen method set = %d methods, want %d readers", pointer.NumMethod(), len(readers))
	}
}

// A frozen value carries no fact that belongs to one mount or one solve. That
// is what lets every mount of one program share the same published value
// instead of a per-mount copy: there is nothing in it a second mount would
// have to disagree with.
func TestFrozenCarriesNoMountOrQueryFacts(t *testing.T) {
	frozenType := reflect.TypeOf(Frozen{})
	pointer := reflect.PointerTo(frozenType)
	for _, absent := range []string{"Queries", "Bind", "RegisterQuery"} {
		if _, present := pointer.MethodByName(absent); present {
			t.Errorf("Frozen exposes %s, which is a hot-publication fact", absent)
		}
	}
	builderType := reflect.PointerTo(reflect.TypeOf(FrozenBuilder{}))
	for _, absent := range []string{"Bind", "RegisterQuery"} {
		if _, present := builderType.MethodByName(absent); present {
			t.Errorf("FrozenBuilder exposes %s, which is a hot-publication write", absent)
		}
	}
}

// One frozen value mounted twice is one value. Two distinct module keys name
// same published content, the reads agree, and carrying the value costs no
// copy of any row.
func TestFrozenSharedAcrossMountsIsOneValue(t *testing.T) {
	frozen := sealFrozenLaw(t, identity.StoreID(42))
	axis := frozenLawAxis(t)

	first, second := identity.ContentID{1}, identity.ContentID{2}
	if first == second {
		t.Fatal("law needs two distinct mounts")
	}
	mounted := map[identity.ContentID]Frozen{first: frozen, second: frozen}

	for mount, held := range mounted {
		value, status := ReadFrozen(&held, axis, uint64(1))
		if status != ReadHit || value != 7 {
			t.Fatalf("mount %v read reported %v value %d", mount[0], status, value)
		}
		if held.Store() != frozen.Store() || held.Generation() != frozen.Generation() {
			t.Fatalf("mount %v holds another store fence", mount[0])
		}
	}

	carried := frozen
	if allocations := testing.AllocsPerRun(100, func() {
		sink := carried
		if _, status := ReadFrozen(&sink, axis, uint64(2)); status != ReadHit {
			t.Fatal("shared read missed")
		}
	}); allocations != 0 {
		t.Fatalf("sharing a frozen value allocated %.0f times", allocations)
	}
}

// The cold lifecycle changes what a publication admits, never what a read
// means. A frozen publication answers a hit, a proven absence and a miss
// exactly as the same content published into a hot store does.
func TestFrozenReadsMatchAnEquivalentHotPublication(t *testing.T) {
	frozen := sealFrozenLaw(t, identity.StoreID(43))

	hotBuilder := NewBuilder(frozenLawSchema(t), identity.StoreID(44), identity.Generation(1))
	if err := PutColumn(&hotBuilder, frozenLawAxis(t), frozenLawContent(t)); err != nil {
		t.Fatalf("put hot column: %v", err)
	}
	hot, err := hotBuilder.Seal()
	if err != nil {
		t.Fatalf("seal hot: %v", err)
	}

	axis := frozenLawAxis(t)
	for _, probe := range []struct {
		key    uint64
		value  uint64
		status ReadStatus
	}{
		{key: 1, value: 7, status: ReadHit},
		{key: 2, value: 9, status: ReadHit},
		{key: 3, status: ReadProvenAbsent},
		{key: 4, status: ReadMiss},
	} {
		coldValue, coldStatus := ReadFrozen(&frozen, axis, probe.key)
		hotValue, hotStatus := Read(&hot, axis, probe.key)
		if coldStatus != probe.status || coldValue != probe.value {
			t.Errorf("cold key %d reported %v value %d, want %v value %d", probe.key, coldStatus, coldValue, probe.status, probe.value)
		}
		if coldStatus != hotStatus || coldValue != hotValue {
			t.Errorf("cold key %d answered %v/%d, hot answered %v/%d", probe.key, coldStatus, coldValue, hotStatus, hotValue)
		}
	}

	if !frozen.Denominators().Published(frozenLawDenominator(t)) {
		t.Fatal("frozen publication dropped its denominator")
	}
}

// A Locator issued against a frozen store stays valid for the life of the
// value, because the generation it is anchored to never advances.
func TestFrozenLocatorStaysValid(t *testing.T) {
	published, derived := identity.DeriveContentID("snapshot/frozen-law/published", nil)
	if !derived {
		t.Fatal("published identity")
	}
	builder := NewFrozen(frozenLawSchema(t), identity.StoreID(45))
	if err := PutFrozenColumn(&builder, frozenLawAxis(t), frozenLawContent(t)); err != nil {
		t.Fatalf("put cold column: %v", err)
	}
	if err := builder.Publish(published, 0); err != nil {
		t.Fatalf("publish: %v", err)
	}
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	locator, resolved := ResolveFrozen(&frozen, published)
	if !resolved {
		t.Fatal("frozen directory resolved nothing")
	}
	value, status := ReadFrozenAt[uint64, uint64](&frozen, locator, uint64(1))
	if status != ReadHit || value != 7 {
		t.Fatalf("locator read reported %v value %d", status, value)
	}

	shared := frozen
	if _, status := ReadFrozenAt[uint64, uint64](&shared, locator, uint64(1)); status != ReadHit {
		t.Fatal("locator stopped addressing a shared copy of the same frozen value")
	}
}

// A frozen publication is held to the same construction rules a hot one is:
// missing identities and holes in the dense slot range reject.
func TestFrozenSealRejectsIncompletePublication(t *testing.T) {
	empty := NewFrozen(identity.ContentID{}, identity.StoreID(46))
	if _, err := empty.Seal(); err == nil {
		t.Fatal("frozen publication without a schema sealed")
	}
	storeless := NewFrozen(frozenLawSchema(t), identity.StoreID(0))
	if _, err := storeless.Seal(); err == nil {
		t.Fatal("frozen publication without a store sealed")
	}
	holed := NewFrozen(frozenLawSchema(t), identity.StoreID(47))
	if err := PutFrozenColumn(&holed, Axis[uint64, uint64]{SchemaID: frozenLawSchema(t), Slot: 2}, frozenLawContent(t)); err != nil {
		t.Fatalf("put cold column: %v", err)
	}
	if _, err := holed.Seal(); err == nil {
		t.Fatal("frozen publication with a slot hole sealed")
	}
}

// A denominator is a sealed key universe, so its cardinality is a fact it
// already holds. A column keyed by a dense ordinal has no other way to state
// how far it runs, which is what makes this the reader an iterating consumer
// depends on.
func TestDenominatorPublishesItsSize(t *testing.T) {
	frozen := sealFrozenLaw(t, identity.StoreID(48))

	size, published := frozen.Denominators().Size(frozenLawDenominator(t))
	if !published {
		t.Fatal("sealed denominator published no size")
	}
	if size != 3 {
		t.Fatalf("denominator size = %d, want 3", size)
	}

	unpublished, derived := identity.DeriveContentID("snapshot/frozen-law/absent", nil)
	if !derived {
		t.Fatal("absent identity")
	}
	if _, published := frozen.Denominators().Size(unpublished); published {
		t.Fatal("an unpublished denominator reported a size")
	}
}

// The size counts the sealed universe, not the offers that built it: one key
// offered twice is one member, and a second column that names the identity
// alone inherits the very same count.
func TestDenominatorSizeCountsTheSealedUniverse(t *testing.T) {
	schemaID := frozenLawSchema(t)
	denominator := frozenLawDenominator(t)
	builder := NewFrozen(schemaID, identity.StoreID(49))

	first := Axis[uint64, uint64]{SchemaID: schemaID, Slot: 0}
	if err := PutFrozenColumn(&builder, first, Content[uint64, uint64]{
		Rows:        map[uint64]uint64{1: 7},
		Denominator: denominator,
		Members:     []uint64{1, 2, 2, 3},
	}); err != nil {
		t.Fatalf("put first column: %v", err)
	}
	second := Axis[uint64, string]{SchemaID: schemaID, Slot: 1}
	if err := PutFrozenColumn(&builder, second, Content[uint64, string]{
		Rows:        map[uint64]string{2: "b"},
		Denominator: denominator,
	}); err != nil {
		t.Fatalf("put sharing column: %v", err)
	}
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	size, published := frozen.Denominators().Size(denominator)
	if !published || size != 3 {
		t.Fatalf("shared denominator size = %d (published %t), want 3", size, published)
	}
	if _, status := ReadFrozen(&frozen, second, uint64(3)); status != ReadProvenAbsent {
		t.Fatalf("sharing column reported %v where the shared universe proves absence", status)
	}
}

// Every read and write entry point fails closed on a value that was never
// published. The law is load bearing because the two lifecycles share one
// embedded publication: an entry point that reached for it without checking
// would fault rather than report ReadInvalid.
func TestUnpublishedValuesFailClosed(t *testing.T) {
	axis := frozenLawAxis(t)
	published, derived := identity.DeriveContentID("snapshot/frozen-law/nil", nil)
	if !derived {
		t.Fatal("probe identity")
	}

	var absentSnapshot *Snapshot
	if _, status := Read(absentSnapshot, axis, uint64(1)); status != ReadInvalid {
		t.Errorf("nil snapshot read reported %v", status)
	}
	if _, status := ReadAt[uint64, uint64](absentSnapshot, Locator{}, uint64(1)); status != ReadInvalid {
		t.Errorf("nil snapshot locator read reported %v", status)
	}
	if _, resolved := Resolve(absentSnapshot, published); resolved {
		t.Error("nil snapshot resolved a locator")
	}
	if _, opened := OpenQuery[uint64, uint64](absentSnapshot, published); opened {
		t.Error("nil snapshot opened a query")
	}

	var absentFrozen *Frozen
	if _, status := ReadFrozen(absentFrozen, axis, uint64(1)); status != ReadInvalid {
		t.Errorf("nil frozen read reported %v", status)
	}
	if _, status := ReadFrozenAt[uint64, uint64](absentFrozen, Locator{}, uint64(1)); status != ReadInvalid {
		t.Errorf("nil frozen locator read reported %v", status)
	}
	if _, resolved := ResolveFrozen(absentFrozen, published); resolved {
		t.Error("nil frozen resolved a locator")
	}

	zeroSnapshot, zeroFrozen := Snapshot{}, Frozen{}
	if _, status := Read(&zeroSnapshot, axis, uint64(1)); status != ReadInvalid {
		t.Errorf("zero snapshot read reported %v", status)
	}
	if _, status := ReadFrozen(&zeroFrozen, axis, uint64(1)); status != ReadInvalid {
		t.Errorf("zero frozen read reported %v", status)
	}

	var absentBuilder *Builder
	if _, status := ReadOverlay(absentBuilder, axis, uint64(1)); status != ReadInvalid {
		t.Errorf("nil builder overlay read reported %v", status)
	}
	if err := PutColumn(absentBuilder, axis, frozenLawContent(t)); err == nil {
		t.Error("nil builder accepted a column")
	}
	var absentFrozenBuilder *FrozenBuilder
	if err := PutFrozenColumn(absentFrozenBuilder, axis, frozenLawContent(t)); err == nil {
		t.Error("nil frozen builder accepted a column")
	}
	if err := SetRow(absentBuilder, axis, uint64(1), uint64(1)); err == nil {
		t.Error("nil builder accepted a row")
	}
	if err := RemoveRow(absentBuilder, axis, uint64(1)); err == nil {
		t.Error("nil builder accepted a removal")
	}
}
