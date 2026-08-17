package snapshot

import (
	"errors"
	"math/rand"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestTrieAnswersLikeAMap is the storage law. The persistent trie is the one
// place a published row lives, so it must answer exactly what the flat
// mapping it replaces would answer under any sequence of writes and
// withdrawals, including for keys it never held.
func TestTrieAnswersLikeAMap(t *testing.T) {
	const (
		writes      = 4000
		withdrawals = 1500
		probes      = 4000
	)
	plan := mustPlan[int64]()
	source := rand.New(rand.NewSource(20255))
	expected := make(map[int64]int64, writes)
	var stored *trie[int64, int64]

	for write := 0; write < writes; write++ {
		key := source.Int63n(writes * 2)
		value := source.Int63()
		expected[key] = value
		stored, _ = trieInsert(stored, 0, trieEntry[int64, int64]{hash: hashKey(plan, key), key: key, value: value})
	}
	for withdrawal := 0; withdrawal < withdrawals; withdrawal++ {
		key := source.Int63n(writes * 2)
		delete(expected, key)
		stored, _ = trieRemove(stored, 0, hashKey(plan, key), key)
	}
	for probe := int64(0); probe < probes; probe++ {
		key := source.Int63n(writes * 3)
		value, held := trieLookup(stored, hashKey(plan, key), key)
		wantValue, wantHeld := expected[key]
		if held != wantHeld || value != wantValue {
			t.Fatalf("key %d = (%d, %t), want (%d, %t)", key, value, held, wantValue, wantHeld)
		}
	}
	for key, value := range expected {
		if held, stillHeld := trieLookup(stored, hashKey(plan, key), key); !stillHeld || held != value {
			t.Fatalf("key %d = (%d, %t), want (%d, true)", key, held, stillHeld, value)
		}
	}
	for key := range expected {
		stored, _ = trieRemove(stored, 0, hashKey(plan, key), key)
	}
	if stored != nil {
		t.Fatal("a trie emptied of every row is not empty")
	}
}

// TestTrieBuildAgreesWithInsertion fixes that sealing a whole column and
// inserting its rows one by one publish the same storage. Sealing constructs
// each node once instead of copying paths, so the two write modes must be
// indistinguishable to every read, and a key offered twice is one row.
func TestTrieBuildAgreesWithInsertion(t *testing.T) {
	const rows = 3000
	plan := mustPlan[int64]()
	source := rand.New(rand.NewSource(7717))
	entries := make([]trieEntry[int64, int64], 0, rows)
	expected := make(map[int64]int64, rows)
	for row := 0; row < rows; row++ {
		key := source.Int63n(rows)
		value := source.Int63()
		entries = append(entries, trieEntry[int64, int64]{hash: hashKey(plan, key), key: key, value: value})
		expected[key] = value
	}
	var inserted *trie[int64, int64]
	for _, entry := range entries {
		inserted, _ = trieInsert(inserted, 0, entry)
	}
	built := trieBuild(entries, make([]trieEntry[int64, int64], len(entries)), 0)
	for key, value := range expected {
		hash := hashKey(plan, key)
		fromBuild, built := trieLookup(built, hash, key)
		fromInsert, inserted := trieLookup(inserted, hash, key)
		if !built || !inserted || fromBuild != value || fromInsert != value {
			t.Fatalf("key %d = built (%d, %t), inserted (%d, %t), want %d", key, fromBuild, built, fromInsert, inserted, value)
		}
	}
	for key := int64(rows); key < rows*2; key++ {
		hash := hashKey(plan, key)
		if _, held := trieLookup(built, hash, key); held {
			t.Fatalf("a sealed column holds key %d, which was never offered", key)
		}
	}
	for key := range expected {
		var removed bool
		built, removed = trieRemove(built, 0, hashKey(plan, key), key)
		if !removed {
			t.Fatalf("a sealed column does not hold key %d as one row", key)
		}
	}
	if built != nil {
		t.Fatal("a sealed column emptied of every row is not empty, so a key was stored twice")
	}
}

// TestTrieHoldsCollidingHashes fixes what happens when two keys agree over
// the whole hash width. The trie stores them together and still answers each
// one by key, so a hash collision costs a comparison and never an answer.
func TestTrieHoldsCollidingHashes(t *testing.T) {
	var stored *trie[int, string]
	for _, key := range []int{1, 2, 3} {
		stored, _ = trieInsert(stored, 0, trieEntry[int, string]{hash: 0x99, key: key, value: name(key)})
	}
	for _, key := range []int{1, 2, 3} {
		if value, held := trieLookup(stored, 0x99, key); !held || value != name(key) {
			t.Fatalf("colliding key %d = (%q, %t), want (%q, true)", key, value, held, name(key))
		}
	}
	if _, held := trieLookup(stored, 0x99, 4); held {
		t.Fatal("a key that only shares a hash reads as held")
	}
	replaced, added := trieInsert(stored, 0, trieEntry[int, string]{hash: 0x99, key: 2, value: "replaced"})
	if added {
		t.Fatal("replacing a colliding row reports a new row")
	}
	if value, _ := trieLookup(replaced, 0x99, 2); value != "replaced" {
		t.Fatalf("replaced colliding row = %q", value)
	}
	if value, _ := trieLookup(stored, 0x99, 2); value != name(2) {
		t.Fatalf("replacement reached the trie it derived from: %q", value)
	}
	withdrawn, removed := trieRemove(replaced, 0, 0x99, 2)
	if !removed {
		t.Fatal("removing a colliding row reports nothing removed")
	}
	if _, held := trieLookup(withdrawn, 0x99, 2); held {
		t.Fatal("a withdrawn colliding row still answers")
	}
	for _, key := range []int{1, 3} {
		if _, held := trieLookup(withdrawn, 0x99, key); !held {
			t.Fatalf("withdrawing one colliding row lost key %d", key)
		}
	}
}

// TestTrieDerivationsAreIndependent is the persistence law. Two publications
// derived from one trie hold their own rows and neither reaches the other or
// the trie they were derived from.
func TestTrieDerivationsAreIndependent(t *testing.T) {
	plan := mustPlan[int]()
	var base *trie[int, int]
	for key := 0; key < 200; key++ {
		base, _ = trieInsert(base, 0, trieEntry[int, int]{hash: hashKey(plan, key), key: key, value: key})
	}
	first, _ := trieInsert(base, 0, trieEntry[int, int]{hash: hashKey(plan, 7), key: 7, value: 700})
	second, _ := trieRemove(base, 0, hashKey(plan, 7), 7)

	if value, _ := trieLookup(base, hashKey(plan, 7), 7); value != 7 {
		t.Fatalf("base row = %d, want 7", value)
	}
	if value, _ := trieLookup(first, hashKey(plan, 7), 7); value != 700 {
		t.Fatalf("derived row = %d, want 700", value)
	}
	if _, held := trieLookup(second, hashKey(plan, 7), 7); held {
		t.Fatal("a withdrawn row answers in the publication that withdrew it")
	}
	for key := 0; key < 200; key++ {
		if key == 7 {
			continue
		}
		for name, held := range map[string]*trie[int, int]{"base": base, "set": first, "removed": second} {
			if value, stored := trieLookup(held, hashKey(plan, key), key); !stored || value != key {
				t.Fatalf("%s lost row %d as (%d, %t)", name, key, value, stored)
			}
		}
	}
}

// TestTrieAnswersWhereItHoldsNothing fixes the edges of the storage where
// there is nothing to walk. Sealing no rows publishes no storage rather than
// an empty node, withdrawing from storage that holds nothing changes nothing,
// and withdrawing a key that only shares a hash with the rows a collision node
// holds leaves that node exactly as it was.
func TestTrieAnswersWhereItHoldsNothing(t *testing.T) {
	t.Run("sealing no rows publishes no storage", func(t *testing.T) {
		if built := trieBuild[int, int](nil, nil, 0); built != nil {
			t.Fatal("sealing no rows published a node")
		}
		if built := trieBuild([]trieEntry[int, int]{}, []trieEntry[int, int]{}, 0); built != nil {
			t.Fatal("sealing an empty row set published a node")
		}
	})
	t.Run("withdrawing from no storage", func(t *testing.T) {
		plan := mustPlan[int]()
		withdrawn, removed := trieRemove[int, int](nil, 0, hashKey(plan, 1), 1)
		if withdrawn != nil || removed {
			t.Fatalf("withdrawal from no storage = (%v, %t), want (nil, false)", withdrawn, removed)
		}
	})
	t.Run("withdrawing a key a collision node does not hold", func(t *testing.T) {
		const shared = uint64(0x99)
		var stored *trie[int, string]
		for _, key := range []int{1, 2} {
			stored, _ = trieInsert(stored, 0, trieEntry[int, string]{hash: shared, key: key, value: name(key)})
		}
		withdrawn, removed := trieRemove(stored, 0, shared, 3)
		if removed {
			t.Fatal("withdrawing a key the storage never held reports a removal")
		}
		if withdrawn != stored {
			t.Fatal("a withdrawal that removed nothing copied the storage it walked")
		}
		for _, key := range []int{1, 2} {
			if value, held := trieLookup(withdrawn, shared, key); !held || value != name(key) {
				t.Fatalf("colliding row %d = (%q, %t) after a withdrawal of another key", key, value, held)
			}
		}
	})
}

// TestKeyHashFollowsEquality is the hashing law. A key is hashed by what
// makes two keys equal and by nothing else: two equal keys hash alike whether
// they share storage or not, padding a struct carries never enters the hash,
// and the two zeros of a float hash alike.
func TestKeyHashFollowsEquality(t *testing.T) {
	t.Run("string contents rather than header", func(t *testing.T) {
		plan := mustPlan[string]()
		first := "materialized"
		second := string([]byte("materialized"))
		if hashKey(plan, first) != hashKey(plan, second) {
			t.Fatal("two equal strings in different storage hash differently")
		}
		if hashKey(plan, first) == hashKey(plan, "materialised") {
			t.Fatal("two different strings hash alike")
		}
	})
	t.Run("padding is not part of a key", func(t *testing.T) {
		type padded struct {
			Flag  bool
			Count uint64
		}
		plan := mustPlan[padded]()
		first := padded{Flag: true, Count: 9}
		second := padded{Flag: true, Count: 9}
		scribble := (*[unsafe.Sizeof(padded{})]byte)(unsafe.Pointer(&second))
		for index := uintptr(1); index < unsafe.Offsetof(second.Count); index++ {
			scribble[index] = 0xAB
		}
		if first != second {
			t.Fatal("padding entered equality, so this fixture proves nothing")
		}
		if hashKey(plan, first) != hashKey(plan, second) {
			t.Fatal("padding entered the hash")
		}
	})
	t.Run("the two zeros of a float", func(t *testing.T) {
		plan := mustPlan[float64]()
		positive, negative := 0.0, math0Negative()
		if positive != negative {
			t.Fatal("the two zeros are not equal, so this fixture proves nothing")
		}
		if hashKey(plan, positive) != hashKey(plan, negative) {
			t.Fatal("two equal zeros hash differently")
		}
	})
	t.Run("a struct of a string and a number", func(t *testing.T) {
		type composite struct {
			Name  string
			Count float32
		}
		plan := mustPlan[composite]()
		first := composite{Name: "answer", Count: 2}
		second := composite{Name: string([]byte("answer")), Count: 2}
		if hashKey(plan, first) != hashKey(plan, second) {
			t.Fatal("two equal composite keys hash differently")
		}
		if hashKey(plan, first) == hashKey(plan, composite{Name: "answer", Count: 3}) {
			t.Fatal("two different composite keys hash alike")
		}
	})
	t.Run("identities", func(t *testing.T) {
		first := identity.ContentID{0x01, 0x02}
		if hashKey(identityPlan, first) != hashKey(identityPlan, identity.ContentID{0x01, 0x02}) {
			t.Fatal("two equal identities hash differently")
		}
		if hashKey(identityPlan, first) == hashKey(identityPlan, identity.ContentID{0x01, 0x03}) {
			t.Fatal("two different identities hash alike")
		}
	})
}

// TestKeyPlanCoalescesFlatKeys fixes the shape the hashing schedule derives,
// because a read pays for the schedule on every key it hashes. A key whose
// equality is its bytes hashes in one pass, and only the parts of a key that
// are not raw bytes cost a step of their own.
func TestKeyPlanCoalescesFlatKeys(t *testing.T) {
	type composite struct {
		Name  string
		Count float64
	}
	cases := []struct {
		name  string
		plan  *keyPlan
		steps []keyStep
	}{
		{name: "integer", plan: mustPlan[int64](), steps: []keyStep{{kind: keyBytes, size: 8}}},
		{name: "identity", plan: mustPlan[identity.ContentID](), steps: []keyStep{{kind: keyBytes, size: 32}}},
		{
			name:  "padded struct",
			plan:  mustPlan[record](),
			steps: []keyStep{{kind: keyBytes, size: unsafe.Offsetof(record{}.Marked) + 1}},
		},
		{name: "string", plan: mustPlan[string](), steps: []keyStep{{kind: keyString, size: unsafe.Sizeof("")}}},
		{
			name: "string and float",
			plan: mustPlan[composite](),
			steps: []keyStep{
				{kind: keyString, size: unsafe.Sizeof("")},
				{kind: keyFloat64, offset: unsafe.Offsetof(composite{}.Count), size: 8},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if len(testCase.plan.steps) != len(testCase.steps) {
				t.Fatalf("schedule = %+v, want %+v", testCase.plan.steps, testCase.steps)
			}
			for index, step := range testCase.plan.steps {
				if step != testCase.steps[index] {
					t.Fatalf("step %d = %+v, want %+v", index, step, testCase.steps[index])
				}
			}
		})
	}
}

// TestUnhashableKeyIsRejected fixes what a column cannot be keyed by. An
// interface key compares by a dynamic type the sealed schedule cannot see, so
// the column is refused at construction rather than answered wrongly.
func TestUnhashableKeyIsRejected(t *testing.T) {
	if _, hashable := planFor[any](); hashable {
		t.Fatal("an interface key derives a hashing schedule")
	}
	builder := NewBuilder(fixtureSchema, fixtureStore, fixtureGeneration)
	err := PutColumn(&builder, Axis[any, int]{SchemaID: fixtureSchema, Slot: 0}, Content[any, int]{
		Rows: map[any]int{"present": 1},
	})
	if !errors.Is(err, ErrUnhashableKey) {
		t.Fatalf("error = %v, want %v", err, ErrUnhashableKey)
	}
}

// name renders a small key as a distinct stored value.
func name(key int) string { return string(rune('a' + key)) }

// math0Negative returns negative zero without a constant expression the
// compiler would fold into positive zero.
func math0Negative() float64 {
	zero := 0.0
	return -zero
}
