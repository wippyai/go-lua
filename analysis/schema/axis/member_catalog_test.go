package axis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

func testReducerInput(axis schema.EntryReference) member.ReducerInput {
	return member.ReducerInput{
		Axis:         axis,
		Carrier:      "input/carrier",
		Form:         member.Exact,
		Multiplicity: member.MultiplicityOne,
		Tag:          "",
	}
}

func axisTestAuthorities(keys ...carrier.Key) []carrier.Authority {
	authorities := make([]carrier.Authority, 0, len(keys))
	seen := make(map[carrier.Key]struct{}, len(keys))
	for _, key := range keys {
		if !key.Available() {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		capability := carrier.DecodeOnly
		switch key {
		case "axis/key":
			capability = carrier.Equatable
		case "axis/fact", "axis/other-fact":
			capability = carrier.Ascending
		}
		authorities = append(authorities, carrier.Authority{Carrier: key, Capability: capability})
	}
	return authorities
}

func testMemberCatalog() member.Catalog {
	provider := member.RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}, Member: "relation/input"}
	catalog, ok := member.NewCatalog(
		axisTestAuthorities(
			"relation/subject", "relation/input", "projection/result", "input/carrier",
			"output/carrier", "axis/key", "axis/fact", "axis/other-fact",
		),
		[]carrier.Binding{},
		[]member.Relation{{Key: "relation/input", Subject: "relation/subject", Inputs: []carrier.Key{"relation/input"}, CandidateProvider: member.AxisRelationCandidate(provider)}},
		[]member.Projection{{Key: "projection/key", Relation: "relation/input", Role: member.Key, Result: "projection/result", CandidateProvider: member.AxisRelationCandidate(provider)}},
		[]member.Reducer{{
			Key:     "reducer/output",
			Inputs:  []member.ReducerInput{testReducerInput(schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"})},
			Outputs: []member.ReducerOutput{{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}, Carrier: "output/carrier"}},
		}},
		nil,
	)
	if !ok {
		panic("member catalog rejected")
	}
	return catalog
}

func TestAxisAdmitsCompleteMemberCatalog(t *testing.T) {
	spec := scratchSpec("value", valueRole)
	spec.Catalog = testMemberCatalog()
	spec.Signature = Signature{Key: "axis/key", Fact: "axis/fact"}
	template := mustTemplate(t, spec)
	if got := template.Signature(); got != spec.Signature {
		t.Fatalf("signature accessor = %#v, want %#v", got, spec.Signature)
	}
	if !template.HasMembers() || template.MemberCount() != 3 {
		t.Fatalf("member ratchet = %t/%d", template.HasMembers(), template.MemberCount())
	}
	if ordinal, ok := template.RelationOrdinal("relation/input"); !ok || ordinal != 0 {
		t.Fatalf("relation ordinal = %d/%t", ordinal, ok)
	}
	if ordinal, ok := template.ProjectionOrdinal("projection/key"); !ok || ordinal != 0 {
		t.Fatalf("projection ordinal = %d/%t", ordinal, ok)
	}
	if ordinal, ok := template.ReducerOrdinal("reducer/output"); !ok || ordinal != 0 {
		t.Fatalf("reducer ordinal = %d/%t", ordinal, ok)
	}
	if failure := sealTemplates(t, []*Template[scratchInputs]{template}); failure.Available() {
		t.Fatalf("complete member catalog rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

func TestAxisMemberSignatureRequiresBothNominalCarriers(t *testing.T) {
	for name, signature := range map[string]Signature{
		"missing key":  {Fact: "axis/fact"},
		"missing fact": {Key: "axis/key"},
	} {
		t.Run(name, func(t *testing.T) {
			spec := scratchSpec("value", valueRole)
			spec.Catalog = testMemberCatalog()
			spec.Signature = signature
			if template, ok := New(spec); ok || template != nil {
				t.Fatal("axis admitted an incomplete member signature")
			}
		})
	}
	legacy := scratchSpec("value", valueRole)
	legacy.Signature = Signature{Key: "axis/key"}
	if template, ok := New(legacy); ok || template != nil {
		t.Fatal("axis admitted a partial legacy signature")
	}
}

func TestAxisMemberSignatureIsContentAndSealedLaw(t *testing.T) {
	base := scratchSpec("value", valueRole)
	base.Catalog = testMemberCatalog()
	base.Signature = Signature{Key: "axis/key", Fact: "axis/fact"}
	changed := base
	changed.Signature.Fact = "axis/other-fact"
	left, leftFailure := sealTable(t, []*Template[scratchInputs]{mustTemplate(t, base)})
	right, rightFailure := sealTable(t, []*Template[scratchInputs]{mustTemplate(t, changed)})
	if leftFailure.Available() || rightFailure.Available() {
		t.Fatalf("signature fixtures rejected: left=%+v right=%+v", leftFailure, rightFailure)
	}
	if left.Digest() == right.Digest() {
		t.Fatal("member signature did not affect axis table digest")
	}

	template := mustTemplate(t, base)
	template.signature = Signature{}
	failure := sealTemplates(t, []*Template[scratchInputs]{template})
	if failure.Law != LawMemberSignature || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("incomplete member signature sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

func TestAxisCatalogCopyIsolationAndDigestSensitivity(t *testing.T) {
	catalog := testMemberCatalog()
	spec := scratchSpec("value", valueRole)
	spec.Catalog = catalog
	spec.Signature = Signature{Key: "axis/key", Fact: "axis/fact"}
	template := mustTemplate(t, spec)
	catalog.Relations[0].Inputs[0] = "changed"
	if relation, ok := template.Catalog().Relation("relation/input"); !ok || relation.Inputs[0] != "relation/input" {
		t.Fatalf("template retained source catalog alias: %#v/%t", relation, ok)
	}
	copy := template.Catalog()
	copy.Relations[0].Inputs[0] = "changed-again"
	if relation, ok := template.Catalog().Relation("relation/input"); !ok || relation.Inputs[0] != "relation/input" {
		t.Fatalf("template catalog accessor leaked mutable storage: %#v/%t", relation, ok)
	}

	without := scratchSpec("value", valueRole)
	with := scratchSpec("value", valueRole)
	with.Catalog = testMemberCatalog()
	with.Signature = Signature{Key: "axis/key", Fact: "axis/fact"}
	left, leftFailure := sealTable(t, []*Template[scratchInputs]{mustTemplate(t, without)})
	right, rightFailure := sealTable(t, []*Template[scratchInputs]{mustTemplate(t, with)})
	if leftFailure.Available() || rightFailure.Available() {
		t.Fatalf("catalog digest fixtures rejected: left=%+v right=%+v", leftFailure, rightFailure)
	}
	if left.Digest() == right.Digest() {
		t.Fatal("member catalog did not affect axis table digest")
	}
}

func TestLegacyAxisExplicitlyReportsNoMembers(t *testing.T) {
	template := mustTemplate(t, scratchSpec("value", valueRole))
	if template.HasMembers() || template.MemberCount() != 0 {
		t.Fatalf("legacy member ratchet = %t/%d", template.HasMembers(), template.MemberCount())
	}
}
