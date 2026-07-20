package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLaneCatalogRequiresExplicitIdentitySupportPolicy(t *testing.T) {
	tests := []struct {
		name string
		edit func(*laneSpec)
	}{
		{name: "missing", edit: func(spec *laneSpec) { spec.identitySupport = laneIdentitySupportPolicy{} }},
		{name: "independent-with-enumerator", edit: func(spec *laneSpec) {
			spec.identitySupport = enumeratedIdentitySupport(visitValuesLaneIdentities, func(s State) valueLane { return s.values }, IdentityImageEmbeddedValue)
			spec.identitySupport.kind = laneIdentitiesIndependent
		}},
		{name: "enumerated-without-enumerator", edit: func(spec *laneSpec) {
			spec.identitySupport = laneIdentitySupportPolicy{kind: laneIdentitiesEnumerated}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := valuesLaneSpec
			test.edit(&spec)
			defer func() {
				if recover() == nil {
					t.Fatal("catalog admitted a lane without one exact identity-support policy")
				}
			}()
			_ = newLaneCatalog([]laneSpec{spec})
		})
	}
}

func TestProductDomainLaneIdentitySupportIsOwnedAndExact(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	body := lexicalidentity.FunctionBody(
		lexicalidentity.UnitNamespaceFromContent([]byte("product-lane-identity-support")), 1,
	)
	template := identity.AllocationTerm(identity.ManifestAllocationTemplate(body, 1, 1))
	input := domain.Lattice().Bottom().WriteValue(reg, key.SymbolValue(symbol.ID(4401)), identityvalue.PresentTerm(reg, template))
	factors, err := domain.Decompose(input)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := domain.ProductLane(LaneValues)
	if !ok {
		t.Fatal("Values lane")
	}
	var valueFactor LaneFactor
	for _, factor := range factors {
		if factor.Lane() == values {
			valueFactor = factor
			break
		}
	}
	seen := map[identity.Term]struct{}{}
	if err := domain.VisitLaneIdentityTerms(valueFactor, func(term identity.Term) { seen[term] = struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if _, ok := seen[template]; !ok || len(seen) != 1 {
		t.Fatalf("Values identity support = %#v, want only template", seen)
	}
	contains, err := domain.LaneContainsAllocationTemplate(valueFactor)
	if err != nil || !contains {
		t.Fatalf("LaneContainsAllocationTemplate = %v, %v", contains, err)
	}

	foreignReg, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	foreign := RegisteredProductDomain(foreignReg)
	foreignFactors, err := foreign.Decompose(foreign.Lattice().Bottom())
	if err != nil {
		t.Fatal(err)
	}
	if err := domain.VisitLaneIdentities(foreignFactors[0], func(identity.ID) {}); err == nil {
		t.Fatal("identity visitor accepted a foreign lane factor")
	}
	if err := domain.VisitLaneIdentities(valueFactor, nil); err == nil {
		t.Fatal("identity visitor accepted a nil visitor")
	}
}
