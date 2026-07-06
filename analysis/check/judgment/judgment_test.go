package judgment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestJoinEvidenceChainsKeepsCommonProof(t *testing.T) {
	proof := Evidence{
		Kind:   EvidenceAbstractFact,
		Trust:  EvidenceTrustProven,
		Origin: OriginRef{Point: cfg.Point(7), Key: "x:string"},
	}

	got := JoinEvidenceChains(EvidenceChain{proof}, EvidenceChain{proof})
	if len(got) != 1 {
		t.Fatalf("joined evidence len = %d, want 1: %#v", len(got), got)
	}
	if got[0] != proof {
		t.Fatalf("joined evidence = %#v, want original proof %#v", got[0], proof)
	}
}

func TestJoinEvidenceChainsKeepsOneSidedOriginAsPrecisionBoundary(t *testing.T) {
	proof := Evidence{
		Kind:   EvidenceAbstractFact,
		Trust:  EvidenceTrustProven,
		Origin: OriginRef{Point: cfg.Point(3), Key: "raw:any"},
	}

	got := JoinEvidenceChains(EvidenceChain{proof}, nil)
	if len(got) != 1 {
		t.Fatalf("joined evidence len = %d, want 1: %#v", len(got), got)
	}
	if got[0].Kind != EvidencePrecisionBoundary ||
		got[0].Trust != EvidenceTrustUnknown ||
		got[0].Origin != proof.Origin {
		t.Fatalf("joined evidence = %#v, want unknown precision boundary at original origin", got[0])
	}
}

func TestJoinEvidenceChainsDegradesConflictingTrust(t *testing.T) {
	origin := OriginRef{Point: cfg.Point(11), Key: "value"}
	left := Evidence{Kind: EvidenceUserAssertion, Trust: EvidenceTrustClaimed, Origin: origin}
	right := Evidence{Kind: EvidenceUserAssertion, Trust: EvidenceTrustRefuted, Origin: origin}

	got := JoinEvidenceChains(EvidenceChain{left}, EvidenceChain{right})
	if len(got) != 1 {
		t.Fatalf("joined evidence len = %d, want 1: %#v", len(got), got)
	}
	if got[0].Kind != EvidencePrecisionBoundary ||
		got[0].Trust != EvidenceTrustUnknown ||
		got[0].Origin != origin {
		t.Fatalf("joined evidence = %#v, want degraded precision boundary", got[0])
	}
}

func TestJoinEvidenceChainsDeterministicOrder(t *testing.T) {
	a := Evidence{Kind: EvidenceMissingProof, Trust: EvidenceTrustUnknown, Origin: OriginRef{Point: cfg.Point(9), Key: "b"}}
	b := Evidence{Kind: EvidenceAbstractFact, Trust: EvidenceTrustProven, Origin: OriginRef{Point: cfg.Point(2), Key: "a"}}

	got := JoinEvidenceChains(EvidenceChain{a}, EvidenceChain{b})
	if len(got) != 2 {
		t.Fatalf("joined evidence len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Origin != b.Origin || got[1].Origin != a.Origin {
		t.Fatalf("joined evidence order = %#v, want point/key order", got)
	}
}

func TestSubjectRefStableKeyIncludesFunctionIdentity(t *testing.T) {
	left := NewSubjectRef("body-a", SubjectCallArgument, "point:4/arg:1")
	right := NewSubjectRef("body-b", SubjectCallArgument, "point:4/arg:1")
	if left.StableKey() == right.StableKey() {
		t.Fatalf("stable keys equal across function identities: %q", left.StableKey())
	}
	if got, want := left.StableKey(), "body-a|call_arg|point:4/arg:1"; got != want {
		t.Fatalf("StableKey = %q, want %q", got, want)
	}
}

func TestJudgmentEvidenceLookupUsesChain(t *testing.T) {
	j := Judgment{Evidence: EvidenceChain{
		{Kind: EvidenceAbstractFact, Trust: EvidenceTrustProven},
		{Kind: EvidenceMissingProof, Trust: EvidenceTrustRefuted},
	}}

	if !j.HasEvidence(EvidenceMissingProof) {
		t.Fatal("HasEvidence(EvidenceMissingProof) = false, want true")
	}
	trust, ok := j.EvidenceTrustFor(EvidenceMissingProof)
	if !ok || trust != EvidenceTrustRefuted {
		t.Fatalf("EvidenceTrustFor(EvidenceMissingProof) = %v, %v; want refuted, true", trust, ok)
	}
	if j.HasEvidence(EvidencePrecisionBoundary) {
		t.Fatal("HasEvidence(EvidencePrecisionBoundary) = true, want false")
	}
	if _, ok := j.EvidenceTrustFor(EvidencePrecisionBoundary); ok {
		t.Fatal("EvidenceTrustFor(EvidencePrecisionBoundary) returned ok")
	}
}

func TestJudgmentEvidenceDetailLookupUsesChain(t *testing.T) {
	j := Judgment{Evidence: EvidenceChain{
		{Kind: EvidenceAbstractFact, Trust: EvidenceTrustProven, Detail: EvidenceDetail{Kind: EvidenceDetailMayBeNil, SubjectLabel: "box"}},
		{Kind: EvidenceMissingProof, Trust: EvidenceTrustUnknown, Detail: EvidenceDetail{Kind: EvidenceDetailIndexedReadMissingProof}},
		{Kind: EvidenceMissingProof, Trust: EvidenceTrustRefuted, Detail: EvidenceDetail{Kind: EvidenceDetailMissingRequiredField, Field: "id"}},
	}}

	if !j.HasEvidenceDetail(EvidenceDetailMayBeNil) {
		t.Fatal("HasEvidenceDetail(EvidenceDetailMayBeNil) = false, want true")
	}
	if !j.HasEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailIndexedReadMissingProof) {
		t.Fatal("HasEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailIndexedReadMissingProof) = false, want true")
	}
	if !j.HasAnyEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailMayBeNil, EvidenceDetailIndexedReadMissingProof) {
		t.Fatal("HasAnyEvidenceKindDetail did not match indexed-read detail")
	}
	ev, ok := j.FirstEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailMissingRequiredField)
	if !ok || ev.Detail.Field != "id" {
		t.Fatalf("FirstEvidenceKindDetail missing required field = %#v, %v; want id, true", ev, ok)
	}
	if got := j.EvidenceKindDetails(EvidenceMissingProof, EvidenceDetailMissingRequiredField); len(got) != 1 || got[0].Detail.Field != "id" {
		t.Fatalf("EvidenceKindDetails = %#v, want one missing id field", got)
	}
	if _, ok := j.FirstEvidenceKindDetail(EvidenceUserAssertion, EvidenceDetailMissingRequiredField); ok {
		t.Fatal("FirstEvidenceKindDetail matched wrong outer evidence kind")
	}
}

func TestAssignmentProofQueriesUseStructuredEvidence(t *testing.T) {
	j := Judgment{Evidence: EvidenceChain{
		{
			Kind:   EvidenceUserAssertion,
			Detail: UnderSuppliedCallResultAssignmentEvidenceDetail("load", 1),
			Span:   SpanRef{StartLine: 12, StartCol: 8},
		},
		{
			Kind:   EvidenceAbstractFact,
			Detail: AssignmentCallInvalidationEvidenceDetail("mutate()", "box.value", "box.value"),
		},
		{
			Kind:   EvidenceAbstractFact,
			Detail: DynamicAssignmentTargetEvidenceDetail("slots[k]"),
		},
		{
			Kind:   EvidenceMissingProof,
			Detail: IndexedReadMissingProofEvidenceDetail(),
		},
	}}

	if detail, ok := j.AssignmentUnderSuppliedCallResultDetail(); !ok || !detail.UnderSupplied || detail.FunctionName != "load" {
		t.Fatalf("AssignmentUnderSuppliedCallResultDetail = %#v, %v; want load under-supplied", detail, ok)
	}
	if span, ok := j.AssignmentCallResultReturnSpan(); !ok || span.StartLine != 12 || span.StartCol != 8 {
		t.Fatalf("AssignmentCallResultReturnSpan = %#v, %v; want declared return span", span, ok)
	}
	if !j.AssignmentHasCallInvalidationEvidence() {
		t.Fatal("AssignmentHasCallInvalidationEvidence = false, want true")
	}
	if !j.AssignmentHasDynamicTargetEvidence() {
		t.Fatal("AssignmentHasDynamicTargetEvidence = false, want true")
	}
	if !j.AssignmentMissingProofMayBeNil() {
		t.Fatal("AssignmentMissingProofMayBeNil = false, want true for indexed read")
	}
	if !j.AssignmentMissingProofIndexedRead() {
		t.Fatal("AssignmentMissingProofIndexedRead = false, want true")
	}
	summary := j.AssignmentProof()
	if !summary.MissingProof ||
		!summary.IndexedRead ||
		!summary.MayBeNil ||
		!summary.CallInvalidated ||
		!summary.DynamicTarget ||
		!summary.CallResult {
		t.Fatalf("AssignmentProof = %#v, want missing/indexed/nil/invalidation/dynamic/call-result summary", summary)
	}
	if summary.Reason() != AssignmentProofReasonIndexedRead {
		t.Fatalf("AssignmentProof.Reason = %v, want indexed read", summary.Reason())
	}
	if !summary.BoundaryProofMissing() {
		t.Fatalf("AssignmentProof.BoundaryProofMissing = false, want true")
	}
}

func TestCallArgumentProofSummaryUsesStructuredEvidence(t *testing.T) {
	j := Judgment{
		Verdict: VerdictUnknown,
		Evidence: EvidenceChain{
			{
				Kind:   EvidenceMissingProof,
				Detail: MayBeNilEvidenceDetail(),
			},
			{
				Kind:   EvidenceMissingProof,
				Detail: GenericConflictEvidenceDetail("T"),
			},
			{
				Kind:   EvidenceMissingProof,
				Detail: MissingRequiredFieldEvidenceDetail("id"),
			},
			{
				Kind:   EvidenceMissingProof,
				Detail: MissingRequiredMethodTypeEvidenceDetail("save", nil),
			},
			{
				Kind:   EvidenceMissingProof,
				Detail: MethodTypeMismatchEvidenceDetail("load", nil, nil),
			},
			{
				Kind: EvidenceUserAssertion,
				Detail: CallParamObligationEvidenceDetail(
					"listen",
					"payload",
					"source.primary",
					2,
				),
			},
			{
				Kind: EvidencePrecisionBoundary,
			},
		},
	}

	summary := j.CallArgumentProof()
	if !summary.MayBeNil ||
		!summary.GenericConflict ||
		summary.GenericParam != "T" ||
		summary.MissingRequiredField != "id" ||
		!summary.MissingRequiredMethod ||
		summary.MissingRequiredMethodDetail.Field != "save" ||
		!summary.MethodTypeMismatch ||
		summary.MethodTypeMismatchDetail.Field != "load" ||
		!summary.CallParamObligation ||
		summary.CallParamSubjectLabel != "payload" ||
		!summary.PrecisionBoundary {
		t.Fatalf("CallArgumentProof = %#v, want nil/generic/structure/call-param/precision summary", summary)
	}
	if !summary.Renderable(VerdictUnknown) {
		t.Fatalf("CallArgumentProof.Renderable(unknown) = false, want true for precision boundary")
	}
	if summary.CallParamDetail.ProviderLabel != "source.primary" || summary.CallParamDetail.MemberParam != 2 {
		t.Fatalf("CallArgumentProof call-param detail = %#v", summary.CallParamDetail)
	}
}

func TestCallArityProofSummaryUsesStructuredEvidence(t *testing.T) {
	tooFew := Judgment{Evidence: EvidenceChain{{
		Kind:   EvidenceMissingProof,
		Detail: ArityTooFewEvidenceDetail(3, 1),
	}}}
	summary := tooFew.CallArityProof()
	if !summary.Found ||
		summary.Detail.Kind != EvidenceDetailArityTooFew ||
		summary.Detail.ExpectedCount != 3 ||
		summary.Detail.ActualCount != 1 {
		t.Fatalf("CallArityProof too-few = %#v, want 3 expected / 1 actual", summary)
	}

	tooMany := Judgment{Evidence: EvidenceChain{{
		Kind:   EvidenceMissingProof,
		Detail: ArityTooManyEvidenceDetail(1, 3),
	}}}
	summary = tooMany.CallArityProof()
	if !summary.Found ||
		summary.Detail.Kind != EvidenceDetailArityTooMany ||
		summary.Detail.ExpectedCount != 1 ||
		summary.Detail.ActualCount != 3 {
		t.Fatalf("CallArityProof too-many = %#v, want 1 expected / 3 actual", summary)
	}

	absent := Judgment{Evidence: EvidenceChain{{
		Kind:   EvidenceUserAssertion,
		Detail: ArityTooManyEvidenceDetail(1, 3),
	}}}
	if got := absent.CallArityProof(); got.Found {
		t.Fatalf("CallArityProof matched non-missing-proof evidence: %#v", got)
	}
}

func TestCallCalleeProofSummaryUsesStructuredEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		detail EvidenceDetail
	}{
		{name: "not callable", detail: CalleeNotCallableEvidenceDetail()},
		{name: "may be nil", detail: CalleeMayBeNilEvidenceDetail(true)},
		{name: "missing member", detail: MemberMissingEvidenceDetail("send")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := Judgment{Evidence: EvidenceChain{{
				Kind:   EvidenceMissingProof,
				Detail: tc.detail,
			}}}
			summary := item.CallCalleeProof()
			if !summary.Found || summary.Detail.Kind != tc.detail.Kind {
				t.Fatalf("CallCalleeProof = %#v, want %v", summary, tc.detail.Kind)
			}
			if summary.Detail.Field != tc.detail.Field ||
				summary.Detail.Callable != tc.detail.Callable ||
				summary.Detail.MemberAccess != tc.detail.MemberAccess {
				t.Fatalf("CallCalleeProof detail = %#v, want %#v", summary.Detail, tc.detail)
			}
		})
	}

	absent := Judgment{Evidence: EvidenceChain{{
		Kind:   EvidenceUserAssertion,
		Detail: CalleeNotCallableEvidenceDetail(),
	}}}
	if got := absent.CallCalleeProof(); got.Found {
		t.Fatalf("CallCalleeProof matched non-missing-proof evidence: %#v", got)
	}
}

func TestMemberReadProofSummaryUsesStructuredEvidence(t *testing.T) {
	item := Judgment{Evidence: EvidenceChain{
		{
			Kind:   EvidenceAbstractFact,
			Detail: MemberMissingEvidenceDetail("send"),
		},
		{
			Kind:   EvidenceMissingProof,
			Detail: MemberMissingEvidenceDetail("send"),
		},
	}}
	summary := item.MemberReadProof()
	if !summary.Found || summary.Detail.Kind != EvidenceDetailMemberMissing || summary.Detail.Field != "send" {
		t.Fatalf("MemberReadProof = %#v, want missing send member", summary)
	}

	absent := Judgment{Evidence: EvidenceChain{{
		Kind:   EvidenceAbstractFact,
		Detail: MemberMissingEvidenceDetail("send"),
	}}}
	if got := absent.MemberReadProof(); got.Found {
		t.Fatalf("MemberReadProof matched abstract fact without missing proof: %#v", got)
	}
}

func TestConcatOperandProofSummaryUsesStructuredEvidence(t *testing.T) {
	item := Judgment{Evidence: EvidenceChain{{
		Kind:   EvidenceAbstractFact,
		Detail: EvidenceDetail{Kind: EvidenceDetailConcatOperand, Field: "right"},
	}}}
	summary := item.ConcatOperandProof()
	if !summary.Found || summary.Detail.Kind != EvidenceDetailConcatOperand || summary.Detail.Field != "right" {
		t.Fatalf("ConcatOperandProof = %#v, want right operand", summary)
	}

	absent := Judgment{Evidence: EvidenceChain{{
		Kind:   EvidenceMissingProof,
		Detail: EvidenceDetail{Kind: EvidenceDetailConcatOperand, Field: "right"},
	}}}
	if got := absent.ConcatOperandProof(); got.Found {
		t.Fatalf("ConcatOperandProof matched missing proof instead of abstract fact: %#v", got)
	}
}

func TestLifecycleProofSummaryUsesStructuredEvidence(t *testing.T) {
	item := Judgment{Evidence: EvidenceChain{{
		Kind: EvidenceMissingProof,
		Detail: EvidenceDetail{
			Kind:         EvidenceDetailLifecycleMissingProof,
			Resource:     "tx",
			Protocol:     "transaction",
			CurrentState: "open",
			FinalState:   "closed",
		},
	}}}
	summary := item.LifecycleProof()
	if !summary.Found ||
		summary.Detail.Kind != EvidenceDetailLifecycleMissingProof ||
		summary.Detail.Resource != "tx" ||
		summary.Detail.Protocol != "transaction" ||
		summary.Detail.CurrentState != "open" ||
		summary.Detail.FinalState != "closed" {
		t.Fatalf("LifecycleProof = %#v, want tx transaction obligation", summary)
	}

	absent := Judgment{Evidence: EvidenceChain{{
		Kind: EvidenceAbstractFact,
		Detail: EvidenceDetail{
			Kind:     EvidenceDetailLifecycleMissingProof,
			Resource: "tx",
		},
	}}}
	if got := absent.LifecycleProof(); got.Found {
		t.Fatalf("LifecycleProof matched non-missing-proof evidence: %#v", got)
	}
}

func TestNumericForProofSummaryUsesStructuredEvidence(t *testing.T) {
	item := Judgment{Evidence: EvidenceChain{
		{Kind: EvidenceAbstractFact},
		{Kind: EvidenceUserAssertion},
		{Kind: EvidencePrecisionBoundary},
		{Kind: EvidenceMissingProof},
	}}
	summary := item.NumericForProof()
	if !summary.UserAssertion || !summary.PrecisionBoundary || !summary.MissingProof {
		t.Fatalf("NumericForProof = %#v, want explicit-top proof trio", summary)
	}

	plain := Judgment{Evidence: EvidenceChain{{Kind: EvidenceAbstractFact}}}
	if got := plain.NumericForProof(); got.UserAssertion || got.PrecisionBoundary || got.MissingProof {
		t.Fatalf("NumericForProof = %#v, want no optional proof categories", got)
	}
}

func TestDefaultRegistryValidatesCallArgumentJudgmentShape(t *testing.T) {
	j := Judgment{
		Code:    CodeCallArgType,
		Subject: NewSubjectRef("body", SubjectCallArgument, "arg:0"),
		Verdict: VerdictUnknown,
		Evidence: EvidenceChain{
			{Kind: EvidenceAbstractFact, Trust: EvidenceTrustProven},
			{Kind: EvidenceUserAssertion, Trust: EvidenceTrustClaimed},
			{Kind: EvidenceMissingProof, Trust: EvidenceTrustUnknown},
		},
	}
	if !DefaultRegistry().Validate(j) {
		t.Fatalf("registry rejected valid call-argument judgment: %#v", j)
	}
	j.Subject.Kind = SubjectPath
	if DefaultRegistry().Validate(j) {
		t.Fatalf("registry accepted wrong subject kind: %#v", j.Subject)
	}
}

func TestRegistryLookupReturnsDefensiveCopy(t *testing.T) {
	reg := DefaultRegistry()
	spec, ok := reg.Lookup(CodeCallArgType)
	if !ok {
		t.Fatal("Lookup(CodeCallArgType) returned false")
	}
	spec.RequiredEvidence[0] = EvidencePrecisionBoundary
	again, ok := reg.Lookup(CodeCallArgType)
	if !ok {
		t.Fatal("second Lookup(CodeCallArgType) returned false")
	}
	if again.RequiredEvidence[0] != EvidenceAbstractFact {
		t.Fatalf("registry spec was mutated through lookup: %#v", again.RequiredEvidence)
	}
}

func TestRegistryCodesReturnsDeterministicDefensiveList(t *testing.T) {
	reg := NewRegistry([]CodeSpec{
		{Code: CodeReturn},
		{Code: CodeCallArgType},
		{Code: CodeAssignment},
	})

	got := reg.Codes()
	want := []Code{CodeAssignment, CodeCallArgType, CodeReturn}
	if len(got) != len(want) {
		t.Fatalf("Codes len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Codes[%d] = %s, want %s: %#v", i, got[i], want[i], got)
		}
	}

	got[0] = CodeConcatOperand
	again := reg.Codes()
	if again[0] != CodeAssignment {
		t.Fatalf("Codes returned mutable registry storage: %#v", again)
	}
}

func TestNewRegistryRejectsEmptyAndDuplicateCodes(t *testing.T) {
	requirePanic(t, func() {
		_ = NewRegistry([]CodeSpec{{}})
	})
	requirePanic(t, func() {
		_ = NewRegistry([]CodeSpec{
			{Code: CodeCallArgType},
			{Code: CodeCallArgType},
		})
	})
}

func TestDefaultPolicyCoversDefaultRegistry(t *testing.T) {
	policy := DefaultPolicy()
	for _, code := range DefaultRegistry().Codes() {
		for _, verdict := range []Verdict{VerdictProven, VerdictUnknown, VerdictRefuted} {
			for _, mode := range []StrictnessMode{StrictnessDefault, StrictnessLenient, StrictnessStrict} {
				if _, ok := policy.LevelFor(Judgment{Code: code, Verdict: verdict}, mode); !ok {
					t.Fatalf("default policy missing code=%s verdict=%v mode=%s", code, verdict, mode)
				}
			}
		}
	}
}

func TestDefaultPolicyMapsVerdictWithoutChangingJudgment(t *testing.T) {
	j := Judgment{Code: CodeCallArgType, Verdict: VerdictUnknown}
	defaultLevel, ok := DefaultPolicy().LevelFor(j, StrictnessDefault)
	if !ok {
		t.Fatal("default policy missing unknown call-arg row")
	}
	lenientLevel, ok := DefaultPolicy().LevelFor(j, StrictnessLenient)
	if !ok {
		t.Fatal("default policy missing lenient unknown call-arg row")
	}
	if defaultLevel != LevelError || lenientLevel != LevelWarning {
		t.Fatalf("levels default=%v lenient=%v, want error/warning", defaultLevel, lenientLevel)
	}
	if j.Verdict != VerdictUnknown {
		t.Fatalf("policy mutated judgment verdict to %v", j.Verdict)
	}
}

func TestDefaultPolicyCategories(t *testing.T) {
	tests := []struct {
		name    string
		code    Code
		verdict Verdict
		mode    StrictnessMode
		want    Level
	}{
		{
			name:    "strictness tunable type unknown is warning only in lenient mode",
			code:    CodeCallArgType,
			verdict: VerdictUnknown,
			mode:    StrictnessLenient,
			want:    LevelWarning,
		},
		{
			name:    "strictness tunable type refuted stays error in lenient mode",
			code:    CodeAssignment,
			verdict: VerdictRefuted,
			mode:    StrictnessLenient,
			want:    LevelError,
		},
		{
			name:    "refuted-only hard error does not report unknown evidence",
			code:    CodeMemberRead,
			verdict: VerdictUnknown,
			mode:    StrictnessStrict,
			want:    LevelDisabled,
		},
		{
			name:    "refuted-only hard error reports refuted evidence",
			code:    CodeUnresolvedValue,
			verdict: VerdictRefuted,
			mode:    StrictnessDefault,
			want:    LevelError,
		},
		{
			name:    "lint warning does not report unknown evidence",
			code:    CodeConcatOperand,
			verdict: VerdictUnknown,
			mode:    StrictnessDefault,
			want:    LevelDisabled,
		},
		{
			name:    "lint warning reports refuted evidence as warning",
			code:    CodeDeadAssignment,
			verdict: VerdictRefuted,
			mode:    StrictnessStrict,
			want:    LevelWarning,
		},
	}

	policy := DefaultPolicy()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := policy.LevelFor(Judgment{Code: tt.code, Verdict: tt.verdict}, tt.mode)
			if !ok {
				t.Fatalf("missing policy row for code=%s verdict=%v mode=%s", tt.code, tt.verdict, tt.mode)
			}
			if got != tt.want {
				t.Fatalf("level = %v, want %v", got, tt.want)
			}
		})
	}
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
