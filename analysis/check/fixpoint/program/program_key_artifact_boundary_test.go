package program

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
)

func TestSummaryKeyDigestRejectsProcessLocalCFGReference(t *testing.T) {
	t.Run("portable-symbol-reference", func(t *testing.T) {
		var encoded bytes.Buffer
		writeSummaryKeyDigest(&encoded, summary.SummaryKey{
			Ref: ref.FuncRef{Kind: ref.KindSymbol, ID: 17},
			Entry: summary.EntryKey{
				Values: 3, Facts: 5, References: 7,
			},
		})
		if got, want := encoded.String(), "2/17/3/5/7;"; got != want {
			t.Fatalf("portable summary key encoding = %q, want %q", got, want)
		}
	})

	t.Run("process-local-cfg-reference", func(t *testing.T) {
		var encoded bytes.Buffer
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatalf("persistent summary-key digest serialized process-local KindCFG reference as %q; the artifact boundary must reject it", encoded.String())
			}
		}()
		writeSummaryKeyDigest(&encoded, summary.SummaryKey{
			Ref: ref.FuncRef{Kind: ref.KindCFG, ID: 23},
			Entry: summary.EntryKey{
				Values: 11, Facts: 13, References: 17,
			},
		})
	})
}
