package equation

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
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
	left, leftErr := canonicalClosure(c)
	right, rightErr := canonicalClosure(other)
	return leftErr == nil && rightErr == nil && bytes.Equal(left.bytes(), right.bytes())
}

func (c OutputClosure) bytes() []byte {
	out := appendText(nil, "equation/output-closure/v1")
	appendFacts := func(facts []Fact) {
		out = appendU64(out, uint64(len(facts)))
		for _, fact := range facts {
			out = appendText(out, fact.Key)
			out = appendBytes(out, fact.Value)
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

func canonicalClosure(in OutputClosure) (OutputClosure, error) {
	out := OutputClosure{
		Values: append([]Fact(nil), in.Values...), Outcomes: append([]Fact(nil), in.Outcomes...),
		Diagnostics: append([]Fact(nil), in.Diagnostics...), AllocationRekeys: append([]AllocationRekey(nil), in.AllocationRekeys...),
	}
	canonicalFacts := func(facts []Fact) ([]Fact, error) {
		for index := range facts {
			if facts[index].Key == "" {
				return nil, fmt.Errorf("equation: output fact has no key")
			}
			facts[index].Value = append([]byte(nil), facts[index].Value...)
		}
		sort.Slice(facts, func(i, j int) bool { return facts[i].Key < facts[j].Key })
		unique := facts[:0]
		for _, fact := range facts {
			if len(unique) > 0 && unique[len(unique)-1].Key == fact.Key {
				if !bytes.Equal(unique[len(unique)-1].Value, fact.Value) {
					return nil, fmt.Errorf("equation: conflicting output for %q", fact.Key)
				}
				continue
			}
			unique = append(unique, fact)
		}
		return unique, nil
	}
	var err error
	if out.Values, err = canonicalFacts(out.Values); err != nil {
		return OutputClosure{}, err
	}
	if out.Outcomes, err = canonicalFacts(out.Outcomes); err != nil {
		return OutputClosure{}, err
	}
	if out.Diagnostics, err = canonicalFacts(out.Diagnostics); err != nil {
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

func mergeClosure(left, right OutputClosure) (OutputClosure, error) {
	joined := OutputClosure{
		Values:           append(append([]Fact(nil), left.Values...), right.Values...),
		Outcomes:         append(append([]Fact(nil), left.Outcomes...), right.Outcomes...),
		Diagnostics:      append(append([]Fact(nil), left.Diagnostics...), right.Diagnostics...),
		AllocationRekeys: append(append([]AllocationRekey(nil), left.AllocationRekeys...), right.AllocationRekeys...),
	}
	return canonicalClosure(joined)
}

// Partition is the read-only, fully closed result of completed prior
// transactions.  It is the Stage-3 partition-kernel input; callers cannot
// mutate a partially-built output closure through it.
type Partition struct{ closure OutputClosure }

// PartitionFromClosure closes a published snapshot for a contract kernel.
// It is used by the cyclic adapter to present the same complete read surface
// that the acyclic VM supplies to established kernels.
func PartitionFromClosure(closure OutputClosure) (Partition, error) {
	return PartitionFromClosures(closure)
}

// PartitionFromClosures joins complete snapshot leaves into one kernel read
// partition.  It is intentionally a value constructor: neither VM exposes
// mutable evaluator state through this bridge.
func PartitionFromClosures(closures ...OutputClosure) (Partition, error) {
	combined := OutputClosure{}
	var err error
	for _, closure := range closures {
		combined, err = mergeClosure(combined, closure)
		if err != nil {
			return Partition{}, err
		}
	}
	canonical, err := canonicalClosure(combined)
	if err != nil {
		return Partition{}, err
	}
	return Partition{closure: canonical}, nil
}

func (p Partition) Values() []Fact      { return cloneFacts(p.closure.Values) }
func (p Partition) Outcomes() []Fact    { return cloneFacts(p.closure.Outcomes) }
func (p Partition) Diagnostics() []Fact { return cloneFacts(p.closure.Diagnostics) }
func (p Partition) AllocationRekeys() []AllocationRekey {
	return append([]AllocationRekey(nil), p.closure.AllocationRekeys...)
}

func cloneFacts(facts []Fact) []Fact {
	out := make([]Fact, len(facts))
	for index, fact := range facts {
		out[index] = Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...)}
	}
	return out
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
		result, err := binding.Kernel.Execute(equation, Partition{closure: closure})
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
		closure, err = mergeClosure(closure, result.Closure)
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

// ShadowCase has a production publication and its independently bound
// equation artifact.  Shadow mode is test-only: this package never changes
// the production callback's result or routes it to callers.
type ShadowCase struct {
	Name       string
	Artifact   Artifact
	Entry      EntryBinding
	Production func() (OutputClosure, error)
}

type ShadowReport struct {
	Cases  int
	Passed int
}

// RunShadow requires exact published equality for every supplied acyclic
// case, including values, outcomes, diagnostic candidates, and rekeys.
func RunShadow(vm *AcyclicVM, cases []ShadowCase) (ShadowReport, error) {
	report := ShadowReport{Cases: len(cases)}
	for _, shadow := range cases {
		if shadow.Name == "" || shadow.Production == nil {
			return report, fmt.Errorf("equation: malformed shadow case")
		}
		production, err := shadow.Production()
		if err != nil {
			return report, fmt.Errorf("equation: shadow %s production: %w", shadow.Name, err)
		}
		production, err = canonicalClosure(production)
		if err != nil {
			return report, fmt.Errorf("equation: shadow %s production output: %w", shadow.Name, err)
		}
		bound, err := BindEntry(shadow.Artifact, shadow.Entry)
		if err != nil {
			return report, fmt.Errorf("equation: shadow %s binding: %w", shadow.Name, err)
		}
		evaluation, err := vm.Evaluate(bound)
		if err != nil {
			return report, fmt.Errorf("equation: shadow %s bound evaluation: %w", shadow.Name, err)
		}
		if !production.Equal(evaluation.Closure) {
			return report, fmt.Errorf("equation: shadow %s published output differs", shadow.Name)
		}
		report.Passed++
	}
	return report, nil
}
