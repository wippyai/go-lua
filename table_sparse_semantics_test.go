package lua

import (
	"math/rand"
	"testing"
	"unsafe"
)

func TestSparseStorageBounds(t *testing.T) {
	for _, setter := range []struct {
		name string
		set  func(*LTable, int, LValue)
	}{
		{"integer API", func(tbl *LTable, key int, v LValue) { tbl.RawSetInt(key, v) }},
		{"float key", func(tbl *LTable, key int, v LValue) { tbl.RawSet(LNumber(key), v) }},
		{"integer key", func(tbl *LTable, key int, v LValue) { tbl.RawSet(LInteger(key), v) }},
	} {
		t.Run(setter.name, func(t *testing.T) {
			tbl := newLTable(0, 0)
			setter.set(tbl, 100000, LTrue)
			if got := tbl.RawGetInt(100000); got != LTrue {
				t.Fatalf("sparse read = %v", got)
			}
			if cap(tbl.Array) > defaultArrayCap*2 {
				t.Fatalf("one sparse key allocated %d array slots", cap(tbl.Array))
			}
			setter.set(tbl, 100000, LNil)
			if tbl.RawGetInt(100000) != LNil || len(tbl.Dict) != 0 {
				t.Fatal("sparse deletion retained the value")
			}
		})
	}
	for _, value := range []LValue{LNil, LFalse} {
		t.Run(value.String(), func(t *testing.T) {
			tbl := newLTable(0, 0)
			tbl.RawSetInt(100000, value)
			if tbl.RawGetInt(100000) != value || cap(tbl.Array) > defaultArrayCap*2 {
				t.Fatal("nil/false sparse assignment changed value or allocated a large array")
			}
		})
	}
}

func TestSparseGeometricKeysStayBounded(t *testing.T) {
	tbl := newLTable(0, 0)
	count := 0
	for key := 1; key <= 1<<16; key *= 2 {
		tbl.RawSetInt(key, LTrue)
		count++
		if cap(tbl.Array) > defaultArrayCap*(count+1) {
			t.Fatalf("%d geometric keys allocated %d array slots", count, cap(tbl.Array))
		}
	}
	for key := 1; key <= 1<<16; key *= 2 {
		if tbl.RawGetInt(key) != LTrue {
			t.Fatalf("lost geometric key %d", key)
		}
	}
}

func sparseVisitOnce(t *testing.T, tbl *LTable, want int) {
	t.Helper()
	seen := make(map[LValue]bool)
	key := LNil
	for steps := 0; steps <= want; steps++ {
		var value LValue
		key, value = tbl.Next(key)
		if key == LNil {
			if len(seen) != want {
				t.Fatalf("next visited %d keys, want %d", len(seen), want)
			}
			return
		}
		if seen[key] || value == LNil || value == nil {
			t.Fatalf("next repeated key %v or returned an empty value", key)
		}
		seen[key] = true
	}
	t.Fatal("next did not terminate within the table's key count")
}

func TestSparseIterationBoundaries(t *testing.T) {
	for _, dense := range []int{0, 1, 5} {
		for _, acap := range []int{0, 32} {
			tbl := newLTable(acap, 0)
			for i := 1; i <= dense; i++ {
				tbl.RawSetInt(i, LTrue)
			}
			tbl.RawSetInt(100000, LTrue)
			tbl.RawSet(LInteger(200000), LFalse)
			tbl.RawSet(LNumber(1.5), LTrue)
			tbl.RawSetInt(0, LTrue)
			tbl.RawSetInt(-1, LTrue)
			tbl.RawSet(LFalse, LTrue)
			tbl.RawSetString("name", LTrue)
			sparseVisitOnce(t, tbl, dense+7)
			sparseVisitOnce(t, tbl, dense+7)
			count := 0
			tbl.ForEach(func(_, _ LValue) { count++ })
			if count != dense+7 {
				t.Fatalf("ForEach count = %d", count)
			}
		}
	}
}

func TestSparsePromotionOrders(t *testing.T) {
	for _, order := range []string{"ascending", "descending", "shuffled"} {
		t.Run(order, func(t *testing.T) {
			tbl := newLTable(0, 0)
			tbl.RawSetInt(300, LNumber(300))
			// Initialize lazy iteration metadata before moving hash keys to the array.
			sparseVisitOnce(t, tbl, 1)
			keys := make([]int, 299)
			for i := range keys {
				keys[i] = i + 1
			}
			switch order {
			case "descending":
				for i, j := 0, len(keys)-1; i < j; i, j = i+1, j-1 {
					keys[i], keys[j] = keys[j], keys[i]
				}
			case "shuffled":
				rand.New(rand.NewSource(42)).Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
			}
			for _, key := range keys {
				tbl.RawSetInt(key, LNumber(key))
			}
			if tbl.Len() != 300 || len(tbl.Dict) != 0 {
				t.Fatalf("dense promotion: len=%d hash=%d", tbl.Len(), len(tbl.Dict))
			}
			for i := 1; i <= 300; i++ {
				if tbl.RawGetInt(i) != LNumber(i) {
					t.Fatalf("promotion lost key %d", i)
				}
			}
			sparseVisitOnce(t, tbl, 300)
		})
	}
}

func TestSparseAliasesAndMaximum(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetInt(100000, LTrue)
	if tbl.RawGet(LInteger(100000)) != LTrue || tbl.RawGet(LNumber(100000)) != LTrue {
		t.Fatal("integer/float aliases lost the sparse value")
	}
	tbl.RawSet(LInteger(100000), LFalse)
	if tbl.RawGetInt(100000) != LFalse || tbl.MaxN() != 100000 {
		t.Fatal("sparse overwrite or MaxN failed")
	}
	tbl.RawSet(LNumber(100000), LNil)
	if tbl.MaxN() != 0 || len(tbl.Dict) != 0 {
		t.Fatal("deleting the maximum key retained its value")
	}
}

func TestSparseIntegerConversionAndExplicitNext(t *testing.T) {
	tbl := newLTable(0, 0)
	key := LInteger(1<<32 + 1)
	tbl.RawSet(key, LTrue)
	if len(tbl.Array) != 0 || tbl.RawGet(key) != LTrue || tbl.RawGetInt(1) != LNil {
		t.Fatal("large integer key wrapped into the array-index range")
	}
	tbl.RawSetInt(1, LTrue)
	tbl.RawSetInt(2, LFalse)
	next, value := tbl.Next(LInteger(1))
	if next != LNumber(2) || value != LFalse {
		t.Fatalf("next after integer key 1 returned %v, %v", next, value)
	}
}

func TestSparseForcedHashIntegerIteration(t *testing.T) {
	for _, dense := range []int{0, 2} {
		tbl := newLTable(0, 0)
		for i := 1; i <= dense; i++ {
			tbl.RawSetInt(i, LTrue)
		}
		tbl.RawSetH(LInteger(100000), LFalse)
		tbl.RawSetH(LNumber(200000), LTrue)
		tbl.RawSetH(LInteger(0), LTrue)
		tbl.RawSetH(LInteger(-1), LTrue)
		sparseVisitOnce(t, tbl, dense+4)
	}
}

func TestSparseLuaMaxNHashKeys(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
local t = {[-1] = true, [0] = true, [1.5] = true}
assert(table.maxn(t) == 1.5)
t[67108865] = true
assert(table.maxn(t) == 67108865)
t[67108865] = nil
assert(table.maxn(t) == 1.5)
assert(table.maxn({[-1] = true}) == 0)
`); err != nil {
		t.Fatal(err)
	}
}

func TestSparseSequenceAfterTailDeletion(t *testing.T) {
	for _, operation := range []struct{ name, code string }{
		{"remove", `
local t = {1, 2, 3}
t[100000] = "far"
t[3] = nil
assert(#t == 2)
assert(table.remove(t) == 2 and #t == 1)
assert(t[100000] == "far")
`},
		{"sort", `
local t = {3, 2, 1}
t[100000] = "far"
t[3] = nil
assert(#t == 2)
table.sort(t)
assert(t[1] == 2 and t[2] == 3 and #t == 2)
assert(t[100000] == "far")
`},
		{"insert", `
local t = {1, 2, 3}
t[100000] = "far"
t[3] = nil
table.insert(t, 1, 9)
assert(#t == 3 and t[1] == 9 and t[2] == 1 and t[3] == 2)
assert(t[100000] == "far")
`},
	} {
		t.Run(operation.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString(operation.code); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSparseInsertionModel(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	tbl := newLTable(0, 0)
	model := make(map[int]LValue)
	for step := 0; step < 1000; step++ {
		key := rng.Intn(400) + 1
		if step%5 == 0 {
			key += 100000
		}
		value := LValue(LNumber(step))
		if step%3 == 0 {
			value = LNil
		}
		switch step % 3 {
		case 0:
			tbl.RawSetInt(key, value)
		case 1:
			tbl.RawSet(LNumber(key), value)
		case 2:
			tbl.RawSet(LInteger(key), value)
		}
		if value == LNil {
			delete(model, key)
		} else {
			model[key] = value
		}
		if step%25 == 0 {
			sparseVisitOnce(t, tbl, len(model))
			max := 0
			for k, want := range model {
				if tbl.RawGetInt(k) != want || tbl.RawGet(LInteger(k)) != want || tbl.RawGet(LNumber(k)) != want {
					t.Fatalf("step %d lost key %d", step, k)
				}
				if k > max {
					max = k
				}
			}
			if tbl.MaxN() != max {
				t.Fatalf("step %d maxn=%d, want %d", step, tbl.MaxN(), max)
			}
		}
	}
}

func TestSparseLuaTableContracts(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
local t = {1, 2, 3}
t[100000] = "far"
assert(#t == 3 and table.getn(t) == 3 and table.maxn(t) == 100000)
local n = 0
for i, v in ipairs(t) do assert(i == v); n = n + 1 end
assert(n == 3)
local count = 0
for k, v in pairs(t) do
  count = count + 1
  assert(count <= 4, "pairs did not terminate")
end
assert(count == 4)
local near = {1, 2}
near[300] = 300
for i = 3, 299 do table.insert(near, i) end
assert(#near == 300 and near[300] == 300)
assert(table.remove(near) == 300 and #near == 299)
table.insert(near, 2, 1000)
assert(near[1] == 1 and near[2] == 1000 and near[3] == 2)
assert(table.remove(near, 2) == 1000)
table.sort(near, function(a, b) return a > b end)
assert(near[1] == 299 and near[299] == 1)
assert(table.concat({1, 2, 3}, ",") == "1,2,3")
local a, b, c = unpack({1, 2, 3})
assert(a == 1 and b == 2 and c == 3)
local calls = 0
setmetatable(t, {__index = function() calls = calls + 1; return "fallback" end})
assert(t[100000] == "far" and calls == 0)
t[100000] = nil
assert(t[100000] == "fallback" and calls == 1)
local frozen = {1, 2}
frozen[100000] = true
table.freeze(frozen)
assert(not pcall(function() frozen[100000] = false end))
assert(frozen[100000] == true and #frozen == 2)
`); err != nil {
		t.Fatal(err)
	}
}

func TestTableRemoveReleasesBackingValue(t *testing.T) {
	for _, pos := range []int{-1, 0, 2} {
		tbl := newLTable(2, 0)
		tbl.Append(LTrue)
		value := newLTable(0, 0)
		tbl.Append(value)
		backing := tbl.Array
		got, ok := tbl.Remove(pos)
		if !ok || got != value || len(tbl.Array) != 1 {
			t.Fatal("remove returned the wrong value")
		}
		if backing[1] != nil && backing[1] != LNil {
			t.Fatal("removed value remains retained by array capacity")
		}
	}
}

func TestSparseDenseAccessAllocations(t *testing.T) {
	tbl := newLTable(32, 0)
	for i := 1; i <= 32; i++ {
		tbl.RawSetInt(i, LTrue)
	}
	if allocs := testing.AllocsPerRun(1000, func() { tbl.RawSetInt(1, LFalse) }); allocs != 0 {
		t.Fatalf("dense overwrite allocated %v times", allocs)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if tbl.RawGetInt(2) != LTrue {
			panic("dense read lost its value")
		}
	}); allocs != 0 {
		t.Fatalf("dense read allocated %v times", allocs)
	}
}

func TestSparseEmptyStorageOrigin(t *testing.T) {
	t.Run("numeric array", func(t *testing.T) {
		tbl := newLTable(0, 0)
		tbl.RawSetInt(100000, LTrue)
		tbl.RawSetInt(100000, LNil)
		if tbl.Dict != nil {
			t.Fatal("deleted sparse array key left an object-shaped hash part")
		}
	})
	t.Run("hash origin", func(t *testing.T) {
		tbl := newLTable(0, 0)
		tbl.RawSetH(LTrue, LTrue)
		tbl.RawSetH(LTrue, LNil)
		tbl.RawSetInt(100000, LTrue)
		tbl.RawSetInt(100000, LNil)
		if tbl.Dict == nil {
			t.Fatal("sparse write discarded the existing hash/object origin")
		}
	})
	t.Run("mixed hash origin", func(t *testing.T) {
		tbl := newLTable(0, 0)
		tbl.RawSetInt(100000, LTrue)
		tbl.RawSetH(LTrue, LTrue)
		tbl.RawSetH(LTrue, LNil)
		tbl.RawSetInt(100000, LNil)
		if tbl.Dict == nil {
			t.Fatal("hash keys changed to an array-shaped empty table")
		}
	})
	t.Run("promoted array", func(t *testing.T) {
		tbl := newLTable(0, 0)
		tbl.RawSetInt(100, LTrue)
		for i := 1; i < 100; i++ {
			tbl.RawSetInt(i, LTrue)
		}
		if tbl.Dict != nil {
			t.Fatal("promotion retained the numeric-only hash part")
		}
	})
	t.Run("header copy", func(t *testing.T) {
		tbl := newLTable(0, 0)
		tbl.RawSetInt(100000, LTrue)
		copy := *tbl
		copy.Dict = map[LValue]LValue{LNumber(100000): LTrue}
		copy.RawSetInt(100000, LNil)
		if copy.Dict != nil || tbl.RawGetInt(100000) != LTrue {
			t.Fatal("copy lost numeric storage origin or changed its source")
		}
	})
}

func TestSparseTableHeaderSize(t *testing.T) {
	type legacyTableHeader struct {
		Metatable LValue
		Immutable bool
		Array     []LValue
		Dict      map[LValue]LValue
		Strdict   map[string]LValue
		Keys      []LValue
		K2i       map[LValue]int
	}
	if unsafe.Sizeof(LTable{}) != unsafe.Sizeof(legacyTableHeader{}) {
		t.Fatal("sparse bookkeeping increased per-table header size")
	}
}

func TestSparseIterationDeletionReleasesKeyBacking(t *testing.T) {
	tbl := newLTable(0, 0)
	key := newLTable(0, 0)
	tbl.RawSetH(key, LTrue)
	tbl.RawSetString("name", LTrue)
	sparseVisitOnce(t, tbl, 2)
	backing := tbl.Keys[:cap(tbl.Keys)]
	tbl.RawSetH(key, LNil)
	tbl.RawSetString("name", LNil)
	for _, retained := range backing {
		if retained != nil && retained != LNil {
			t.Fatal("deleted iteration key remains retained by slice capacity")
		}
	}
}
