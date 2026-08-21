//go:build typprobe

package oracle

import (
	"fmt"
	"os"
	"sort"
	"testing"

	engineprobe "github.com/wippyai/go-lua/analysis/engine"
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

	total, cellDistinct, internedLeft := engineprobe.ZZProbeCellPair()
	fmt.Fprintf(os.Stderr, "ZZPROBE CellPair total=%d distinct=%d collapse=%s internedLeft=%d/%d\n",
		total, cellDistinct, zzProbeRatio(total, cellDistinct), internedLeft, total)
	domains := engineprobe.ZZProbeCellPairDomains()
	names := make([]string, 0, len(domains))
	for name := range domains {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		counts := domains[name]
		fmt.Fprintf(os.Stderr, "ZZPROBE CellPair domain=%s total=%d distinct=%d collapse=%s internedLeft=%d/%d\n",
			name, counts[0], counts[1], zzProbeRatio(counts[0], counts[1]), counts[2], counts[0])
	}
	os.Exit(code)
}

func zzProbeRatio(a, b uint64) string {
	if b == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.4f", float64(a)/float64(b))
}
