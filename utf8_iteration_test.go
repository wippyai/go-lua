package lua

import "testing"

func TestUtf8CodesMultibytePositions(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local s = "A\195\164\226\130\172\240\159\152\128"
		local positions, codepoints = {1, 2, 4, 7}, {65, 228, 8364, 128512}
		local n = 0
		for position, codepoint in utf8.codes(s) do
			n = n + 1
			assert(position == positions[n])
			assert(codepoint == codepoints[n])
		end
		assert(n == 4)
		for _ in utf8.codes("") do error("empty input yielded a codepoint") end
		local ok = pcall(function() for _ in utf8.codes("\195") do end end)
		assert(not ok, "truncated input must fail")
		for _, cp in utf8.codes("\239\191\189") do assert(cp == 65533) end
	`); err != nil {
		t.Fatal(err)
	}
}

func TestUtf8OffsetBoundaries(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		assert(utf8.offset("", 0) == 1)
		assert(utf8.offset("a", 0, 2) == 2)
		assert(utf8.offset("abc", -1) == 3)
		assert(utf8.offset("abc", 1, -1) == 3)
		local s = "A\195\164\226\130\172"
		assert(utf8.offset(s, -1) == 4)
		assert(utf8.offset(s, -2) == 2)
		assert(utf8.offset(s, 0, 3) == 2)
		assert(utf8.offset(s, 4) == #s + 1)
		assert(utf8.offset(s, 5) == nil)
		assert(utf8.offset(s, -4) == nil)
		assert(not pcall(utf8.offset, s, 1, 3))
		assert(not pcall(utf8.offset, s, 1, #s + 2))
	`); err != nil {
		t.Fatal(err)
	}
}
