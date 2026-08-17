package snapshot

import (
	"fmt"
	"runtime"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The hashing laws state what the package's own hashing schedule owes every
// column that answers under it. The schedule stands in for the standard
// library's comparable hashing, so it is held to the same contract: a hash is
// a function of what makes two keys equal, of nothing else, and of nothing
// that changes while a snapshot is alive. A defect here is silent -- every
// answer stays correct because a lookup still compares keys -- so it shows up
// as a plan that degrades to a scan rather than as a wrong answer, and only a
// law states it.

// seal stands for the schema object a domain column key is fenced to. The five
// domain columns key on exactly this shape, an owner pointer and a dense
// index, so the laws about pointer-carrying keys are stated on the shape the
// columns actually use.
type seal struct{ rows []int }

// sealedKey is the domain column key shape: equality is the identity of the
// issuing seal and the dense index, never the contents of the seal.
type sealedKey struct {
	owner *seal
	index uint32
}

// boxedKey holds a sealedKey inside another struct so a key can be hashed from
// storage other than a local variable.
type boxedKey struct {
	padding uint16
	key     sealedKey
}

// TestKeyHashIsDerivedFromTheKeyAlone is the determinism law. Every column and
// denominator keyed by one type derives its own schedule, so the schedules
// must agree; a schedule reads a key through its own address, so where the key
// is stored must not enter the hash; and a snapshot outlives arbitrarily many
// collections, so a hash must not change under one.
func TestKeyHashIsDerivedFromTheKeyAlone(t *testing.T) {
	t.Run("independently derived schedules agree", func(t *testing.T) {
		owner := &seal{rows: []int{1, 2}}
		assertSchedulesAgree(t, "sealed key", sealedKey{owner: owner, index: 3})
		assertSchedulesAgree(t, "identity", identity.ContentID{0x11, 0x22})
		assertSchedulesAgree(t, "mount", identity.MountID{0x33})
		assertSchedulesAgree(t, "string", "materialized")
		assertSchedulesAgree(t, "int64", int64(-9007199254740993))
		assertSchedulesAgree(t, "record", record{Weight: 3, Reach: 4, Marked: true})
		runtime.KeepAlive(owner)
	})

	t.Run("the storage holding a key does not enter its hash", func(t *testing.T) {
		owner := &seal{rows: []int{7}}
		key := sealedKey{owner: owner, index: 9}
		plan := mustPlan[sealedKey]()
		want := hashKey(plan, key)

		held := []sealedKey{{}, key}
		mapped := map[string]sealedKey{"key": key}
		nested := boxedKey{padding: 0xFFFF, key: key}
		for name, stored := range map[string]sealedKey{
			"slice element":   held[1],
			"map value":       mapped["key"],
			"struct field":    nested.key,
			"parameter":       throughKey(key),
			"heap indirect":   *(&key),
			"second local":    sealedKey{owner: owner, index: 9},
			"reconstructed":   sealedKey{owner: key.owner, index: key.index},
			"interface round": any(key).(sealedKey),
		} {
			if got := hashKey(plan, stored); got != want {
				t.Fatalf("key hashed from a %s = %x, want %x", name, got, want)
			}
		}
		runtime.KeepAlive(owner)
	})

	t.Run("a collection does not move a key", func(t *testing.T) {
		owner := &seal{rows: make([]int, 16)}
		key := sealedKey{owner: owner, index: 1}
		plan := mustPlan[sealedKey]()
		before := hashKey(plan, key)
		for cycle := 0; cycle < 3; cycle++ {
			churn := make([][]byte, 512)
			for index := range churn {
				churn[index] = make([]byte, 512)
			}
			runtime.GC()
			runtime.KeepAlive(churn)
		}
		if after := hashKey(plan, key); after != before {
			t.Fatalf("key hash after collection = %x, want %x", after, before)
		}
		runtime.KeepAlive(owner)
	})

	t.Run("repeated hashing is stable", func(t *testing.T) {
		plan := mustPlan[string]()
		want := hashKey(plan, "answer")
		for repeat := 0; repeat < 64; repeat++ {
			if got := hashKey(plan, "answer"); got != want {
				t.Fatalf("repeat %d hashed %x, want %x", repeat, got, want)
			}
		}
	})

	t.Run("one schedule answers a column and the denominator it shares", func(t *testing.T) {
		// The column that seals a denominator and the column that later names
		// it derive their own schedules, and the second column's schedule must
		// find the members the first one sealed.
		schema := identity.ContentID{0x5A}
		builder := NewBuilder(schema, identity.StoreID(3), identity.Generation(1))
		owner := &seal{rows: []int{1}}
		member := sealedKey{owner: owner, index: 4}
		first := Axis[sealedKey, int]{SchemaID: schema, Slot: 0}
		second := Axis[sealedKey, string]{SchemaID: schema, Slot: 1}
		put(t, &builder, first, Content[sealedKey, int]{
			Denominator: fixtureDenominator,
			Members:     []sealedKey{member},
		})
		put(t, &builder, second, Content[sealedKey, string]{Denominator: fixtureDenominator})
		sealed, err := builder.Seal()
		if err != nil {
			t.Fatalf("seal: %v", err)
		}
		if _, status := Read(&sealed, first, member); status != ReadProvenAbsent {
			t.Fatalf("sealing column status = %v, want proven-absent", status)
		}
		if _, status := Read(&sealed, second, member); status != ReadProvenAbsent {
			t.Fatalf("sharing column status = %v, want proven-absent", status)
		}
		runtime.KeepAlive(owner)
	})
}

// TestKeyHashCoversEveryKeyShape is the key-type law. A column may be keyed by
// any comparable shape the schedule admits, so every admitted shape must hash
// two equal keys alike and two different keys apart. The table covers the
// shapes the five domain columns and this package's own directory use, and the
// scalar shapes the schedule derives a step of its own for.
func TestKeyHashCoversEveryKeyShape(t *testing.T) {
	owner, other := &seal{rows: []int{1}}, &seal{rows: []int{1}}
	channel, secondChannel := make(chan int), make(chan int)
	first, second := 1, 2

	assertKeyHashLaw(t, "bool", true, true, false)
	assertKeyHashLaw(t, "int", 7, 7, 8)
	assertKeyHashLaw(t, "int8", int8(-3), int8(-3), int8(3))
	assertKeyHashLaw(t, "uint64", uint64(1)<<63, uint64(1)<<63, uint64(1)<<62)
	assertKeyHashLaw(t, "uintptr", uintptr(0xDEAD), uintptr(0xDEAD), uintptr(0xBEEF))
	assertKeyHashLaw(t, "float32", float32(1.5), float32(1.5), float32(-1.5))
	assertKeyHashLaw(t, "float64", 1.5, 1.5, -1.5)
	assertKeyHashLaw(t, "complex64", complex64(complex(1, 2)), complex64(complex(1, 2)), complex64(complex(2, 1)))
	assertKeyHashLaw(t, "complex128", complex(1, 2), complex(1, 2), complex(2, 1))
	assertKeyHashLaw(t, "string", "present", string([]byte("present")), "absent")
	assertKeyHashLaw(t, "byte array", [3]byte{1, 2, 3}, [3]byte{1, 2, 3}, [3]byte{1, 2, 4})
	assertKeyHashLaw(t, "int array", [2]int{5, 6}, [2]int{5, 6}, [2]int{6, 5})
	assertKeyHashLaw(t, "string array", [2]string{"a", "b"}, [2]string{"a", string([]byte("b"))}, [2]string{"a", "c"})
	assertKeyHashLaw(t, "content identity", identity.ContentID{0x01, 0x02}, identity.ContentID{0x01, 0x02}, identity.ContentID{0x01, 0x03})
	assertKeyHashLaw(t, "mount identity", identity.MountID{0x01}, identity.MountID{0x01}, identity.MountID{0x02})
	assertKeyHashLaw(t, "padded record", record{Weight: 1, Marked: true}, record{Weight: 1, Marked: true}, record{Weight: 1})
	assertKeyHashLaw(t, "sealed key", sealedKey{owner: owner, index: 2}, sealedKey{owner: owner, index: 2}, sealedKey{owner: owner, index: 3})
	assertKeyHashLaw(t, "nested struct", boxedKey{padding: 1, key: sealedKey{owner: owner, index: 2}},
		boxedKey{padding: 1, key: sealedKey{owner: owner, index: 2}},
		boxedKey{padding: 1, key: sealedKey{owner: other, index: 2}})
	assertKeyHashLaw(t, "channel", channel, channel, secondChannel)
	assertKeyHashLaw(t, "pointer", &first, &first, &second)
	assertKeyHashLaw(t, "unsafe pointer", unsafe.Pointer(&first), unsafe.Pointer(&first), unsafe.Pointer(&second))

	runtime.KeepAlive(owner)
	runtime.KeepAlive(other)
}

// TestSealedKeysHashByOwnerIdentity is the pointer-key law, and it is the one
// the five domain columns depend on. Their keys are an owner pointer and a
// dense index, so two keys with equal contents issued by two seals are not
// equal keys at all: the pointer is the fence that keeps one seal's dense
// index from answering in another seal's column. The hash follows equality
// exactly, which means it follows the pointer, and a column keyed this way
// answers a key from another seal with a miss rather than with the other
// seal's row.
func TestSealedKeysHashByOwnerIdentity(t *testing.T) {
	plan := mustPlan[sealedKey]()
	owner := &seal{rows: []int{1, 2, 3}}
	twin := &seal{rows: []int{1, 2, 3}}
	fromOwner := sealedKey{owner: owner, index: 2}
	fromTwin := sealedKey{owner: twin, index: 2}

	if fromOwner == fromTwin {
		t.Fatal("two seals issue equal keys, so this fixture proves nothing")
	}
	if hashKey(plan, fromOwner) == hashKey(plan, fromTwin) {
		t.Fatal("keys fenced to two seals hash alike, so the fence costs a comparison on every read")
	}
	if hashKey(plan, fromOwner) != hashKey(plan, sealedKey{owner: owner, index: 2}) {
		t.Fatal("two equal keys of one seal hash differently")
	}
	if hashKey(plan, sealedKey{index: 2}) == hashKey(plan, fromOwner) {
		t.Fatal("an unissued key hashes like an issued one")
	}

	schema := identity.ContentID{0x6B}
	axis := Axis[sealedKey, int]{SchemaID: schema, Slot: 0}
	builder := NewBuilder(schema, identity.StoreID(5), identity.Generation(1))
	put(t, &builder, axis, Content[sealedKey, int]{
		Rows:        map[sealedKey]int{fromOwner: 41},
		Denominator: fixtureDenominator,
		Members:     []sealedKey{fromOwner},
	})
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if value, status := Read(&sealed, axis, fromOwner); value != 41 || status != ReadHit {
		t.Fatalf("issuing seal's key = (%d, %v), want (41, hit)", value, status)
	}
	if value, status := Read(&sealed, axis, fromTwin); value != 0 || status != ReadMiss {
		t.Fatalf("another seal's key = (%d, %v), want (0, miss)", value, status)
	}
	runtime.KeepAlive(owner)
	runtime.KeepAlive(twin)
}

// TestKeyHashSpreadsARepresentativeCorpus is the distribution law. The trie
// branches on five bits of a hash at a time, so a schedule that leaves
// structure in a key's bits publishes a column that reads by scanning. Every
// key shape a column uses is corpus-tested: distinct keys must occupy distinct
// hashes, and the branch a key takes at the root must be spread across the
// whole branch factor rather than clustered by the corpus's own structure.
func TestKeyHashSpreadsARepresentativeCorpus(t *testing.T) {
	const corpus = 1 << 15

	assertHashSpread(t, "sequential integers", corpus, func(index int) int64 { return int64(index) })
	assertHashSpread(t, "sparse integers", corpus, func(index int) int64 { return int64(index) << 12 })
	assertHashSpread(t, "row names", corpus, func(index int) string { return fmt.Sprintf("row-%d", index) })
	assertHashSpread(t, "content identities", corpus, func(index int) identity.ContentID {
		var id identity.ContentID
		id[0], id[1], id[2] = byte(index), byte(index>>8), byte(index>>16)
		return id
	})
	owner := &seal{rows: []int{1}}
	assertHashSpread(t, "sealed keys", corpus, func(index int) sealedKey {
		return sealedKey{owner: owner, index: uint32(index)}
	})
	assertHashSpread(t, "padded records", corpus, func(index int) record {
		return record{Weight: uint64(index), Reach: uint64(index) * 3, Marked: index%2 == 0}
	})
	runtime.KeepAlive(owner)
}

// TestKeyHashSeparatesVariableLengthFields is the field-boundary law. A key of
// two strings is equal to another only when both fields are equal field by
// field, so the schedule must hash the boundary between them: without it the
// schedule hashes the concatenation, and every pair of keys that splits one
// character sequence at a different point collides over the whole hash width.
// Such keys land in one scanned collision node, so a column keyed by two
// variable length fields answers by linear scan instead of by branch.
func TestKeyHashSeparatesVariableLengthFields(t *testing.T) {
	type qualified struct {
		Module string
		Name   string
	}
	plan := mustPlan[qualified]()
	split := qualified{Module: "analysis", Name: "snapshot"}
	shifted := qualified{Module: "analysi", Name: "ssnapshot"}
	if split == shifted {
		t.Fatal("two differently split keys are equal, so this fixture proves nothing")
	}
	if hashKey(plan, split) == hashKey(plan, shifted) {
		t.Fatalf("keys %+v and %+v hash alike: the schedule hashes the concatenation of its "+
			"variable length fields and never their boundary", split, shifted)
	}

	arrayPlan := mustPlan[[2]string]()
	if hashKey(arrayPlan, [2]string{"ab", "c"}) == hashKey(arrayPlan, [2]string{"a", "bc"}) {
		t.Fatal("an array of two strings hashes by concatenation")
	}
}

// TestKeyShapesTheScheduleRefuses fixes what a column cannot be keyed by. A
// key whose equality is dynamic cannot be hashed structurally, and the refusal
// is total: an interface anywhere inside a key, at any depth and behind any
// number of arrays, refuses the whole schedule rather than hashing the parts
// it does understand.
func TestKeyShapesTheScheduleRefuses(t *testing.T) {
	type inner struct{ Dynamic any }
	type outer struct {
		Count uint32
		Inner inner
	}
	if _, hashable := planFor[struct{ Dynamic any }](); hashable {
		t.Fatal("a struct holding an interface derives a schedule")
	}
	if _, hashable := planFor[[2]any](); hashable {
		t.Fatal("an array of interfaces derives a schedule")
	}
	if _, hashable := planFor[outer](); hashable {
		t.Fatal("an interface nested two structs deep derives a schedule")
	}
	if _, hashable := planFor[[2][3]any](); hashable {
		t.Fatal("a nested array of interfaces derives a schedule")
	}
	if _, hashable := planFor[struct{ Keys [2]any }](); hashable {
		t.Fatal("a struct holding an array of interfaces derives a schedule")
	}
	if _, hashable := planFor[error](); hashable {
		t.Fatal("a named interface derives a schedule")
	}
}

// TestInternalKeyTypeIsHashableOrFatal fixes what mustPlan is for. The
// directory, the denominator publication, the mount bindings and the query
// publication are keyed by types this package fixes at compile time, so a type
// that cannot be hashed is a defect in this package rather than a caller's
// input, and it stops publication instead of producing a column that answers
// by scanning.
func TestInternalKeyTypeIsHashableOrFatal(t *testing.T) {
	if identityPlan == nil || mountPlan == nil {
		t.Fatal("the package's own key schedules were not derived")
	}
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("an unhashable internal key type derives a schedule")
		}
		if message, named := recovered.(string); !named || message != "snapshot: internal key type is not hashable" {
			t.Fatalf("panic = %v, want the internal key type refusal", recovered)
		}
	}()
	mustPlan[any]()
}

// TestKeyWithoutEqualityRelevantBytes fixes the degenerate schedule. A key type
// with no equality relevant bytes has exactly one value, so its schedule is
// empty, every key hashes alike, and the column it keys holds one row.
func TestKeyWithoutEqualityRelevantBytes(t *testing.T) {
	plan, hashable := planFor[struct{}]()
	if !hashable {
		t.Fatal("a key with no fields is refused")
	}
	if len(plan.steps) != 0 {
		t.Fatalf("schedule = %+v, want no steps", plan.steps)
	}
	if hashKey(plan, struct{}{}) != hashKey(plan, struct{}{}) {
		t.Fatal("the one value of a single valued key hashes two ways")
	}

	schema := identity.ContentID{0x7C}
	axis := Axis[struct{}, int]{SchemaID: schema, Slot: 0}
	builder := NewBuilder(schema, identity.StoreID(9), identity.Generation(1))
	put(t, &builder, axis, Content[struct{}, int]{Rows: map[struct{}]int{{}: 5}})
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if value, status := Read(&sealed, axis, struct{}{}); value != 5 || status != ReadHit {
		t.Fatalf("single valued key = (%d, %v), want (5, hit)", value, status)
	}
}

// assertSchedulesAgree fixes that two schedules derived for one key type hash
// one key alike, which is what lets a denominator sealed by one column answer
// under the hash another column computed.
func assertSchedulesAgree[K comparable](t *testing.T, name string, key K) {
	t.Helper()
	first, hashable := planFor[K]()
	if !hashable {
		t.Fatalf("%s: key type derives no schedule", name)
	}
	second, hashable := planFor[K]()
	if !hashable {
		t.Fatalf("%s: key type derives no schedule on a second derivation", name)
	}
	if first == second {
		t.Fatalf("%s: two derivations returned one schedule, so this fixture proves nothing", name)
	}
	if len(first.steps) != len(second.steps) {
		t.Fatalf("%s: schedules differ: %+v and %+v", name, first.steps, second.steps)
	}
	for index := range first.steps {
		if first.steps[index] != second.steps[index] {
			t.Fatalf("%s: step %d differs: %+v and %+v", name, index, first.steps[index], second.steps[index])
		}
	}
	if hashKey(first, key) != hashKey(second, key) {
		t.Fatalf("%s: two schedules hash one key differently", name)
	}
}

// assertKeyHashLaw fixes the hashing law on one key shape: two equal keys hash
// alike and two different keys do not.
func assertKeyHashLaw[K comparable](t *testing.T, name string, key, equal, different K) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		plan, hashable := planFor[K]()
		if !hashable {
			t.Fatalf("key shape derives no schedule")
		}
		if key != equal {
			t.Fatalf("fixture keys %v and %v are not equal, so this fixture proves nothing", key, equal)
		}
		if key == different {
			t.Fatalf("fixture keys %v and %v are equal, so this fixture proves nothing", key, different)
		}
		if hashKey(plan, key) != hashKey(plan, equal) {
			t.Fatalf("two equal keys hash differently")
		}
		if hashKey(plan, key) == hashKey(plan, different) {
			t.Fatalf("two different keys hash alike")
		}
	})
}

// assertHashSpread fixes the distribution law on one key shape: a corpus of
// distinct keys occupies distinct hashes, and the root branch each key takes
// is spread across the whole branch factor.
func assertHashSpread[K comparable](t *testing.T, name string, corpus int, keyAt func(int) K) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		plan, hashable := planFor[K]()
		if !hashable {
			t.Fatal("key shape derives no schedule")
		}
		hashes := make(map[uint64]K, corpus)
		var branches [trieWidth]int
		for index := 0; index < corpus; index++ {
			key := keyAt(index)
			hash := hashKey(plan, key)
			if held, taken := hashes[hash]; taken && held != key {
				t.Fatalf("keys %v and %v collide over the whole hash width", held, key)
			}
			hashes[hash] = key
			branches[hash&trieMask]++
		}
		if len(hashes) != corpus {
			t.Fatalf("a corpus of %d distinct keys occupies %d hashes", corpus, len(hashes))
		}
		// The corpus is large enough that a branch this far from the mean is
		// structure the schedule failed to spread rather than sampling noise:
		// the expected occupancy is corpus/32 with a standard deviation near
		// its square root, so the bound is many deviations wide.
		expected := corpus / trieWidth
		low, high := expected-expected/3, expected+expected/3
		for branch, occupancy := range branches {
			if occupancy < low || occupancy > high {
				t.Fatalf("root branch %d holds %d of %d keys, want between %d and %d",
					branch, occupancy, corpus, low, high)
			}
		}
	})
}

// throughKey passes a key by value so the copy under test is a real parameter
// copy rather than an alias.
func throughKey(key sealedKey) sealedKey { return key }
