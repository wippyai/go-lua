package lua

import (
	"testing"
	"unsafe"
)

const sparseKeyArrayLimit = 1024

func arrayBytes(tbl *LTable) int {
	return cap(tbl.Array) * int(unsafe.Sizeof(LValue(nil)))
}

func TestTableRawSetIntSparseKeyKeepsArraySmall(t *testing.T) {
	for _, key := range []int{3350000, 20000000, 40013095, MaxArrayIndex - 1} {
		tbl := newLTable(0, 0)
		tbl.RawSetInt(key, LTrue)
		errorIfNotEqual(t, LTrue, tbl.RawGetInt(key))
		errorIfFalse(t, len(tbl.Array) <= sparseKeyArrayLimit,
			"key %d grew the array part to %d slots (%d MB)", key, len(tbl.Array), arrayBytes(tbl)/1024/1024)
	}
}

func TestTableScriptSparseKeyKeepsArraySmall(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `t = {}; t[40013095] = true`)
	tbl := L.GetGlobal("t").(*LTable)
	errorIfNotEqual(t, LTrue, tbl.RawGetInt(40013095))
	errorIfFalse(t, len(tbl.Array) <= sparseKeyArrayLimit,
		"t[40013095] = true grew the array part to %d slots (%d MB)", len(tbl.Array), arrayBytes(tbl)/1024/1024)
}

func TestTableSparseKeyMovesToArrayWhenTheGapFills(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
		local t = {}
		t[1000] = "last"
		assert(#t == 0)
		for i = 1, 999 do t[i] = i end
		assert(#t == 1000)
		assert(t[1000] == "last")
		local seen = 0
		for i, v in ipairs(t) do seen = i end
		assert(seen == 1000)
		local count = 0
		for k, v in pairs(t) do count = count + 1 end
		assert(count == 1000)
	`)
}

func TestTableSparseKeyWithTableLibrary(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
		local t = {}
		t[500] = "far"
		for i = 1, 498 do table.insert(t, i) end
		table.insert(t, "near")
		assert(#t == 500)
		assert(t[499] == "near" and t[500] == "far")
		assert(table.remove(t) == "far")
		assert(#t == 499)
		local packed = {unpack({1, 2, 3})}
		assert(#packed == 3)
		local sorted = {}
		sorted[200] = 1
		for i = 1, 199 do sorted[i] = 200 - i end
		table.sort(sorted)
		assert(sorted[1] == 1 and sorted[200] == 199)
	`)
}

func TestTableSparseKeyNilAndIntegerKinds(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetInt(40013095, LTrue)
	errorIfNotEqual(t, LTrue, tbl.RawGet(LNumber(40013095)))
	errorIfNotEqual(t, LTrue, tbl.RawGet(LInteger(40013095)))
	tbl.RawSet(LInteger(40013095), LFalse)
	errorIfNotEqual(t, LFalse, tbl.RawGetInt(40013095))
	tbl.RawSetInt(40013095, LNil)
	errorIfNotEqual(t, LNil, tbl.RawGetInt(40013095))
	errorIfNotEqual(t, 0, len(tbl.Dict))
	tbl.RawSetInt(20000000, LNil)
	errorIfNotEqual(t, 0, len(tbl.Array))
}

func TestTableSparseKeyNextVisitsEveryKeyOnce(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetInt(1, LString("a"))
	tbl.RawSetInt(2, LString("b"))
	tbl.RawSetInt(40013095, LString("c"))
	tbl.RawSetString("name", LString("d"))
	seen := map[string]int{}
	key, value := tbl.Next(LNil)
	for key != LNil {
		seen[value.String()]++
		key, value = tbl.Next(key)
	}
	errorIfNotEqual(t, 4, len(seen))
	for name, count := range seen {
		errorIfFalse(t, count == 1, "value %s was visited %d times", name, count)
	}
}
