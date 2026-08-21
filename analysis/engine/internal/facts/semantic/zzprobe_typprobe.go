//go:build typprobe

package semantic

import "sync"

// ZZPROBE: join-distinctness counters for the solver-ladder apply-cache
// measurement lane (KB go-lua-solver-mathematics-post-unwind step 0). For
// every pairwise join the many-way contribution merge performs (the same
// join counted by dbgSemantic.CellPairs), this records the operand identity
// pair and tallies total joins against distinct pairs.
//
// The right operand is always an already-interned terminal.ID[V] - an arena
// page+slot pair, boxed through `any` for a type-erased hook signature - read
// straight from the diagram traversal's published input planes. The left
// operand (the running accumulator) carries that same interned identity only
// on a cell's first fold step, where it is literally the decoded first
// present terminal; every later step folds an intermediate join result that
// this engine deliberately never interns (see JoinContributionsMany's own
// doc comment: "No intermediate Join prefix receives a terminal identity").
// For that majority case the key falls back to the domain's own structural
// Fingerprint of the intermediate value - the cheapest identity available,
// since nothing more specific exists. That fallback share (reported as
// internedLeft/total) is itself the interning finding, not a probe artifact.
var (
	zzProbeCellPairMu    sync.Mutex
	zzProbeCellPairTotal uint64
	zzProbeCellPairLeft  uint64
	zzProbeCellPairSeen  = make(map[zzProbeCellPairKey]struct{})
	zzProbeCellPairByDom = make(map[string]*zzProbeDomainCounts)
)

type zzProbeCellPairKey struct {
	domain       string
	leftInterned bool
	left         any
	leftFP       uint64
	right        any
}

type zzProbeDomainCounts struct {
	total uint64
	left  uint64
	seen  map[zzProbeCellPairKey]struct{}
}

func init() {
	zzProbeCellPairHook = func(domain string, leftInterned bool, left any, leftFingerprint uint64, right any) {
		key := zzProbeCellPairKey{domain: domain, leftInterned: leftInterned, right: right}
		if leftInterned {
			key.left = left
		} else {
			key.leftFP = leftFingerprint
		}

		zzProbeCellPairMu.Lock()
		zzProbeCellPairTotal++
		if leftInterned {
			zzProbeCellPairLeft++
		}
		zzProbeCellPairSeen[key] = struct{}{}

		perDomain := zzProbeCellPairByDom[domain]
		if perDomain == nil {
			perDomain = &zzProbeDomainCounts{seen: make(map[zzProbeCellPairKey]struct{})}
			zzProbeCellPairByDom[domain] = perDomain
		}
		perDomain.total++
		if leftInterned {
			perDomain.left++
		}
		perDomain.seen[key] = struct{}{}
		zzProbeCellPairMu.Unlock()
	}
}

// ZZProbeCellPairCounters reports the accumulated join-distinctness
// counters: total pairwise joins the many-way contribution merge performed,
// distinct (domain, left, right) operand keys among them (the apply-cache
// collapse factor is total/distinct), and how many of those joins had a
// fully interned left operand.
func ZZProbeCellPairCounters() (total, distinct, internedLeft uint64) {
	zzProbeCellPairMu.Lock()
	defer zzProbeCellPairMu.Unlock()
	return zzProbeCellPairTotal, uint64(len(zzProbeCellPairSeen)), zzProbeCellPairLeft
}

// ZZProbeCellPairDomain reports the same counters restricted to one V-type
// domain label (the %T of the terminal payload - value lattice, heap cell,
// FDD diagram node, etc, whichever concrete Factor kinds the corpus run
// actually exercised).
func ZZProbeCellPairDomains() map[string][3]uint64 {
	zzProbeCellPairMu.Lock()
	defer zzProbeCellPairMu.Unlock()
	out := make(map[string][3]uint64, len(zzProbeCellPairByDom))
	for domain, counts := range zzProbeCellPairByDom {
		out[domain] = [3]uint64{counts.total, uint64(len(counts.seen)), counts.left}
	}
	return out
}
