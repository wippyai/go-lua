package lua

import "testing"

func TestNumericAndStringConformance(t *testing.T) {
	for name, script := range map[string]string{
		"numeric for": `
			local function check(first, last, step, ...)
				local expected = {...}
				local n = 0
				for i = first, last, step do
					n = n + 1
					assert(i == expected[n], "unexpected iteration " .. n)
				end
				assert(n == #expected)
			end
			local lo, hi = math.mininteger, math.maxinteger
			check(hi-1, hi, 1, hi-1, hi)
			check(lo+1, lo, -1, lo+1, lo)
			check(lo, hi, hi, lo, -1, hi-1)
			check(hi, lo, lo, hi, -1)
			check(hi, lo, 1)
			check(lo, hi, -1)
			check(hi, math.huge, 1, hi)
			check(lo, -math.huge, -1, lo)
			check(1, 2.5, 1, 1, 2)
			check(3, 1.5, -1, 3, 2)
			check(0.5, 1.5, 0.5, 0.5, 1, 1.5)
			assert(not pcall(function() for i=1,2,0 do break end end))
			assert(not pcall(function() for i=1,2,0.0 do break end end))
		`,
		"ordered numbers": `
			local wide, rounded = 9007199254740993, 2^53
			local function ordered(a,b)
				assert(a < b and a <= b and b > a and b >= a)
				assert(not (a >= b or a > b or b <= a or b < a))
			end
			ordered(9007199254740992, wide)
			ordered(rounded, wide)
			ordered(-wide, -rounded)
			ordered(math.maxinteger, 2^63)
			ordered(-math.huge, math.mininteger)
			ordered(math.maxinteger, math.huge)
			ordered(-1.5, -1)
			ordered(1, 1.5)
			assert(math.mininteger <= -(2^63) and -(2^63) <= math.mininteger)
			local nan = 0/0
			assert(not (nan < 1 or 1 < nan or nan <= 1 or 1 <= nan))
		`,
		"repeat separator": `
			assert(string.rep("ab", 3, ",") == "ab,ab,ab")
			assert(string.rep("", 3, "é") == "éé")
			assert(string.rep("x", 1, ",") == "x")
			assert(string.rep("x", 0, ",") == "")
			assert(string.rep("x", -1, ",") == "")
			assert(string.rep("x", 3, nil) == "xxx")
			assert(string.rep("", math.maxinteger, "") == "")
			assert(not pcall(string.rep,"ab",math.maxinteger,","))
		`,
	} {
		t.Run(name, func(t *testing.T) {
			l := NewState()
			defer l.Close()
			if err := l.DoString(script); err != nil {
				t.Fatal(err)
			}
		})
	}
}
