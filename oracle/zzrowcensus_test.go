//go:build zzsolveprobe

package oracle

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"

	engineprobe "github.com/wippyai/go-lua/analysis/engine"
)

// TestZZRowCensusPrefix prints the sealed construction row population for one
// edge-matrix prefix. ZZROWS names the case count. Scratch instrumentation.
func TestZZRowCensusPrefix(t *testing.T) {
	value := os.Getenv("ZZROWS")
	if value == "" {
		t.Skip("set ZZROWS to a prefix case count")
	}
	cases, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	engineprobe.DbgProgramRowsReset()
	// The prefix lane fatals on an incomplete answer, which is exactly the
	// state these prefixes reach, so the census is read on the way out.
	defer func() {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		fmt.Printf("ZZROWCENSUS cases=%d %+v heapAllocMB=%d heapSysMB=%d totalAllocMB=%d\n",
			cases, engineprobe.DbgProgramRows(), stats.HeapAlloc>>20, stats.HeapSys>>20, stats.TotalAlloc>>20)
	}()
	analyzeEdgeMatrixPrefix(t, cases)
}
