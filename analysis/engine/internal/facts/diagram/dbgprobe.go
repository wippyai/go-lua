package diagram

import (
	"fmt"
	"strings"
	"sync/atomic"
)

var (
	DbgBegin    atomic.Int64
	DbgSeal     atomic.Int64
	DbgDiscard  atomic.Int64
	DbgSet      atomic.Int64
	DbgPut      atomic.Int64
	DbgDelete   atomic.Int64
	DbgPatchRow atomic.Int64

	DbgMergeSole        atomic.Int64
	DbgMergeSoleRegions atomic.Int64
	DbgMergeSoleKey     atomic.Int64
	DbgMergeSoleChanges atomic.Int64
	DbgMergeSoleRows    atomic.Int64
	DbgMergeMany        atomic.Int64
	DbgMergeManyRows    atomic.Int64
	DbgMergeManyOperand atomic.Int64
	DbgTransformSole    atomic.Int64
	DbgTransformRows    atomic.Int64

	DbgMakeKey    atomic.Int64
	DbgMakeFactor atomic.Int64
	DbgDecision   atomic.Int64
	DbgUpdate     atomic.Int64
	DbgImport     atomic.Int64
	DbgTerminal   atomic.Int64

	DbgCompare  atomic.Int64
	DbgRelate   atomic.Int64
	DbgEqual    atomic.Int64
	DbgForEach  atomic.Int64
	DbgKeyMax   atomic.Int64
	DbgKeyWidth atomic.Int64

	DbgBalanceKey     atomic.Int64
	DbgSetKeyNode     atomic.Int64
	DbgPopKeyMin      atomic.Int64
	DbgJoinKey3       atomic.Int64
	DbgBuildPatchKeys atomic.Int64
	DbgApplyPatches   atomic.Int64
	DbgConcatKeys     atomic.Int64
)

var dbgAll = []struct {
	name    string
	counter *atomic.Int64
}{
	{"begin", &DbgBegin},
	{"seal", &DbgSeal},
	{"discard", &DbgDiscard},
	{"set", &DbgSet},
	{"put", &DbgPut},
	{"delete", &DbgDelete},
	{"patch_rows_applied", &DbgPatchRow},
	{"merge_sole", &DbgMergeSole},
	{"merge_sole_regions", &DbgMergeSoleRegions},
	{"merge_sole_key", &DbgMergeSoleKey},
	{"merge_sole_changes", &DbgMergeSoleChanges},
	{"merge_sole_rows_visited", &DbgMergeSoleRows},
	{"merge_many", &DbgMergeMany},
	{"merge_many_rows_visited", &DbgMergeManyRows},
	{"merge_many_operands", &DbgMergeManyOperand},
	{"transform_sole", &DbgTransformSole},
	{"transform_rows", &DbgTransformRows},
	{"makeKey_pathcopy", &DbgMakeKey},
	{"makeFactor_pathcopy", &DbgMakeFactor},
	{"bdd_decision_nodes", &DbgDecision},
	{"bdd_update_calls", &DbgUpdate},
	{"bdd_import_nodes", &DbgImport},
	{"bdd_terminal_nodes", &DbgTerminal},
	{"compare_root", &DbgCompare},
	{"relate_root", &DbgRelate},
	{"equal_root", &DbgEqual},
	{"foreach_root", &DbgForEach},
	{"key_max_observed", &DbgKeyMax},
	{"key_distinct_writes", &DbgKeyWidth},
	{"balanceKey", &DbgBalanceKey},
	{"setKey", &DbgSetKeyNode},
	{"popKeyMin", &DbgPopKeyMin},
	{"joinKey3", &DbgJoinKey3},
	{"buildSolePatchKeys", &DbgBuildPatchKeys},
	{"applySolePatches", &DbgApplyPatches},
	{"concatKeys", &DbgConcatKeys},
}

func DbgReset() {
	for _, row := range dbgAll {
		row.counter.Store(0)
	}
}

func DbgReport() string {
	var out strings.Builder
	for _, row := range dbgAll {
		fmt.Fprintf(&out, "  diagram.%-24s %12d\n", row.name, row.counter.Load())
	}
	return out.String()
}

func dbgObserveKey(key uint64) {
	DbgKeyWidth.Add(1)
	for {
		prior := DbgKeyMax.Load()
		if int64(key) <= prior || DbgKeyMax.CompareAndSwap(prior, int64(key)) {
			return
		}
	}
}
