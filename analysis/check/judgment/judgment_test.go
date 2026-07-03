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

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
