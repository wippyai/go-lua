package lua

import (
	"fmt"
	"strings"
	"testing"
)

func TestNumericForBoundaries(t *testing.T) {
	cases := []struct {
		name, bounds string
		want         LNumber
	}{
		{"maxinteger", "math.maxinteger - 2, math.maxinteger", 3},
		{"mininteger", "math.mininteger + 2, math.mininteger, -1", 3},
		{"positive_step_two", "math.maxinteger - 4, math.maxinteger, 2", 3},
		{"negative_step_two", "math.mininteger + 4, math.mininteger, -2", 3},
		{"positive_step_overshoots", "math.maxinteger - 3, math.maxinteger, 2", 2},
		{"negative_step_overshoots", "math.mininteger + 3, math.mininteger, -2", 2},
		{"start_above_limit", "math.maxinteger, math.maxinteger - 1", 0},
		{"start_below_limit", "math.mininteger, math.mininteger + 1, -1", 0},
		{"positive_prep_underflow", "math.mininteger, math.mininteger + 2", 3},
		{"negative_prep_overflow", "math.maxinteger, math.maxinteger - 2, -1", 3},
		{"largest_positive_step", "math.mininteger, math.maxinteger, math.maxinteger", 3},
		{"smallest_negative_step", "math.maxinteger, math.mininteger, math.mininteger", 2},
		{"float_max_limit", "math.maxinteger - 2, math.maxinteger + 0.0", 3},
		{"float_min_limit", "math.mininteger + 2, math.mininteger + 0.0, -1", 3},
		{"float_limit_floor", "1, 3.9", 3},
		{"float_limit_ceil", "3, 0.1, -1", 3},
		{"float_limit_floor_empty", "1, 0.9", 0},
		{"float_limit_ceil_empty", "0, 0.1, -1", 0},
		{"positive_infinite_limit", "math.maxinteger - 2, math.huge", 3},
		{"negative_infinite_limit", "math.mininteger + 2, -math.huge, -1", 3},
		{"positive_limit_out_of_range", "math.mininteger, -math.huge", 0},
		{"negative_limit_out_of_range", "math.maxinteger, math.huge, -1", 0},
	}
	for _, tc := range cases {
		for _, budget := range []int64{-1, 1} {
			t.Run(fmt.Sprintf("%s/budget_%d", tc.name, budget), func(t *testing.T) {
				L := NewState()
				defer L.Close()
				src := fmt.Sprintf(`
local n = 0
for i = %s do
    assert(math.type(i) == "integer")
    n = n + 1
    assert(n <= 16, "numeric for does not terminate")
end
return n`, tc.bounds)
				if budget < 0 {
					if err := L.DoString(src); err != nil {
						t.Fatal(err)
					}
					expectNumbers(t, []LValue{L.Get(-1)}, tc.want)
				} else {
					ret, _, preempts := runToCompletion(t, L, src, budget)
					expectNumbers(t, ret, tc.want)
					if tc.want > 0 && preempts == 0 {
						t.Fatal("expected preemption")
					}
				}
			})
		}
	}
}

func TestNumericForZeroStep(t *testing.T) {
	for _, budget := range []int64{-1, 1} {
		t.Run(fmt.Sprintf("budget_%d", budget), func(t *testing.T) {
			L := NewState()
			defer L.Close()
			ret, _, _ := runToCompletion(t, L, `
local ok, err = pcall(function() for i = 1, 2, 0 do break end end)
return ok, err`, budget)
			if len(ret) != 2 || ret[0] != LFalse || !strings.Contains(ret[1].String(), "'for' step is zero") {
				t.Fatalf("expected zero-step error, got %v", ret)
			}
		})
	}
}

func TestNumericForFullIntegerRange(t *testing.T) {
	for _, bounds := range []string{
		"math.mininteger, math.maxinteger",
		"math.maxinteger, math.mininteger, -1",
	} {
		for _, budget := range []int64{-1, 1} {
			t.Run(fmt.Sprintf("%s/budget_%d", bounds, budget), func(t *testing.T) {
				L := NewState()
				defer L.Close()
				ret, _, _ := runToCompletion(t, L, fmt.Sprintf(`
local n = 0
for i = %s do
    assert(math.type(i) == "integer")
    n = n + 1
    if n == 3 then break end
end
return n`, bounds), budget)
				expectNumbers(t, ret, 3)
			})
		}
	}
}

func TestNumericForFloatControl(t *testing.T) {
	cases := []struct {
		bounds string
		want   LNumber
	}{
		{"1.0, 3", 3},
		{"1, 3, 0.5", 5},
		{"3.0, 1, -0.5", 5},
		{"3.0, 1", 0},
		{"1.0, 0, 0", 1},
	}
	for _, tc := range cases {
		for _, budget := range []int64{-1, 1} {
			t.Run(fmt.Sprintf("%s/budget_%d", tc.bounds, budget), func(t *testing.T) {
				L := NewState()
				defer L.Close()
				ret, _, _ := runToCompletion(t, L, fmt.Sprintf(`
local n = 0
for i = %s do
    assert(math.type(i) == "float")
    n = n + 1
    assert(n <= 16, "numeric for does not terminate")
    if %t then break end
end
return n`, tc.bounds, tc.bounds == "1.0, 0, 0"), budget)
				expectNumbers(t, ret, tc.want)
			})
		}
	}
}

func TestNumericForCounterSnapshot(t *testing.T) {
	for _, budget := range []int64{-1, 1} {
		t.Run(fmt.Sprintf("budget_%d", budget), func(t *testing.T) {
			L := NewState()
			defer L.Close()
			ret, _, _ := runToCompletion(t, L, `
local saved
for i = 1, 3 do
    if i == 1 then
        for slot = 1, 8 do
            local name, value = debug.getlocal(1, slot)
            if name == "(for limit)" then
                assert(math.type(value) == "integer")
                saved = value
                break
            end
        end
    end
end
return saved`, budget)
			expectNumbers(t, ret, 2)
		})
	}
}

func TestNumericForCounterSetLocal(t *testing.T) {
	L := NewState()
	defer L.Close()
	ret, _, _ := runToCompletion(t, L, `
local n = 0
for i = 1, 3 do
    if i == 1 then
        for slot = 1, 8 do
            local name = debug.getlocal(1, slot)
            if name == "(for limit)" then
                debug.setlocal(1, slot, 0)
                break
            end
        end
    end
    n = n + 1
end
return n`, 1)
	expectNumbers(t, ret, 1)
}

func TestNumericForInitialSafepoint(t *testing.T) {
	for _, bounds := range []string{"1, 3", "3, 1"} {
		t.Run(bounds, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.SetGlobal("hits", LInteger(0))
			fn, err := L.LoadString(fmt.Sprintf(`for i = %s do hits = hits + 1 end return hits`, bounds))
			if err != nil {
				t.Fatal(err)
			}
			th, cancel := L.NewThread()
			defer cancel()
			L.SetTickBudget(0)
			status, _, err := L.Resume(th, fn)
			if err != nil || status != ResumePreempted {
				t.Fatalf("expected initial preemption, got %v, %v", status, err)
			}
			if got := L.GetGlobal("hits"); got != LInteger(0) {
				t.Fatalf("loop body runs before the first tick: hits=%v", got)
			}
			ret, _, _ := resumeThrough(t, L, th, fn, 1)
			want := LNumber(3)
			if bounds == "3, 1" {
				want = 0
			}
			expectNumbers(t, ret, want)
		})
	}
}
