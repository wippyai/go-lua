package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func mustTransformerCoordinateFactorInventory(
	t testing.TB,
	authority *factapply.PathSemanticAuthority,
	domain state.ProductDomain,
	slots []state.CoordinateSlot,
) state.CoordinateFactorInventory {
	t.Helper()
	inventory, err := authority.SealCoordinateFactorInventory(domain, slots)
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}

func emptyBuilder(t testing.TB, reg *axis.Registry, shape Shape, caps *OutputCapabilityRegistry) (*Builder, SemanticCertificate) {
	t.Helper()
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificate, err := CertifyPlan(plan, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	return NewBuilder(reg, shape, caps, plan), certificate
}
