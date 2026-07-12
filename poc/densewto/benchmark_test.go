package densewto

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func BenchmarkRepresentativeBody(b *testing.B) {
	reg := standard.Registry()
	f := newFixture(reg)
	ordinary, direct := configs(b, f, reg, nil, 0)
	b.Run("current-transfer-wto", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = transfer.Run(ordinary)
		}
	})
	b.Run("dense-direct-wto", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = direct.Run()
		}
	})
}
