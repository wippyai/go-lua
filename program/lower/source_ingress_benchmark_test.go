package lower_test

import (
	"testing"

	programlower "github.com/wippyai/go-lua/program/lower"
)

// BenchmarkLowerSourceIngress reports the end-to-end allocation cost of the
// sole byte-reader ingress path. It is deliberately observational: the
// parser and Program construction have legitimate allocation costs, while a
// fixed allocation budget would turn a performance measurement into a false
// correctness gate.
func BenchmarkLowerSourceIngress(b *testing.B) {
	source := programlower.Source{
		Name: "benchmark/ingress.lua",
		Text: []byte("local value = 41\nreturn value + 1"),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := programlower.Lower(source); err != nil {
			b.Fatal(err)
		}
	}
}
