package lua

import "testing"

func TestBaseTypeNames(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		local cases = {
			{ nil, "nil" }, { true, "boolean" }, { 1.5, "number" }, { 7, "number" },
			{ "text", "string" }, { print, "function" }, { {}, "table" },
			{ coroutine.create(function() end), "thread" },
		}
		for index = 1, #cases do
			local value, expected = cases[index][1], cases[index][2]
			assert(type(value) == expected, "type " .. index .. ": " .. tostring(type(value)))
		end
	`); err != nil {
		t.Fatal(err)
	}
}

func TestBaseTypeDoesNotAllocatePerCall(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`
		local value, hits = { type = "x" }, 0
		for _ = 1, 1000 do
			if type(value) ~= "table" then hits = hits + 1 end
			if type(value.type) ~= "string" then hits = hits + 1 end
		end
		return hits
	`)
	if err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(20, func() {
		L.Push(fn)
		if err := L.PCall(0, 1, nil); err != nil {
			t.Fatal(err)
		}
		L.Pop(1)
	})
	if allocs > 10 {
		t.Fatalf("2000 type() calls allocated %.0f times", allocs)
	}
}

func BenchmarkBaseType(b *testing.B) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`
		local value, hits = { type = "x" }, 0
		for _ = 1, 1000 do
			if type(value) ~= "table" then hits = hits + 1 end
			if type(value.type) ~= "string" then hits = hits + 1 end
		end
		return hits
	`)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		if err := L.PCall(0, 1, nil); err != nil {
			b.Fatal(err)
		}
		L.Pop(1)
	}
}
