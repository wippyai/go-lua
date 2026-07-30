package equation

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

// ErrIncompleteTransaction is returned when a canonical kernel cannot produce
// its complete result.  In that case the VM publishes no part of the body
// closure.
var ErrIncompleteTransaction = errors.New("equation: incomplete transaction")

// EntryBinding closes the one formal input admitted by an equation artifact.
// Value is opaque semantic content owned by the caller of the VM.
type EntryBinding struct {
	Parameter EntryParameter
	Value     []byte
}

func (b EntryBinding) valid() bool { return b.Parameter.valid() && len(b.Value) != 0 }

// BoundOperand is an operand after the artifact's sole formal parameter has
// been substituted.  Kernels never receive an open Term.
type BoundOperand struct {
	Role  OperandRole
	Value []byte
}

// BoundEquation is an equation specialized to one body entry.  ContractID
// remains attached so a kernel cannot be selected only by a spelling string.
type BoundEquation struct {
	Target       Coordinate
	Dependencies []Coordinate
	Occurrence   Occurrence
	KernelID     string
	Guards       []Guard
	Operands     []BoundOperand
}

// BoundArtifact is an immutable entry specialization of one acyclic body.
// It deliberately retains no State and no production executor callback.
type BoundArtifact struct {
	Body      BodyID
	Entry     EntryBinding
	Equations []BoundEquation
}

// BindEntry closes all entry operands in artifact.  An artifact passed to the
// Stage-3 evaluator is one lexical body; mixing bodies or formal parameters is
// rejected before any kernel can run.
func BindEntry(artifact Artifact, entry EntryBinding) (BoundArtifact, error) {
	if !entry.valid() || artifact.CanonicalBytes() == nil {
		return BoundArtifact{}, fmt.Errorf("equation: invalid artifact or entry binding")
	}
	if len(artifact.Equations) == 0 {
		return BoundArtifact{}, fmt.Errorf("equation: empty acyclic artifact")
	}
	bound := BoundArtifact{Body: entry.Parameter.Body, Entry: EntryBinding{Parameter: entry.Parameter, Value: append([]byte(nil), entry.Value...)}, Equations: make([]BoundEquation, 0, len(artifact.Equations))}
	for _, equation := range artifact.Equations {
		if equation.Target.Body != bound.Body || equation.Entry != entry.Parameter {
			return BoundArtifact{}, fmt.Errorf("equation: entry binding does not own %s", equation.Target.Name)
		}
		result := BoundEquation{
			Target: equation.Target, Dependencies: append([]Coordinate(nil), equation.Dependencies...), Occurrence: equation.Occurrence, KernelID: equation.KernelID,
			Guards: append([]Guard(nil), equation.Guards...), Operands: make([]BoundOperand, 0, len(equation.Operands)),
		}
		for _, operand := range equation.Operands {
			value := operand.Term.Encoding
			if operand.Term.Entry {
				value = entry.Value
			}
			result.Operands = append(result.Operands, BoundOperand{Role: operand.Role, Value: append([]byte(nil), value...)})
		}
		bound.Equations = append(bound.Equations, result)
	}
	sort.Slice(bound.Equations, func(i, j int) bool { return bound.Equations[i].Target.less(bound.Equations[j].Target) })
	return bound, nil
}

// Fact is one published value, outcome, or diagnostic candidate.  Its key and
// bytes are semantic content supplied by the source-owned canonical kernel.
type Fact struct {
	Key   string
	Value []byte
	// Guards retain the CFG provenance of a fact.  A fact is usable only in a
	// partition whose active guards include every guard recorded here.  Keeping
	// this on the published fact, rather than in evaluator-local state, means a
	// closure can safely cross the lexical evaluator boundary.
	Guards []Guard
}

// AllocationRekey records the complete allocation identity transport emitted
// by a transaction.  It is a first-class output rather than a hidden mutation
// of an evaluator-local allocation table.
type AllocationRekey struct {
	From string
	To   string
}

// OutputClosure is the complete published result of an acyclic body.
type OutputClosure struct {
	Values           []Fact
	Outcomes         []Fact
	Diagnostics      []Fact
	AllocationRekeys []AllocationRekey
}

// Equal compares canonical published results.  It is used by shadow mode and
// intentionally includes every output channel, not just normal values.
func (c OutputClosure) Equal(other OutputClosure) bool {
	left, leftErr := joinClosure(c)
	right, rightErr := joinClosure(other)
	return leftErr == nil && rightErr == nil && bytes.Equal(left.bytes(), right.bytes())
}

func (c OutputClosure) bytes() []byte {
	out := appendText(nil, "equation/output-closure/v1")
	appendFacts := func(facts []Fact) {
		out = appendU64(out, uint64(len(facts)))
		for _, fact := range facts {
			out = appendText(out, fact.Key)
			out = appendBytes(out, fact.Value)
			out = appendU64(out, uint64(len(fact.Guards)))
			for _, guard := range fact.Guards {
				out = appendBytes(out, guard.Body[:])
				out = appendBytes(out, guard.Encoding)
			}
		}
	}
	appendFacts(c.Values)
	appendFacts(c.Outcomes)
	appendFacts(c.Diagnostics)
	out = appendU64(out, uint64(len(c.AllocationRekeys)))
	for _, rekey := range c.AllocationRekeys {
		out = appendText(out, rekey.From)
		out = appendText(out, rekey.To)
	}
	return out
}

// FactLane names the publication lane a merge is asked about. A lattice
// answers differently per lane: a value has a type domain to widen into, an
// outcome is a proof that can only be withdrawn, and a diagnostic must survive
// a disagreement because withdrawing it would silently drop a report.
type FactLane uint8

const (
	LaneValue FactLane = iota
	LaneOutcome
	LaneDiagnostic
)

// FactMerge is the fact owner's resolution for two publications that share a
// key and a guard cube but disagree on their payload. It exists only for
// recurrent evaluation: one trip of a loop and the next trip publish the same
// operation's result, and the equation layer owns neither the value encoding
// nor the question of what their union is. Reporting false withdraws the fact,
// which is the fail-closed answer for a family that has no lattice of its own.
type FactMerge func(lane FactLane, key string, left, right []byte) ([]byte, bool)

func joinClosure(closures ...OutputClosure) (OutputClosure, error) {
	return mergeClosures(nil, closures...)
}

// mergeClosures is joinClosure with a fact-owner resolution for conflicting
// payloads. A nil merge keeps the strict reading: inside a single evaluation
// two publications of one key under one guard cube must agree, and a
// disagreement is a lowering defect rather than a lattice question.
func mergeClosures(merge FactMerge, closures ...OutputClosure) (OutputClosure, error) {
	var out OutputClosure
	for _, closure := range closures {
		out.Values = append(out.Values, closure.Values...)
		out.Outcomes = append(out.Outcomes, closure.Outcomes...)
		out.Diagnostics = append(out.Diagnostics, closure.Diagnostics...)
		out.AllocationRekeys = append(out.AllocationRekeys, closure.AllocationRekeys...)
	}
	// One universe of interned cubes serves all three lanes of this merge: the
	// facts a join brings together repeat a handful of guard cubes thousands of
	// times, and every fact carrying a cube can share the one canonical set.
	var cubes guardSets
	canonicalFacts := func(lane FactLane, facts []Fact) ([]Fact, error) {
		payload := 0
		for index := range facts {
			payload += len(facts[index].Value)
		}
		values := make([]byte, 0, payload)
		for index := range facts {
			if facts[index].Key == "" {
				return nil, fmt.Errorf("equation: output fact has no key")
			}
			facts[index].Value = cutBytes(&values, facts[index].Value)
			facts[index].Guards = cubes.canonical(facts[index].Guards)
			for _, guard := range facts[index].Guards {
				if !guard.valid() {
					return nil, fmt.Errorf("equation: malformed output fact guard")
				}
			}
		}
		sort.Slice(facts, func(i, j int) bool {
			if facts[i].Key != facts[j].Key {
				return facts[i].Key < facts[j].Key
			}
			if len(facts[i].Guards) == 0 || len(facts[j].Guards) == 0 {
				return len(facts[i].Guards) < len(facts[j].Guards)
			}
			return guardsCompare(facts[i].Guards, facts[j].Guards) < 0
		})
		unique := facts[:0]
		// A withdrawn row stays in place until the group is finished so that a
		// third publication of the same key is compared against the group, not
		// appended behind it. The whole group is dropped in one compaction pass.
		dropped := make([]bool, 0, len(facts))
		withdrawn := false
		for _, fact := range facts {
			if last := len(unique) - 1; last >= 0 && unique[last].Key == fact.Key && sameGuards(unique[last].Guards, fact.Guards) {
				if dropped[last] || bytes.Equal(unique[last].Value, fact.Value) {
					continue
				}
				if merge == nil {
					return nil, fmt.Errorf("equation: conflicting output for %q", fact.Key)
				}
				resolved, keep := merge(lane, fact.Key, unique[last].Value, fact.Value)
				if !keep {
					dropped[last] = true
					withdrawn = true
					continue
				}
				unique[last].Value = append([]byte(nil), resolved...)
				continue
			}
			unique = append(unique, fact)
			dropped = append(dropped, false)
		}
		if withdrawn {
			kept := unique[:0]
			for index, fact := range unique {
				if dropped[index] {
					continue
				}
				kept = append(kept, fact)
			}
			unique = kept
		}
		return unique, nil
	}
	var err error
	if out.Values, err = canonicalFacts(LaneValue, out.Values); err != nil {
		return OutputClosure{}, err
	}
	if merge != nil {
		out.Values = withdrawContradictoryBranchProofs(out.Values)
	}
	if out.Outcomes, err = canonicalFacts(LaneOutcome, out.Outcomes); err != nil {
		return OutputClosure{}, err
	}
	if out.Diagnostics, err = canonicalFacts(LaneDiagnostic, out.Diagnostics); err != nil {
		return OutputClosure{}, err
	}
	for _, rekey := range out.AllocationRekeys {
		if rekey.From == "" || rekey.To == "" {
			return OutputClosure{}, fmt.Errorf("equation: malformed allocation rekey")
		}
	}
	sort.Slice(out.AllocationRekeys, func(i, j int) bool {
		if out.AllocationRekeys[i].From != out.AllocationRekeys[j].From {
			return out.AllocationRekeys[i].From < out.AllocationRekeys[j].From
		}
		return out.AllocationRekeys[i].To < out.AllocationRekeys[j].To
	})
	for index := 1; index < len(out.AllocationRekeys); index++ {
		previous, current := out.AllocationRekeys[index-1], out.AllocationRekeys[index]
		if previous.From == current.From && previous.To != current.To {
			return OutputClosure{}, fmt.Errorf("equation: conflicting allocation rekey for %q", current.From)
		}
	}
	return out, nil
}

// Partition is the read-only, fully closed result of completed prior
// transactions.  It is the Stage-3 partition-kernel input; callers cannot
// mutate a partially-built output closure through it.
//
// Its reads are served from a view built once for the snapshot rather than
// recomputed per call: nothing a read depends on -- the closure, the active
// cube, the branch proofs that promote it -- can change while a kernel holds
// the partition, so one answer stands for every read of it.
type Partition struct {
	closure OutputClosure
	guards  []Guard
	shared  *partitionView
}

func newPartition(closure OutputClosure, guards []Guard) Partition {
	return Partition{closure: closure, guards: guards, shared: &partitionView{closure: closure, guards: guards}}
}

// PartitionFromClosuresWithGuards constructs a closed predecessor snapshot for
// a guarded consumer. The guards belong to the consuming operation, not to
// any predecessor leaf, so cyclic evaluators can retain the same branch view
// as the acyclic VM without mutating a published closure.
func PartitionFromClosuresWithGuards(guards []Guard, closures ...OutputClosure) (Partition, error) {
	// A cyclic snapshot can contribute many predecessor leaves.  They are all
	// already closed publications, so aggregate their fact lanes before the
	// single canonical merge.  Re-canonicalizing after each append is the same
	// lattice join, but turns one snapshot read into a quadratic sort/copy path.
	canonical, err := joinClosure(closures...)
	if err != nil {
		return Partition{}, err
	}
	return newPartition(canonical, canonicalGuards(guards)), nil
}

// view returns this partition's read index.  A partition that carries none --
// only the zero value, which publishes nothing -- answers from a view of its
// own, so a read is never wrong for want of an index.
func (p Partition) view() *partitionView {
	if p.shared != nil {
		return p.shared
	}
	return &partitionView{closure: p.closure, guards: p.guards}
}

// FactCount reports how many closed facts this partition presents to a
// kernel. It is a work measure rather than a semantic query: no fact key,
// value, or guard is exposed, so it cannot participate in any type decision.
// A kernel's cost is at least linear in this count, which makes it the unit an
// evaluation budget charges.
func (p Partition) FactCount() int {
	return len(p.closure.Values) + len(p.closure.Outcomes) + len(p.closure.Diagnostics)
}

func (p Partition) Values() []Fact   { return p.view().visibleValues() }
func (p Partition) Outcomes() []Fact { return p.view().visibleOutcomes() }
func (p Partition) Diagnostics() []Fact {
	return p.view().visibleDiagnostics()
}

// AllValues is the unfiltered publication history.  Joining mutually exclusive
// values at a post-dominator is Reconverged's work: it owns the guard algebra
// and the completeness rule, so no consumer has to reconstruct either.  This
// remains for the one consumer that reads a guard inventory recorded inside the
// payloads rather than in the fact guards.  Ordinary reads use Values and
// therefore cannot accidentally observe an incompatible guard.
func (p Partition) AllValues() []Fact { return sealFacts(p.closure.Values) }
func (p Partition) AllocationRekeys() []AllocationRekey {
	return append([]AllocationRekey(nil), p.closure.AllocationRekeys...)
}

// Value returns one visible value fact by its already-published key.  Point
// lookups keep consumers that need one current publication from reading every
// visible fact first.
func (p Partition) Value(key string) (Fact, bool) {
	view := p.view()
	active := view.activeGuards()
	lane := p.closure.Values
	if !view.orderedValues() {
		for _, fact := range lane {
			if fact.Key == key && guardsIncluded(fact.Guards, active) {
				return fact, true
			}
		}
		return Fact{}, false
	}
	// The lane is ordered by key, so one key's publications form a run and the
	// first visible row of that run is the one an ordered scan reaches first.
	for index := sort.Search(len(lane), func(i int) bool { return lane[i].Key >= key }); index < len(lane) && lane[index].Key == key; index++ {
		if guardsIncluded(lane[index].Guards, active) {
			return lane[index], true
		}
	}
	return Fact{}, false
}

// ValuesPrefix returns the visible value publications whose keys start with
// prefix. It is the prefix-scoped form of Values: a consumer that reads one
// fact family must not have to read the whole partition in order to filter it.
func (p Partition) ValuesPrefix(prefix string) []Fact {
	view := p.view()
	active := view.activeGuards()
	lane := p.closure.Values
	if view.orderedValues() {
		start, end := prefixRange(lane, prefix)
		return selectFacts(lane[start:end:end], func(fact Fact) bool { return guardsIncluded(fact.Guards, active) })
	}
	var out []Fact
	for _, fact := range lane {
		if strings.HasPrefix(fact.Key, prefix) && guardsIncluded(fact.Guards, active) {
			out = append(out, fact)
		}
	}
	return out
}

// ValuesPrefixIterator is the allocation-free iterator form of ValuesPrefix.
// It is for kernels that consume a prefix range once rather than retaining the
// selected slice.
type ValuesPrefixIterator struct {
	prefix string
	lane   []Fact
	active []Guard
	index  int
}

// IterateValuesPrefix selects the same visible rows as ValuesPrefix without
// materializing the guarded subset.
func (p Partition) IterateValuesPrefix(prefix string) ValuesPrefixIterator {
	view := p.view()
	lane := p.closure.Values
	if view.orderedValues() {
		start, end := prefixRange(lane, prefix)
		lane = lane[start:end:end]
	}
	return ValuesPrefixIterator{prefix: prefix, lane: lane, active: view.activeGuards()}
}

// Next returns the next visible row in the selected prefix range.
func (it *ValuesPrefixIterator) Next() (Fact, bool) {
	for it.index < len(it.lane) {
		fact := it.lane[it.index]
		it.index++
		if strings.HasPrefix(fact.Key, it.prefix) && guardsIncluded(fact.Guards, it.active) {
			return fact, true
		}
	}
	return Fact{}, false
}

// LatestValuePrefix returns the lexically latest visible publication under
// prefix.  Versioned engine facts encode their current epoch in the key, so
// this preserves the same selection as a Values scan without visiting
// unrelated guarded facts.
func (p Partition) LatestValuePrefix(prefix string) (Fact, bool) {
	view := p.view()
	active := view.activeGuards()
	lane := p.closure.Values
	if !view.orderedValues() {
		var latest Fact
		found := false
		for _, fact := range lane {
			if strings.HasPrefix(fact.Key, prefix) && guardsIncluded(fact.Guards, active) && (!found || fact.Key > latest.Key) {
				latest, found = fact, true
			}
		}
		return latest, found
	}
	// The greatest key under the prefix is the last run of the range.  A run
	// whose every row is guarded away publishes nothing here, so the search
	// continues into the run before it, and within the run that does publish the
	// first visible row wins -- a later row only replaces a strictly greater key.
	start, end := prefixRange(lane, prefix)
	for last := end - 1; last >= start; {
		first := last
		for first > start && lane[first-1].Key == lane[last].Key {
			first--
		}
		for candidate := first; candidate <= last; candidate++ {
			if guardsIncluded(lane[candidate].Guards, active) {
				return lane[candidate], true
			}
		}
		last = first - 1
	}
	return Fact{}, false
}

// prefixRange returns the bounds of the run of facts whose keys start with
// prefix.  It requires a key-ordered lane, where such a run is contiguous and
// begins at the first key that is not below the prefix itself.
func prefixRange(facts []Fact, prefix string) (int, int) {
	start := sort.Search(len(facts), func(i int) bool { return facts[i].Key >= prefix })
	end := start
	for end < len(facts) && strings.HasPrefix(facts[end].Key, prefix) {
		end++
	}
	return start, end
}

// partitionView is the read index of one closed partition.  Every read of a
// partition asks about the same immutable snapshot under the same active cube,
// so each question is answered once here and shared by every later read.
//
// What it hands back is that shared state, not a copy of it.  A consumer may
// read, range over, and retain a result; it may not write through one.  The
// capacities are clamped so that appending to a returned lane allocates instead
// of writing into the snapshot, and every payload it exposes was already sealed
// by the canonical merge that published it.
//
// A view belongs to exactly one transaction, like the operand and guard rows a
// compiled evaluation binds beside it: the VM that hands a partition to a
// kernel owns the storage and rebinds it for the next transaction, which is
// what keeps the normal execution path free of per-transaction allocation.  A
// view is therefore not safe for concurrent use, and no consumer may retain a
// partition past the kernel call that received it.
type partitionView struct {
	closure OutputClosure
	guards  []Guard

	indexed bool
	// proofs are the value rows that can promote a branch guard.  Resolution
	// iterates them instead of the whole lane: a row that states no proof cannot
	// contribute to the fixed point at any step of it.
	proofs  []branchProof
	ordered bool
	active  []Guard

	valuesReady bool
	values      []Fact

	outcomesReady bool
	outcomes      []Fact

	diagnosticsReady bool
	diagnostics      []Fact
}

// branchProof is one published proof and the guard it certifies, recovered from
// its key once per partition rather than on every read.
type branchProof struct {
	required []Guard
	guard    Guard
}

// reset rebinds this view to the next transaction's snapshot.  The proof list
// keeps its capacity: consecutive transactions of one body index closures of
// the same shape, so the row storage is provisioned once and refilled.
func (v *partitionView) reset(closure OutputClosure, guards []Guard) {
	proofs := v.proofs[:0]
	*v = partitionView{closure: closure, guards: guards, proofs: proofs}
}

// clear releases everything this view holds.  A view that lives in worker
// scratch outlives the evaluation that filled it, so its rows are dropped for
// the same reason the operand and guard rows beside it are.
func (v *partitionView) clear() { v.reset(OutputClosure{}, nil) }

func (v *partitionView) index() {
	if v.indexed {
		return
	}
	v.indexed, v.ordered = true, true
	for position, fact := range v.closure.Values {
		if position != 0 && v.closure.Values[position-1].Key > fact.Key {
			v.ordered = false
		}
		if factkey.DecodeTruth(fact.Value) != factkey.TruthProven {
			continue
		}
		guard, ok := branchProofGuard(fact.Key)
		if !ok {
			continue
		}
		v.proofs = append(v.proofs, branchProof{required: fact.Guards, guard: guard})
	}
	v.active = resolvedBranchGuards(v.proofs, v.guards)
}

// orderedValues reports whether the value lane is ordered by key, which is the
// state a canonical merge leaves it in.  A lane assembled some other way is
// still read correctly, by scanning rather than searching.
func (v *partitionView) orderedValues() bool {
	v.index()
	return v.ordered
}

func (v *partitionView) activeGuards() []Guard {
	v.index()
	return v.active
}

func (v *partitionView) visibleValues() []Fact {
	if !v.valuesReady {
		v.values, v.valuesReady = v.visible(v.closure.Values), true
	}
	return v.values
}

func (v *partitionView) visibleOutcomes() []Fact {
	if !v.outcomesReady {
		v.outcomes, v.outcomesReady = v.visible(v.closure.Outcomes), true
	}
	return v.outcomes
}

func (v *partitionView) visibleDiagnostics() []Fact {
	if !v.diagnosticsReady {
		v.diagnostics, v.diagnosticsReady = v.visible(v.closure.Diagnostics), true
	}
	return v.diagnostics
}

// visible selects the rows this partition's active cube admits.  A lane every
// row of which is admitted is presented as it stands: the common partition has
// no guard to exclude anything, and restating the lane would be the copy this
// view exists to remove.
func (v *partitionView) visible(lane []Fact) []Fact {
	active := v.activeGuards()
	return selectFacts(lane, func(fact Fact) bool { return guardsIncluded(fact.Guards, active) })
}

// selectFacts returns the rows include admits, sharing the published rows
// rather than restating them.  The result is capacity-clamped: a consumer that
// appends to it allocates instead of writing past the selection.
func selectFacts(facts []Fact, include func(Fact) bool) []Fact {
	selected := 0
	for _, fact := range facts {
		if include(fact) {
			selected++
		}
	}
	if selected == len(facts) {
		return sealFacts(facts)
	}
	if selected == 0 {
		return nil
	}
	out := make([]Fact, 0, selected)
	for _, fact := range facts {
		if include(fact) {
			out = append(out, fact)
		}
	}
	return sealFacts(out)
}

// sealFacts presents a published lane for reading.  Clamping the capacity is
// what makes sharing safe: the rows stay the snapshot's, and an append by a
// consumer reallocates instead of writing into it.
func sealFacts(facts []Fact) []Fact {
	if len(facts) == 0 {
		return nil
	}
	return facts[:len(facts):len(facts)]
}

// cutBytes appends src to batch and returns exactly the range that holds it.
// An empty payload stays nil, matching a standalone copy.
func cutBytes(batch *[]byte, src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	start := len(*batch)
	*batch = append(*batch, src...)
	return (*batch)[start:len(*batch):len(*batch)]
}

// cloneFact detaches one fact from the snapshot it was selected from.  A
// reconvergence result is the one row a partition does not present as published
// -- it is a value the join derived, stamped with the residual cube -- so it is
// built as storage of its own rather than shared.
func cloneFact(fact Fact) Fact {
	return Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...), Guards: cloneGuards(fact.Guards)}
}

// resolvedBranchGuards promotes only an already-published deterministic branch
// edge whose own enclosing guards are visible. This is the post-dominator
// bridge for a branch proven constant by the kernel: facts from its sole live
// arm become visible after the branch, while unknown or alternate edges remain
// guarded and therefore unavailable.
func resolvedBranchGuards(proofs []branchProof, active []Guard) []Guard {
	resolved, owned := active, false
	for changed := true; changed; {
		changed = false
		for _, proof := range proofs {
			guard := proof.guard
			if !guardsIncluded(proof.required, resolved) || guardsContain(resolved, guard) {
				continue
			}
			if !owned {
				// The active cube belongs to the partition; a promotion writes only
				// into a set this resolution owns.
				resolved, owned = append(make([]Guard, 0, len(resolved)+4), resolved...), true
			}
			resolved = append(resolved, guard)
			changed = true
		}
	}
	if !owned {
		// No branch proof promoted anything, so the answer is the cube the
		// partition already holds.  Every consumer of a resolved cube reads it,
		// which is what lets the read borrow it instead of restating it.
		if guardsCanonical(active) {
			return active
		}
	}
	return canonicalGuards(resolved)
}

// guardsContain reports whether active already fixes one guard.  It is the
// single-guard form of guardsIncluded, spelled without the one-element slice
// that shape would otherwise allocate on every branch-proof candidate.
func guardsContain(active []Guard, guard Guard) bool {
	for _, candidate := range active {
		if candidate.Body == guard.Body && bytes.Equal(candidate.Encoding, guard.Encoding) {
			return true
		}
	}
	return false
}

// branchProofGuard recovers the guard one published branch proof states. The
// key's shape and the guard encoding it maps onto are both declared by factkey,
// so this and the reconvergence reader cannot drift apart; what stays here is
// the body identity, which only this package can validate.
func branchProofGuard(key string) (Guard, bool) {
	proof, ok := factkey.ParseBranchProof(key)
	if !ok || proof.Name == "" {
		return Guard{}, false
	}
	body, err := hex.DecodeString(proof.Body)
	if err != nil || len(body) != len(BodyID{}) {
		return Guard{}, false
	}
	var id BodyID
	copy(id[:], body)
	if !id.Valid() {
		return Guard{}, false
	}
	return Guard{Body: id, Encoding: []byte(proof.Encoding())}, true
}

func cloneGuards(in []Guard) []Guard {
	out := make([]Guard, len(in))
	for i, guard := range in {
		out[i] = Guard{Body: guard.Body, Encoding: append([]byte(nil), guard.Encoding...)}
	}
	return out
}

// shareGuards copies the guard list without copying the encodings.  A guard
// encoding is sealed syntax owned by the body that published it, so a set built
// for comparison shares the bytes; only a set handed to a consumer as part of a
// detached fact clones them.  The result is capacity-clamped, which is what
// keeps a later append off the source array.
func shareGuards(in []Guard) []Guard {
	out := make([]Guard, len(in))
	copy(out, in)
	return out
}

// appendGuard returns a set holding every guard of in plus one more, without
// writing into in.  A cube is shared by the partitions and facts that carry it,
// so extending one always builds a set of its own.
func appendGuard(in []Guard, guard Guard) []Guard {
	out := make([]Guard, 0, len(in)+1)
	out = append(out, in...)
	return append(out, guard)
}

// guardsCanonical reports whether in is already sorted and duplicate-free,
// which is the state every canonical producer leaves a cube in.
func guardsCanonical(in []Guard) bool {
	for i := 1; i < len(in); i++ {
		if !in[i-1].less(in[i]) {
			return false
		}
	}
	return true
}

// canonicalGuards returns the sorted, duplicate-free form of in as a set this
// package owns.  The input is always borrowed: a compiled evaluation binds an
// operation's guards in worker scratch that the next operation rebinds, so a
// cube that outlives one transaction can never alias what it was built from.
// The result is capacity-clamped, which keeps a later append off it too.
func canonicalGuards(in []Guard) []Guard {
	if len(in) == 0 {
		return nil
	}
	if guardsCanonical(in) {
		return shareGuards(in)
	}
	out := shareGuards(in)
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	unique := out[:0]
	for _, guard := range out {
		if len(unique) != 0 && unique[len(unique)-1].Body == guard.Body && bytes.Equal(unique[len(unique)-1].Encoding, guard.Encoding) {
			continue
		}
		unique = append(unique, guard)
	}
	return unique[:len(unique):len(unique)]
}

// guardSets interns the canonical guard cubes of one closure construction.  Its
// lifetime is the call that declares it -- a merge, a stamp -- so a cube lives
// exactly as long as the facts that carry it and nothing accumulates between
// evaluations.  Its size is bounded by the number of distinct cubes among the
// facts being canonicalized, which is why it needs no eviction rule.
//
// Interning is what makes a cube shareable: every fact under one cube points at
// the same immutable set instead of holding a private clone of it, which removes
// both the per-fact sort and the per-fact allocation from every join.
type guardSets struct {
	// A closure states a handful of distinct cubes, so the first few are held
	// inline and matched by content directly.  The hashed spill exists for the
	// bodies that genuinely nest more decisions than that.
	inline [8][]Guard
	count  int
	byHash map[uint64][][]Guard
}

// canonical returns the interned canonical form of in.  Two calls with the same
// cube content return the identical slice, so a caller may compare, sort, and
// store the result but never write through it.  The set is always one this
// table owns: a repeat of a cube already seen costs nothing, and the first
// sighting of a cube copies it away from the borrowed storage it arrived in.
func (s *guardSets) canonical(in []Guard) []Guard {
	if len(in) == 0 {
		return nil
	}
	if guardsCanonical(in) {
		if interned, found := s.lookup(in); found {
			return interned
		}
		return s.intern(shareGuards(in))
	}
	out := canonicalGuards(in)
	if interned, found := s.lookup(out); found {
		return interned
	}
	return s.intern(out)
}

func (s *guardSets) lookup(cube []Guard) ([]Guard, bool) {
	for _, existing := range s.inline[:s.count] {
		if sameGuards(existing, cube) {
			return existing, true
		}
	}
	if s.byHash == nil {
		return nil, false
	}
	for _, existing := range s.byHash[guardsHash(cube)] {
		if sameGuards(existing, cube) {
			return existing, true
		}
	}
	return nil, false
}

func (s *guardSets) intern(cube []Guard) []Guard {
	if s.count < len(s.inline) {
		s.inline[s.count] = cube
		s.count++
		return cube
	}
	if s.byHash == nil {
		s.byHash = make(map[uint64][][]Guard, 8)
	}
	hash := guardsHash(cube)
	s.byHash[hash] = append(s.byHash[hash], cube)
	return cube
}

// union returns the interned canonical cube holding every guard of both sets.
// The stamp path applies one cube to a whole closure, so the two sides that
// matter -- an unguarded fact and a fact already carrying the stamp -- resolve
// without building an intermediate set at all.
func (s *guardSets) union(left, right []Guard) []Guard {
	switch {
	case len(right) == 0:
		return s.canonical(left)
	case len(left) == 0:
		return s.canonical(right)
	case guardsIncluded(right, left):
		return s.canonical(left)
	}
	joined := make([]Guard, 0, len(left)+len(right))
	joined = append(joined, left...)
	joined = append(joined, right...)
	return s.canonical(joined)
}

const guardHashOffset, guardHashPrime = uint64(14695981039346656037), uint64(1099511628211)

// guardsHash is an FNV-1a digest of the same byte sequence guardsKey spells,
// used only to bucket interned cubes.  Equality is always decided by content.
func guardsHash(guards []Guard) uint64 {
	hash := guardHashOffset
	mix := func(b byte) { hash = (hash ^ uint64(b)) * guardHashPrime }
	for _, guard := range guards {
		for _, b := range guard.Body {
			mix(b)
		}
		mix(0)
		for _, b := range guard.Encoding {
			mix(b)
		}
		mix(0)
	}
	return hash
}

func guardsKey(guards []Guard) string {
	var out []byte
	for _, guard := range guards {
		out = append(out, guard.Body[:]...)
		out = append(out, 0)
		out = append(out, guard.Encoding...)
		out = append(out, 0)
	}
	return string(out)
}

// guardsCompare orders two cubes exactly as comparing their guardsKey strings
// does, without materializing either key.  The keys are the dominant allocation
// of a canonical sort otherwise: the comparator runs O(n log n) times per join
// and each call would spell out both sides in full.
//
// Body identities are fixed width, so a difference there decides immediately.
// Encodings are variable width and terminated by a zero byte in the key, so a
// shorter encoding loses to a longer one that extends it -- unless the extending
// byte is itself zero, the one shape a stream comparison cannot settle locally.
func guardsCompare(left, right []Guard) int {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index].Body != right[index].Body {
			return bytes.Compare(left[index].Body[:], right[index].Body[:])
		}
		leftEncoding, rightEncoding := left[index].Encoding, right[index].Encoding
		shared := len(leftEncoding)
		if len(rightEncoding) < shared {
			shared = len(rightEncoding)
		}
		if order := bytes.Compare(leftEncoding[:shared], rightEncoding[:shared]); order != 0 {
			return order
		}
		if len(leftEncoding) == len(rightEncoding) {
			continue
		}
		longer := rightEncoding
		if len(leftEncoding) > len(rightEncoding) {
			longer = leftEncoding
		}
		if longer[shared] == 0 {
			return strings.Compare(guardsKey(left), guardsKey(right))
		}
		if len(leftEncoding) < len(rightEncoding) {
			return -1
		}
		return 1
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	}
	return 0
}

// sameGuards compares two canonical guard sets element-wise. Both sides come
// from canonicalGuards, so ordering is already fixed and no key has to be
// materialized to decide equality.
func sameGuards(left, right []Guard) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Body != right[index].Body || !bytes.Equal(left[index].Encoding, right[index].Encoding) {
			return false
		}
	}
	return true
}

func guardsIncluded(required, active []Guard) bool {
	for _, guard := range required {
		found := false
		for _, candidate := range active {
			if guard.Body == candidate.Body && bytes.Equal(guard.Encoding, candidate.Encoding) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func stampClosure(closure OutputClosure, guards []Guard) OutputClosure {
	// Every fact of this closure receives the same cube, so the distinct results
	// number at most one per distinct fact cube.  Interning them here is what
	// keeps a stamp linear in the closure rather than in its guard payload.
	var cubes guardSets
	stamp := func(facts []Fact) {
		for index := range facts {
			facts[index].Guards = cubes.union(facts[index].Guards, guards)
		}
	}
	stamp(closure.Values)
	stamp(closure.Outcomes)
	stamp(closure.Diagnostics)
	return closure
}

// TransactionResult is returned by exactly one existing canonical kernel.
// Complete=false is never a publishable partial result.
type TransactionResult struct {
	Complete bool
	Closure  OutputClosure
	Access   AccessRecord
}

// Kernel is the single contract-bound execution authority for an equation.
// The VM binds operands and schedules transactions; it does not interpret a
// guard, a term, or a transfer itself.
type Kernel interface {
	Execute(BoundEquation, Partition) (TransactionResult, error)
}

type KernelFunc func(BoundEquation, Partition) (TransactionResult, error)

func (f KernelFunc) Execute(equation BoundEquation, partition Partition) (TransactionResult, error) {
	return f(equation, partition)
}

// KernelBinding makes the contract/kernel pairing explicit.  Verify is the
// source-owned Stage-1 access verifier and, when supplied, runs after a
// complete kernel call without affecting its semantics.
type KernelBinding struct {
	KernelID   string
	ContractID ContentID
	Kernel     Kernel
	Verify     func(AccessRecord) error
}

type KernelRegistry struct {
	bindings map[string]map[ContentID]KernelBinding
}

func NewKernelRegistry(bindings []KernelBinding) (*KernelRegistry, error) {
	registry := &KernelRegistry{bindings: make(map[string]map[ContentID]KernelBinding)}
	for _, binding := range bindings {
		if binding.KernelID == "" || !binding.ContractID.Valid() || binding.Kernel == nil {
			return nil, fmt.Errorf("equation: malformed kernel binding")
		}
		byContract := registry.bindings[binding.KernelID]
		if byContract == nil {
			byContract = make(map[ContentID]KernelBinding)
			registry.bindings[binding.KernelID] = byContract
		}
		if _, exists := byContract[binding.ContractID]; exists {
			return nil, fmt.Errorf("equation: duplicate kernel binding %q", binding.KernelID)
		}
		byContract[binding.ContractID] = binding
	}
	return registry, nil
}

func (r *KernelRegistry) resolve(equation BoundEquation) (KernelBinding, bool) {
	if r == nil {
		return KernelBinding{}, false
	}
	binding, found := r.bindings[equation.KernelID][equation.Occurrence.ContractID]
	return binding, found
}

// AcyclicVM is the Stage-3 transaction VM.  It operates only on a bound
// acyclic artifact, accumulates an unpublished closure, then returns it once
// every contract-bound kernel has completed successfully.
type AcyclicVM struct{ registry *KernelRegistry }

func NewAcyclicVM(registry *KernelRegistry) (*AcyclicVM, error) {
	if registry == nil {
		return nil, fmt.Errorf("equation: nil kernel registry")
	}
	return &AcyclicVM{registry: registry}, nil
}

type Evaluation struct {
	Closure      OutputClosure
	Transactions int
}

func (vm *AcyclicVM) Evaluate(bound BoundArtifact) (Evaluation, error) {
	if vm == nil || vm.registry == nil || !bound.Entry.valid() || !bound.Body.Valid() || bound.Body != bound.Entry.Parameter.Body || len(bound.Equations) == 0 {
		return Evaluation{}, fmt.Errorf("equation: invalid bound acyclic artifact")
	}
	ordered, err := acyclicOrder(bound)
	if err != nil {
		return Evaluation{}, err
	}
	closure := OutputClosure{}
	// One read index serves the whole evaluation, rebound to each transaction's
	// snapshot: the partition a kernel receives lives exactly as long as its
	// transaction, so the storage behind it is provisioned once.
	var view partitionView
	for _, equation := range ordered {
		if equation.Target.Body != bound.Body || !equation.Occurrence.valid() || equation.KernelID == "" {
			return Evaluation{}, fmt.Errorf("equation: malformed bound equation")
		}
		binding, found := vm.registry.resolve(equation)
		if !found {
			return Evaluation{}, fmt.Errorf("equation: no contract-bound kernel for %s", equation.Target.Name)
		}
		view.reset(closure, equation.Guards)
		result, err := binding.Kernel.Execute(equation, Partition{closure: closure, guards: equation.Guards, shared: &view})
		if err != nil {
			return Evaluation{}, fmt.Errorf("equation: transaction %s: %w", equation.Target.Name, err)
		}
		if !result.Complete {
			return Evaluation{}, fmt.Errorf("equation: transaction %s: %w", equation.Target.Name, ErrIncompleteTransaction)
		}
		if binding.Verify != nil {
			if err := binding.Verify(result.Access); err != nil {
				return Evaluation{}, fmt.Errorf("equation: transaction %s access audit: %w", equation.Target.Name, err)
			}
		}
		closure, err = joinClosure(closure, stampClosure(result.Closure, equation.Guards))
		if err != nil {
			return Evaluation{}, fmt.Errorf("equation: transaction %s output: %w", equation.Target.Name, err)
		}
	}
	return Evaluation{Closure: closure, Transactions: len(ordered)}, nil
}

func acyclicOrder(bound BoundArtifact) ([]BoundEquation, error) {
	byTarget := make(map[Coordinate]BoundEquation, len(bound.Equations))
	dependents := make(map[Coordinate][]Coordinate, len(bound.Equations))
	inDegree := make(map[Coordinate]int, len(bound.Equations))
	for _, equation := range bound.Equations {
		if _, exists := byTarget[equation.Target]; exists {
			return nil, fmt.Errorf("equation: duplicate bound target %s", equation.Target.Name)
		}
		byTarget[equation.Target] = equation
		inDegree[equation.Target] = len(equation.Dependencies)
	}
	for _, equation := range bound.Equations {
		seen := make(map[Coordinate]struct{}, len(equation.Dependencies))
		for _, dependency := range equation.Dependencies {
			if dependency.Body != bound.Body {
				return nil, fmt.Errorf("equation: foreign bound dependency")
			}
			if _, duplicate := seen[dependency]; duplicate {
				return nil, fmt.Errorf("equation: duplicate bound dependency")
			}
			seen[dependency] = struct{}{}
			if _, found := byTarget[dependency]; !found {
				return nil, fmt.Errorf("equation: dependency %s has no equation", dependency.Name)
			}
			dependents[dependency] = append(dependents[dependency], equation.Target)
		}
	}
	ready := make([]Coordinate, 0, len(bound.Equations))
	for target, degree := range inDegree {
		if degree == 0 {
			ready = append(ready, target)
		}
	}
	less := func(i, j int) bool { return ready[i].less(ready[j]) }
	sort.Slice(ready, less)
	ordered := make([]BoundEquation, 0, len(bound.Equations))
	for len(ready) != 0 {
		target := ready[0]
		ready = ready[1:]
		ordered = append(ordered, byTarget[target])
		for _, dependent := range dependents[target] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
		sort.Slice(ready, less)
	}
	if len(ordered) != len(bound.Equations) {
		return nil, fmt.Errorf("equation: bound artifact is cyclic")
	}
	return ordered, nil
}

type ShadowReport struct {
	Cases  int
	Passed int
}
