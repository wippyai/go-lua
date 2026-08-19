package typ

// ZZPROBE: hook points for the typprobe measurement lane (journal 3756
// amendment 11, hash-consing duplication ratios). Nil in the default build;
// zzprobe_typprobe.go installs the real counters under the typprobe build
// tag. Do not remove after the measurement lands - kept for re-measurement.
var (
	zzProbeConstructHook func(k uint64, h uint64)
	zzProbeRefineHook    func(nodes int, classes int)
)

// zzProbeConstruct records one type-node construction at kind k with
// structural hash h (M2). No-op unless the typprobe build tag is set.
func zzProbeConstruct(k uint64, h uint64) {
	if zzProbeConstructHook != nil {
		zzProbeConstructHook(k, h)
	}
}

// zzProbeRefine records one encoder refine() exit: nodes discovered and
// distinct bisimulation classes assigned (M1). No-op unless the typprobe
// build tag is set.
func zzProbeRefine(nodes, classes int) {
	if zzProbeRefineHook != nil {
		zzProbeRefineHook(nodes, classes)
	}
}
