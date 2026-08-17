package semantic

import (
	"fmt"
	"strings"
	"sync/atomic"
)

var (
	DbgJoinUnder                atomic.Int64
	DbgJoinContributions        atomic.Int64
	DbgJoinContributionsMany    atomic.Int64
	DbgJoinContributionsManyIn  atomic.Int64
	DbgJoinContributionChanges  atomic.Int64
	DbgJoinContributionChangeIn atomic.Int64
	DbgWidenUnderKeys           atomic.Int64
	DbgNarrowUnderKeys          atomic.Int64
	DbgSelectUnderKeys          atomic.Int64
	DbgPreserveUnder            atomic.Int64
	DbgNarrowUnder              atomic.Int64
	DbgReplaceUnder             atomic.Int64
	DbgMergeUnder               atomic.Int64
	DbgMu                       atomic.Int64
	DbgReindex                  atomic.Int64
	DbgReindexContribution      atomic.Int64
	DbgRestrict                 atomic.Int64
	DbgCloseContribution        atomic.Int64
	DbgEqualUnder               atomic.Int64
	DbgLessOrEqUnder            atomic.Int64
	DbgLessOrEqContribution     atomic.Int64
	DbgSummary                  atomic.Int64
	DbgForEachNonDefault        atomic.Int64
	DbgPartition                atomic.Int64
	DbgPartitionKey             atomic.Int64
	DbgEmpty                    atomic.Int64
)

var dbgAll = []struct {
	name    string
	counter *atomic.Int64
}{
	{"JoinUnder", &DbgJoinUnder},
	{"JoinContributions", &DbgJoinContributions},
	{"JoinContributionsMany", &DbgJoinContributionsMany},
	{"JoinContributionsMany_inputs", &DbgJoinContributionsManyIn},
	{"JoinContributionChanges", &DbgJoinContributionChanges},
	{"JoinContributionChanges_rows", &DbgJoinContributionChangeIn},
	{"WidenUnderKeys", &DbgWidenUnderKeys},
	{"NarrowUnderKeys", &DbgNarrowUnderKeys},
	{"SelectUnderKeys", &DbgSelectUnderKeys},
	{"PreserveUnder", &DbgPreserveUnder},
	{"NarrowUnder", &DbgNarrowUnder},
	{"ReplaceUnder", &DbgReplaceUnder},
	{"mergeUnder_total", &DbgMergeUnder},
	{"Mu", &DbgMu},
	{"Reindex", &DbgReindex},
	{"ReindexContribution", &DbgReindexContribution},
	{"Restrict", &DbgRestrict},
	{"CloseContribution", &DbgCloseContribution},
	{"EqualUnder", &DbgEqualUnder},
	{"LessOrEqUnder", &DbgLessOrEqUnder},
	{"LessOrEqContribution", &DbgLessOrEqContribution},
	{"Summary", &DbgSummary},
	{"ForEachNonDefault", &DbgForEachNonDefault},
	{"Partition", &DbgPartition},
	{"PartitionKey", &DbgPartitionKey},
	{"Empty", &DbgEmpty},
}

func DbgReset() {
	for _, row := range dbgAll {
		row.counter.Store(0)
	}
}

func DbgReport() string {
	var out strings.Builder
	for _, row := range dbgAll {
		fmt.Fprintf(&out, "  semantic.%-30s %12d\n", row.name, row.counter.Load())
	}
	return out.String()
}
