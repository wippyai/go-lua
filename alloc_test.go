package lua

import (
	"math"
	"testing"
)

func TestAllocatorLNumber2I_Preloaded(t *testing.T) {
	al := newAllocator(32)

	// Test preloaded range [0, 256)
	for i := 0; i < preloadLimit; i++ {
		v := al.LNumber2I(LNumber(i))
		if v.Type() != LTNumber {
			t.Errorf("preloaded %d: expected LTNumber, got %v", i, v.Type())
		}
		if float64(v.(LNumber)) != float64(i) {
			t.Errorf("preloaded %d: expected %d, got %v", i, i, v)
		}
		// Verify it's the same preloaded instance
		if v != preloads[i] {
			t.Errorf("preloaded %d: not using preloaded instance", i)
		}
	}
}

func TestAllocatorLNumber2I_OutsidePreload(t *testing.T) {
	al := newAllocator(32)

	tests := []LNumber{
		LNumber(preloadLimit),     // just outside preload
		LNumber(preloadLimit + 1), // outside preload
		LNumber(1000),             // larger value
		LNumber(-1),               // negative
		LNumber(-1000),            // larger negative
		LNumber(0.5),              // float
		LNumber(1.5),              // float that's not integer
		LNumber(math.Pi),          // irrational
		LNumber(math.MaxFloat64),  // max float
		LNumber(-math.MaxFloat64), // min float
	}

	for _, num := range tests {
		v := al.LNumber2I(num)
		if v.Type() != LTNumber {
			t.Errorf("LNumber(%v): expected LTNumber, got %v", num, v.Type())
		}
		if float64(v.(LNumber)) != float64(num) {
			t.Errorf("LNumber(%v): expected %v, got %v", num, num, v)
		}
	}
}

func TestAllocatorLNumber2I_FloatIntBoundary(t *testing.T) {
	al := newAllocator(32)

	// Test that 5.0 uses preload but 5.5 doesn't
	v5 := al.LNumber2I(LNumber(5))
	if v5 != preloads[5] {
		t.Error("5.0 should use preloaded value")
	}

	v55 := al.LNumber2I(LNumber(5.5))
	if v55 == preloads[5] {
		t.Error("5.5 should not use preloaded value")
	}
}

func TestAllocatorLInteger2I_Preloaded(t *testing.T) {
	al := newAllocator(32)

	// Test preloaded range [-65536, 65536)
	testCases := []int64{
		0, 1, -1, 100, -100,
		intPreloadLimit - 1,
		-intPreloadLimit,
	}

	for _, i := range testCases {
		v := al.LInteger2I(LInteger(i))
		if v.Type() != LTInteger {
			t.Errorf("preloaded %d: expected LTInteger, got %v", i, v.Type())
		}
		if int64(v.(LInteger)) != i {
			t.Errorf("preloaded %d: expected %d, got %v", i, i, v)
		}
		// Verify it's the same preloaded instance
		if v != intPreloads[i+intPreloadLimit] {
			t.Errorf("preloaded %d: not using preloaded instance", i)
		}
	}
}

func TestAllocatorLInteger2I_OutsidePreload(t *testing.T) {
	al := newAllocator(32)

	tests := []LInteger{
		LInteger(intPreloadLimit),      // just outside preload
		LInteger(-intPreloadLimit - 1), // just outside negative preload
		LInteger(100000),               // larger value
		LInteger(-100000),              // larger negative
		LInteger(math.MaxInt64),        // max int
		LInteger(math.MinInt64),        // min int
	}

	for _, num := range tests {
		v := al.LInteger2I(num)
		if v.Type() != LTInteger {
			t.Errorf("LInteger(%v): expected LTInteger, got %v", num, v.Type())
		}
		if int64(v.(LInteger)) != int64(num) {
			t.Errorf("LInteger(%v): expected %v, got %v", num, num, v)
		}
	}
}

func TestAllocatorPageAllocation(t *testing.T) {
	al := newAllocator(4) // small page size

	// Force multiple page allocations
	values := make([]LValue, 20)
	for i := 0; i < 20; i++ {
		values[i] = al.LNumber2I(LNumber(1000 + i)) // outside preload range
	}

	// Verify all values are correct
	for i := 0; i < 20; i++ {
		expected := LNumber(1000 + i)
		if float64(values[i].(LNumber)) != float64(expected) {
			t.Errorf("value %d: expected %v, got %v", i, expected, values[i])
		}
	}
}

func TestAllocatorPreloadInit(t *testing.T) {
	// Verify preloads are initialized correctly
	for i := 0; i < preloadLimit; i++ {
		if preloads[i].Type() != LTNumber {
			t.Errorf("preloads[%d] type: expected LTNumber, got %v", i, preloads[i].Type())
		}
		if float64(preloads[i].(LNumber)) != float64(i) {
			t.Errorf("preloads[%d]: expected %d, got %v", i, i, preloads[i])
		}
	}

	// Verify intPreloads are initialized correctly
	for i := -intPreloadLimit; i < intPreloadLimit; i++ {
		idx := i + intPreloadLimit
		if intPreloads[idx].Type() != LTInteger {
			t.Errorf("intPreloads[%d] type: expected LTInteger, got %v", i, intPreloads[idx].Type())
		}
		if int64(intPreloads[idx].(LInteger)) != int64(i) {
			t.Errorf("intPreloads[%d]: expected %d, got %v", i, i, intPreloads[idx])
		}
	}
}

func TestAllocatorSpecialFloats(t *testing.T) {
	al := newAllocator(32)

	// Test special float values
	tests := []struct {
		name string
		val  LNumber
	}{
		{"positive infinity", LNumber(math.Inf(1))},
		{"negative infinity", LNumber(math.Inf(-1))},
		{"NaN", LNumber(math.NaN())},
		{"smallest positive", LNumber(math.SmallestNonzeroFloat64)},
		{"negative zero", LNumber(math.Copysign(0, -1))},
	}

	for _, tt := range tests {
		v := al.LNumber2I(tt.val)
		got := float64(v.(LNumber))

		if math.IsNaN(float64(tt.val)) {
			if !math.IsNaN(got) {
				t.Errorf("%s: expected NaN, got %v", tt.name, got)
			}
		} else if got != float64(tt.val) {
			t.Errorf("%s: expected %v, got %v", tt.name, tt.val, got)
		}
	}
}

func BenchmarkAllocatorLNumber2I_Preloaded(b *testing.B) {
	al := newAllocator(32)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = al.LNumber2I(LNumber(i % preloadLimit))
	}
}

func BenchmarkAllocatorLNumber2I_NonPreloaded(b *testing.B) {
	al := newAllocator(1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = al.LNumber2I(LNumber(i + preloadLimit))
	}
}

func BenchmarkAllocatorLInteger2I_Preloaded(b *testing.B) {
	al := newAllocator(32)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = al.LInteger2I(LInteger(i % intPreloadLimit))
	}
}

func BenchmarkAllocatorLInteger2I_NonPreloaded(b *testing.B) {
	al := newAllocator(1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = al.LInteger2I(LInteger(i + intPreloadLimit))
	}
}
