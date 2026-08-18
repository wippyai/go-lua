package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

func TestPredecessorRuleInputForgetsDecisionsAbsentAtTarget(t *testing.T) {
	shared := predecessorLawKey(1)
	extra := predecessorLawKey(2)
	fromID := predecessorLawContent(3)
	toID := predecessorLawContent(4)
	sharedDecision, sharedOK := equation.NewDecision(shared)
	extraDecision, extraOK := equation.NewDecision(extra)
	fromScope, fromScopeOK := equation.NewScope(sharedDecision, extraDecision)
	toScope, toScopeOK := equation.NewScope(sharedDecision)
	if !sharedOK || !extraOK || !fromScopeOK || !toScopeOK {
		t.Fatal("decision scopes")
	}
	batch := equation.NewBatch()
	fromSite, fromSiteOK := batch.AdmitSite(predecessorLawKey(5), fromScope, equation.FalseExpr(), equation.InitAbsent)
	toSite, toSiteOK := batch.AdmitSite(predecessorLawKey(6), toScope, equation.FalseExpr(), equation.InitAbsent)
	if !fromSiteOK || !toSiteOK || !batch.Seal() {
		t.Fatal("source sites")
	}

	sharedID := predecessorLawContent(7)
	extraID := predecessorLawContent(8)
	topology := &artifactReceiptTopology{
		sites: map[identity.ContentID]equation.Site{fromID: fromSite, toID: toSite},
		pointMeta: map[identity.ContentID]artifactPointMetadata{
			fromID: {decisions: []identity.ContentID{sharedID, extraID}},
			toID:   {decisions: []identity.ContentID{sharedID}},
		},
	}
	edge := artifactEnvironmentRow{
		id: predecessorLawContent(9), from: fromID, to: toID, route: predecessorLawContent(10),
		arm: rows.ArtifactStructuralArmTrue,
	}
	input, ok := artifactPredecessorRuleInput(topology, edge, fromSite, toSite, toID, predecessorLawKey(11))
	if !ok || !input.Available() {
		t.Fatal("predecessor input must forget a source decision that is not live at the target")
	}
}

func predecessorLawKey(value byte) composition.Key {
	var id composition.ID
	id[0] = value
	return composition.Key{ID: id, Version: 1}
}

func predecessorLawContent(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}
