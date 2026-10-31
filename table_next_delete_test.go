package lua

import (
	"fmt"
	"testing"
)

var nextDeleteCases = []struct {
	name, setup string
}{
	{"hash", `local object = {}; local t = {a = 1, b = 2, c = 3, d = 4, [false] = 5, [object] = 6}`},
	{"array", `local t = {1, 2, 3, 4, 5, 6}`},
	{"sparse", `local t = {}; t[100000] = 1; t[200000] = 2; t[40013095] = 3; t[67108863] = 4`},
	{"mixed", `local object = {}; local t = {1, 2, 3}; t.name = 4; t[false] = 5; t[object] = 6; t[100000] = 7; t[200000] = 8`},
}

func TestTablePairsClearCurrentKey(t *testing.T) {
	testTableClearCurrentKey(t, false)
}

func TestTableNextClearCurrentKey(t *testing.T) {
	testTableClearCurrentKey(t, true)
}

func testTableClearCurrentKey(t *testing.T, explicitNext bool) {
	t.Helper()
	for _, tc := range nextDeleteCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, deletion := range []string{"all", "second"} {
				t.Run(deletion, func(t *testing.T) {
					L := NewState()
					defer L.Close()
					iterator := "pairs(t)"
					if explicitNext {
						iterator = "next, t, nil"
					}
					script := tc.setup + fmt.Sprintf(`
local expected, total = {}, 0
for k, v in pairs(t) do expected[k] = v; total = total + 1 end
local seen, count = {}, 0
for k, v in %s do
  count = count + 1
  assert(count <= total, "iteration did not terminate")
  assert(not seen[k], "iteration repeated a key")
  assert(expected[k] == v, "iteration returned an unexpected key or value")
  seen[k] = true
  if %t or count == 2 then t[k] = nil end
end
assert(count == total, "clearing the current key ended iteration early")
for k in pairs(expected) do assert(seen[k], "iteration skipped a key") end
local remaining = 0
for k, v in pairs(t) do
  remaining = remaining + 1
  assert(remaining <= total, "subsequent traversal did not terminate")
  assert(seen[k] and expected[k] == v)
end
assert(remaining == (%t and 0 or total - 1), "wrong number of remaining keys")
`, iterator, deletion == "all", deletion == "all")
					if err := L.DoString(script); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestTableNextClearCurrentKeyGo(t *testing.T) {
	for _, tc := range nextDeleteCases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString(tc.setup + `; return t`); err != nil {
				t.Fatal(err)
			}
			tbl := L.Get(-1).(*LTable)
			expected := make(map[LValue]LValue)
			tbl.ForEach(func(k, v LValue) { expected[k] = v })
			seen := make(map[LValue]bool)
			key := LNil
			for steps := 0; ; steps++ {
				var value LValue
				key, value = tbl.Next(key)
				if key == LNil {
					if value != LNil || len(seen) != len(expected) {
						t.Fatalf("next ended with %d/%d keys visited and value %v", len(seen), len(expected), value)
					}
					break
				}
				if steps >= len(expected) || seen[key] || expected[key] != value {
					t.Fatalf("next repeated or returned an unexpected key/value: %v, %v", key, value)
				}
				seen[key] = true
				if !tbl.RawSet(key, LNil) || tbl.RawGet(key) != LNil {
					t.Fatalf("failed to clear key %v", key)
				}
			}
			if key, value := tbl.Next(LNil); key != LNil || value != LNil {
				t.Fatalf("cleared table returned %v, %v", key, value)
			}
		})
	}
}

func TestTableNextDeletedCursorSurvivesNestedTraversal(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
local t = {a = 1, b = 2, c = 3, d = 4}
local seen, count = {}, 0
for k in pairs(t) do
  assert(not seen[k], "outer traversal repeated a key")
  seen[k] = true
  count = count + 1
  assert(count <= 4, "outer traversal did not terminate")
  t[k] = nil
  local remaining = 0
  for other in pairs(t) do
    remaining = remaining + 1
    assert(remaining <= 4 - count, "nested traversal did not terminate")
    assert(not seen[other])
  end
  assert(remaining == 4 - count, "nested traversal skipped a key")
end
assert(count == 4, "nested traversal invalidated the deleted outer cursor")
`); err != nil {
		t.Fatal(err)
	}
}

func TestTableNextDeletedCursorsStayBounded(t *testing.T) {
	for _, strings := range []bool{false, true} {
		t.Run(fmt.Sprintf("strings=%t", strings), func(t *testing.T) {
			tbl := newLTable(0, 0)
			// Start lazy metadata before repeatedly replacing hash keys.
			tbl.Next(LNil)
			var previous LValue
			for i := 0; i < 100; i++ {
				var key LValue = newLTable(0, 0)
				if strings {
					key = LString(fmt.Sprintf("key%d", i))
				}
				tbl.RawSet(key, LTrue)
				if len(tbl.Keys) != 1 || len(tbl.K2i) != 1 {
					t.Fatal("insertion retained deleted iteration metadata")
				}
				if previous != nil {
					if _, retained := tbl.K2i[iterationKey(previous)]; retained {
						t.Fatal("insertion retained a deleted key reference")
					}
				}
				if got, value := tbl.Next(LNil); got != key || value != LTrue {
					t.Fatalf("next returned %v, %v, want %v, true", got, value, key)
				}
				tbl.RawSet(key, LNil)
				if tbl.Keys[0] != nil || tbl.RawGet(key) != LNil {
					t.Fatal("deletion retained the key in the slice or its value")
				}
				if got, value := tbl.Next(key); got != LNil || value != LNil {
					t.Fatalf("next after deleting the last key returned %v, %v", got, value)
				}
				previous = key
			}
		})
	}
}

func TestTableNextDeletedPointerKeysReleaseReferences(t *testing.T) {
	L := NewState()
	defer L.Close()
	for _, key := range []LValue{newLTable(0, 0), L.NewFunction(func(*LState) int { return 0 }), L.NewUserData(), L} {
		t.Run(key.Type().String(), func(t *testing.T) {
			tbl := newLTable(0, 0)
			tbl.RawSetH(key, LTrue)
			tbl.RawSetString("tail", LFalse)
			// String keys precede Dict keys in the lazy iteration snapshot.
			first, _ := tbl.Next(LNil)
			got, _ := tbl.Next(first)
			if got != key {
				t.Fatalf("next returned %v, want the pointer key %v", got, key)
			}
			tbl.RawSetH(key, LNil)
			for _, retained := range tbl.Keys[:cap(tbl.Keys)] {
				if retained == key {
					t.Fatal("deleted pointer key remains in the slice backing")
				}
			}
			for retained := range tbl.K2i {
				if retained == key {
					t.Fatal("deleted pointer key remains in the cursor map")
				}
			}
			if next, value := tbl.Next(key); next != LNil || value != LNil {
				t.Fatalf("deleted pointer cursor did not terminate: %v, %v", next, value)
			}
		})
	}
}

func TestTableNextReinsertDeletedKey(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetString("a", LTrue)
	tbl.RawSetH(LFalse, LTrue)
	sparseVisitOnce(t, tbl, 2)
	for _, key := range []LValue{LString("a"), LFalse} {
		tbl.RawSet(key, LNil)
		tbl.RawSet(key, LFalse)
		sparseVisitOnce(t, tbl, 2)
		if tbl.RawGet(key) != LFalse || len(tbl.K2i) != 2 || len(tbl.Keys) != 2 {
			t.Fatal("reinsertion lost a value or retained a dead cursor")
		}
	}
}

func TestTableNextDeletedSparseIntegerCursor(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSet(LInteger(100000), LTrue)
	tbl.RawSet(LNumber(200000), LFalse)
	key, _ := tbl.Next(LNil)
	tbl.RawSet(LInteger(key.(LNumber)), LNil)
	next, value := tbl.Next(LInteger(key.(LNumber)))
	if next == LNil || next == key || value != tbl.RawGet(next) {
		t.Fatalf("integer cursor lost the remaining sparse key: %v, %v", next, value)
	}
	if last, value := tbl.Next(next); last != LNil || value != LNil {
		t.Fatalf("sparse traversal did not terminate: %v, %v", last, value)
	}
}
