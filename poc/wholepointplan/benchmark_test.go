package wholepointplan

import (
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func BenchmarkCompileCanonicalWholePoint(b *testing.B) {
	input, _ := randomWholePoint(rand.New(rand.NewSource(0x51de)), cfg.Point(1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Compile(input, Config{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCanonicalCursor(b *testing.B) {
	const point cfg.Point = 1
	input, _ := randomWholePoint(rand.New(rand.NewSource(0x51de)), point)
	plan, err := Compile(input, Config{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	count := 0
	for i := 0; i < b.N; i++ {
		cursor := plan.Cursor(point)
		for {
			_, ok := cursor.Next()
			if !ok {
				break
			}
			count++
		}
	}
	if count == 0 {
		b.Fatal("benchmark row is empty")
	}
}
