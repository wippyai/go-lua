package heap

// dbgprobe.go carries temporary structural counters for the admission fence.
// Admission runs on the solve loop's single thread, so the counters are plain
// fields.

// DbgHeapCounters records how often an admission re-derives what it was
// already handed. PartitionValidations counts complete Partition validity
// proofs and DefaultDerivations counts residual-default folds over a runtime
// kind set. Both are obligations of the admitted Object, discharged once
// where the Object is proved, so a count that scales with the Object's kinds,
// exceptions, or present coordinates names a re-proof.
type DbgHeapCounters struct {
	PartitionValidations uint64
	DefaultDerivations   uint64

	// FreshRootMaterializations counts fresh Root rows built out of the
	// sealed catalog. A reader that only needs to know a fresh row exists,
	// or what kind every fresh row has, builds none.
	FreshRootMaterializations uint64
}

var dbgHeap DbgHeapCounters

// DbgHeap returns the accumulated admission counters.
func DbgHeap() DbgHeapCounters { return dbgHeap }

// DbgHeapReset clears the accumulated admission counters.
func DbgHeapReset() { dbgHeap = DbgHeapCounters{} }
