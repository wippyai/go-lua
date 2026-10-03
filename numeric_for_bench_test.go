package lua

import "testing"

func BenchmarkForNumericLarge(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()
	fn, err := L.LoadString(`for i = 1, 100000 do end`)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		if err := L.PCall(0, 0, nil); err != nil {
			b.Fatal(err)
		}
	}
}
