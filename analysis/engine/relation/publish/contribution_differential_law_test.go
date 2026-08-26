package publish_test

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applycontribution "github.com/wippyai/go-lua/analysis/engine/relation/apply/contribution"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// contributionDifferential applies the same invocation twice, retaining the
// two original proposal leases.  The helper deliberately does not rebuild a
// batch from proposals: Differential must own the exact side leases.
func contributionDifferential(t *testing.T, value fixture, before, after []binding.Proposal) applydifferential.Differential {
	t.Helper()
	value.worker.result = outcome.Result{Code: outcome.Produced}
	value.worker.proposals = before
	left := value.application(t, outcome.Produced)
	value.worker.proposals = after
	right := value.application(t, outcome.Produced)
	delta, ok := applydifferential.New(left, right)
	if !ok || !delta.Available() {
		t.Fatal("differential")
	}
	return delta
}

func TestContributionDifferentialAllSignedShapes(t *testing.T) {
	value := newContributionFixture(t)
	base := append([]binding.Proposal(nil), value.proposals...)
	newValue, ok := value.mounted.IssueValue(value.typeID, content("differential-successor"))
	if !ok {
		t.Fatal("successor value")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	replacement, ok := binding.NewProposal(value.token, newValue, presence)
	if !ok {
		t.Fatal("replacement proposal")
	}
	changed := append([]binding.Proposal(nil), base...)
	changed[1] = replacement

	cases := []struct {
		name       string
		before     []binding.Proposal
		after      []binding.Proposal
		beforeSide bool
		afterSide  bool
		replace    bool
		want       int
	}{
		{name: "replacement", before: base, after: changed, beforeSide: true, afterSide: true, replace: true, want: 1},
		{name: "before-only", before: base, after: nil, beforeSide: true, want: 1},
		{name: "after-only", before: nil, after: changed, afterSide: true, want: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var delta applydifferential.Differential
			if test.before == nil {
				value.worker.result = outcome.Result{Code: outcome.Produced}
				value.worker.proposals = test.after
				after := value.application(t, outcome.Produced)
				var ok bool
				delta, ok = applydifferential.New(apply.Application{}, after)
				if !ok {
					t.Fatal("after-only differential")
				}
			} else if test.after == nil {
				value.worker.result = outcome.Result{Code: outcome.Produced}
				value.worker.proposals = test.before
				before := value.application(t, outcome.Produced)
				var ok bool
				delta, ok = applydifferential.New(before, apply.Application{})
				if !ok {
					t.Fatal("before-only differential")
				}
			} else {
				delta = contributionDifferential(t, value, test.before, test.after)
			}
			got, ok := applycontribution.TransitionsForDifferential(value.mounted, delta)
			if !ok || len(got) != test.want {
				t.Fatalf("transitions ok=%t len=%d", ok, len(got))
			}
			if len(got) != 0 {
				if got[0].Replacement() != test.replace {
					t.Fatalf("replacement=%t want=%t", got[0].Replacement(), test.replace)
				}
				_, gotBefore := got[0].Before()
				_, gotAfter := got[0].After()
				if gotBefore != test.beforeSide || gotAfter != test.afterSide {
					t.Fatalf("sides before=%t after=%t", gotBefore, gotAfter)
				}
				if test.replace {
					beforeSide, _ := got[0].Before()
					afterSide, _ := got[0].After()
					if !beforeSide.Value().Same(value.value) || !afterSide.Value().Same(newValue) || beforeSide.Lineage() != value.lineage || afterSide.Lineage() != value.lineage {
						t.Fatal("replacement did not retain each side's signed payload")
					}
				}
			}
		})
	}
}

func TestContributionDifferentialIgnoresOrdinaryAndCanonicalizesPermutation(t *testing.T) {
	value := newContributionFixture(t)
	base := append([]binding.Proposal(nil), value.proposals...)
	// The fixture's first and third proposals are ordinary output columns;
	// only the middle proposal is mounted as a contribution descriptor.
	ordinary := []binding.Proposal{base[2], base[0]}
	value.worker.result = outcome.Result{Code: outcome.Produced}
	value.worker.proposals = ordinary
	ordinaryApplication := value.application(t, outcome.Produced)
	value.worker.proposals = base
	contributionApplication := value.application(t, outcome.Produced)
	ordinaryOnly, ok := applydifferential.New(apply.Application{}, ordinaryApplication)
	if !ok {
		t.Fatal("ordinary differential")
	}
	got, ok := applycontribution.TransitionsForDifferential(value.mounted, ordinaryOnly)
	if !ok || len(got) != 0 {
		t.Fatalf("ordinary transitions ok=%t len=%d", ok, len(got))
	}

	permuted := []binding.Proposal{base[2], base[1], base[0]}
	value.worker.proposals = permuted
	permutedApplication := value.application(t, outcome.Produced)
	delta, ok := applydifferential.New(contributionApplication, permutedApplication)
	if !ok {
		t.Fatal("permuted differential")
	}
	got, ok = applycontribution.TransitionsForDifferential(value.mounted, delta)
	if !ok || len(got) != 1 || !got[0].Replacement() {
		t.Fatalf("permuted transitions ok=%t len=%d", ok, len(got))
	}
}

func TestContributionDifferentialChangedDestinationIsDeleteThenInsert(t *testing.T) {
	cardinality, ok := model.NewCardinality(model.BoundedMany, 2)
	if !ok {
		t.Fatal("cardinality")
	}
	value := newFixtureWithContribution(t, 0x9a, 2, cardinality, true)
	second, ok := value.mounted.IssueCell(mustDenominator(t, value), value.scope, value.column, value.rows[1])
	if !ok {
		t.Fatal("second destination")
	}
	secondValue, ok := value.mounted.IssueValue(value.typeID, content("second-destination-value"))
	if !ok {
		t.Fatal("second value")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	proposal, ok := binding.NewProposal(second, secondValue, presence)
	if !ok {
		t.Fatal("second proposal")
	}
	// Restrict both leases to one row so the destination move is the only
	// contribution key in flight; the second mounted row is used solely as the
	// successor destination.
	before := append([]binding.Proposal(nil), value.proposals[:3]...)
	after := append([]binding.Proposal(nil), value.proposals[:3]...)
	after[1] = proposal
	delta := contributionDifferential(t, value, before, after)
	got, ok := applycontribution.TransitionsForDifferential(value.mounted, delta)
	if !ok || len(got) != 2 {
		t.Fatalf("changed destination ok=%t len=%d", ok, len(got))
	}
	if _, beforeOK := got[0].Before(); !beforeOK {
		t.Fatal("old destination did not carry Before")
	}
	if _, afterOK := got[0].After(); afterOK {
		t.Fatal("old destination unexpectedly carried After")
	}
	if _, beforeOK := got[1].Before(); beforeOK {
		t.Fatal("new destination unexpectedly carried Before")
	}
	if _, afterOK := got[1].After(); !afterOK {
		t.Fatal("new destination did not carry After")
	}
}

func TestContributionDifferentialRejectsRemovalAndDuplicate(t *testing.T) {
	value := newContributionFixture(t)
	removal, ok := binding.NewRemovalProposal(value.token)
	if !ok {
		t.Fatal("removal")
	}
	value.worker.result = outcome.Result{Code: outcome.Produced}
	value.worker.proposal = removal
	value.worker.proposals = nil
	removalApplication := value.application(t, outcome.Produced)
	delta, ok := applydifferential.New(apply.Application{}, removalApplication)
	if !ok {
		t.Fatal("removal differential")
	}
	if got, accepted := applycontribution.TransitionsForDifferential(value.mounted, delta); accepted || got != nil {
		t.Fatal("removal crossed classifier")
	}

	value.worker.proposals = value.proposals
	application := value.application(t, outcome.Produced)
	batch, ok := application.Proposals()
	if !ok {
		t.Fatal("batch")
	}
	duplicate := append([]binding.Proposal(nil), value.proposals...)
	duplicate[2] = duplicate[1]
	field := reflect.ValueOf(&batch).Elem().FieldByName("proposals")
	if !field.IsValid() {
		t.Fatal("proposal field")
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(duplicate))
	applicationField := reflect.ValueOf(&application).Elem().FieldByName("batch")
	if !applicationField.IsValid() {
		t.Fatal("application batch field")
	}
	reflect.NewAt(applicationField.Type(), unsafe.Pointer(applicationField.UnsafeAddr())).Elem().Set(reflect.ValueOf(batch))
	delta, ok = applydifferential.New(apply.Application{}, application)
	if !ok {
		t.Fatal("duplicate differential")
	}
	if got, accepted := applycontribution.TransitionsForDifferential(value.mounted, delta); accepted || got != nil {
		t.Fatal("duplicate crossed classifier")
	}
}

func mustDenominator(t *testing.T, value fixture) binding.DenominatorWitness {
	t.Helper()
	witness, ok := value.mounted.Denominator(value.denominator)
	if !ok {
		t.Fatal("denominator")
	}
	return witness
}
