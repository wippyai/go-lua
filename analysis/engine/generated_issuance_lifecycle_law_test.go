package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

type generatedIssuanceLifecycleFixture struct {
	bindingFixture generatedBindingLawFixture
	rows           *programRows
	cell           *generatedRuleBindingCell
	site           equation.Site
	source         composition.Key
	entity         composition.Key
	coords         OperandCoords
}

func newGeneratedIssuanceLifecycleFixture(t testing.TB) generatedIssuanceLifecycleFixture {
	t.Helper()
	bindingFixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole))
	bindingFixture.owner = generatedBindingLawOwnerForDescriptor(t, bindingFixture)
	bindGeneratedLawOwner(t, &bindingFixture, 0)
	if !bindingFixture.binding.Seal() {
		t.Fatal("generated lifecycle binding seal")
	}
	rows, rowsOK := newProgramRows(bindingFixture.binding)
	if !rowsOK || rows == nil || rows.batch == nil || rows.batch.Sealed() {
		t.Fatal("generated lifecycle open program rows")
	}
	axisSemantic, axisOK := vocabulary.Key(string(generatedRuleLawAxisRole))
	operandSemantic, operandOK := vocabulary.Key(string(generatedRuleLawOperandRole))
	if !axisOK || !operandOK {
		t.Fatal("generated lifecycle semantic keys")
	}
	source := compositionKeyOf(axisSemantic)
	site, siteOK := rows.admitSite(source, equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	if !siteOK || !rows.batch.OwnsOpenSite(site) || site.Available() {
		t.Fatal("generated lifecycle source site was not an open opaque capability")
	}
	entity := compositionKeyOf(operandSemantic)
	cell := generatedLawCell(t, bindingFixture)
	return generatedIssuanceLifecycleFixture{
		bindingFixture: bindingFixture,
		rows:           rows,
		cell:           cell,
		site:           site,
		source:         source,
		entity:         entity,
		coords: OperandCoords{
			Mount:      bindingFixture.mount,
			Occurrence: bindingFixture.occurrence,
		},
	}
}

// TestGeneratedIssuanceRelationOwnerServesIndependentProgramRows proves one
// sealed Factor relation owner remains usable for separate program
// constructions. Each construction gets a fresh source Batch and Site, then
// reaches the complete generated declaration seam independently.
func TestGeneratedIssuanceRelationOwnerServesIndependentProgramRows(t *testing.T) {
	fixture := newGeneratedIssuanceLifecycleFixture(t)
	secondRows, rowsOK := newProgramRows(fixture.bindingFixture.binding)
	if !rowsOK || secondRows == nil || secondRows == fixture.rows || secondRows.batch == fixture.rows.batch || secondRows.batch.Sealed() {
		t.Fatal("second generated lifecycle construction did not open an independent program batch")
	}
	secondSite, siteOK := secondRows.admitSite(fixture.source, equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	if !siteOK || !secondRows.batch.OwnsOpenSite(secondSite) || secondSite.Available() {
		t.Fatal("second generated lifecycle construction did not admit a fresh open Site")
	}
	second := generatedIssuanceLifecycleFixture{
		bindingFixture: fixture.bindingFixture,
		rows:           secondRows,
		cell:           fixture.cell,
		site:           secondSite,
		source:         fixture.source,
		entity:         fixture.entity,
		coords:         fixture.coords,
	}
	firstDeclared, firstOK := fixture.declare()
	secondDeclared, secondOK := second.declare()
	for _, test := range []struct {
		name     string
		declared pendingRuleIssuance
		rows     *programRows
		ok       bool
	}{
		{name: "first", declared: firstDeclared, rows: fixture.rows, ok: firstOK},
		{name: "second", declared: secondDeclared, rows: secondRows, ok: secondOK},
	} {
		if !test.ok {
			t.Fatalf("%s generated issuance refused", test.name)
		}
		if !test.rows.batch.OwnsOpenOccurrence(test.declared.anchor.occurrence) || !test.rows.batch.OwnsOpenOperandFor(test.declared.anchor.operand, test.declared.anchor.occurrence) {
			t.Fatalf("%s generated issuance did not complete its open source anchor", test.name)
		}
		candidate := uint32(0)
		if test.declared.generated != nil {
			candidate = test.declared.generated.candidate
		}
		if test.declared.generated == nil || candidate != 1 || len(test.declared.surfaces.reads) != 1 || len(test.declared.surfaces.writes) != 1 || test.declared.surfaces.carries != 1 {
			t.Fatalf("%s generated issuance was incomplete: candidate=%d generated=%t reads=%d writes=%d carries=%d", test.name, candidate, test.declared.generated != nil, len(test.declared.surfaces.reads), len(test.declared.surfaces.writes), test.declared.surfaces.carries)
		}
	}
}

func (fixture generatedIssuanceLifecycleFixture) declare() (pendingRuleIssuance, bool) {
	return declareGeneratedIssuanceSurfaces(
		fixture.rows,
		fixture.bindingFixture.binding.state,
		fixture.cell,
		// This fixture's rule publishes a fact, so the Link directory it would
		// fan an activation branch over is never reached.
		executioncontext.Directory{},
		fixture.coords,
		fixture.site,
		fixture.entity,
		pendingRuleIssuance{role: fixture.bindingFixture.cap},
	)
}

// TestGeneratedIssuanceAcceptsOpenOwnedSiteAndAdmitsAnchor exercises the
// generated declaration at its source-admission boundary. The Site is still
// open and therefore deliberately reports !Available; only ownership by the
// exact open programRows Batch is sufficient. The declaration then admits its
// occurrence and operand into that same Batch before returning Factor-issued
// surfaces.
func TestGeneratedIssuanceAcceptsOpenOwnedSiteAndAdmitsAnchor(t *testing.T) {
	fixture := newGeneratedIssuanceLifecycleFixture(t)
	declared, ok := fixture.declare()
	if !ok {
		t.Fatal("generated issuance refused an open Site owned by program rows")
	}
	if !fixture.rows.batch.OwnsOpenOccurrence(declared.anchor.occurrence) {
		t.Fatal("generated issuance did not admit its occurrence into program rows")
	}
	if !fixture.rows.batch.OwnsOpenOperandFor(declared.anchor.operand, declared.anchor.occurrence) {
		t.Fatal("generated issuance did not admit its operand for the occurrence")
	}
	if declared.anchor.occurrence.Available() || declared.anchor.operand.Available() {
		t.Fatal("generated issuance exposed open source capabilities as sealed")
	}
	if !declared.anchor.occurrence.IdentityKey().Available() || !declared.anchor.operand.IdentityKey().Available() {
		t.Fatal("generated issuance anchor identities were not derivable while open")
	}
	candidate := uint32(0)
	if declared.generated != nil {
		candidate = declared.generated.candidate
	}
	if declared.generated == nil || candidate != 1 || len(declared.surfaces.reads) != 1 || len(declared.surfaces.writes) != 1 || declared.surfaces.carries != 1 {
		t.Fatalf("generated issuance declaration was incomplete: candidate=%d generated=%t reads=%d writes=%d carries=%d", candidate, declared.generated != nil, len(declared.surfaces.reads), len(declared.surfaces.writes), declared.surfaces.carries)
	}
}

// TestGeneratedIssuanceRejectsNearestLifecycleNegatives keeps the ownership,
// coordinate, and owner-projection fences adjacent to the positive lifecycle
// law. Each case gets a fresh open Batch because a foreign Site is rejected by
// the Batch authority itself.
func TestGeneratedIssuanceRejectsNearestLifecycleNegatives(t *testing.T) {
	t.Run("foreign-open-batch-site", func(t *testing.T) {
		fixture := newGeneratedIssuanceLifecycleFixture(t)
		foreign := equation.NewBatch()
		axisSemantic, axisOK := vocabulary.Key(string(generatedRuleLawAxisRole))
		if !axisOK {
			t.Fatal("foreign lifecycle semantic key")
		}
		foreignSite, siteOK := foreign.AdmitSite(compositionKeyOf(axisSemantic), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
		if !siteOK || !foreign.OwnsOpenSite(foreignSite) {
			t.Fatal("foreign open Site fixture")
		}
		fixture.site = foreignSite
		if _, ok := fixture.declare(); ok {
			t.Fatal("generated issuance accepted a Site from a foreign open Batch")
		}
	})

	t.Run("absent-coordinates-and-entity", func(t *testing.T) {
		tests := []struct {
			name   string
			coords OperandCoords
			entity composition.Key
		}{
			{name: "mount", coords: OperandCoords{Occurrence: identity.ContentID{}}},
			{name: "occurrence", coords: OperandCoords{Mount: identity.ContentID{}}},
			{name: "entity", coords: OperandCoords{Mount: identity.ContentID{}, Occurrence: identity.ContentID{}}},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				fixture := newGeneratedIssuanceLifecycleFixture(t)
				coords := fixture.coords
				entity := fixture.entity
				switch test.name {
				case "mount":
					coords.Mount = identity.ContentID{}
				case "occurrence":
					coords.Occurrence = identity.ContentID{}
				case "entity":
					entity = composition.Key{}
				}
				fixture.coords, fixture.entity = coords, entity
				if _, ok := fixture.declare(); ok {
					t.Fatal("generated issuance accepted absent coordinate/entity")
				}
			})
		}
	})

	t.Run("wrong-owner-projection", func(t *testing.T) {
		fixture := newGeneratedIssuanceLifecycleFixture(t)
		descriptor := fixture.cell.generated.program
		delete(fixture.bindingFixture.owner.projections, [2]uint32{descriptor.JoinRelation().Member, descriptor.KeyProjection().Member})
		if _, ok := fixture.declare(); ok {
			t.Fatal("generated issuance accepted a missing owner projection")
		}
	})
}
