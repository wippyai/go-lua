package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func issuedCandidateLawID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0], id[1] = 0xc4, value
	return id
}

// issuedCandidateLawState publishes one empty cold Program so a candidate
// ordinal has an authenticated publication behind it.
func issuedCandidateLawState(t *testing.T) programstate.State {
	t.Helper()
	catalog, catalogOK := programcatalog.CatalogID(issuedCandidateLawID(1))
	if !catalogOK {
		t.Fatal("law catalog identity unavailable")
	}
	frozen, sealed := programpublication.Publication{}.Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("law publication refused sealing")
	}
	state, stateOK := programstate.New(frozen, catalog)
	if !stateOK {
		t.Fatal("law program state unavailable")
	}
	return state
}

// issuedCandidateLawDescriptor is the smallest descriptor that states the
// issued arm. Its geometry is irrelevant to the law: what is under test is
// which authority the candidate is resolved through.
func issuedCandidateLawDescriptor(t *testing.T) generated.CompiledRule {
	t.Helper()
	descriptor, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 1, InputCount: 0, IssuedCandidate: true,
		Candidate: ruleplan.RelationAddr{},
		Reads:     []generated.ReadPlan{},
		Outputs: []generated.OutputPlan{{
			Mode: ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
	})
	if !ok {
		t.Fatal("law descriptor refused the issued arm")
	}
	return descriptor
}

func issuedCandidateLawRows(source artifactMountedRuleSource, key artifactMountedRule) *programRows {
	return &programRows{mountedRows: &mountedArtifactRows{
		ruleSet: map[artifactMountedRule]artifactMountedRuleSource{key: source},
	}}
}

// TestIssuedCandidateIsTransportedNotResolved is the engine half of the cut.
// A rule whose candidates are Program rows takes the ordinal issuance already
// computed straight off the mounted placement. No axis owner is consulted -
// the fixture below has no Factor directory at all, so an arm that reached for
// one could not answer.
func TestIssuedCandidateIsTransportedNotResolved(t *testing.T) {
	state := issuedCandidateLawState(t)
	role := RuleSlotCapability{}
	coords := OperandCoords{
		Mount:      issuedCandidateLawID(2),
		Point:      issuedCandidateLawID(3),
		Occurrence: issuedCandidateLawID(4),
	}
	key := artifactMountedRule{role: role, mount: coords.Mount, point: coords.Point, occurrence: coords.Occurrence}
	rows := issuedCandidateLawRows(artifactMountedRuleSource{state: state, ordinal: 7, present: true}, key)

	ordinal, source, ok := declaredGeneratedCandidate(rows, nil, issuedCandidateLawDescriptor(t), role, coords)
	if !ok {
		t.Fatal("issued candidate refused a placement that carries one")
	}
	if ordinal != 7 {
		t.Fatalf("issued candidate ordinal = %d, want 7", ordinal)
	}
	if !source.Available() || source.State().CatalogID() != state.CatalogID() {
		t.Fatal("issued candidate lost the publication its ordinal addresses")
	}
	carried, carriedOK := source.Ordinal()
	if !carriedOK || carried != ordinal {
		t.Fatalf("capability ordinal = %d/%t, want %d", carried, carriedOK, ordinal)
	}
}

// TestIssuedCandidateRefusesAPlacementWithoutOne keeps the arm total. A rule
// declaring an issued candidate cannot run on a placement that names no
// candidate row, on a coordinate the mounted plane does not address, or on the
// Link plane, which has no mount for an ordinal to be relative to.
func TestIssuedCandidateRefusesAPlacementWithoutOne(t *testing.T) {
	state := issuedCandidateLawState(t)
	role := RuleSlotCapability{}
	coords := OperandCoords{
		Mount:      issuedCandidateLawID(2),
		Point:      issuedCandidateLawID(3),
		Occurrence: issuedCandidateLawID(4),
	}
	key := artifactMountedRule{role: role, mount: coords.Mount, point: coords.Point, occurrence: coords.Occurrence}
	descriptor := issuedCandidateLawDescriptor(t)

	sourceless := issuedCandidateLawRows(artifactMountedRuleSource{}, key)
	if _, _, ok := declaredGeneratedCandidate(sourceless, nil, descriptor, role, coords); ok {
		t.Fatal("a placement carrying no candidate row admitted an issued candidate")
	}

	addressed := issuedCandidateLawRows(artifactMountedRuleSource{state: state, ordinal: 7, present: true}, key)
	elsewhere := coords
	elsewhere.Occurrence = issuedCandidateLawID(5)
	if _, _, ok := declaredGeneratedCandidate(addressed, nil, descriptor, role, elsewhere); ok {
		t.Fatal("an unaddressed coordinate admitted an issued candidate")
	}

	link := coords
	link.Mount = identity.ContentID{}
	if _, _, ok := declaredGeneratedCandidate(addressed, nil, descriptor, role, link); ok {
		t.Fatal("the mount-neutral plane admitted a mount-relative candidate ordinal")
	}
}

// TestProgramSourceRefusesAnOrdinalWithNoPublication states the capability's
// own law: an ordinal is an address into a publication, so there is no such
// thing as a candidate row without one.
func TestProgramSourceRefusesAnOrdinalWithNoPublication(t *testing.T) {
	if _, ok := execution.NewProgramSource(programstate.State{}, 0); ok {
		t.Fatal("a candidate ordinal was sealed with no publication behind it")
	}
	state := issuedCandidateLawState(t)
	source, ok := execution.NewProgramSource(state, 0)
	if !ok || !source.Available() {
		t.Fatal("a published state was refused")
	}
	ordinal, ordinalOK := source.Ordinal()
	if !ordinalOK || ordinal != 0 {
		t.Fatalf("ordinal zero is a real row: got %d/%t", ordinal, ordinalOK)
	}
}

// issuedCandidateLawTemplate seals the smallest mountable artifact: one
// initial Point and one placement on it, carrying whatever candidate row the
// caller states.
func issuedCandidateLawTemplate(t *testing.T, source uint32, present bool) (*rows.ArtifactScalarTemplate, rows.ArtifactScalarRole) {
	t.Helper()
	spec, specOK := rows.NewArtifactScalarSpec(issuedCandidateLawID(10), issuedCandidateLawID(11), issuedCandidateLawID(12), rows.ArtifactScalarCapacity{Points: 1, Regions: 1, Events: 3, Bodies: 1, Roles: 1, Rules: 1})
	if !specOK {
		t.Fatal("law scalar spec unavailable")
	}
	role, roleOK := spec.DeclareRole(issuedCandidateLawID(13))
	point, region, body := issuedCandidateLawID(14), issuedCandidateLawID(16), issuedCandidateLawID(17)
	_, pointOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: point, Initial: true})
	regionIndex, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: region, Head: point})
	bodyIndex, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: body})
	ruleOK := roleOK && pointOK && regionOK && bodyOK &&
		spec.AddRegionMember(regionIndex, point) &&
		spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: region}) &&
		spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: point}) &&
		spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: region}) &&
		spec.AddBodyEntry(bodyIndex, point) && spec.AddBodyExit(bodyIndex, point) &&
		spec.AddRule(rows.ArtifactScalarRule{
			Role: role, Stage: programissuance.StageBase, Point: point,
			ID: issuedCandidateLawID(15), Source: source, SourcePresent: present,
		})
	if !ruleOK {
		t.Fatal("law scalar rows refused")
	}
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	if !templateOK {
		t.Fatal("law scalar template refused")
	}
	return template, role
}

// TestSealedTemplateCarriesTheCandidateRowUnchanged is the artifact half of
// the transport. The ordinal issuance published survives the scalar template
// exactly as issued, including ordinal zero, which is a real candidate row and
// not an absent one.
func TestSealedTemplateCarriesTheCandidateRowUnchanged(t *testing.T) {
	for name, expected := range map[string]rows.ArtifactScalarRule{
		"issued":      {Source: 9, SourcePresent: true},
		"issued-zero": {Source: 0, SourcePresent: true},
		"sourceless":  {},
	} {
		t.Run(name, func(t *testing.T) {
			template, _ := issuedCandidateLawTemplate(t, expected.Source, expected.SourcePresent)
			if template.RuleCount() != 1 {
				t.Fatalf("template rule count = %d, want 1", template.RuleCount())
			}
			sealed, sealedOK := template.RuleAt(0)
			if !sealedOK {
				t.Fatal("sealed template lost its placement")
			}
			if sealed.SourcePresent != expected.SourcePresent || sealed.Source != expected.Source {
				t.Fatalf("sealed candidate row = %d/%t, want %d/%t", sealed.Source, sealed.SourcePresent, expected.Source, expected.SourcePresent)
			}
		})
	}
}
