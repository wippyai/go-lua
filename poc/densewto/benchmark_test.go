package densewto

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/solve/concreteflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func BenchmarkRepresentativeBody(b *testing.B) {
	reg := standard.Registry()
	f := newFixture(reg)
	ordinary, direct := configs(b, f, reg, nil, 0)
	productionPlan, err := concreteflow.Compile(f.graph, operationplan.New(f.graph, f.input), ordinary.WTOPlan)
	if err != nil {
		b.Fatal(err)
	}
	production := ordinary
	production.ConcreteFlow = productionPlan
	production.CanonicalConcreteTransactions = true
	domain, err := state.TryDomainWithOptionalLanesAndOptions(reg, nil, production.StateOptions)
	if err != nil {
		b.Fatal(err)
	}
	production.PreparedDomain = &domain
	production.FuseConcreteIdentity = true
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
	b.Run("production-dense-wto", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = transfer.Run(production)
		}
	})
}
