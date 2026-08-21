package semantic

import "fmt"

// ZZPROBE: hook point for the solver-ladder join-distinctness measurement
// lane (KB go-lua-solver-mathematics-post-unwind step 0: does an apply-cache
// over the pairwise contribution join pay). Nil in the default build;
// zzprobe_typprobe.go installs the real counter under the typprobe build
// tag. Do not remove after the measurement lands - kept for re-measurement.
var zzProbeCellPairHook func(domain string, leftInterned bool, left any, leftFingerprint uint64, right any)

// zzProbeCellPair records one pairwise join the many-way contribution merge
// performs: the running accumulator ("left") and the newly folded operand
// ("right"). No-op unless the typprobe build tag is set. Every call site
// must still gate its own Fingerprint/boxing/%T work behind
// zzProbeCellPairHook != nil, the same way domain/type/typ's
// zzProbeConstructLazy callers do, so a disabled probe costs nothing on this
// pairwise-join hot path.
func zzProbeCellPair(domain string, leftInterned bool, left any, leftFingerprint uint64, right any) {
	if zzProbeCellPairHook != nil {
		zzProbeCellPairHook(domain, leftInterned, left, leftFingerprint, right)
	}
}

// zzProbeDomainLabel names the concrete Factor payload type folded at this
// call site (the value lattice, a heap cell, an FDD diagram node, or
// whichever concrete V a Factor binds), so the probe can report a per-domain
// split. Only evaluated behind a zzProbeCellPairHook != nil check.
func zzProbeDomainLabel[V any](value V) string {
	return fmt.Sprintf("%T", value)
}
