package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func contributionForPlan(t *testing.T, value fixture) output.ContributionSpec {
	t.Helper()
	operation, ok := model.IssueOperationID(value.owner, testToken(t, "contribution-operation"))
	if !ok {
		t.Fatal("operation")
	}
	denominator, ok := model.NewDenominatorRef(value.relationA.ID(), value.keyA.ID())
	if !ok {
		t.Fatal("denominator")
	}
	semantic, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schemaID},
		Outputs: []signature.Output{{
			Relation: value.relationA.ID(), Column: value.columnA.ID(), Type: value.columnA.Type(),
			Presence: signature.ProducePresent, Denominator: denominator,
		}},
	})
	if !ok {
		t.Fatal("signature")
	}
	capability, ok := model.NewAscendingCapability(value.columnA.Type())
	if !ok {
		t.Fatal("capability")
	}
	contribution, ok := output.Seal(output.Spec{
		Signature: semantic,
		Port: output.OutputPort{
			Operation: semantic.Identity(), Column: value.columnA.ID(),
		},
		ValueType: value.columnA.Type(), Algebra: capability, Reducer: output.Contributions,
	})
	if !ok {
		t.Fatal("contribution")
	}
	return contribution
}

func TestContributionDeclarationIsDigestCoveredAndDefensive(t *testing.T) {
	value := newFixture(t)
	contribution := contributionForPlan(t, value)
	capability := contribution.Algebra()

	withoutBuilder := NewBuilder(value.schemaID)
	withoutBuilder.AddTypeCapability(capability)
	withoutBuilder.AddSignature(signatureForContribution(t, value, contribution))
	without, ok := withoutBuilder.Build()
	if !ok {
		t.Fatal("schema without contribution")
	}

	withBuilder := NewBuilder(value.schemaID)
	withBuilder.AddTypeCapability(capability)
	withBuilder.AddSignature(signatureForContribution(t, value, contribution))
	if !withBuilder.AddContribution(contribution) {
		t.Fatal("contribution declaration")
	}
	with, ok := withBuilder.Build()
	if !ok {
		t.Fatal("schema with contribution")
	}
	if without.Digest() == with.Digest() {
		t.Fatal("contribution declaration did not change execution-schema digest")
	}
	values := with.Contributions()
	if len(values) != 1 || !values[0].Equal(contribution) {
		t.Fatalf("sealed contribution vector = %+v", values)
	}
	values[0] = output.ContributionSpec{}
	if got := with.Contributions(); len(got) != 1 || !got[0].Equal(contribution) {
		t.Fatal("contribution accessor exposed mutable schema state")
	}
}

func signatureForContribution(t *testing.T, value fixture, contribution output.ContributionSpec) signature.Signature {
	t.Helper()
	semantic, ok := signature.Seal(signature.Spec{
		Identity: contribution.Port().Operation,
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schemaID},
		Outputs: []signature.Output{{
			Relation: value.relationA.ID(), Column: value.columnA.ID(), Type: value.columnA.Type(),
			Presence: signature.ProducePresent,
			Denominator: func() model.DenominatorRef {
				denominator, _ := model.NewDenominatorRef(value.relationA.ID(), value.keyA.ID())
				return denominator
			}(),
		}},
	})
	if !ok {
		t.Fatal("signature")
	}
	return semantic
}
