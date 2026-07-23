package equation

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// OperandKind is the admission-time storage class of one equation operand.
//
// The equation IR deliberately keeps closed terms opaque.  Consequently this
// first compiled-plan slice classifies only facts it can prove from that IR:
// an EntryTerm is an entry-projection slot and every other term is a canonical
// constant.  The remaining kinds are reserved for later lowering surfaces;
// they must never be guessed from term bytes.
type OperandKind uint8

const (
	OperandCanonicalConstant OperandKind = iota + 1
	OperandEntryProjection
	OperandEquationCell
	OperandLocalFrame
	OperandCaptureCell
	OperandHeapPath
	OperandCallResult
)

// CompiledOperand is a scalar descriptor into a CompiledArtifact's immutable
// byte arena.  Offsets are intentionally used instead of per-operand []byte
// values so a future fast evaluator can resolve entry slots without rebinding
// or cloning every operand.
type CompiledOperand struct {
	Kind                     OperandKind
	RoleOffset, RoleLength   uint32
	ValueOffset, ValueLength uint32
}

// CompiledOp is one contract-bound operation.  It retains no executable
// closure: kernel lookup remains contract-ID based, exactly as it is in the
// reference VM.
type CompiledOp struct {
	TargetOffset, TargetLength         uint32
	OccurrenceOffset, OccurrenceLength uint32
	KernelOffset, KernelLength         uint32
	ContractID                         ContentID
	OperandStart, OperandCount         uint32
	GuardStart, GuardCount             uint32
	DependencyStart, DependencyCount   uint32
}

// CompiledBlock identifies an admission-proven straight-line block.  Cyclic
// WTO regions are retained separately by CompiledCyclicArtifact; this avoids
// accidentally treating an SCC as a fusion opportunity.
type CompiledBlock struct {
	OpStart, OpCount uint32
}

// CompiledCellLayout declares the normal-path scalar capacity needed by the
// later evaluator scratch arena.  It is intentionally a value-only layout.
type CompiledCellLayout struct {
	OperationCount uint32
	OperandCount   uint32
	GuardCount     uint32
}

// CompiledArtifact is the immutable, admission-compiled representation of an
// acyclic equation artifact.  Its private byte arena is the source of truth;
// accessors return copies so callers cannot change a retained plan.
type CompiledArtifact struct {
	id        ContentID
	canonical []byte
	body      BodyID
	entryName string
	bytes     []byte
	ops       []CompiledOp
	operands  []CompiledOperand
	guards    []compiledRange
	deps      []uint32
	blocks    []CompiledBlock
	layout    CompiledCellLayout
}

type compiledRange struct{ offset, length uint32 }

func (a CompiledArtifact) ContentID() ContentID       { return a.id }
func (a CompiledArtifact) CanonicalBytes() []byte     { return append([]byte(nil), a.canonical...) }
func (a CompiledArtifact) Layout() CompiledCellLayout { return a.layout }
func (a CompiledArtifact) Operations() []CompiledOp   { return append([]CompiledOp(nil), a.ops...) }
func (a CompiledArtifact) Operands() []CompiledOperand {
	return append([]CompiledOperand(nil), a.operands...)
}
func (a CompiledArtifact) Blocks() []CompiledBlock { return append([]CompiledBlock(nil), a.blocks...) }

// OperandBytes returns the canonical operand spelling.  Entry projection
// operands have their formal name here, never a caller-owned entry value.
func (a CompiledArtifact) OperandBytes(operand CompiledOperand) []byte {
	return a.rangeBytes(operand.ValueOffset, operand.ValueLength)
}

func (a CompiledArtifact) rangeBytes(offset, length uint32) []byte {
	end := uint64(offset) + uint64(length)
	if end > uint64(len(a.bytes)) {
		return nil
	}
	return append([]byte(nil), a.bytes[offset:end]...)
}

// CompileArtifact performs artifact admission for the acyclic evaluator
// lane.  It freezes a deterministic topological operation order but never
// changes an equation, its contract, or a kernel binding.
func CompileArtifact(artifact Artifact) (CompiledArtifact, error) {
	ordered, body, entry, err := compiledAcyclicOrder(artifact)
	if err != nil {
		return CompiledArtifact{}, err
	}
	return compileArtifact(artifact, ordered, body, entry)
}

func compileArtifact(artifact Artifact, ordered []Equation, body BodyID, entry EntryParameter) (CompiledArtifact, error) {
	canonical := artifact.CanonicalBytes()
	if canonical == nil || len(ordered) == 0 || !body.Valid() || !entry.valid() {
		return CompiledArtifact{}, fmt.Errorf("equation: invalid artifact for compiled admission")
	}
	result := CompiledArtifact{
		id: contentID(canonical), canonical: append([]byte(nil), canonical...), body: body, entryName: entry.Name,
		ops: make([]CompiledOp, 0, len(ordered)), blocks: []CompiledBlock{{OpStart: 0, OpCount: uint32(len(ordered))}},
	}
	byTarget := make(map[Coordinate]uint32, len(ordered))
	for index, equation := range ordered {
		byTarget[equation.Target] = uint32(index)
	}
	appendBytes := func(value []byte) (uint32, uint32) {
		offset := uint32(len(result.bytes))
		result.bytes = append(result.bytes, value...)
		return offset, uint32(len(value))
	}
	for _, equation := range ordered {
		targetOffset, targetLength := appendBytes([]byte(equation.Target.Name))
		occurrenceOffset, occurrenceLength := appendBytes([]byte(equation.Occurrence.Kind))
		kernelOffset, kernelLength := appendBytes([]byte(equation.KernelID))
		op := CompiledOp{TargetOffset: targetOffset, TargetLength: targetLength, OccurrenceOffset: occurrenceOffset, OccurrenceLength: occurrenceLength,
			KernelOffset: kernelOffset, KernelLength: kernelLength, ContractID: equation.Occurrence.ContractID,
			OperandStart: uint32(len(result.operands)), GuardStart: uint32(len(result.guards)), DependencyStart: uint32(len(result.deps))}
		for _, operand := range equation.Operands {
			roleOffset, roleLength := appendBytes([]byte(operand.Role))
			valueOffset, valueLength := appendBytes(operand.Term.Encoding)
			kind := OperandCanonicalConstant
			if operand.Term.Entry {
				kind = OperandEntryProjection
			}
			result.operands = append(result.operands, CompiledOperand{Kind: kind, RoleOffset: roleOffset, RoleLength: roleLength, ValueOffset: valueOffset, ValueLength: valueLength})
		}
		for _, guard := range equation.Guards {
			offset, length := appendBytes(guard.Encoding)
			result.guards = append(result.guards, compiledRange{offset: offset, length: length})
		}
		for _, dependency := range equation.Dependencies {
			index, ok := byTarget[dependency]
			if !ok {
				return CompiledArtifact{}, fmt.Errorf("equation: compiled dependency %s has no operation", dependency.Name)
			}
			result.deps = append(result.deps, index)
		}
		op.OperandCount = uint32(len(result.operands)) - op.OperandStart
		op.GuardCount = uint32(len(result.guards)) - op.GuardStart
		op.DependencyCount = uint32(len(result.deps)) - op.DependencyStart
		result.ops = append(result.ops, op)
	}
	result.layout = CompiledCellLayout{OperationCount: uint32(len(result.ops)), OperandCount: uint32(len(result.operands)), GuardCount: uint32(len(result.guards))}
	if reconstructed, err := result.ReferenceArtifact(); err != nil || !bytes.Equal(reconstructed.CanonicalBytes(), result.canonical) {
		return CompiledArtifact{}, fmt.Errorf("equation: compiled admission changed canonical artifact")
	}
	return result, nil
}

func compiledAcyclicOrder(artifact Artifact) ([]Equation, BodyID, EntryParameter, error) {
	canonical := artifact.CanonicalBytes()
	if canonical == nil || len(artifact.Equations) == 0 {
		return nil, BodyID{}, EntryParameter{}, fmt.Errorf("equation: invalid acyclic artifact for compiled admission")
	}
	equations := append([]Equation(nil), artifact.Equations...)
	for index := range equations {
		canonical, err := canonicalEquation(equations[index])
		if err != nil {
			return nil, BodyID{}, EntryParameter{}, err
		}
		equations[index] = canonical
	}
	body, entry := equations[0].Target.Body, equations[0].Entry
	byTarget := make(map[Coordinate]Equation, len(equations))
	dependents := make(map[Coordinate][]Coordinate, len(equations))
	degree := make(map[Coordinate]int, len(equations))
	for _, equation := range equations {
		if equation.Target.Body != body || equation.Entry != entry {
			return nil, BodyID{}, EntryParameter{}, fmt.Errorf("equation: compiled artifact mixes bodies or entry parameters")
		}
		if _, duplicate := byTarget[equation.Target]; duplicate {
			return nil, BodyID{}, EntryParameter{}, fmt.Errorf("equation: compiled artifact has duplicate target %s", equation.Target.Name)
		}
		byTarget[equation.Target], degree[equation.Target] = equation, len(equation.Dependencies)
	}
	for _, equation := range equations {
		for _, dependency := range equation.Dependencies {
			if _, found := byTarget[dependency]; !found {
				return nil, BodyID{}, EntryParameter{}, fmt.Errorf("equation: compiled dependency %s has no equation", dependency.Name)
			}
			dependents[dependency] = append(dependents[dependency], equation.Target)
		}
	}
	ready := make([]Coordinate, 0, len(equations))
	for target, count := range degree {
		if count == 0 {
			ready = append(ready, target)
		}
	}
	ordered := make([]Equation, 0, len(equations))
	for len(ready) != 0 {
		sort.Slice(ready, func(i, j int) bool { return ready[i].less(ready[j]) })
		target := ready[0]
		ready = ready[1:]
		ordered = append(ordered, byTarget[target])
		for _, dependent := range dependents[target] {
			degree[dependent]--
			if degree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	if len(ordered) != len(equations) {
		return nil, BodyID{}, EntryParameter{}, fmt.Errorf("equation: compiled artifact is cyclic")
	}
	return ordered, body, entry, nil
}

// ReferenceArtifact reconstructs the exact equation artifact from the compact
// plan.  It is intentionally used only by the differential harness until the
// generated opcode executor lands in the next slice.
func (a CompiledArtifact) ReferenceArtifact() (Artifact, error) {
	if !a.id.Valid() || !a.body.Valid() || a.entryName == "" || len(a.ops) == 0 {
		return Artifact{}, fmt.Errorf("equation: invalid compiled artifact")
	}
	entry := EntryParameter{Body: a.body, Name: a.entryName}
	targets := make([]Coordinate, len(a.ops))
	for index, op := range a.ops {
		name := string(a.rangeBytes(op.TargetOffset, op.TargetLength))
		if name == "" {
			return Artifact{}, fmt.Errorf("equation: compiled operation has no target")
		}
		targets[index] = Coordinate{Body: a.body, Name: name}
	}
	equations := make([]Equation, 0, len(a.ops))
	for _, op := range a.ops {
		endOperands := uint64(op.OperandStart) + uint64(op.OperandCount)
		endGuards := uint64(op.GuardStart) + uint64(op.GuardCount)
		endDependencies := uint64(op.DependencyStart) + uint64(op.DependencyCount)
		if endOperands > uint64(len(a.operands)) || endGuards > uint64(len(a.guards)) || endDependencies > uint64(len(a.deps)) {
			return Artifact{}, fmt.Errorf("equation: compiled operation range is invalid")
		}
		equation := Equation{Target: Coordinate{Body: a.body, Name: string(a.rangeBytes(op.TargetOffset, op.TargetLength))}, Entry: entry,
			Occurrence: Occurrence{Kind: string(a.rangeBytes(op.OccurrenceOffset, op.OccurrenceLength)), ContractID: op.ContractID}, KernelID: string(a.rangeBytes(op.KernelOffset, op.KernelLength))}
		for _, guard := range a.guards[op.GuardStart:endGuards] {
			equation.Guards = append(equation.Guards, Guard{Body: a.body, Encoding: a.rangeBytes(guard.offset, guard.length)})
		}
		for _, operand := range a.operands[op.OperandStart:endOperands] {
			if operand.Kind != OperandCanonicalConstant && operand.Kind != OperandEntryProjection {
				return Artifact{}, fmt.Errorf("equation: compiled operand kind %d cannot be reconstructed", operand.Kind)
			}
			equation.Operands = append(equation.Operands, Operand{Role: string(a.rangeBytes(operand.RoleOffset, operand.RoleLength)), Term: Term{Encoding: a.rangeBytes(operand.ValueOffset, operand.ValueLength), Entry: operand.Kind == OperandEntryProjection}})
		}
		for _, dependency := range a.deps[op.DependencyStart:endDependencies] {
			if dependency >= uint32(len(targets)) {
				return Artifact{}, fmt.Errorf("equation: compiled dependency index is invalid")
			}
			equation.Dependencies = append(equation.Dependencies, targets[dependency])
		}
		equations = append(equations, equation)
	}
	artifact := Artifact{Equations: equations}
	if canonical := artifact.CanonicalBytes(); canonical == nil || !bytes.Equal(canonical, a.canonical) || artifact.ContentID() != a.id {
		return Artifact{}, fmt.Errorf("equation: compiled artifact canonical confirmation failed")
	}
	return artifact, nil
}

// CompiledDifferentialCase compares the retained VM input with the compact
// plan reconstructed VM input.  It is the admission gate for the generated
// evaluator lane: later executors plug into this same closure-equality oracle.
type CompiledDifferentialCase struct {
	Name     string
	Artifact Artifact
	Compiled CompiledArtifact
	Entry    EntryBinding
}

func RunCompiledDifferential(vm *AcyclicVM, cases []CompiledDifferentialCase) (ShadowReport, error) {
	report := ShadowReport{Cases: len(cases)}
	for _, test := range cases {
		if test.Name == "" {
			return report, fmt.Errorf("equation: malformed compiled differential case")
		}
		if test.Artifact.ContentID() != test.Compiled.ContentID() || !bytes.Equal(test.Artifact.CanonicalBytes(), test.Compiled.CanonicalBytes()) {
			return report, fmt.Errorf("equation: compiled differential %s artifact identity differs", test.Name)
		}
		reference, err := BindEntry(test.Artifact, test.Entry)
		if err != nil {
			return report, fmt.Errorf("equation: compiled differential %s reference binding: %w", test.Name, err)
		}
		compiledArtifact, err := test.Compiled.ReferenceArtifact()
		if err != nil {
			return report, fmt.Errorf("equation: compiled differential %s plan: %w", test.Name, err)
		}
		bound, err := BindEntry(compiledArtifact, test.Entry)
		if err != nil {
			return report, fmt.Errorf("equation: compiled differential %s compiled binding: %w", test.Name, err)
		}
		want, err := vm.Evaluate(reference)
		if err != nil {
			return report, fmt.Errorf("equation: compiled differential %s reference evaluation: %w", test.Name, err)
		}
		got, err := vm.Evaluate(bound)
		if err != nil {
			return report, fmt.Errorf("equation: compiled differential %s compiled evaluation: %w", test.Name, err)
		}
		if !want.Closure.Equal(got.Closure) || want.Transactions != got.Transactions {
			return report, fmt.Errorf("equation: compiled differential %s published output differs", test.Name)
		}
		report.Passed++
	}
	return report, nil
}

// CompiledCyclicArtifact records the compact operation schema alongside the
// already-frozen WTO certificate.  This slice does not re-plan a cyclic body:
// it preserves the certificate for the cyclic differential gate and reserves
// compiled execution for the generated-stencil slice.
type CompiledCyclicArtifact struct {
	Artifact CompiledArtifact
	blocks   []CompiledWTOBlock
	frozen   CyclicArtifact
}

type CompiledWTOBlock struct {
	Operation              uint32
	ChildStart, ChildCount uint32
}

func (a CompiledCyclicArtifact) Blocks() []CompiledWTOBlock {
	return append([]CompiledWTOBlock(nil), a.blocks...)
}

func CompileCyclicArtifact(artifact CyclicArtifact) (CompiledCyclicArtifact, error) {
	if artifact.CanonicalBytes() == nil || artifact.Plan == nil {
		return CompiledCyclicArtifact{}, fmt.Errorf("equation: invalid cyclic artifact for compiled admission")
	}
	equations := append([]Equation(nil), artifact.Artifact.Equations...)
	if len(equations) == 0 {
		return CompiledCyclicArtifact{}, fmt.Errorf("equation: empty cyclic artifact")
	}
	body, entry := equations[0].Target.Body, equations[0].Entry
	for index := range equations {
		canonical, err := canonicalEquation(equations[index])
		if err != nil {
			return CompiledCyclicArtifact{}, err
		}
		equations[index] = canonical
		if equations[index].Target.Body != body || equations[index].Entry != entry {
			return CompiledCyclicArtifact{}, fmt.Errorf("equation: compiled cyclic artifact mixes bodies or entry parameters")
		}
	}
	sort.Slice(equations, func(i, j int) bool { return equations[i].Target.less(equations[j].Target) })
	compiled, err := compileArtifact(artifact.Artifact, equations, body, entry)
	if err != nil {
		return CompiledCyclicArtifact{}, err
	}
	opForCell := make(map[CellID]uint32, len(artifact.CellForTarget))
	for index, equation := range equations {
		opForCell[artifact.CellForTarget[equation.Target]] = uint32(index)
	}
	blocks := make([]CompiledWTOBlock, 0, len(equations))
	var appendPlan func([]solve.WTOElement[CellID]) (uint32, uint32, error)
	appendPlan = func(elements []solve.WTOElement[CellID]) (uint32, uint32, error) {
		start := uint32(len(blocks))
		for _, element := range elements {
			op, ok := opForCell[element.Vertex]
			if !ok {
				return 0, 0, fmt.Errorf("equation: compiled cyclic plan has unknown cell %q", element.Vertex)
			}
			index := uint32(len(blocks))
			blocks = append(blocks, CompiledWTOBlock{Operation: op})
			childStart, childCount, err := appendPlan(element.Body)
			if err != nil {
				return 0, 0, err
			}
			blocks[index].ChildStart, blocks[index].ChildCount = childStart, childCount
		}
		return start, uint32(len(blocks)) - start, nil
	}
	if _, _, err := appendPlan(artifact.Plan.Elements()); err != nil {
		return CompiledCyclicArtifact{}, err
	}
	canonical, err := NewCyclicArtifact(artifact.Artifact, artifact.CellForTarget, artifact.Plan, artifact.Dependencies, artifact.Selectors, artifact.ParameterCells, artifact.WidenCells)
	if err != nil {
		return CompiledCyclicArtifact{}, err
	}
	return CompiledCyclicArtifact{Artifact: compiled, blocks: blocks, frozen: canonical}, nil
}

// RunCompiledCyclicDifferential preserves both oracle observations: complete
// closure equality and the exact widening trace.  The reference VM remains
// the sole execution authority until cyclic generated stencils land.
func RunCompiledCyclicDifferential(ctx context.Context, vm *CyclicVM, artifact CyclicArtifact, compiled CompiledCyclicArtifact, entry EntryBinding, selectors []string) error {
	if artifact.ContentID() != compiled.frozen.ContentID() || !bytes.Equal(artifact.CanonicalBytes(), compiled.frozen.CanonicalBytes()) {
		return fmt.Errorf("equation: compiled cyclic differential artifact identity differs")
	}
	wantBound, err := BindCyclicEntry(artifact, entry)
	if err != nil {
		return err
	}
	gotBound, err := BindCyclicEntry(compiled.frozen, entry)
	if err != nil {
		return err
	}
	want, err := vm.Evaluate(ctx, wantBound, selectors)
	if err != nil {
		return err
	}
	got, err := vm.Evaluate(ctx, gotBound, selectors)
	if err != nil {
		return err
	}
	if !want.Closure.Equal(got.Closure) || want.Transactions != got.Transactions || len(want.WideningTrace) != len(got.WideningTrace) {
		return fmt.Errorf("equation: compiled cyclic differential differs")
	}
	for index := range want.WideningTrace {
		if !want.WideningTrace[index].Equal(got.WideningTrace[index]) {
			return fmt.Errorf("equation: compiled cyclic widening trace differs")
		}
	}
	return nil
}
