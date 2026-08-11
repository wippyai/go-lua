package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func TestAssemblyPointAdmissionRejectsForeignSiteWithoutPoisoning(t *testing.T) {
	composition := sealAdmissionFixture(t, testTrustedTheorem[uint64](12001))

	newBatchSite := func(source SemanticKey) (*equation.Batch, equation.Site) {
		batch := equation.NewBatch()
		scope := equation.EmptyScope()
		site, admitted := batch.AdmitSite(source.compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
		if !admitted || !batch.Seal() {
			t.Fatalf("source site admission failed: admitted=%t sealed=%t", admitted, batch.Sealed())
		}
		return batch, site
	}

	batch, exactSite := newBatchSite(coldKey(12002))
	_, foreignSite := newBatchSite(coldKey(12003))
	assembly := newAssembly(composition, batch)
	if assembly == nil {
		t.Fatal("assembly")
	}

	foreignPoint := &assemblyPoint{assembly: assembly, site: foreignSite}
	if validPoint(assembly, foreignPoint) {
		t.Fatal("foreign site passed point validation")
	}
	if point := admitPoint(assembly, foreignSite); point != nil {
		t.Fatal("foreign site was admitted as a point")
	}
	if !validAssembly(assembly) || len(assembly.points) != 0 || len(assembly.pointBySite) != 0 {
		t.Fatalf("foreign admission changed assembly: valid=%t points=%d index=%d", validAssembly(assembly), len(assembly.points), len(assembly.pointBySite))
	}

	point := admitPoint(assembly, exactSite)
	if point == nil || !validPoint(assembly, point) || len(assembly.points) != 1 {
		t.Fatalf("exact site was not admitted after foreign rejection: point=%p valid=%t points=%d", point, validPoint(assembly, point), len(assembly.points))
	}
}
