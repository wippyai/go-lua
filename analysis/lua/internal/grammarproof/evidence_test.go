package grammarproof

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
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
	if err := Generate(root, filepath.Join(root, "analysis", "lua", "internal", "grammarproof", "evidence_gen.go"), true); err != nil {
		t.Fatal(err)
	}
}

// moduleRoot walks up from this test source until it finds the directory that
// owns go.mod. Anchoring on the module marker keeps the proof independent of
// where the grammarproof tree sits inside the module.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate grammarproof source")
	}
	root := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("module root: no go.mod above test file")
		}
		root = parent
	}
}

func collectLive(root string) ([]liveProduction, string, error) {
	grammar, err := parsersource.ExtractGrammar(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		return nil, "", err
	}
	sources, err := corpus(root)
	if err != nil {
		return nil, "", err
	}
	return liveFromGrammar(grammar), evidenceDigest(grammar, sources), nil
}
