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

// zzProbeConstructLazy is zzProbeConstruct for a hash that is itself lazily
// derived (Generic, Instantiated, Record, Function): it must not force that
// derivation - a full graph walk, one construction at a time, while a large
// product is still being built bottom-up - just to feed a hook that is nil
// outside the typprobe build tag.
func zzProbeConstructLazy(k uint64, h func() uint64) {
	if zzProbeConstructHook != nil {
		zzProbeConstructHook(k, h())
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
