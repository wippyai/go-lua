package lua

import "testing"

var numericBenchmarkResult LValue

func BenchmarkNumericTableLookup(b *testing.B) {
	for name, key := range map[string]LValue{"dense_integer": LInteger(12), "dense_float": LNumber(12), "hash_integer": LInteger(1 << 40), "hash_float": LNumber(1 << 40), "wide_integer": LInteger(1<<53 + 1)} {
		b.Run(name, func(b *testing.B) {
			table := CreateTable(0, 0)
			table.RawSet(key, LTrue)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				numericBenchmarkResult = table.RawGet(key)
			}
		})
	}
}

func BenchmarkNumericConstantCompile(b *testing.B) {
	L := NewState()
	defer L.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := L.LoadString("return ((9007199254740993+7)*3)>>2, 7.0//2.5, (0-1)>>1"); err != nil {
			b.Fatal(err)
		}
	}
}
