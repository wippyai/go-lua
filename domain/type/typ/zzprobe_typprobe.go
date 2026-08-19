//go:build typprobe

package typ

import (
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/internal/hash"
)

// ZZPROBE: M1 (intra-graph) and M2 (cross-analysis) duplication-ratio
// counters for the typ hash-consing measurement lane (journal 3756
// amendment 11). Compiled only with -tags typprobe; the default build never
// pays for these fields.
var (
	zzProbeRefineNodes   uint64
	zzProbeRefineClasses uint64

	zzProbeConstructTotal uint64
	zzProbeConstructMu    sync.Mutex
	zzProbeConstructSeen  = make(map[uint64]struct{})
)

func init() {
	zzProbeRefineHook = func(nodes, classes int) {
		atomic.AddUint64(&zzProbeRefineNodes, uint64(nodes))
		atomic.AddUint64(&zzProbeRefineClasses, uint64(classes))
	}
	zzProbeConstructHook = func(k uint64, h uint64) {
		atomic.AddUint64(&zzProbeConstructTotal, 1)
		key := hash.MixHash(k, h)
		zzProbeConstructMu.Lock()
		zzProbeConstructSeen[key] = struct{}{}
		zzProbeConstructMu.Unlock()
	}
}

// ZZProbeCounters reports the accumulated measurement-lane counters:
//   - refineNodes: total distinct Type pointers discovered across all
//     canonical encodes (M1 numerator, accumulated).
//   - refineClasses: total distinct bisimulation classes across the same
//     encodes (M1 denominator, accumulated).
//   - constructTotal: total type-node constructions observed (M2 numerator).
//   - constructDistinct: distinct (kind, Hash()) construction keys observed
//     (M2 denominator; hash-collision noise is acceptable for a ratio).
func ZZProbeCounters() (refineNodes, refineClasses, constructTotal, constructDistinct uint64) {
	zzProbeConstructMu.Lock()
	distinct := uint64(len(zzProbeConstructSeen))
	zzProbeConstructMu.Unlock()
	return atomic.LoadUint64(&zzProbeRefineNodes), atomic.LoadUint64(&zzProbeRefineClasses),
		atomic.LoadUint64(&zzProbeConstructTotal), distinct
}
