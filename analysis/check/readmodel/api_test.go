package readmodel

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestPlanAssignmentCheckHonorsTrustedValueProofBeforePlainSubtype(t *testing.T) {
	check := PlanAssignmentCheck(AssignmentCheckPlan{
		Assignment: Assignment{
			TypeWithPresence: typ.Number,
			Expected:         typ.String,
		},
		ValueAdmissible: true,
		IsSubtype: func(sub, super typ.Type) bool {
			return false
		},
	})
	if !check.Admissible {
		t.Fatalf("assignment check = %#v, want trusted value proof to admit assignment", check)
	}
}

func TestPlanCallArgumentCheckRequiresValueProofForSolvedSubtype(t *testing.T) {
	check := PlanCallArgumentCheck(CallArgumentCheckPlan{
		Argument: CallArgument{
			TypeWithPresence: typeexpr.Optional(typ.String),
		},
		Expected: typeexpr.Optional(typ.String),
	})
	if check.Admissible {
		t.Fatalf("call argument check = %#v, want solved subtype alone to stay inadmissible without value proof", check)
	}
	if check.ProvenMismatch {
		t.Fatalf("call argument check = %#v, want no proven mismatch for matching solved subtype", check)
	}
}

func TestPlanCallArgumentCheckDoesNotTrustUntrustedTopSubtype(t *testing.T) {
	check := PlanCallArgumentCheck(CallArgumentCheckPlan{
		Argument: CallArgument{
			TypeWithPresence:   typ.String,
			UntrustedTopOrigin: true,
		},
		Expected:               typ.String,
		FunctionTypeAdmissible: true,
	})
	if check.Admissible {
		t.Fatalf("call argument check = %#v, want untrusted-top argument to require proof", check)
	}
}

func TestNumericForDefinitelyNotNumberClassifiesSolvedOperandTypes(t *testing.T) {
	cases := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{name: "plain string", t: typ.String, want: true},
		{name: "plain number", t: typ.Number, want: false},
		{name: "integer subtype", t: typ.Integer, want: false},
		{name: "partly numeric union", t: typeexpr.Union(typ.Number, typ.String), want: false},
		{name: "non numeric union", t: typeexpr.Union(typ.String, typ.Boolean), want: true},
		{name: "alias to number", t: typ.NewAlias("Count", typ.Number), want: false},
		{name: "nil is non numeric", t: typ.Nil, want: true},
		{name: "any stays inconclusive", t: typ.Any, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NumericForDefinitelyNotNumber(tc.t); got != tc.want {
				t.Fatalf("NumericForDefinitelyNotNumber(%v) = %v, want %v", tc.t, got, tc.want)
			}
		})
	}
}

func TestCallArgumentExpectedTypeHasObjectEntries(t *testing.T) {
	reader := typ.NewInterface("Reader", []typ.Method{
		{Name: "read", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	if CallArgumentExpectedTypeHasObjectEntries(reader) {
		t.Fatal("interface expected type must keep the whole object literal as the mismatch subject")
	}
	if !CallArgumentExpectedTypeHasObjectEntries(typetable.NewRecord().Field("name", typ.String).Build()) {
		t.Fatal("record expected type should allow object-entry mismatch subjects")
	}
	if !CallArgumentExpectedTypeHasObjectEntries(typeexpr.Optional(typ.NewArray(typ.String))) {
		t.Fatal("optional array expected type should allow object-entry mismatch subjects")
	}
	if !CallArgumentExpectedTypeHasObjectEntries(typeexpr.Union(reader, typetable.NewMap(typ.String, typ.Number))) {
		t.Fatal("union with an object-entry arm should allow object-entry mismatch subjects")
	}
}

func TestNonNilAssertionOperandNilOnlyClassifiesSolvedOperandTypes(t *testing.T) {
	cases := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{name: "plain nil", t: typ.Nil, want: true},
		{name: "optional string", t: typeexpr.Optional(typ.String), want: false},
		{name: "plain string", t: typ.String, want: false},
		{name: "any", t: typ.Any, want: false},
		{name: "unknown", t: typ.Unknown, want: false},
		{name: "never", t: typ.Never, want: false},
		{name: "alias to nil remains inconclusive", t: typ.NewAlias("NilAlias", typ.Nil), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NonNilAssertionOperandNilOnly(tc.t); got != tc.want {
				t.Fatalf("NonNilAssertionOperandNilOnly(%v) = %v, want %v", tc.t, got, tc.want)
			}
		})
	}
}

func TestObligationTypeContainsFreeTypeParamSeesOptionalValueArm(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	cases := []struct {
		name string
		in   typ.Type
		want bool
	}{
		{name: "closed", in: typ.String, want: false},
		{name: "free", in: tp, want: true},
		{name: "optional free", in: typeexpr.Optional(tp), want: true},
		{name: "optional closed", in: typeexpr.Optional(typ.String), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ObligationTypeContainsFreeTypeParam(tc.in); got != tc.want {
				t.Fatalf("ObligationTypeContainsFreeTypeParam(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestReturnEffectiveActualTypeAndProofPolicy(t *testing.T) {
	untrusted := Return{
		TypeWithPresence:   typ.Unknown,
		Expected:           typ.String,
		UntrustedTopOrigin: true,
		Check:              ReturnCheck{},
	}
	if !typ.TypeEquals(untrusted.EffectiveActualType(), typ.Any) {
		t.Fatalf("untrusted effective actual = %v, want any", untrusted.EffectiveActualType())
	}
	if untrusted.MissingProofRefuted() {
		t.Fatal("untrusted missing proof was refuted; want precision boundary")
	}

	concrete := Return{
		TypeWithPresence: typ.Number,
		Expected:         typ.String,
		Check: ReturnCheck{
			ProvenMismatch: true,
		},
	}
	if !concrete.ActualTypeKnown() {
		t.Fatal("concrete return actual type was not marked known")
	}
	if concrete.HasUnownedTopActual() {
		t.Fatal("concrete return was classified as unowned top")
	}
	if !typ.TypeEquals(concrete.EffectiveActualType(), typ.Number) {
		t.Fatalf("concrete effective actual = %v, want number", concrete.EffectiveActualType())
	}
	if !concrete.MissingProofRefuted() {
		t.Fatal("concrete mismatch did not refute missing proof")
	}

	implicitUnknown := Return{TypeWithPresence: typ.Unknown}
	if !implicitUnknown.HasUnownedTopActual() {
		t.Fatal("implicit unknown return was not classified as unowned top")
	}

	explicitAny := Return{TypeWithPresence: typ.Any, ExplicitTopOrigin: true}
	if explicitAny.HasUnownedTopActual() {
		t.Fatal("explicit any return was classified as unowned top")
	}

	cascade := Return{
		TypeWithPresence:    typ.Any,
		ExplicitTopOrigin:   true,
		BodyParamObligation: true,
	}
	if !cascade.BodyParamObligationCascade() {
		t.Fatal("body-owned param obligation top return was not classified as a cascade")
	}

	concreteCascade := Return{
		TypeWithPresence:    typ.String,
		ExplicitTopOrigin:   true,
		BodyParamObligation: true,
	}
	if concreteCascade.BodyParamObligationCascade() {
		t.Fatal("concrete body-owned return mismatch was classified as a top cascade")
	}
}

func TestAssignmentEffectiveActualTypeAndProofPolicy(t *testing.T) {
	untrusted := Assignment{
		TypeWithPresence:   typ.Unknown,
		Expected:           typ.String,
		UntrustedTopOrigin: true,
		Check:              AssignmentCheck{},
	}
	if !typ.TypeEquals(untrusted.EffectiveActualType(), typ.Any) {
		t.Fatalf("untrusted effective actual = %v, want any", untrusted.EffectiveActualType())
	}
	if untrusted.MissingProofRefuted() {
		t.Fatal("untrusted missing proof was refuted; want precision boundary")
	}

	concrete := Assignment{
		TypeWithPresence: typ.Number,
		Expected:         typ.String,
		Check: AssignmentCheck{
			ProvenMismatch: true,
		},
	}
	if !concrete.ActualTypeKnown() {
		t.Fatal("concrete assignment actual type was not marked known")
	}
	if !typ.TypeEquals(concrete.EffectiveActualType(), typ.Number) {
		t.Fatalf("concrete effective actual = %v, want number", concrete.EffectiveActualType())
	}
	if !concrete.MissingProofRefuted() {
		t.Fatal("concrete mismatch did not refute missing proof")
	}

	missing := Assignment{}
	if !typ.TypeEquals(missing.EffectiveActualType(), typ.Unknown) {
		t.Fatalf("missing effective actual = %v, want unknown", missing.EffectiveActualType())
	}

	explicitAny := Assignment{
		TypeWithPresence:  typ.Any,
		Expected:          typ.String,
		ExplicitTopOrigin: true,
		Check: AssignmentCheck{
			ProvenMismatch: true,
		},
	}
	if !typ.TypeEquals(explicitAny.EffectiveActualType(), typ.Any) {
		t.Fatalf("explicit any effective actual = %v, want any", explicitAny.EffectiveActualType())
	}

	untrustedRecord := Assignment{
		TypeWithPresence:   typetable.NewRecord().Field("owner", typ.LiteralString("ops")).Field("retry", typ.LiteralInt(3)).Build(),
		Expected:           typetable.NewMap(typ.String, typ.String),
		UntrustedTopOrigin: true,
	}
	if !typ.TypeEquals(untrustedRecord.EffectiveActualType(), untrustedRecord.TypeWithPresence) {
		t.Fatalf("untrusted structural actual = %v, want %v", untrustedRecord.EffectiveActualType(), untrustedRecord.TypeWithPresence)
	}
	if untrustedRecord.MissingProofRefuted() {
		t.Fatal("untrusted structural assignment refuted missing proof; want precision boundary")
	}
}

func TestCallArgumentCheckActualTypeAndProofPolicy(t *testing.T) {
	unknown := CallArgumentCheck{}
	if unknown.ActualTypeKnown() {
		t.Fatal("empty call argument check actual type was marked known")
	}
	if unknown.MissingProofRefuted() {
		t.Fatal("empty call argument check refuted missing proof")
	}

	concrete := CallArgumentCheck{
		Argument: CallArgument{
			TypeWithPresence: typ.String,
		},
		ProvenMismatch: true,
	}
	if !concrete.ActualTypeKnown() {
		t.Fatal("concrete call argument actual type was not marked known")
	}
	if !concrete.MissingProofRefuted() {
		t.Fatal("concrete call argument mismatch did not refute missing proof")
	}

	untrustedRecord := CallArgumentCheck{
		Argument: CallArgument{
			TypeWithPresence:   typetable.NewRecord().Field("id", typ.LiteralString("ok")).Build(),
			UntrustedTopOrigin: true,
		},
		Expected: typetable.NewRecord().Field("id", typ.String).Build(),
	}
	wantRecord := typetable.NewRecord().Field("id", typ.LiteralString("ok")).Build()
	if !typ.TypeEquals(untrustedRecord.EffectiveActualType(), wantRecord) {
		t.Fatalf("untrusted record effective actual = %v, want structural candidate %v", untrustedRecord.EffectiveActualType(), wantRecord)
	}
	if untrustedRecord.MissingProofRefuted() {
		t.Fatal("untrusted record call argument refuted missing proof; want precision boundary")
	}

	untrustedNil := CallArgumentCheck{
		Argument: CallArgument{
			TypeWithPresence:   typ.Nil,
			UntrustedTopOrigin: true,
		},
		Expected: typ.String,
	}
	if !typ.TypeEquals(untrustedNil.EffectiveActualType(), typ.Any) {
		t.Fatalf("untrusted nil effective actual = %v, want any", untrustedNil.EffectiveActualType())
	}
}

func TestPlanAssignmentCheckRejectsMissingRequiredFieldBeforeValueProof(t *testing.T) {
	expected := typetable.NewRecord().Field("name", typ.String).Build()
	check := PlanAssignmentCheck(AssignmentCheckPlan{
		Assignment: Assignment{
			TypeWithPresence: typetable.NewRecord().Build(),
			Expected:         expected,
		},
		ValueAdmissible:      true,
		MissingRequiredField: "name",
		IsSubtype:            func(typ.Type, typ.Type) bool { return true },
	})
	if check.Admissible {
		t.Fatal("missing required field was treated as admissible")
	}
	if check.Mismatch.Kind != AssignmentMismatchMissingRequiredField || check.Mismatch.Field != "name" {
		t.Fatalf("mismatch = %#v, want missing required field name", check.Mismatch)
	}
}

func TestPlanReturnCheckRejectsMissingRequiredFieldBeforeValueProof(t *testing.T) {
	expected := typetable.NewRecord().Field("name", typ.String).Build()
	check := PlanReturnCheck(ReturnCheckPlan{
		Return: Return{
			TypeWithPresence: typetable.NewRecord().Build(),
			Expected:         expected,
		},
		ValueAdmissible:          true,
		MissingRequiredField:     "name",
		MissingRequiredFieldType: typ.String,
		IsSubtype:                func(typ.Type, typ.Type) bool { return true },
	})
	if check.Admissible {
		t.Fatal("missing required return field was treated as admissible")
	}
	if !check.ProvenMismatch {
		t.Fatal("missing required return field was not a proven mismatch")
	}
	if check.Mismatch.Kind != ReturnMismatchMissingRequiredField || check.Mismatch.Field != "name" {
		t.Fatalf("mismatch = %#v, want missing required field name", check.Mismatch)
	}
}

func TestPlanReturnCheckClassifiesMayBeNilMismatch(t *testing.T) {
	check := PlanReturnCheck(ReturnCheckPlan{
		Return: Return{
			TypeWithPresence: typeexpr.Optional(typ.String),
			Expected:         typ.String,
		},
		IsSubtype: func(sub, super typ.Type) bool {
			return false
		},
	})
	if check.Mismatch.Kind != ReturnMismatchMayBeNil {
		t.Fatalf("mismatch = %#v, want may-be-nil", check.Mismatch)
	}
	if !check.ProvenMismatch {
		t.Fatal("may-be-nil return mismatch was not marked as proven mismatch")
	}
}

func TestPlanAssignmentCheckClassifiesMayBeNilMismatch(t *testing.T) {
	check := PlanAssignmentCheck(AssignmentCheckPlan{
		Assignment: Assignment{
			TypeWithPresence: typeexpr.Optional(typ.String),
			Expected:         typ.String,
		},
		IsSubtype: func(sub, super typ.Type) bool {
			return false
		},
	})
	if check.Mismatch.Kind != AssignmentMismatchMayBeNil {
		t.Fatalf("mismatch = %#v, want may-be-nil", check.Mismatch)
	}
	if !check.ProvenMismatch {
		t.Fatal("may-be-nil concrete mismatch was not marked as proven mismatch")
	}
}

func TestPlanAssignmentCheckMarksConcreteSubtypeFailureAsMismatch(t *testing.T) {
	check := PlanAssignmentCheck(AssignmentCheckPlan{
		Assignment: Assignment{
			TypeWithPresence: typ.String,
			Expected:         typ.Number,
		},
		IsSubtype: func(sub, super typ.Type) bool {
			return false
		},
	})
	if check.Admissible {
		t.Fatal("concrete subtype failure was treated as admissible")
	}
	if !check.ProvenMismatch {
		t.Fatal("concrete subtype failure was not marked as proven mismatch")
	}
}

func TestPlanCallArgumentReportsOwnsPrecedence(t *testing.T) {
	args := []CallArgument{
		{Index: 0, Span: SourceSpan{StartLine: 10}},
		{Index: 1, Span: SourceSpan{StartLine: 11}},
		{Index: 2, Span: SourceSpan{StartLine: 12}},
		{Index: 3, Span: SourceSpan{StartLine: 13}},
		{Index: 4, Span: SourceSpan{StartLine: 14}},
	}
	checks := map[int]bool{
		1: false,
		2: false,
		3: true,
		4: true,
	}
	reports := PlanCallArgumentReports(CallArgumentReportPlan{
		Args: args,
		GenericConflicts: []CallGenericInferenceConflict{
			{
				Index:         0,
				ParamName:     "T",
				Span:          SourceSpan{StartLine: 20},
				Contributions: []CallGenericInferenceContribution{{Type: typ.String}, {Type: typ.Number}},
			},
		},
		GenericConstraints: []IndexedCallArgumentObligation{
			{
				Index:      0,
				Obligation: CallArgumentObligation{Type: typ.Number, ExpectedLabel: "constraint should be hidden"},
			},
			{
				Index:      1,
				Obligation: CallArgumentObligation{Type: typ.Number, ExpectedLabel: "generic constraint"},
			},
			{
				Index:      4,
				Obligation: CallArgumentObligation{Type: typ.Number, ExpectedLabel: "admissible constraint"},
			},
		},
		ExplicitParams: []IndexedCallArgumentObligation{
			{
				Index:      1,
				Obligation: CallArgumentObligation{Type: typ.String, ExpectedLabel: "explicit should be hidden"},
			},
			{
				Index:      2,
				Obligation: CallArgumentObligation{Type: typ.String, ExpectedLabel: "explicit param"},
			},
			{
				Index:      3,
				Obligation: CallArgumentObligation{Type: typ.String, ExpectedLabel: "admissible param"},
			},
			{
				Index:      4,
				Obligation: CallArgumentObligation{Type: typ.String, ExpectedLabel: "reserved by admissible constraint"},
			},
		},
		OutcomeParams: []IndexedCallArgumentObligation{
			{
				Index:      2,
				Obligation: CallArgumentObligation{Type: typ.Boolean, ExpectedLabel: "outcome should be hidden"},
			},
		},
		Check: func(arg CallArgument, obligation CallArgumentObligation) CallArgumentCheck {
			return CallArgumentCheck{
				Argument:   arg,
				Expected:   obligation.Type,
				Admissible: checks[arg.Index],
			}
		},
	})
	if len(reports) != 3 {
		t.Fatalf("PlanCallArgumentReports produced %d reports, want 3", len(reports))
	}
	if reports[0].Kind != CallArgumentReportGenericConflict || reports[0].Argument.Index != 0 {
		t.Fatalf("report[0] = %#v, want generic conflict for arg 0", reports[0])
	}
	if reports[1].Kind != CallArgumentReportObligation || reports[1].Argument.Index != 1 || reports[1].Obligation.ExpectedLabel != "generic constraint" {
		t.Fatalf("report[1] = %#v, want generic constraint for arg 1", reports[1])
	}
	if reports[2].Kind != CallArgumentReportObligation || reports[2].Argument.Index != 2 || reports[2].Obligation.ExpectedLabel != "explicit param" {
		t.Fatalf("report[2] = %#v, want explicit param for arg 2", reports[2])
	}
}

func TestPlanCallArgumentReportsSkipsInvalidInputs(t *testing.T) {
	reports := PlanCallArgumentReports(CallArgumentReportPlan{
		Args: []CallArgument{{Index: 1}},
		GenericConflicts: []CallGenericInferenceConflict{
			{Index: 0, Contributions: []CallGenericInferenceContribution{{Type: typ.String}}},
		},
		GenericConstraints: []IndexedCallArgumentObligation{
			{Index: 1, Obligation: CallArgumentObligation{}},
			{Index: 2, Obligation: CallArgumentObligation{Type: typ.String}},
		},
		ExplicitParams: []IndexedCallArgumentObligation{
			{Index: 3, Obligation: CallArgumentObligation{Type: typ.String}},
		},
		Check: func(arg CallArgument, obligation CallArgumentObligation) CallArgumentCheck {
			t.Fatalf("Check should not be called for invalid inputs")
			return CallArgumentCheck{}
		},
	})
	if len(reports) != 0 {
		t.Fatalf("PlanCallArgumentReports produced %#v, want none", reports)
	}
}

func TestPlanCallArgumentReportsSkipsGradualExpectedTypesWithoutReservingIndex(t *testing.T) {
	var checked []typ.Type
	reports := PlanCallArgumentReports(CallArgumentReportPlan{
		Args: []CallArgument{{Index: 0}},
		GenericConstraints: []IndexedCallArgumentObligation{
			{Index: 0, Obligation: CallArgumentObligation{Type: typ.Any, ExpectedLabel: "any constraint"}},
		},
		ExplicitParams: []IndexedCallArgumentObligation{
			{Index: 0, Obligation: CallArgumentObligation{Type: typ.Unknown, ExpectedLabel: "unknown param"}},
		},
		OutcomeParams: []IndexedCallArgumentObligation{
			{Index: 0, Obligation: CallArgumentObligation{Type: typ.String, ExpectedLabel: "precise outcome"}},
		},
		Check: func(arg CallArgument, obligation CallArgumentObligation) CallArgumentCheck {
			checked = append(checked, obligation.Type)
			return CallArgumentCheck{
				Argument: arg,
				Expected: obligation.Type,
			}
		},
	})
	if len(reports) != 1 {
		t.Fatalf("PlanCallArgumentReports produced %#v, want one precise outcome report", reports)
	}
	if reports[0].Obligation.ExpectedLabel != "precise outcome" {
		t.Fatalf("report = %#v, want skipped gradual obligations to leave index available", reports[0])
	}
	if len(checked) != 1 || !typ.TypeEquals(checked[0], typ.String) {
		t.Fatalf("checked obligations = %#v, want only precise string obligation", checked)
	}
}

func TestPlanCallArgumentReportsKeepsStricterOutcomeAfterAdmissibleExplicitParam(t *testing.T) {
	reports := PlanCallArgumentReports(CallArgumentReportPlan{
		Args: []CallArgument{{Index: 0, TypeWithPresence: typ.LiteralString("not-number")}},
		ExplicitParams: []IndexedCallArgumentObligation{
			{Index: 0, Obligation: CallArgumentObligation{Type: typ.MaterializeUnion([]typ.Type{typ.Number, typ.String}), ExpectedLabel: "declared union"}},
		},
		OutcomeParams: []IndexedCallArgumentObligation{
			{Index: 0, Obligation: CallArgumentObligation{Type: typ.Number, ExpectedLabel: "callee-use obligation"}},
		},
		Check: func(arg CallArgument, obligation CallArgumentObligation) CallArgumentCheck {
			return CallArgumentCheck{
				Argument:   arg,
				Expected:   obligation.Type,
				Admissible: obligation.ExpectedLabel == "declared union",
			}
		},
	})
	if len(reports) != 1 {
		t.Fatalf("PlanCallArgumentReports produced %#v, want stricter outcome report", reports)
	}
	if reports[0].Obligation.ExpectedLabel != "callee-use obligation" {
		t.Fatalf("report = %#v, want outcome obligation after admissible explicit param", reports[0])
	}
}

func TestPlanCallArgumentReportsSkipsSignatureSurfaceAfterAdmissibleExplicitParam(t *testing.T) {
	reports := PlanCallArgumentReports(CallArgumentReportPlan{
		Args: []CallArgument{{Index: 0, TypeWithPresence: typeexpr.Optional(typ.String)}},
		ExplicitParams: []IndexedCallArgumentObligation{
			{Index: 0, Obligation: CallArgumentObligation{Type: typeexpr.Optional(typ.String), ExpectedLabel: "declared optional"}},
		},
		OutcomeParams: []IndexedCallArgumentObligation{
			{Index: 0, Obligation: CallArgumentObligation{Type: typ.String, ExpectedLabel: "narrowed signature view", SignatureSurface: true}},
		},
		Check: func(arg CallArgument, obligation CallArgumentObligation) CallArgumentCheck {
			return CallArgumentCheck{
				Argument:   arg,
				Expected:   obligation.Type,
				Admissible: obligation.ExpectedLabel == "declared optional",
			}
		},
	})
	if len(reports) != 0 {
		t.Fatalf("PlanCallArgumentReports produced %#v, want signature-surface cascade suppressed", reports)
	}
}

func TestCallArgumentObligationTypeReportableTreatsOptionalGradualAsInternal(t *testing.T) {
	if CallArgumentObligationTypeReportable(typ.MaterializeOptional(typ.Any)) {
		t.Fatal("any? should be an internal gradual obligation, not a reportable contract")
	}
	if CallArgumentObligationTypeReportable(typ.MaterializeOptional(typ.Unknown)) {
		t.Fatal("unknown? should be an internal gradual obligation, not a reportable contract")
	}
	recordWithAnyField := typetable.NewRecord().Field("id", typ.Any).Build()
	if !CallArgumentObligationTypeReportable(recordWithAnyField) {
		t.Fatal("structured contracts containing any should remain reportable for their shape")
	}
}

func TestObligationTypeReportableTreatsTopLikeTypesAsInternal(t *testing.T) {
	if !ObligationTypeReportable(typ.String) {
		t.Fatal("concrete obligation type should be reportable")
	}
	if ObligationTypeReportable(typ.Any) || ObligationTypeReportable(typ.Unknown) {
		t.Fatal("top-like obligation types should stay internal")
	}
	if ObligationTypeReportable(typ.NewTypeParam("T", nil)) {
		t.Fatal("free type-parameter obligation type should stay internal")
	}
}

func TestPlanGenericInferenceContributionSpan(t *testing.T) {
	fallback := SourceSpan{StartLine: 1}
	shallow := SourceSpan{StartLine: 2}
	deep := SourceSpan{StartLine: 3}
	got := PlanGenericInferenceContributionSpan(GenericInferenceContributionSpanPlan{
		Fallback: fallback,
		Candidates: []GenericInferenceContributionSpanCandidate{
			{Span: shallow, SegmentDepth: 1, Matches: true},
			{Span: SourceSpan{StartLine: 9}, SegmentDepth: 4, Matches: false},
			{Span: deep, SegmentDepth: 2, Matches: true},
		},
	})
	if got != deep {
		t.Fatalf("PlanGenericInferenceContributionSpan = %#v, want deepest matching span %#v", got, deep)
	}
	if got := PlanGenericInferenceContributionSpan(GenericInferenceContributionSpanPlan{
		Fallback:   fallback,
		Candidates: []GenericInferenceContributionSpanCandidate{{Span: shallow, SegmentDepth: 1}},
	}); got != fallback {
		t.Fatalf("PlanGenericInferenceContributionSpan(no match) = %#v, want fallback %#v", got, fallback)
	}
}

func TestPlanCallArityReport(t *testing.T) {
	tests := []struct {
		name string
		plan CallArityReportPlan
		want CallArityReport
	}{
		{
			name: "no contract",
			plan: CallArityReportPlan{HasContract: false, ActualCount: 0, RequiredCount: 1},
			want: CallArityReport{},
		},
		{
			name: "too few",
			plan: CallArityReportPlan{
				HasContract:    true,
				CallableName:   "send",
				ActualCount:    1,
				RequiredCount:  2,
				FixedCount:     2,
				CallSpan:       SourceSpan{StartLine: 10},
				ParameterSpans: []SourceSpan{{StartLine: 19}, {StartLine: 20}},
			},
			want: CallArityReport{
				Kind:            CallArityReportTooFew,
				CallableName:    "send",
				ExpectedCount:   2,
				ActualCount:     1,
				CallSpan:        SourceSpan{StartLine: 10},
				DeclarationSpan: SourceSpan{StartLine: 20},
			},
		},
		{
			name: "too many fixed args",
			plan: CallArityReportPlan{
				HasContract:    true,
				CallableName:   "send",
				ActualCount:    3,
				RequiredCount:  1,
				FixedCount:     2,
				HasVararg:      false,
				CallSpan:       SourceSpan{StartLine: 10},
				ParameterSpans: []SourceSpan{{StartLine: 20}, {StartLine: 21}},
				ArgumentSpans:  []SourceSpan{{StartLine: 10}, {StartLine: 11}, {StartLine: 12}},
			},
			want: CallArityReport{
				Kind:            CallArityReportTooMany,
				CallableName:    "send",
				ExpectedCount:   2,
				ActualCount:     3,
				CallSpan:        SourceSpan{StartLine: 10},
				DeclarationSpan: SourceSpan{StartLine: 21},
				ExtraSpan:       SourceSpan{StartLine: 12},
			},
		},
		{
			name: "vararg accepts extra args",
			plan: CallArityReportPlan{HasContract: true, ActualCount: 3, RequiredCount: 1, FixedCount: 2, HasVararg: true},
			want: CallArityReport{},
		},
		{
			name: "valid arity",
			plan: CallArityReportPlan{HasContract: true, ActualCount: 2, RequiredCount: 1, FixedCount: 2},
			want: CallArityReport{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PlanCallArityReport(tt.plan); got != tt.want {
				t.Fatalf("PlanCallArityReport = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPlanCallCalleeReport(t *testing.T) {
	tests := []struct {
		name string
		plan CallCalleeReportPlan
		want CallCalleeReport
	}{
		{
			name: "any is gradual not a callee report",
			plan: CallCalleeReportPlan{CallableName: "target", Type: typ.Any},
			want: CallCalleeReport{},
		},
		{
			name: "any member access needs callable proof",
			plan: CallCalleeReportPlan{CallableName: "target.member", Type: typ.Any, MemberAccess: true, ImpreciseMemberRequiresProof: true},
			want: CallCalleeReport{
				Kind:         CallCalleeReportNotCallable,
				CallableName: "target.member",
				Type:         typ.Any,
				MemberAccess: true,
			},
		},
		{
			name: "any member namespace without receiver proof stays gradual",
			plan: CallCalleeReportPlan{CallableName: "target.member", Type: typ.Any, MemberAccess: true},
			want: CallCalleeReport{},
		},
		{
			name: "unknown is not a callee report",
			plan: CallCalleeReportPlan{CallableName: "target", Type: typ.Unknown},
			want: CallCalleeReport{},
		},
		{
			name: "unknown member access needs callable proof",
			plan: CallCalleeReportPlan{CallableName: "target.member", Type: typ.Unknown, MemberAccess: true, ImpreciseMemberRequiresProof: true},
			want: CallCalleeReport{
				Kind:         CallCalleeReportNotCallable,
				CallableName: "target.member",
				Type:         typ.Unknown,
				MemberAccess: true,
			},
		},
		{
			name: "never is not a callee report",
			plan: CallCalleeReportPlan{CallableName: "target", Type: typ.Never},
			want: CallCalleeReport{},
		},
		{
			name: "callable type is clean",
			plan: CallCalleeReportPlan{CallableName: "target", Type: typ.Func().Build(), Callable: true},
			want: CallCalleeReport{},
		},
		{
			name: "non-callable type",
			plan: CallCalleeReportPlan{
				CallableName: "x",
				Type:         typ.Number,
				Span:         SourceSpan{StartLine: 10},
			},
			want: CallCalleeReport{
				Kind:         CallCalleeReportNotCallable,
				CallableName: "x",
				Type:         typ.Number,
				Span:         SourceSpan{StartLine: 10},
			},
		},
		{
			name: "unnamed empty callee span falls back to call target and call span",
			plan: CallCalleeReportPlan{
				Type:     typ.Number,
				CallSpan: SourceSpan{StartLine: 30},
			},
			want: CallCalleeReport{
				Kind:         CallCalleeReportNotCallable,
				CallableName: "call target",
				Type:         typ.Number,
				Span:         SourceSpan{StartLine: 30},
			},
		},
		{
			name: "optional callable may be nil",
			plan: CallCalleeReportPlan{
				CallableName: "maybe",
				Type:         typ.MaterializeOptional(typ.Func().Build()),
				Callable:     true,
				Span:         SourceSpan{StartLine: 11},
			},
			want: CallCalleeReport{
				Kind:         CallCalleeReportMayBeNil,
				CallableName: "maybe",
				Type:         typ.MaterializeOptional(typ.Func().Build()),
				Callable:     true,
				Span:         SourceSpan{StartLine: 11},
			},
		},
		{
			name: "optional non-callable member reports not callable",
			plan: CallCalleeReportPlan{
				CallableName: "pkg.run",
				Type:         typ.MaterializeOptional(typ.Number),
				MemberAccess: true,
				Span:         SourceSpan{StartLine: 12},
			},
			want: CallCalleeReport{
				Kind:         CallCalleeReportNotCallable,
				CallableName: "pkg.run",
				Type:         typ.MaterializeOptional(typ.Number),
				MemberAccess: true,
				Span:         SourceSpan{StartLine: 12},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanCallCalleeReport(tt.plan)
			if got.Kind != tt.want.Kind ||
				got.CallableName != tt.want.CallableName ||
				got.Callable != tt.want.Callable ||
				got.Span != tt.want.Span ||
				!typ.TypeEquals(got.Type, tt.want.Type) {
				t.Fatalf("PlanCallCalleeReport = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCallArgumentMemberLabelFormatsNestedSubject(t *testing.T) {
	segs := []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
		{Kind: segment.SegmentIndexString, Name: "name"},
		{Kind: segment.SegmentIndexInt, Index: 2},
	}
	got := CallArgumentMemberLabel(1, segs, "request.payload.name[2]")
	want := "argument 2.payload.name[2] (request.payload.name[2])"
	if got != want {
		t.Fatalf("CallArgumentMemberLabel = %q, want %q", got, want)
	}
}

func TestCallArgumentExpectedLabelSuffixFormatsSegments(t *testing.T) {
	segs := []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
		{Kind: segment.SegmentIndexInt, Index: 3},
	}
	if got := CallArgumentExpectedLabelSuffix(segs); got != ".payload[3]" {
		t.Fatalf("CallArgumentExpectedLabelSuffix = %q", got)
	}
	if got := ExpectedLabelWithSuffix("handler parameter 1", ".payload[3]"); got != "handler parameter 1.payload[3]" {
		t.Fatalf("ExpectedLabelWithSuffix = %q", got)
	}
}

func TestCallArgumentLabelsRejectUnsupportedSegments(t *testing.T) {
	segs := []segment.Segment{{Kind: segment.SegmentKind(99)}}
	if got := CallArgumentMemberLabel(0, segs, ""); got != "" {
		t.Fatalf("CallArgumentMemberLabel(dynamic) = %q, want empty", got)
	}
	if got := CallArgumentExpectedLabelSuffix(segs); got != "" {
		t.Fatalf("CallArgumentExpectedLabelSuffix(dynamic) = %q, want empty", got)
	}
}

func TestCallArgumentMayBeNilMismatch(t *testing.T) {
	if !CallArgumentMayBeNilMismatch(typ.MaterializeOptional(typ.String), typ.String) {
		t.Fatal("optional string into string should be a may-nil mismatch")
	}
	if CallArgumentMayBeNilMismatch(typ.String, typ.String) {
		t.Fatal("string into string should not be a may-nil mismatch")
	}
	if CallArgumentMayBeNilMismatch(typ.MaterializeOptional(typ.String), typ.MaterializeOptional(typ.String)) {
		t.Fatal("optional string into optional string should not be a may-nil mismatch")
	}
	if CallArgumentMayBeNilMismatch(nil, typ.String) {
		t.Fatal("nil actual type should not be a may-nil mismatch")
	}
	if CallArgumentMayBeNilMismatch(typ.String, nil) {
		t.Fatal("nil expected type should not be a may-nil mismatch")
	}
}

func TestPlanCallArgumentMismatchSubjectSelectsFirstFailingMember(t *testing.T) {
	original := CallArgument{Index: 0, Label: "payload"}
	first := CallArgument{Index: 0, Label: "payload.ok"}
	second := CallArgument{Index: 0, Label: "payload.bad"}
	got, ok := PlanCallArgumentMismatchSubject(CallArgumentMismatchSubjectPlan{
		Argument: original,
		Expected: typ.String,
		Candidates: []CallArgumentMismatchCandidate{
			{Argument: first, Expected: typ.Number, LabelSuffix: ".ok", Admissible: true},
			{Argument: second, Expected: typ.Boolean, LabelSuffix: ".bad", Admissible: false},
		},
	})
	if !ok {
		t.Fatal("PlanCallArgumentMismatchSubject returned false, want selected member")
	}
	if got.Argument.Label != "payload.bad" || got.Expected != typ.Boolean || got.LabelSuffix != ".bad" {
		t.Fatalf("selected subject = %#v, want second failing member", got)
	}
}

func TestPlanCallArgumentMismatchSubjectSelectsMissingField(t *testing.T) {
	original := CallArgument{Index: 1, Label: "record"}
	got, ok := PlanCallArgumentMismatchSubject(CallArgumentMismatchSubjectPlan{
		Argument:             original,
		Expected:             typ.String,
		MissingRequiredField: "name",
	})
	if !ok {
		t.Fatal("PlanCallArgumentMismatchSubject returned false, want missing field")
	}
	if got.Argument.Label != "record" ||
		got.Argument.Mismatch.Kind != CallArgumentMismatchMissingRequiredField ||
		got.Argument.Mismatch.Field != "name" ||
		got.Expected != typ.String ||
		got.LabelSuffix != ".name" {
		t.Fatalf("missing-field subject = %#v, want field name mismatch", got)
	}
}

func TestPlanCallArgumentMismatchSubjectNoSelection(t *testing.T) {
	_, ok := PlanCallArgumentMismatchSubject(CallArgumentMismatchSubjectPlan{
		Argument: CallArgument{Index: 0},
		Expected: typ.String,
		Candidates: []CallArgumentMismatchCandidate{
			{Argument: CallArgument{Index: 0}, Expected: typ.Number, LabelSuffix: ".ok", Admissible: true},
		},
	})
	if ok {
		t.Fatal("PlanCallArgumentMismatchSubject returned true for admissible-only candidates")
	}
}

func TestCallArgumentObligationTypeReportable(t *testing.T) {
	if !CallArgumentObligationTypeReportable(typ.String) {
		t.Fatal("concrete string obligation type should be reportable")
	}
	if CallArgumentObligationTypeReportable(nil) {
		t.Fatal("nil obligation type should not be reportable")
	}
	if CallArgumentObligationTypeReportable(typ.Any) {
		t.Fatal("any obligation type should not be reportable")
	}
	if CallArgumentObligationTypeReportable(typ.Unknown) {
		t.Fatal("unknown obligation type should not be reportable")
	}
	if CallArgumentObligationTypeReportable(typ.NewTypeParam("T", nil)) {
		t.Fatal("free type-parameter obligation type should not be reportable")
	}
}

func TestCallArgumentProofAdmissible(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	arg := CallArgument{FunctionType: fn}

	if !CallArgumentProofAdmissible(CallArgumentProofPlan{
		Argument:        CallArgument{},
		ValueAdmissible: true,
	}) {
		t.Fatal("value proof admissibility should make the argument admissible")
	}
	if !CallArgumentProofAdmissible(CallArgumentProofPlan{
		Argument:               arg,
		FunctionTypeAdmissible: true,
	}) {
		t.Fatal("contextual function type should make the argument admissible when it is a subtype")
	}
	if CallArgumentProofAdmissible(CallArgumentProofPlan{
		Argument: arg,
	}) {
		t.Fatal("contextual function type should not be admissible against an unrelated expected type")
	}
}

func TestCallArgumentWitnessProvenMismatch(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	arg := CallArgument{FunctionType: fn}

	if !CallArgumentWitnessProvenMismatch(CallArgumentProofPlan{
		Argument:            CallArgument{},
		ValueProvenMismatch: true,
	}) {
		t.Fatal("value witness mismatch should make the argument a proven mismatch")
	}
	if !CallArgumentWitnessProvenMismatch(CallArgumentProofPlan{
		Argument:                   arg,
		FunctionTypeProvenMismatch: true,
	}) {
		t.Fatal("contextual function type should be a proven mismatch when it rejects the expected type")
	}
	if CallArgumentWitnessProvenMismatch(CallArgumentProofPlan{
		Argument: arg,
	}) {
		t.Fatal("contextual function type should not be a mismatch when it satisfies the expected type")
	}
	if CallArgumentWitnessProvenMismatch(CallArgumentProofPlan{
		Argument: arg,
	}) {
		t.Fatal("any expected type should not produce a contextual function mismatch")
	}
	if CallArgumentWitnessProvenMismatch(CallArgumentProofPlan{
		Argument: arg,
	}) {
		t.Fatal("unknown expected type should not produce a contextual function mismatch")
	}
	if CallArgumentWitnessProvenMismatch(CallArgumentProofPlan{
		Argument: arg,
	}) {
		t.Fatal("nil expected type should not produce a contextual function mismatch")
	}
}

func TestPlanCallArgumentCheckOwnsSubjectAndVerdicts(t *testing.T) {
	original := CallArgument{
		Index:            0,
		TypeWithPresence: typ.MaterializeOptional(typ.String),
		Label:            "payload",
	}
	member := CallArgument{
		Index:            0,
		TypeWithPresence: typ.MaterializeOptional(typ.Number),
		Label:            "payload.count",
	}
	subjectPlan := CallArgumentMismatchSubjectPlan{
		Argument: original,
		Expected: typ.String,
		Candidates: []CallArgumentMismatchCandidate{
			{
				Argument:    member,
				Expected:    typ.Number,
				LabelSuffix: ".count",
				Admissible:  false,
			},
		},
	}

	got := PlanCallArgumentCheck(CallArgumentCheckPlan{
		Argument:            original,
		Expected:            typ.String,
		ExpectedLabel:       "handler parameter 1",
		ValueAdmissible:     false,
		ValueProvenMismatch: true,
		SubjectPlan:         &subjectPlan,
	})

	if got.Argument.Label != "payload.count" || got.Expected != typ.Number || got.ExpectedLabel != "handler parameter 1.count" {
		t.Fatalf("PlanCallArgumentCheck subject = %#v expected=%v label=%q, want nested count subject", got.Argument, got.Expected, got.ExpectedLabel)
	}
	if got.Argument.Mismatch.Kind != CallArgumentMismatchMayBeNil {
		t.Fatalf("PlanCallArgumentCheck mismatch = %#v, want may-nil on selected nested subject", got.Argument.Mismatch)
	}
	if got.Admissible {
		t.Fatal("PlanCallArgumentCheck admissible = true, want false")
	}
	if !got.ProvenMismatch {
		t.Fatal("PlanCallArgumentCheck proven mismatch = false, want true")
	}
}

func TestCallCalleeDeclaredTypeMoreInformative(t *testing.T) {
	if !CallCalleeDeclaredTypeMoreInformative(typ.Nil, typ.MaterializeOptional(typ.Func().Returns(typ.String).Build())) {
		t.Fatal("nil solved callee with optional declared callable should prefer declared type")
	}
	if CallCalleeDeclaredTypeMoreInformative(typ.Nil, typ.String) {
		t.Fatal("nil solved callee with non-optional declared type should not prefer declared type")
	}
	if CallCalleeDeclaredTypeMoreInformative(typ.String, typ.MaterializeOptional(typ.String)) {
		t.Fatal("non-nil solved callee should not prefer declared optional type")
	}
	if CallCalleeDeclaredTypeMoreInformative(typ.Nil, typ.Any) {
		t.Fatal("declared any should not be treated as more informative")
	}
	if CallCalleeDeclaredTypeMoreInformative(typ.Nil, typ.Unknown) {
		t.Fatal("declared unknown should not be treated as more informative")
	}
	if CallCalleeDeclaredTypeMoreInformative(typ.Nil, nil) {
		t.Fatal("nil declared type should not be treated as more informative")
	}
}

func TestCallCalleeDeclaredNilOwnedByDeclaration(t *testing.T) {
	nonNilCallable := typ.Func().Returns(typ.String).Build()
	if !CallCalleeDeclaredNilOwnedByDeclaration(typ.Nil, nonNilCallable) {
		t.Fatal("nil value for a non-nil declared callable should be owned by the declaration")
	}
	if CallCalleeDeclaredNilOwnedByDeclaration(typ.Nil, typ.MaterializeOptional(nonNilCallable)) {
		t.Fatal("optional callable nil should remain a call-site may-nil obligation")
	}
	if CallCalleeDeclaredNilOwnedByDeclaration(typ.Nil, typ.String) {
		t.Fatal("non-callable declarations should not use callable cascade ownership")
	}
	if CallCalleeDeclaredNilOwnedByDeclaration(typ.String, nonNilCallable) {
		t.Fatal("non-nil values should not be declaration-owned nil cascades")
	}
}

func TestCallContractSourceFormatsParameterLabels(t *testing.T) {
	tests := []struct {
		name string
		src  CallContractSource
		want string
	}{
		{
			name: "unknown named source falls back to parameter",
			src:  CallContractSource{Kind: CallContractSourceUnknown, Name: "ignored"},
			want: "parameter 2",
		},
		{
			name: "local function",
			src:  CallContractSource{Kind: CallContractSourceLocalFunction, Name: "validate"},
			want: "validate parameter 2",
		},
		{
			name: "imported signature",
			src:  CallContractSource{Kind: CallContractSourceImportedSignature, Name: "http.post"},
			want: "http.post parameter 2",
		},
		{
			name: "function value",
			src:  CallContractSource{Kind: CallContractSourceFunctionValue, Name: "handler"},
			want: "handler parameter 2",
		},
		{
			name: "member function",
			src:  CallContractSource{Kind: CallContractSourceMemberFunction, Name: "client:send"},
			want: "client:send parameter 2",
		},
		{
			name: "unnamed",
			src:  CallContractSource{Kind: CallContractSourceLocalFunction},
			want: "parameter 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.src.ParameterLabel(1); got != tt.want {
				t.Fatalf("ParameterLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCallContractSourceReturnsParameterSpan(t *testing.T) {
	want := SourceSpan{StartLine: 3, StartCol: 4, EndLine: 3, EndCol: 12}
	src := CallContractSource{ParameterSpans: []SourceSpan{{StartLine: 1}, want}}
	if got := src.ParameterSpan(1); got != want {
		t.Fatalf("ParameterSpan(1) = %#v, want %#v", got, want)
	}
	if got := src.ParameterSpan(-1); got != (SourceSpan{}) {
		t.Fatalf("ParameterSpan(-1) = %#v, want zero", got)
	}
	if got := src.ParameterSpan(2); got != (SourceSpan{}) {
		t.Fatalf("ParameterSpan(2) = %#v, want zero", got)
	}
}
