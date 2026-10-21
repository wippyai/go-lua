package lua

import (
	"math"
	"strconv"
	"testing"
)

func TestTableNumericKeyIdentity(t *testing.T) {
	for _, pair := range []struct {
		name string
		i    LInteger
		f    LNumber
	}{
		{"zero", 0, 0},
		{"signed_zero", 0, LNumber(math.Copysign(0, -1))},
		{"negative", -2, -2},
		{"positive", 2, 2},
		{"sparse", 100000, 100000},
		{"array_limit", LInteger(MaxArrayIndex), LNumber(MaxArrayIndex)},
		{"above_array_limit", LInteger(MaxArrayIndex + 1), LNumber(MaxArrayIndex + 1)},
		{"exact_large", 1 << 53, 1 << 53},
		{"minimum_integer", math.MinInt64, math.MinInt64},
		{"largest_integral_float", LInteger(math.MaxInt64 - 1023), LNumber(math.MaxInt64 - 1023)},
	} {
		t.Run(pair.name, func(t *testing.T) {
			for _, hash := range []bool{false, true} {
				for _, reverse := range []bool{false, true} {
					t.Run(strconv.FormatBool(hash)+"/"+strconv.FormatBool(reverse), func(t *testing.T) {
						tbl := newLTable(0, 0)
						set := tbl.RawSet
						if hash {
							set = tbl.RawSetH
						}
						var key, alias LValue = pair.i, pair.f
						if reverse {
							key, alias = alias, key
						}
						set(key, LTrue)
						if got := tbl.RawGet(alias); got != LTrue {
							t.Errorf("cross-type lookup = %v, want true", got)
						}
						if hash && tbl.RawGetH(alias) != LTrue {
							t.Error("cross-type hash lookup loses the value")
						}
						set(alias, LFalse)
						if tbl.RawGet(key) != LFalse {
							t.Error("cross-type overwrite does not update the original key")
						}
						sparseVisitOnce(t, tbl, 1)
						tbl.RawSetString("after", LTrue)
						if next, value := tbl.Next(alias); next != LString("after") || value != LTrue {
							t.Errorf("next with equal cursor = %v, %v, want after, true", next, value)
						}
						set(alias, LNil)
						if tbl.RawGet(key) != LNil {
							t.Error("cross-type deletion retains the original key")
						}
						if next, value := tbl.Next(key); next != LString("after") || value != LTrue {
							t.Errorf("next with deleted equal cursor = %v, %v, want after, true", next, value)
						}
						sparseVisitOnce(t, tbl, 1)
					})
				}
			}
		})
	}
}

func TestTableNumericKeysRemainDistinct(t *testing.T) {
	keys := []LValue{
		LInteger(1 << 53), LInteger(1<<53 + 1), LInteger(1<<53 + 2),
		LInteger(math.MaxInt64), LInteger(math.MaxInt64 - 1),
		LInteger(math.MinInt64), LInteger(math.MinInt64 + 1),
		LNumber(0.5), LNumber(-0.5), LNumber(0),
		LNumber(0x1p63), LNumber(-0x1.0000000000001p63),
		LNumber(math.Inf(1)), LNumber(math.Inf(-1)),
	}
	tbl := newLTable(0, 0)
	for i, key := range keys {
		tbl.RawSet(key, LInteger(i))
	}
	for i, key := range keys {
		if got := tbl.RawGet(key); got != LInteger(i) {
			t.Errorf("key %v (%T) = %v, want %d", key, key, got, i)
		}
	}
	sparseVisitOnce(t, tbl, len(keys))
}

func TestTableIntegerAccessorsPreserveKeys(t *testing.T) {
	keys := []int{0, -2, MaxArrayIndex, MaxArrayIndex + 1}
	if strconv.IntSize == 64 {
		for _, key := range []int64{1 << 53, 1<<53 + 1, math.MaxInt64, math.MinInt64} {
			keys = append(keys, int(key))
		}
	}
	tbl := newLTable(0, 0)
	for i, key := range keys {
		tbl.RawSetInt(key, LInteger(i))
	}
	for i, key := range keys {
		if tbl.RawGetInt(key) != LInteger(i) || tbl.RawGet(LInteger(key)) != LInteger(i) {
			t.Errorf("integer accessor loses key %d", key)
		}
		tbl.RawSet(LInteger(key), LFalse)
		if tbl.RawGetInt(key) != LFalse {
			t.Errorf("integer accessor misses generic overwrite for %d", key)
		}
		tbl.RawSetInt(key, LNil)
		if tbl.RawGet(LInteger(key)) != LNil {
			t.Errorf("integer accessor retains deleted key %d", key)
		}
	}
}
