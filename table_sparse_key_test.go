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
