package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/judgment"
)

func TestJudgmentRenderersCoverDefaultRegistry(t *testing.T) {
	registered := map[judgment.RenderKey]bool{}
	for _, code := range judgment.DefaultRegistry().Codes() {
		spec, ok := judgment.DefaultRegistry().Lookup(code)
		if !ok {
			t.Fatalf("default registry code disappeared: %s", code)
		}
		if spec.Family == "" || spec.Policy == "" {
			t.Fatalf("judgment code %s missing family or policy", code)
		}
		if len(spec.DiagnosticCodes) == 0 {
			t.Fatalf("judgment code %s missing diagnostic code mapping", code)
		}
		switch spec.DiagnosticDefault {
		case judgment.DiagnosticDefaultEnabled, judgment.DiagnosticDefaultOptIn:
		default:
			t.Fatalf("judgment code %s missing diagnostic default", code)
		}
		registered[spec.Render] = true
		if judgmentDiagnosticRenderers[spec.Render] == nil {
			t.Fatalf("missing judgment renderer for %s", code)
		}
	}

	for render := range judgmentDiagnosticRenderers {
		if !registered[render] {
			t.Fatalf("renderer registered for unused judgment render key %s", render)
		}
	}
}

func TestRefutedJudgmentPresentationPreservesExplicitWitnessTraceOptIn(t *testing.T) {
	detail := judgment.ArityTooFewEvidenceDetail(2, 1)
	item := judgment.Judgment{
		Code:    judgment.CodeCallArity,
		Subject: judgment.NewSubjectRef("main", judgment.SubjectCallExpression, "call:41:arity").WithLabel("encode"),
		Verdict: judgment.VerdictRefuted,
		Spans: []judgment.SpanRef{{
			File: "main.lua", StartLine: 41, StartCol: 1, EndLine: 41, EndCol: 8,
		}},
		Evidence: judgment.EvidenceChain{
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{Key: "arity:actual"},
				Detail: detail,
				Span:   judgment.SpanRef{File: "main.lua", StartLine: 41, StartCol: 1, EndLine: 41, EndCol: 8},
			},
			{
				Kind:   judgment.EvidenceUserAssertion,
				Trust:  judgment.EvidenceTrustClaimed,
				Origin: judgment.OriginRef{Key: "arity:declaration"},
				Detail: detail,
				Span:   judgment.SpanRef{File: "main.lua", StartLine: 12, StartCol: 1, EndLine: 12, EndCol: 8},
			},
			{
				Kind:   judgment.EvidenceMissingProof,
				Trust:  judgment.EvidenceTrustRefuted,
				Origin: judgment.OriginRef{Key: "arity:missing-proof"},
				Detail: detail,
				Span:   judgment.SpanRef{File: "main.lua", StartLine: 41, StartCol: 1, EndLine: 41, EndCol: 8},
			},
		},
	}

	diags := RenderJudgments([]judgment.Judgment{item}, Config{})
	if len(diags) != 1 {
		t.Fatalf("RenderJudgments diagnostics = %#v, want one", diags)
	}
	if diags[0].Explanation.WitnessTrace() {
		t.Fatalf("refuted judgment explanation enabled witness trace without an explicit render option: %#v", diags[0])
	}
	evidence := diags[0].Explanation.Evidence()
	if len(evidence) != 3 || evidence[0].Span.StartLine != 41 || evidence[1].Span.StartLine != 12 || evidence[2].Span.StartLine != 41 {
		t.Fatalf("diagnostic evidence changed before rendering = %#v, want original evidence order", evidence)
	}

	presentation := EvidenceForJudgment(item)
	if len(presentation) != 3 || presentation[0].Span.StartLine != 12 || presentation[1].Span.StartLine != 41 || presentation[2].Span.StartLine != 41 {
		t.Fatalf("readmodel presentation source order = %#v, want lines 12, 41, 41", presentation)
	}
}

func TestDeduplicateWitnessOriginsKeepsDistinctContent(t *testing.T) {
	item := judgment.Judgment{Evidence: judgment.EvidenceChain{
		{Origin: judgment.OriginRef{Key: "birth"}, Detail: judgment.EvidenceDetail{Message: "nil enters"}},
		{Origin: judgment.OriginRef{Key: "join"}, Detail: judgment.EvidenceDetail{Message: "nil survives join"}},
		{Origin: judgment.OriginRef{Key: "birth"}, Detail: judgment.EvidenceDetail{Message: "nil enters"}},
		{Origin: judgment.OriginRef{Key: "birth"}, Detail: judgment.EvidenceDetail{Message: "the declaration is optional"}},
	}}

	got := deduplicateWitnessOrigins(item).Evidence
	if len(got) != 3 || got[0].Detail.Message != "nil enters" || got[1].Detail.Message != "nil survives join" || got[2].Detail.Message != "the declaration is optional" {
		t.Fatalf("deduplicated witness origins = %#v, want duplicate birth removed and distinct birth content retained", got)
	}
}
