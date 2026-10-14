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
