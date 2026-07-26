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
	Role  string
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
			facts[index].Guards = canonicalGuards(facts[index].Guards)
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
			return guardsKey(facts[i].Guards) < guardsKey(facts[j].Guards)
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
type Partition struct {
	closure OutputClosure
	guards  []Guard
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
	return Partition{closure: canonical, guards: canonicalGuards(guards)}, nil
}

// FactCount reports how many closed facts this partition presents to a
// kernel. It is a work measure rather than a semantic query: no fact key,
// value, or guard is exposed, so it cannot participate in any type decision.
// A kernel's cost is at least linear in this count -- every partition read
// filters and clones the whole fact lane -- which makes it the unit an
// evaluation budget charges.
func (p Partition) FactCount() int {
	return len(p.closure.Values) + len(p.closure.Outcomes) + len(p.closure.Diagnostics)
}

func (p Partition) Values() []Fact { return visibleFacts(p.closure.Values, p.closure.Values, p.guards) }
func (p Partition) Outcomes() []Fact {
	return visibleFacts(p.closure.Outcomes, p.closure.Values, p.guards)
}
func (p Partition) Diagnostics() []Fact {
	return visibleFacts(p.closure.Diagnostics, p.closure.Values, p.guards)
}

// AllValues is the unfiltered publication history.  Joining mutually exclusive
// values at a post-dominator is Reconverged's work: it owns the guard algebra
// and the completeness rule, so no consumer has to reconstruct either.  This
// remains for the one consumer that reads a guard inventory recorded inside the
// payloads rather than in the fact guards.  Ordinary reads use Values and
// therefore cannot accidentally observe an incompatible guard.
func (p Partition) AllValues() []Fact { return copyFacts(p.closure.Values, nil) }
func (p Partition) AllocationRekeys() []AllocationRekey {
	return append([]AllocationRekey(nil), p.closure.AllocationRekeys...)
}

// Value returns one visible value fact by its already-published key.  Like
// Values, it returns a detached copy: a kernel must not be able to mutate the
// closed predecessor snapshot it was given.  Point lookups keep consumers
// that need one current publication from copying every visible fact first.
func (p Partition) Value(key string) (Fact, bool) {
	active := resolvedBranchGuards(p.closure.Values, p.guards)
	for _, fact := range p.closure.Values {
		if fact.Key == key && guardsIncluded(fact.Guards, active) {
			return cloneFact(fact), true
		}
	}
	return Fact{}, false
}

// ValuesPrefix returns the visible value publications whose keys start with
// prefix. It is the prefix-scoped form of Values: a consumer that reads one
// fact family must not have to copy the whole partition in order to filter it.
func (p Partition) ValuesPrefix(prefix string) []Fact {
	active := resolvedBranchGuards(p.closure.Values, p.guards)
	var out []Fact
	for _, fact := range p.closure.Values {
		if strings.HasPrefix(fact.Key, prefix) && guardsIncluded(fact.Guards, active) {
			out = append(out, cloneFact(fact))
		}
	}
	return out
}

// LatestValuePrefix returns the lexically latest visible publication under
// prefix.  Versioned engine facts encode their current epoch in the key, so
// this preserves the same selection as a Values scan without materializing
// unrelated guarded facts.
func (p Partition) LatestValuePrefix(prefix string) (Fact, bool) {
	active := resolvedBranchGuards(p.closure.Values, p.guards)
	var latest Fact
	found := false
	for _, fact := range p.closure.Values {
		if strings.HasPrefix(fact.Key, prefix) && guardsIncluded(fact.Guards, active) && (!found || fact.Key > latest.Key) {
			latest, found = fact, true
		}
	}
	if !found {
		return Fact{}, false
	}
	return cloneFact(latest), true
}

func visibleFacts(facts, evidence []Fact, active []Guard) []Fact {
	active = resolvedBranchGuards(evidence, active)
	return copyFacts(facts, func(fact Fact) bool { return guardsIncluded(fact.Guards, active) })
}

// copyFacts detaches the selected facts from the closed snapshot. The payloads
// are cut from one backing array per copy instead of one allocation per fact:
// a partition read is the hottest operation in the solver, and per-fact
// payload allocation dominated both its cost and the collector's scan set.
//
// Each returned payload keeps the same guarantee as an individual copy. The
// ranges are disjoint and cut with full slice bounds, so a kernel can reach
// neither the snapshot nor another fact's payload through the one it was
// given, and an append on it always reallocates.
func copyFacts(facts []Fact, include func(Fact) bool) []Fact {
	selected, valueBytes, guardCount, guardBytes := 0, 0, 0, 0
	for _, fact := range facts {
		if include != nil && !include(fact) {
			continue
		}
		selected++
		valueBytes += len(fact.Value)
		guardCount += len(fact.Guards)
		for _, guard := range fact.Guards {
			guardBytes += len(guard.Encoding)
		}
	}
	out := make([]Fact, 0, selected)
	values := make([]byte, 0, valueBytes)
	guards := make([]Guard, 0, guardCount)
	encodings := make([]byte, 0, guardBytes)
	for _, fact := range facts {
		if include != nil && !include(fact) {
			continue
		}
		out = append(out, Fact{Key: fact.Key, Value: cutBytes(&values, fact.Value), Guards: cutGuards(&guards, &encodings, fact.Guards)})
	}
	return out
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

func cutGuards(batch *[]Guard, encodings *[]byte, in []Guard) []Guard {
	start := len(*batch)
	for _, guard := range in {
		*batch = append(*batch, Guard{Body: guard.Body, Encoding: cutBytes(encodings, guard.Encoding)})
	}
	return (*batch)[start:len(*batch):len(*batch)]
}

func cloneFact(fact Fact) Fact {
	return Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...), Guards: cloneGuards(fact.Guards)}
}

// resolvedBranchGuards promotes only an already-published deterministic branch
// edge whose own enclosing guards are visible. This is the post-dominator
// bridge for a branch proven constant by the kernel: facts from its sole live
// arm become visible after the branch, while unknown or alternate edges remain
// guarded and therefore unavailable.
func resolvedBranchGuards(evidence []Fact, active []Guard) []Guard {
	resolved := cloneGuards(active)
	for changed := true; changed; {
		changed = false
		for _, fact := range evidence {
			if !guardsIncluded(fact.Guards, resolved) || string(fact.Value) != "proven" {
				continue
			}
			guard, ok := branchProofGuard(fact.Key)
			if !ok || guardsIncluded([]Guard{guard}, resolved) {
				continue
			}
			resolved = append(resolved, guard)
			changed = true
		}
	}
	return canonicalGuards(resolved)
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

func canonicalGuards(in []Guard) []Guard {
	if len(in) == 0 {
		return nil
	}
	out := cloneGuards(in)
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	unique := out[:0]
	for _, guard := range out {
		if len(unique) != 0 && unique[len(unique)-1].Body == guard.Body && bytes.Equal(unique[len(unique)-1].Encoding, guard.Encoding) {
			continue
		}
		unique = append(unique, guard)
	}
	return unique
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
	stamp := func(facts []Fact) {
		for index := range facts {
			facts[index].Guards = canonicalGuards(append(facts[index].Guards, guards...))
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
	for _, equation := range ordered {
		if equation.Target.Body != bound.Body || !equation.Occurrence.valid() || equation.KernelID == "" {
			return Evaluation{}, fmt.Errorf("equation: malformed bound equation")
		}
		binding, found := vm.registry.resolve(equation)
		if !found {
			return Evaluation{}, fmt.Errorf("equation: no contract-bound kernel for %s", equation.Target.Name)
		}
		result, err := binding.Kernel.Execute(equation, Partition{closure: closure, guards: equation.Guards})
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
