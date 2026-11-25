package lua

import (
	"strings"
	"testing"
)

// boundedState is a state whose registry holds at most 1024 values, far fewer than the
// elements these tests concatenate.
func boundedState() *LState {
	return NewState(Options{RegistrySize: 256, RegistryMaxSize: 1024})
}

func TestTableConcatBeyondRegistryBound(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"million strings with separator", `local t = {} for i = 1, 1000000 do t[i] = "x" end
			local s = table.concat(t, ",")
			return #s .. ":" .. s:sub(1, 5) .. ":" .. s:sub(-3)`, "1999999:x,x,x:x,x"},
		{"million strings without separator", `local t = {} for i = 1, 1000000 do t[i] = "ab" end
			return #table.concat(t)`, "2000000"},
		{"million numbers", `local t = {} for i = 1, 1000000 do t[i] = i end
			local s = table.concat(t, " ")
			return s:sub(1, 6) .. "|" .. s:sub(-14)`, "1 2 3 |999999 1000000"},
		{"sub-range of a million", `local t = {} for i = 1, 1000000 do t[i] = tostring(i) end
			local s = table.concat(t, "-", 500001, 999999)
			return #s .. ":" .. s:sub(1, 13) .. ":" .. s:sub(-6)`, "3499992:500001-500002:999999"},
		{"multi-byte separator", `local t = {} for i = 1, 100000 do t[i] = "a" end
			local s = table.concat(t, "<>")
			return #s .. ":" .. s:sub(1, 4)`, "299998:a<>a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := boundedState()
			defer L.Close()
			if err := L.DoString(tc.src); err != nil {
				t.Fatal(err)
			}
			if got := L.Get(-1); got.String() != tc.want {
				t.Fatalf("got %q, want %q", got.String(), tc.want)
			}
		})
	}
}

func TestTableConcatBeyondRegistryBoundErrors(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"table element past the registry bound", `local t = {} for i = 1, 1000000 do t[i] = "x" end
			t[900000] = {}
			return table.concat(t, ",")`, "invalid value (table) at index 900000 in table for concat"},
		{"boolean element", `local t = {} for i = 1, 200000 do t[i] = "x" end
			t[150000] = true
			return table.concat(t)`, "invalid value (boolean) at index 150000 in table for concat"},
		{"range past the border", `local t = {} for i = 1, 200000 do t[i] = "x" end
			return table.concat(t, ",", 1, 200001)`, "invalid value (nil) at index 200001 in table for concat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := boundedState()
			defer L.Close()
			err := L.DoString(tc.src)
			if err == nil {
				t.Fatalf("expected error, got %q", L.Get(-1).String())
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func benchmarkTableConcat(b *testing.B, n int) {
	L := NewState()
	defer L.Close()
	tbl := L.NewTable()
	for i := 1; i <= n; i++ {
		tbl.RawSetInt(i, LString("item"))
	}
	concat := L.GetField(L.GetGlobal("table"), "concat")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(concat)
		L.Push(tbl)
		L.Push(LString(","))
		L.Call(2, 1)
		L.Pop(1)
	}
}

func BenchmarkTableConcat4(b *testing.B)    { benchmarkTableConcat(b, 4) }
func BenchmarkTableConcat64(b *testing.B)   { benchmarkTableConcat(b, 64) }
func BenchmarkTableConcat4096(b *testing.B) { benchmarkTableConcat(b, 4096) }
