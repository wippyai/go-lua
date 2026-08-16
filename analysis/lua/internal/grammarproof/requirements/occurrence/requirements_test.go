package occurrence

import (
	"path/filepath"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/grammar"
)

func TestParserFieldStateInventoryRemainsIndependentOfSemanticLaws(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "..", "..", ".."))
	schema, err := grammar.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	required, err := Derive(schema)
	if err != nil {
		t.Fatal(err)
	}
	traces, err := grammarproof.SemanticTraces(root)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Observe(required, schema, traces)
	if err != nil {
		t.Fatal(err)
	}
	if report.RequiredCount() == 0 || report.ObservedCount() == 0 {
		t.Fatalf("field-state report = %#v", report)
	}
	// Parser residue remains explicit until action-level parser-impossibility
	// or exact semantic-family witnesses account for it. This is not a Program
	// completion gate and an observation is not a lowering proof.
	t.Logf("parser field-state inventory: required=%d observed=%d residue=%d", report.RequiredCount(), report.ObservedCount(), report.ResidueCount())
	dispositions, err := ClassifyResidue(root, report, schema)
	if err != nil {
		t.Fatal(err)
	}
	parserImpossible, sourceReachable, ingressRejected := 0, 0, 0
	for _, disposition := range dispositions {
		switch disposition.Kind {
		case DispositionParserImpossible:
			if disposition.Parser == ParserLawInvalid || disposition.Semantic != SemanticLawInvalid {
				t.Fatalf("invalid parser-impossibility disposition: %#v", disposition)
			}
			parserImpossible++
		case DispositionSourceReachable:
			if disposition.Semantic == SemanticLawInvalid || disposition.Parser != ParserLawInvalid {
				t.Fatalf("invalid source-reachable disposition: %#v", disposition)
			}
			if source, ok := SemanticWitnessSource(disposition.Semantic); !ok || source == "" {
				t.Fatalf("source-reachable disposition has no canonical ingress witness: %#v", disposition)
			}
			sourceReachable++
		case DispositionPublicIngressRejected:
			if disposition.Ingress != IngressLawMalformedNumericLiteral || disposition.Parser != ParserLawInvalid || disposition.Semantic != SemanticLawInvalid {
				t.Fatalf("invalid public-ingress-rejected disposition: %#v", disposition)
			}
			ingressRejected++
		default:
			t.Fatalf("invalid residue disposition: %#v", disposition)
		}
	}
	if parserImpossible+sourceReachable+ingressRejected != report.ResidueCount() || len(dispositions) != report.ResidueCount() {
		t.Fatalf("residue dispositions = impossible %d reachable %d ingress-rejected %d total %d, want total %d", parserImpossible, sourceReachable, ingressRejected, len(dispositions), report.ResidueCount())
	}
}
