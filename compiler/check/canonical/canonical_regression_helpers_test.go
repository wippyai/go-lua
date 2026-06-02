package canonical_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func canonicalMessages(t *testing.T, src string) []string {
	t.Helper()
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	return testutil.ErrorMessages(res.Diagnostics)
}

func requireCanonicalClean(t *testing.T, src string) {
	t.Helper()
	if msgs := canonicalMessages(t, src); len(msgs) != 0 {
		t.Fatalf("expected clean canonical check, got diagnostics: %v", msgs)
	}
}

func requireCanonicalDiagnosticContains(t *testing.T, src, needle string) {
	t.Helper()
	msgs := canonicalMessages(t, src)
	for _, msg := range msgs {
		if strings.Contains(msg, needle) {
			return
		}
	}
	t.Fatalf("expected diagnostic containing %q, got %v", needle, msgs)
}
