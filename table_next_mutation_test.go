package lua

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

func TestTableNextDeletedSparseKeyBecomesArray(t *testing.T) {
	for _, grow := range []struct {
		name string
		set  func(*LTable, int)
	}{
		{"int", func(tbl *LTable, i int) { tbl.RawSetInt(i, LNumber(i)) }},
		{"number", func(tbl *LTable, i int) { tbl.RawSet(LNumber(i), LNumber(i)) }},
		{"integer", func(tbl *LTable, i int) { tbl.RawSet(LInteger(i), LNumber(i)) }},
		{"append", func(tbl *LTable, i int) { tbl.Append(LNumber(i)) }},
		{"insert", func(tbl *LTable, i int) { tbl.Insert(1, LNumber(i)) }},
		{"gap", func(tbl *LTable, i int) {
			if i%16 == 1 {
				end := min(i+15, 65)
				tbl.RawSetInt(end, LNumber(end))
			}
			tbl.RawSetInt(i, LNumber(i))
		}},
	} {
		for _, clearCurrent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/clear=%t", grow.name, clearCurrent), func(t *testing.T) {
				tbl := newLTable(0, 0)
				tbl.RawSetString("tail", LTrue)
				tbl.RawSetInt(64, LTrue)
				sparseVisitOnce(t, tbl, 2)
				tbl.RawSetInt(64, LNil)
				for i := 1; i <= 65; i++ {
					grow.set(tbl, i)
				}
				if key, value := tbl.Next(LInteger(64)); key != LNumber(65) || value != tbl.RawGetInt(65) {
					t.Fatalf("integer cursor skipped array tail: %v, %v", key, value)
				}
				seen := make(map[LValue]bool)
				for key, value := tbl.Next(LNil); key != LNil; key, value = tbl.Next(key) {
					if seen[key] || value != tbl.RawGet(key) || len(seen) >= 66 {
						t.Fatalf("invalid traversal at %v, %v", key, value)
					}
					seen[key] = true
					if clearCurrent {
						tbl.RawSet(key, LNil)
					}
				}
				if len(seen) != 66 {
					t.Fatalf("visited %d keys, want 66 (deleted sparse key became an array key)", len(seen))
				}
			})
		}
	}
}

func TestTableNextExistingUpdatesPreserveDeletedCursors(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.Next(LNil)
	keys := []LValue{LString("first"), newLTable(0, 0), LFalse, LNumber(100000), LString("last")}
	for _, key := range keys {
		tbl.RawSet(key, LTrue)
	}
	for i, key := range keys {
		tbl.RawSet(key, LNil)
		// Repeated deletion and overwriting live fields are allowed during next.
		tbl.RawSet(key, LNil)
		for _, remaining := range keys[i+1:] {
			tbl.RawSet(remaining, LFalse)
		}
		sparseVisitOnce(t, tbl, len(keys)-i-1)
		got, value := tbl.Next(key)
		if i+1 < len(keys) {
			if got != keys[i+1] || value != LFalse {
				t.Fatalf("deleted cursor %v resumed at %v, %v", key, got, value)
			}
		} else if got != LNil || value != LNil {
			t.Fatalf("last deleted cursor returned %v, %v", got, value)
		}
	}
}

func TestTableNextDeletedForcedHashIntegerBecomesArray(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetH(LInteger(64), LTrue)
	tbl.Next(LNil)
	tbl.RawSetH(LInteger(64), LNil)
	for i := 1; i <= 65; i++ {
		tbl.Append(LNumber(i))
	}
	if key, value := tbl.Next(LInteger(64)); key != LNumber(65) || value != LNumber(65) {
		t.Fatalf("deleted integer hash cursor hid array tail: %v, %v", key, value)
	}
	sparseVisitOnce(t, tbl, 65)
}

func TestTableNextDeletedSparseCursorAfterArrayRemove(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetString("tail", LTrue)
	tbl.RawSetInt(64, LTrue)
	tbl.Next(LNil)
	tbl.RawSetInt(64, LNil)
	for i := 1; i <= 64; i++ {
		tbl.Append(LNumber(i))
	}
	if value, ok := tbl.Remove(64); !ok || value != LNumber(64) {
		t.Fatal("array removal failed")
	}
	if key, value := tbl.Next(LNumber(64)); key != LString("tail") || value != LTrue {
		t.Fatalf("removed array tail resurrected its old sparse cursor: %v, %v", key, value)
	}
}

func TestTableNextOverwriteSparsePreservesCursor(t *testing.T) {
	for _, overwriteCurrent := range []bool{false, true} {
		t.Run(fmt.Sprint(overwriteCurrent), func(t *testing.T) {
			tbl := newLTable(0, 0)
			tbl.RawSetInt(64, LTrue)
			tbl.RawSetInt(32, LTrue) // 64 is now near the array but remains hashed.
			tbl.RawSetString("middle", LTrue)
			key, _ := tbl.Next(LNil)
			key, _ = tbl.Next(key)
			if key != LString("middle") {
				t.Fatalf("unexpected initial cursor: %v", key)
			}
			if overwriteCurrent {
				key, _ = tbl.Next(key)
			}
			tbl.RawSetInt(64, LFalse)
			next, value := tbl.Next(key)
			if overwriteCurrent {
				if next != LNil || value != LNil {
					t.Fatalf("overwriting current sparse key repeated a key: %v, %v", next, value)
				}
			} else if next != LNumber(64) || value != LFalse {
				t.Fatalf("overwriting unvisited sparse key skipped it: %v, %v", next, value)
			}
		})
	}
}

func TestTablePairsAfterSparseDeleteAndArrayGrowth(t *testing.T) {
	state := NewState()
	defer state.Close()
	if err := state.DoString(`
local t = {[64] = true}
assert(next(t) == 64)
t[64] = nil
for i = 1, 65 do t[i] = i end
local count, seen = 0, {}
for k, v in pairs(t) do
  assert(k == v and not seen[k])
  seen[k] = true
  count = count + 1
  assert(count <= 65)
  t[k] = nil
end
assert(count == 65, 'deleted sparse cursor hid the array tail')
assert(next(t) == nil)
`); err != nil {
		t.Fatal(err)
	}
}

func TestTableNextMutationModel(t *testing.T) {
	for _, seed := range []int64{1, 42, 123} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			tbl := newLTable(0, 0)
			keys := []LValue{LString("a"), LString("b"), LFalse, LTrue, newLTable(0, 0)}
			for i := 1; i <= 96; i++ {
				keys = append(keys, LNumber(i))
			}
			keys = append(keys, LNumber(100000), LNumber(200000))
			model := make(map[LValue]LValue)
			for step := 0; step < 2000; step++ {
				key := keys[rng.Intn(len(keys))]
				value := LValue(LNumber(step))
				if rng.Intn(3) == 0 {
					value = LNil
					delete(model, key)
				} else {
					model[key] = value
				}
				tbl.RawSet(key, value)
				seen := make(map[LValue]bool)
				for k, v := tbl.Next(LNil); k != LNil; k, v = tbl.Next(k) {
					if seen[k] || model[k] != v || len(seen) >= len(model) {
						t.Fatalf("step %d: unexpected or repeated key %v, value %v", step, k, v)
					}
					seen[k] = true
				}
				if len(seen) != len(model) {
					t.Fatalf("step %d: visited %d/%d keys", step, len(seen), len(model))
				}
			}
		})
	}
}

func TestTableNextChurnMetadataBound(t *testing.T) {
	const live = 128
	tbl := newLTable(0, 0)
	for i := 0; i < live; i++ {
		tbl.RawSetString(fmt.Sprint(i), LTrue)
	}
	tbl.Next(LNil)
	for i := 0; i < live*10; i++ {
		tbl.RawSetString(fmt.Sprint(i), LNil)
		tbl.RawSetString(fmt.Sprint(i+live), LTrue)
		if len(tbl.Keys) > 2*live || len(tbl.K2i) > 2*live {
			t.Fatal("churn retained unbounded cursor metadata")
		}
		sparseVisitOnce(t, tbl, live)
	}
}

func BenchmarkTableHashDeleteReinsert(b *testing.B) {
	for _, size := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			tbl := newLTable(0, size)
			for i := 0; i < size; i++ {
				tbl.RawSetString(fmt.Sprint(i), LTrue)
			}
			tbl.Next(LNil)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tbl.RawSetString("0", LNil)
				tbl.RawSetString("0", LTrue)
			}
		})
	}
}

func TestTableDeletedObjectsAreCollectible(t *testing.T) {
	collected := make(chan string, 2)
	tbl := newLTable(0, 0)
	func() {
		key := newLTable(0, 0)
		value := &LUserData{Value: make([]byte, 1024)}
		runtime.SetFinalizer(key, func(*LTable) { collected <- "key" })
		runtime.SetFinalizer(value, func(*LUserData) { collected <- "value" })
		tbl.RawSetH(key, value)
		tbl.Next(LNil)
		tbl.RawSetH(key, LNil)
	}()
	// Keep the owning table alive: collection must follow deletion, rather
	// than collection of the whole table or a later compaction/insertion.
	defer runtime.KeepAlive(tbl)
	deadline := time.Now().Add(5 * time.Second)
	seen := make(map[string]bool)
	for len(seen) < 2 && time.Now().Before(deadline) {
		runtime.GC()
		select {
		case name := <-collected:
			seen[name] = true
		case <-time.After(10 * time.Millisecond):
		}
	}
	if len(seen) != 2 {
		t.Fatalf("deleted objects retained by live table; collected: %v", seen)
	}
}
