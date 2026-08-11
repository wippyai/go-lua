package target

import (
	"math"
	"testing"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func bootLiteralKey(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}

func bootExactKey(t *testing.T, contract *Contract, text string) ExactKey {
	t.Helper()
	want := bootLiteralKey(text)
	for index := 0; index < contract.ExactKeyCount(); index++ {
		key, ok := contract.ExactKeyAt(index)
		value, valueOK := contract.ExactKeyValue(key)
		if ok && valueOK && value == want {
			return key
		}
	}
	t.Fatalf("missing exact key %q", text)
	return 0
}

func bootExactKeyText(contract *Contract, key ExactKey) string {
	value, ok := contract.ExactKeyValue(key)
	if !ok || value.Kind != keyspace.LiteralString {
		return ""
	}
	return value.String
}

func TestBootLedgerSealsTypedCanonicalRows(t *testing.T) {
	contract := mustSeal(t, completeBootSpec("Lua 5.3", InitialMutable))
	if got := contract.InitialRootCount(); got != 2 {
		t.Fatalf("initial roots = %d, want 2", got)
	}
	global, ok := contract.InitialRootAt(0)
	if !ok {
		t.Fatal("missing canonical GlobalEnvRoot")
	}
	if identity, ok := contract.InitialRootIdentity(global); !ok || identity != "GlobalEnvRoot" {
		t.Fatalf("root identity = %q/%v", identity, ok)
	}
	if root, ok := contract.GlobalEnvRoot(); !ok || root != global {
		t.Fatalf("GlobalEnvRoot = %d/%v, want %d", root, ok, global)
	}
	shape, ok := contract.InitialRootBootShape(global)
	if !ok || shape != 1 {
		t.Fatalf("GlobalEnvRoot shape = %d/%v", shape, ok)
	}
	if aggregate, ok := contract.BootShapeAggregate(shape); !ok || aggregate != BootAggregateTable {
		t.Fatalf("boot aggregate = %d/%v", aggregate, ok)
	}
	if immutable, ok := contract.BootShapeImmutable(shape); !ok || immutable {
		t.Fatalf("boot immutable header = %v/%v, want mutable", immutable, ok)
	}
	shapeValue, ok := contract.BootShapeValue(shape)
	if !ok {
		t.Fatal("missing boot shape value")
	}
	if alias, ok := contract.InitialValueRoot(shapeValue); !ok || alias != global {
		t.Fatalf("boot root alias = %d/%v, want %d", alias, ok, global)
	}
	globalValue, _, ok := contract.InitialEntry(global, bootExactKey(t, contract, "_G"))
	if class, value, root, key, ok := contract.InitialBinding("_G"); !ok || class != InitialBindingOrdinary || value != globalValue || root != global || bootExactKeyText(contract, key) != "_G" {
		t.Fatalf("_G binding = %d/%d/%d/%d/%v", class, value, root, key, ok)
	}

	assertValue, mutability, ok := contract.InitialEntry(global, bootExactKey(t, contract, "assert"))
	if !ok || mutability != InitialMutable {
		t.Fatalf("assert initial entry = %d/%d/%v", assertValue, mutability, ok)
	}
	assertOp, ok := contract.InitialValueOperation(assertValue)
	if !ok {
		t.Fatal("assert is not an exact operation initial value")
	}
	if want, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"assert"}}); !ok || assertOp != want {
		t.Fatalf("assert initial operation = %d, want %d/%v", assertOp, want, ok)
	}
	if class, value, root, key, ok := contract.InitialBinding("assert"); !ok || class != InitialBindingAdmitted || value != assertValue || root != global || bootExactKeyText(contract, key) != "assert" {
		t.Fatalf("assert binding = %d/%d/%d/%d/%v", class, value, root, key, ok)
	}

	denied, _, ok := contract.InitialEntry(global, bootExactKey(t, contract, "load"))
	if !ok {
		t.Fatal("missing denied load entry")
	}
	if namespace, ok := contract.InitialValueDeniedNamespace(denied); !ok || namespace != BindingBuiltin || contract.InitialValueDeniedMemberCount(denied) != 1 {
		t.Fatalf("denied load identity = %d/%v", namespace, ok)
	}
	if member, ok := contract.InitialValueDeniedMemberAt(denied, 0); !ok || member != "load" {
		t.Fatalf("denied member = %q/%v", member, ok)
	}
	if class, value, _, _, ok := contract.InitialBinding("load"); !ok || class != InitialBindingDenied || value != denied {
		t.Fatalf("load binding class = %d/%v", class, ok)
	}
	version, _, ok := contract.InitialEntry(global, bootExactKey(t, contract, "_VERSION"))
	if text, ok := contract.InitialValueString(version); !ok || text != "Lua 5.3" {
		t.Fatalf("version = %q/%v", text, ok)
	}
	pi, _, ok := contract.InitialEntry(global, bootExactKey(t, contract, "pi"))
	if bits, ok := contract.InitialValueFloatBits(pi); !ok || bits != math.Float64bits(math.Pi) {
		t.Fatalf("pi bits = %x/%v", bits, ok)
	}
	absent, _, ok := contract.InitialEntry(global, bootExactKey(t, contract, "not_present"))
	if kind, ok := contract.InitialValueKind(absent); !ok || kind != InitialValueAbsent {
		t.Fatalf("absent kind = %d/%v", kind, ok)
	}
	if class, value, _, _, ok := contract.InitialBinding("not_present"); !ok || class != InitialBindingOrdinary || value != absent {
		t.Fatalf("absent binding class = %d/%v", class, ok)
	}
	if value, ok := contract.InitialAbsent(); !ok || value != absent {
		t.Fatalf("InitialAbsent = %d/%v, want %d", value, ok, absent)
	}
}

func TestBootLedgerRejectsMalformedRows(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Spec)
	}{
		{"duplicate root", func(spec *Spec) { spec.InitialRoots = append(spec.InitialRoots, spec.InitialRoots[0]) }},
		{"missing shape", func(spec *Spec) { spec.InitialRoots[0].Shape = BootShapeSpec{} }},
		{"non-root shape value", func(spec *Spec) {
			spec.InitialRoots[0].Shape.Value = InitialValueSpec{Kind: InitialValueString, String: "not a root"}
		}},
		{"cross-root shape value", func(spec *Spec) {
			spec.InitialRoots[0].Shape.Value = InitialValueSpec{Kind: InitialValueRoot, Root: "GlobalEnvRoot"}
		}},
		{"foreign entry root", func(spec *Spec) { spec.InitialEntries[0].Root = "foreign" }},
		{"duplicate entry", func(spec *Spec) { spec.InitialEntries = append(spec.InitialEntries, spec.InitialEntries[0]) }},
		{"missing binding entry", func(spec *Spec) { spec.InitialBindings[0].Key = bootLiteralKey("missing") }},
		{"binding name differs from key", func(spec *Spec) { spec.InitialBindings[0].Name = "different" }},
		{"frozen global binding", func(spec *Spec) { spec.InitialEntries[1].Mutability = InitialFrozen }},
		{"global root is not table", func(spec *Spec) { spec.InitialRoots[1].Shape.Aggregate = BootAggregateMetatable }},
		{"multiple global roots", func(spec *Spec) {
			spec.InitialRoots = append(spec.InitialRoots, InitialRootSpec{Identity: "OtherGlobalRoot", Shape: BootShapeSpec{Aggregate: BootAggregateTable, Value: InitialValueSpec{Kind: InitialValueRoot, Root: "OtherGlobalRoot"}}})
			spec.InitialEntries = append(spec.InitialEntries, InitialEntrySpec{Root: "OtherGlobalRoot", Key: bootLiteralKey("other"), Value: InitialValueSpec{Kind: InitialValueOperation, Operation: BindingSpec{Namespace: BindingBuiltin, Member: []string{"assert"}}}, Mutability: InitialMutable})
			spec.InitialBindings = append(spec.InitialBindings, InitialBindingSpec{Name: "other", Root: "OtherGlobalRoot", Key: bootLiteralKey("other")})
		}},
		{"missing absent", func(spec *Spec) { spec.InitialEntries[5].Value = InitialValueSpec{Kind: InitialValueNil} }},
		{"foreign operation", func(spec *Spec) {
			spec.InitialEntries[1].Value = InitialValueSpec{Kind: InitialValueOperation, Operation: BindingSpec{Namespace: BindingBuiltin, Member: []string{"foreign"}}}
		}},
		{"invalid denied operation", func(spec *Spec) { spec.InitialEntries[2].Value = InitialValueSpec{Kind: InitialValueDeniedOperation} }},
		{"admitted denied operation", func(spec *Spec) {
			spec.InitialEntries[2].Value = InitialValueSpec{Kind: InitialValueDeniedOperation, Operation: BindingSpec{Namespace: BindingBuiltin, Member: []string{"assert"}}}
		}},
		{"invalid typed union", func(spec *Spec) {
			spec.InitialEntries[3].Value = InitialValueSpec{Kind: InitialValueString, String: "version", Integer: 1}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			spec := completeBootSpec("Lua 5.3", InitialMutable)
			test.edit(&spec)
			if _, err := Seal(&spec); err == nil {
				t.Fatal("Seal accepted malformed boot ledger")
			}
		})
	}
}

func TestBootLedgerRequiresGlobalSelfAlias(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Spec)
	}{
		{"missing _G binding", func(spec *Spec) { spec.InitialBindings = spec.InitialBindings[1:] }},
		{"frozen _G", func(spec *Spec) { spec.InitialEntries[0].Mutability = InitialFrozen }},
		{"absent _G", func(spec *Spec) {
			spec.InitialEntries[0].Value = InitialValueSpec{Kind: InitialValueAbsent}
		}},
		{"denied _G", func(spec *Spec) {
			spec.InitialEntries[0].Value = InitialValueSpec{Kind: InitialValueDeniedOperation, Operation: BindingSpec{Namespace: BindingBuiltin, Member: []string{"load"}}}
		}},
		{"operation _G", func(spec *Spec) {
			spec.InitialEntries[0].Value = InitialValueSpec{Kind: InitialValueOperation, Operation: BindingSpec{Namespace: BindingBuiltin, Member: []string{"assert"}}}
		}},
		{"other-root _G", func(spec *Spec) {
			spec.InitialEntries[0].Value = InitialValueSpec{Kind: InitialValueRoot, Root: "StringMetatableRoot"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := completeBootSpec("Lua 5.3", InitialMutable)
			test.edit(&spec)
			if _, err := Seal(&spec); err == nil {
				t.Fatal("Seal accepted an invalid _G global self-alias")
			}
		})
	}

	spec := completeBootSpec("Lua 5.3", InitialMutable)
	spec.InitialBindings = nil
	contract := mustSeal(t, spec)
	if contract.InitialBindingCount() != 0 {
		t.Fatal("zero-binding boot ledger exposed a global binding")
	}
	if _, ok := contract.GlobalEnvRoot(); ok {
		t.Fatal("zero-binding boot ledger exposed a global root")
	}
}

// Whole-object bootstrap immutability is an explicit BootShape property.  It
// is neither reconstructed from nor a synonym for the exact-slot policies in
// InitialEntrySpec.
func TestBootShapeImmutableIsCanonicalAndIndependentOfEntryPolicy(t *testing.T) {
	mutableSpec := completeBootSpec("Lua 5.3", InitialMutable)
	frozenSpec := completeBootSpec("Lua 5.3", InitialMutable)
	for index := range frozenSpec.InitialRoots {
		if frozenSpec.InitialRoots[index].Identity == "GlobalEnvRoot" {
			frozenSpec.InitialRoots[index].Shape.Immutable = true
		}
	}
	mutable := mustSeal(t, mutableSpec)
	frozen := mustSeal(t, frozenSpec)
	mutableRoot, mutableRootOK := mutable.GlobalEnvRoot()
	frozenRoot, frozenRootOK := frozen.GlobalEnvRoot()
	mutableShape, mutableShapeOK := mutable.InitialRootBootShape(mutableRoot)
	frozenShape, frozenShapeOK := frozen.InitialRootBootShape(frozenRoot)
	mutableHeader, mutableHeaderOK := mutable.BootShapeImmutable(mutableShape)
	frozenHeader, frozenHeaderOK := frozen.BootShapeImmutable(frozenShape)
	if !mutableRootOK || !frozenRootOK || !mutableShapeOK || !frozenShapeOK || !mutableHeaderOK || !frozenHeaderOK || mutableHeader || !frozenHeader {
		t.Fatalf("explicit boot headers = mutable:%v/%v frozen:%v/%v", mutableHeader, mutableHeaderOK, frozenHeader, frozenHeaderOK)
	}
	if mutable.ContentID() == frozen.ContentID() {
		t.Fatal("boot-header delta did not change ContentID")
	}
	// Both fixtures retain the same mutable _G entry.  A frozen header must
	// therefore not be inferred from individual boot-slot policies.
	mutableValue, mutablePolicy, mutableEntryOK := mutable.InitialEntry(mutableRoot, bootExactKey(t, mutable, "_G"))
	frozenValue, frozenPolicy, frozenEntryOK := frozen.InitialEntry(frozenRoot, bootExactKey(t, frozen, "_G"))
	if !mutableEntryOK || !frozenEntryOK || mutablePolicy != InitialMutable || frozenPolicy != InitialMutable || mutableValue == 0 || frozenValue == 0 {
		t.Fatalf("header changed initial slot policy = mutable:%d/%v frozen:%d/%v", mutablePolicy, mutableEntryOK, frozenPolicy, frozenEntryOK)
	}
}

func TestInitialMetatableAttachmentIsTypedCanonicalBootstrapData(t *testing.T) {
	spec := completeBootSpec("Lua 5.3", InitialMutable)
	spec.InitialMetatables = []InitialMetatableAttachmentSpec{{Base: InitialValueString, Metatable: "StringMetatableRoot"}}
	contract := mustSeal(t, spec)
	if got := contract.InitialMetatableAttachmentCount(); got != 1 {
		t.Fatalf("initial metatable attachments = %d, want 1", got)
	}
	base, metatable, ok := contract.InitialMetatableAttachmentAt(0)
	if !ok || base != InitialValueString {
		t.Fatalf("initial metatable attachment base = %v/%v", base, ok)
	}
	if identity, rootOK := contract.InitialRootIdentity(metatable); !rootOK || identity != "StringMetatableRoot" {
		t.Fatalf("initial metatable attachment root = %q/%v", identity, rootOK)
	}
	shape, shapeOK := contract.InitialRootBootShape(metatable)
	aggregate, aggregateOK := contract.BootShapeAggregate(shape)
	if !shapeOK || !aggregateOK || aggregate != BootAggregateMetatable {
		t.Fatal("initial metatable attachment did not retain a metatable root")
	}
	if _, _, ok := contract.InitialMetatableAttachmentAt(1); ok {
		t.Fatal("out-of-range initial metatable attachment accepted")
	}

	without := completeBootSpec("Lua 5.3", InitialMutable)
	if contract.ContentID() == mustSeal(t, without).ContentID() {
		t.Fatal("initial metatable attachment changed no target identity")
	}

	for _, edit := range []func(*Spec){
		func(spec *Spec) { spec.InitialMetatables[0].Base = InitialValueInteger },
		func(spec *Spec) { spec.InitialMetatables[0].Metatable = "GlobalEnvRoot" },
		func(spec *Spec) { spec.InitialMetatables[0].Metatable = "missing" },
		func(spec *Spec) { spec.InitialMetatables = append(spec.InitialMetatables, spec.InitialMetatables[0]) },
	} {
		malformed := completeBootSpec("Lua 5.3", InitialMutable)
		malformed.InitialMetatables = []InitialMetatableAttachmentSpec{{Base: InitialValueString, Metatable: "StringMetatableRoot"}}
		edit(&malformed)
		if _, err := Seal(&malformed); err == nil {
			t.Fatal("Seal accepted malformed initial metatable attachment")
		}
	}
}

func TestBootLedgerCanonicalOrderAndContentID(t *testing.T) {
	left := mustSeal(t, completeBootSpec("Lua 5.3", InitialMutable))
	rightSpec := completeBootSpec("Lua 5.3", InitialMutable)
	reverseInitialRoots(rightSpec.InitialRoots)
	reverseInitialEntries(rightSpec.InitialEntries)
	reverseInitialBindings(rightSpec.InitialBindings)
	right := mustSeal(t, rightSpec)
	if left.ContentID() != right.ContentID() {
		t.Fatal("permuted boot ledger changed ContentID")
	}
	if got, want := bootSnapshot(left), bootSnapshot(right); got != want {
		t.Fatalf("permuted boot ledger query snapshot differs\n got: %q\nwant: %q", got, want)
	}

	for _, test := range []struct {
		name string
		spec Spec
	}{
		{"static value", completeBootSpec("Luau", InitialMutable)},
		{"mutability", bootMutabilityDelta()},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := mustSeal(t, test.spec).ContentID(); got == left.ContentID() {
				t.Fatal("boot semantic delta did not change ContentID")
			}
		})
	}
}

func bootMutabilityDelta() Spec {
	spec := completeBootSpec("Lua 5.3", InitialMutable)
	spec.InitialEntries[4].Mutability = InitialMutable // pi is not a global binding.
	return spec
}

func TestBootLedgerPreservesClosedTypedLiterals(t *testing.T) {
	spec := completeBootSpec("Lua 5.3", InitialMutable)
	spec.InitialEntries = append(spec.InitialEntries,
		InitialEntrySpec{Root: "GlobalEnvRoot", Key: bootLiteralKey("false"), Value: InitialValueSpec{Kind: InitialValueBoolean, Boolean: false}, Mutability: InitialFrozen},
		InitialEntrySpec{Root: "GlobalEnvRoot", Key: bootLiteralKey("mininteger"), Value: InitialValueSpec{Kind: InitialValueInteger, Integer: math.MinInt64}, Mutability: InitialFrozen},
		InitialEntrySpec{Root: "GlobalEnvRoot", Key: bootLiteralKey("maxinteger"), Value: InitialValueSpec{Kind: InitialValueInteger, Integer: math.MaxInt64}, Mutability: InitialFrozen},
		InitialEntrySpec{Root: "GlobalEnvRoot", Key: bootLiteralKey("signed_zero"), Value: InitialValueSpec{Kind: InitialValueFloat, FloatBits: math.Float64bits(math.Copysign(0, -1))}, Mutability: InitialFrozen},
		InitialEntrySpec{Root: "GlobalEnvRoot", Key: bootLiteralKey("bytes"), Value: InitialValueSpec{Kind: InitialValueString, String: "\x00\x7f\xc2\xf4"}, Mutability: InitialFrozen},
		InitialEntrySpec{Root: "GlobalEnvRoot", Key: bootLiteralKey("nil"), Value: InitialValueSpec{Kind: InitialValueNil}, Mutability: InitialFrozen},
	)
	contract := mustSeal(t, spec)
	global, _ := contract.InitialRootAt(0)
	for _, test := range []struct {
		key  string
		kind InitialValueKind
	}{
		{"false", InitialValueBoolean}, {"mininteger", InitialValueInteger}, {"maxinteger", InitialValueInteger},
		{"signed_zero", InitialValueFloat}, {"bytes", InitialValueString}, {"nil", InitialValueNil},
	} {
		value, _, ok := contract.InitialEntry(global, bootExactKey(t, contract, test.key))
		if kind, kindOK := contract.InitialValueKind(value); !ok || !kindOK || kind != test.kind {
			t.Fatalf("%s kind = %d/%v/%v", test.key, kind, kindOK, ok)
		}
	}
	min, _, _ := contract.InitialEntry(global, bootExactKey(t, contract, "mininteger"))
	if got, ok := contract.InitialValueInteger(min); !ok || got != math.MinInt64 {
		t.Fatalf("mininteger = %d/%v", got, ok)
	}
	max, _, _ := contract.InitialEntry(global, bootExactKey(t, contract, "maxinteger"))
	if got, ok := contract.InitialValueInteger(max); !ok || got != math.MaxInt64 {
		t.Fatalf("maxinteger = %d/%v", got, ok)
	}
	signedZero, _, _ := contract.InitialEntry(global, bootExactKey(t, contract, "signed_zero"))
	if got, ok := contract.InitialValueFloatBits(signedZero); !ok || got != math.Float64bits(math.Copysign(0, -1)) {
		t.Fatalf("signed zero bits = %x/%v", got, ok)
	}
	bytes, _, _ := contract.InitialEntry(global, bootExactKey(t, contract, "bytes"))
	if got, ok := contract.InitialValueString(bytes); !ok || got != "\x00\x7f\xc2\xf4" {
		t.Fatalf("string bytes = %q/%v", got, ok)
	}
}

func TestBootLedgerOptionalAsAWholeButNeverPartial(t *testing.T) {
	withoutBoot := mustSeal(t, Spec{Operations: []OperationSpec{bootOperation("plain")}})
	if withoutBoot.InitialRootCount() != 0 || withoutBoot.InitialEntryCount() != 0 || withoutBoot.InitialBindingCount() != 0 {
		t.Fatal("empty boot ledger did not remain empty")
	}
	if _, ok := withoutBoot.InitialRootAt(0); ok {
		t.Fatal("empty boot ledger exposed a root")
	}
	if _, ok := withoutBoot.GlobalEnvRoot(); ok {
		t.Fatal("empty boot ledger exposed a global root")
	}
	if _, ok := withoutBoot.InitialAbsent(); ok {
		t.Fatal("empty boot ledger exposed an absent value")
	}
	if !withoutBoot.ContentID().Available() {
		t.Fatal("empty complete boot ledger lacks ContentID")
	}
	partial := Spec{Operations: []OperationSpec{bootOperation("plain")}, InitialEntries: []InitialEntrySpec{{Root: "missing", Key: bootLiteralKey("key"), Value: InitialValueSpec{Kind: InitialValueNil}, Mutability: InitialFrozen}}}
	if _, err := Seal(&partial); err == nil {
		t.Fatal("partial boot ledger sealed without roots")
	}
}

func TestBootLedgerContentIDUsesNewSchemaVersion(t *testing.T) {
	withoutBoot := mustSeal(t, Spec{Operations: []OperationSpec{bootOperation("plain")}})
	withBoot := mustSeal(t, completeBootSpec("Lua 5.3", InitialMutable))
	if withoutBoot.ContentID() == withBoot.ContentID() {
		t.Fatal("boot ledger did not separate target ContentID")
	}
}

func TestBootLedgerQueriesAllocateNothing(t *testing.T) {
	contract := mustSeal(t, completeBootSpec("Lua 5.3", InitialMutable))
	global, _ := contract.InitialRootAt(0)
	if allocations := testing.AllocsPerRun(1000, func() {
		value, _, _ := contract.InitialEntry(global, bootExactKey(t, contract, "assert"))
		_, _, _, _, _ = contract.InitialBinding("assert")
		_, _ = contract.InitialValueOperation(value)
		_, _ = contract.InitialRootBootShape(global)
		shape, _ := contract.InitialRootBootShape(global)
		_, _ = contract.BootShapeImmutable(shape)
		_, _ = contract.GlobalEnvRoot()
		_, _ = contract.InitialAbsent()
	}); allocations != 0 {
		t.Fatalf("boot queries allocated %v times", allocations)
	}
}

func TestBootLedgerScalesWithoutSemanticFallback(t *testing.T) {
	spec := completeBootSpec("Lua 5.3", InitialMutable)
	for index := 0; index < 20000; index++ {
		key := "entry-" + fixedBootDecimal(index)
		spec.InitialEntries = append(spec.InitialEntries, InitialEntrySpec{
			Root: "GlobalEnvRoot", Key: bootLiteralKey(key),
			Value: InitialValueSpec{Kind: InitialValueInteger, Integer: int64(index)}, Mutability: InitialMutable,
		})
	}
	contract := mustSeal(t, spec)
	global, _ := contract.InitialRootAt(0)
	value, _, ok := contract.InitialEntry(global, bootExactKey(t, contract, "entry-19999"))
	if integer, integerOK := contract.InitialValueInteger(value); !ok || !integerOK || integer != 19999 {
		t.Fatalf("scaled initial entry = %d/%v/%v", integer, integerOK, ok)
	}
	if !contract.ContentID().Available() {
		t.Fatal("scaled boot ledger lacks ContentID")
	}
}

func completeBootSpec(version string, versionMutability InitialMutability) Spec {
	return Spec{
		Operations: []OperationSpec{bootOperation("assert")},
		InitialRoots: []InitialRootSpec{
			{Identity: "StringMetatableRoot", Shape: BootShapeSpec{Aggregate: BootAggregateMetatable, Value: InitialValueSpec{Kind: InitialValueRoot, Root: "StringMetatableRoot"}}},
			{Identity: "GlobalEnvRoot", Shape: BootShapeSpec{Aggregate: BootAggregateTable, Value: InitialValueSpec{Kind: InitialValueRoot, Root: "GlobalEnvRoot"}}},
		},
		InitialEntries: []InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: bootLiteralKey("_G"), Value: InitialValueSpec{Kind: InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: InitialMutable},
			{Root: "GlobalEnvRoot", Key: bootLiteralKey("assert"), Value: InitialValueSpec{Kind: InitialValueOperation, Operation: BindingSpec{Namespace: BindingBuiltin, Member: []string{"assert"}}}, Mutability: InitialMutable},
			{Root: "GlobalEnvRoot", Key: bootLiteralKey("load"), Value: InitialValueSpec{Kind: InitialValueDeniedOperation, Operation: BindingSpec{Namespace: BindingBuiltin, Member: []string{"load"}}}, Mutability: InitialMutable},
			{Root: "GlobalEnvRoot", Key: bootLiteralKey("_VERSION"), Value: InitialValueSpec{Kind: InitialValueString, String: version}, Mutability: versionMutability},
			{Root: "GlobalEnvRoot", Key: bootLiteralKey("pi"), Value: InitialValueSpec{Kind: InitialValueFloat, FloatBits: math.Float64bits(math.Pi)}, Mutability: InitialFrozen},
			{Root: "GlobalEnvRoot", Key: bootLiteralKey("not_present"), Value: InitialValueSpec{Kind: InitialValueAbsent}, Mutability: InitialMutable},
			{Root: "StringMetatableRoot", Key: bootLiteralKey("__index"), Value: InitialValueSpec{Kind: InitialValueRoot, Root: "StringMetatableRoot"}, Mutability: InitialMutable},
		},
		InitialBindings: []InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: bootLiteralKey("_G")},
			{Name: "assert", Root: "GlobalEnvRoot", Key: bootLiteralKey("assert")},
			{Name: "load", Root: "GlobalEnvRoot", Key: bootLiteralKey("load")},
			{Name: "_VERSION", Root: "GlobalEnvRoot", Key: bootLiteralKey("_VERSION")},
			{Name: "not_present", Root: "GlobalEnvRoot", Key: bootLiteralKey("not_present")},
		},
	}
}

func bootOperation(name string) OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
}

func bootSnapshot(contract *Contract) string {
	var out string
	for index := 0; index < contract.InitialRootCount(); index++ {
		root, _ := contract.InitialRootAt(index)
		identity, _ := contract.InitialRootIdentity(root)
		shape, _ := contract.InitialRootBootShape(root)
		immutable, _ := contract.BootShapeImmutable(shape)
		if immutable {
			out += identity + "/1;"
		} else {
			out += identity + "/0;"
		}
	}
	for index := 0; index < contract.InitialEntryCount(); index++ {
		root, key, value, mutability, _ := contract.InitialEntryAt(index)
		out += fixedBootDecimal(int(root)) + "/" + bootExactKeyText(contract, key) + "/" + fixedBootDecimal(int(value)) + "/" + fixedBootDecimal(int(mutability)) + ";"
	}
	return out
}

func reverseInitialRoots(items []InitialRootSpec) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func reverseInitialEntries(items []InitialEntrySpec) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func reverseInitialBindings(items []InitialBindingSpec) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func fixedBootDecimal(value int) string {
	if value == 0 {
		return "0"
	}
	var bytes [20]byte
	position := len(bytes)
	for value > 0 {
		position--
		bytes[position] = byte('0' + value%10)
		value /= 10
	}
	return string(bytes[position:])
}
