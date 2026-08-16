package grammarproof

import (
	"path/filepath"
	"runtime"
	"testing"
)

// The generated ingress ledger makes the parser-production witness useful to
// Program work: every production's accepted witness must also have traversed
// public Program ingress, whose one lower transaction parses, binds, lowers,
// and seals. Freshness is checked by the expensive
// generator test below; this law validates the checked-in relation directly.
func TestGeneratedEvidenceCoversLiveGrammarAndPublicIngress(t *testing.T) {
	root := moduleRoot(t)
	live, digest, err := collectLive(root)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := corpus(root)
	if err != nil {
		t.Fatal(err)
	}
	traceDigest, err := traceInputDigest(root, sources)
	if err != nil {
		t.Fatal(err)
	}
	if err := Generated.Validate(live, sources, digest, traceDigest); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedEvidenceRejectsChangedTraceInputs(t *testing.T) {
	root := moduleRoot(t)
	live, digest, err := collectLive(root)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := corpus(root)
	if err != nil {
		t.Fatal(err)
	}
	traceDigest, err := traceInputDigest(root, sources)
	if err != nil {
		t.Fatal(err)
	}
	stale := Generated
	stale.TraceDigest = "different-trace-inputs"
	if err := stale.Validate(live, sources, digest, traceDigest); err == nil {
		t.Fatal("trace input change accepted")
	}
}

// TestGeneratedEvidenceTraceIsCurrent validates the complete input commitment
// and replays public ingress in check mode. The throw-away reduction probe is
// generation-only work, so this routine freshness gate remains fast without
// accepting a trace from different parser, corpus, or probe inputs.
func TestGeneratedEvidenceTraceIsCurrent(t *testing.T) {
	root := moduleRoot(t)
	if err := Generate(root, filepath.Join(root, "program", "internal", "grammarproof", "evidence_gen.go"), true); err != nil {
		t.Fatal(err)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate grammarproof source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func collectLive(root string) ([]liveProduction, string, error) {
	grammar, err := extractGrammar(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		return nil, "", err
	}
	sources, err := corpus(root)
	if err != nil {
		return nil, "", err
	}
	return liveFromGrammar(grammar), evidenceDigest(grammar, sources), nil
}
