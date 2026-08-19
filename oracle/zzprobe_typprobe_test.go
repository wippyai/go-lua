//go:build typprobe

package oracle

import (
	"fmt"
	"os"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
)

// ZZPROBE: prints the typ hash-consing measurement-lane counters (journal
// 3756 amendment 11) once the oracle corpus run finishes. Compiled only with
// -tags typprobe; the default oracle test binary never links this file.
func TestMain(m *testing.M) {
	code := m.Run()
	nodes, classes, constructs, distinct := typ.ZZProbeCounters()
	fmt.Fprintf(os.Stderr, "ZZPROBE M1 refineNodes=%d refineClasses=%d ratio=%s\n",
		nodes, classes, zzProbeRatio(nodes, classes))
	fmt.Fprintf(os.Stderr, "ZZPROBE M2 constructions=%d distinct=%d ratio=%s\n",
		constructs, distinct, zzProbeRatio(constructs, distinct))
	os.Exit(code)
}

func zzProbeRatio(a, b uint64) string {
	if b == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.4f", float64(a)/float64(b))
}
