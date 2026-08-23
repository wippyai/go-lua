package codegen

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestModelCarriesExactCatalogFenceAndAbsentCandidateGuard(t *testing.T) {
	digest := identity.ContentID{0: 1}
	model := Model{
		digest: digest,
		axes:   []Axis{{ordinal: 0, key: "axis/codegen"}},
		rules: []Rule{{
			ordinal: 4,
			candidate: Candidate{
				address: ruleplan.RelationAddr{Axis: 0, Member: 0},
				axis:    "axis/codegen",
				key:     "relation/candidate",
				subject: memberdefinition.GoType{Name: "Candidate"},
			},
			reducer: ReducerCall{
				Address:           ruleplan.ReducerAddr{Axis: 0, Member: 0},
				Axis:              "axis/codegen",
				Key:               "reducer/identity",
				CandidateConstant: true,
				Inputs: []ReducerInput{{
					Join: 0, Type: memberdefinition.GoType{Name: "Fact"},
					Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
				}},
				Outputs: []Output{{
					address: ruleplan.OutputAddr{Axis: 0, Frame: 0},
					mode:    program.ModeExact, slot: 0,
					valueType: memberdefinition.GoType{Name: "Fact"},
				}},
			},
		}},
	}
	if !model.Available() || model.Digest() != digest {
		t.Fatalf("model fence/availability = %t/%v", model.Available(), model.Digest())
	}
	row, rowOK := model.At(0)
	if !rowOK || row.Reducer().CandidateGuard() != true {
		t.Fatal("absent reducer candidate did not retain the true guard")
	}
	if got := row.Reducer().CandidatePresent; got {
		t.Fatal("absent reducer candidate was marked present")
	}
}

func TestBuildRefusesUnavailableCatalogBeforeRosterInspection(t *testing.T) {
	_, err := Build(nil, ruleplan.Catalog{})
	if err == nil {
		t.Fatal("unavailable rule-plan catalog admitted")
	}
	failure, failureOK := err.(Failure)
	if !failureOK || failure.Kind != ProblemDigest {
		t.Fatalf("unavailable catalog failure = %#v, want digest refusal", err)
	}
}
