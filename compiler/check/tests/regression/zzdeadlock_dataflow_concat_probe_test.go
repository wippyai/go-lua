package regression

import (
	"strings"
	"testing"

	checkpkg "github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZDeadlockDataflowConcatProbe verifies the canonical engine no longer
// rejects the two `..` concatenations in deadlock-dataflow-node (main.lua:421
// and :437), which read fields off `self: table` (the bare builtin `table`
// top). Those reads now project gradual `any` and are admitted as stringable,
// so `"..." .. self.dataflow_id` type-checks. The fixture must produce ZERO
// "cannot concatenate" diagnostics.
func TestZZDeadlockDataflowConcatProbe(t *testing.T) {
	src := readFixtureSource(t, "deadlock-dataflow-node")

	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(checkpkg.WithCanonicalFlow()))

	for _, d := range res.Diagnostics {
		if strings.Contains(d.Message, "cannot concatenate") {
			t.Errorf("unexpected concat diagnostic at %v: %s", d.Span, d.Message)
		}
	}
	t.Logf("deadlock-dataflow-node canonical diagnostics: total=%d errors=%d", len(res.Diagnostics), len(res.Errors))
	for _, d := range res.Diagnostics {
		t.Logf("  diag: %v %s", d.Span, d.Message)
	}
}
