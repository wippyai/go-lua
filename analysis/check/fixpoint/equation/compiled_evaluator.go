package equation

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/evalscratch"
)

// ErrEvaluatorScratchOverflow means that a caller attempted a nested
// evaluation beyond the frames it provisioned.  It is deliberately explicit:
// growing scratch while an evaluation is live would hide a cold allocation in
// the normal execution path and could retain caller-owned entry bytes.
var ErrEvaluatorScratchOverflow = errors.New("equation: compiled evaluator scratch overflow")

// EvaluatorScratch is worker-owned storage for compiled evaluation.  Cells are
// scalar operation slots; the other rows are a bounded adapter for the
// existing kernel boundary.  They are allocated when the worker is created,
// never while a normal evaluation is executing.
//
// A scratch instance is intentionally not safe for concurrent use.  Give each
// worker its own instance.  Nested calls consume another preallocated frame.
type EvaluatorScratch struct {
	frames []compiledEvaluatorFrame
	depth  evalscratch.Depth
}

type compiledEvaluatorFrame struct {
	cells        []uint32
	operands     []BoundOperand
	guards       []Guard
	dependencies []Coordinate
	// view is the partition read index this frame lends to each transaction.
	// It is bound to one snapshot at a time, exactly like the operand and guard
	// rows above, and is released with them.
	view partitionView
}

// NewEvaluatorScratch provisions four re-entrant frames for artifact.  The
// plan supplies all normal-path capacities, so this is the only allocation
// point for the evaluator's transient storage.
func NewEvaluatorScratch(artifact CompiledArtifact) (*EvaluatorScratch, error) {
	if err := artifact.validCompiledPlan(); err != nil {
		return nil, err
	}
	maxOperands, maxGuards, maxDependencies := artifact.maxFrameWidths()
	frames := make([]compiledEvaluatorFrame, 4)
	for index := range frames {
		frames[index] = compiledEvaluatorFrame{
			cells:        make([]uint32, artifact.layout.OperationCount),
			operands:     make([]BoundOperand, maxOperands),
			guards:       make([]Guard, maxGuards),
			dependencies: make([]Coordinate, maxDependencies),
		}
	}
	return &EvaluatorScratch{frames: frames, depth: evalscratch.NewDepth(len(frames))}, nil
}

// OverflowCount reports attempts to exceed the provisioned nesting depth.
// It is telemetry for a cold configuration error, not an estimate.
func (s *EvaluatorScratch) OverflowCount() uint64 {
	if s == nil {
		return 0
	}
	return s.depth.OverflowCount()
}

func (s *EvaluatorScratch) acquire(artifact CompiledArtifact) (*compiledEvaluatorFrame, error) {
	if s == nil {
		return nil, ErrEvaluatorScratchOverflow
	}
	index, ok := s.depth.Push()
	if !ok || int(index) >= len(s.frames) {
		return nil, ErrEvaluatorScratchOverflow
	}
	frame := &s.frames[index]
	if uint32(len(frame.cells)) < artifact.layout.OperationCount {
		s.depth.Pop()
		s.depth.Reject()
		return nil, ErrEvaluatorScratchOverflow
	}
	return frame, nil
}

func (s *EvaluatorScratch) release(frame *compiledEvaluatorFrame) {
	// These rows carry slice and string headers.  Clearing them before reuse
	// prevents a worker scratch from retaining entry-owned bytes or kernel data.
	for index := range frame.operands {
		frame.operands[index] = BoundOperand{}
	}
	for index := range frame.guards {
		frame.guards[index] = Guard{}
	}
	for index := range frame.dependencies {
		frame.dependencies[index] = Coordinate{}
	}
	for index := range frame.cells {
		frame.cells[index] = 0
	}
	frame.view.clear()
	s.depth.Pop()
}

// FastEvaluator executes an admitted CompiledArtifact directly from its
// scalar byte arena.  It keeps the AcyclicVM's registry and transaction rules
// as the semantic authority; only binding and scheduling are compiled.
type FastEvaluator struct{ vm *AcyclicVM }

func NewFastEvaluator(vm *AcyclicVM) (*FastEvaluator, error) {
	if vm == nil || vm.registry == nil {
		return nil, fmt.Errorf("equation: nil acyclic VM")
	}
	return &FastEvaluator{vm: vm}, nil
}

// Evaluate runs artifact in its admission-frozen topological order.  It does
// not reconstruct an Artifact or call BindEntry: the only entry substitution
// is the direct slice assignment for OperandEntryProjection.
func (e *FastEvaluator) Evaluate(artifact CompiledArtifact, entry EntryBinding, scratch *EvaluatorScratch) (evaluation Evaluation, err error) {
	if e == nil || e.vm == nil || e.vm.registry == nil || !entry.valid() || entry.Parameter.Body != artifact.body || entry.Parameter.Name != artifact.entryName {
		return Evaluation{}, fmt.Errorf("equation: invalid compiled evaluation input")
	}
	if err := artifact.validCompiledPlan(); err != nil {
		return Evaluation{}, err
	}
	frame, err := scratch.acquire(artifact)
	if err != nil {
		return Evaluation{}, err
	}
	defer scratch.release(frame)

	closure := OutputClosure{}
	for _, block := range artifact.blocks {
		end := uint64(block.OpStart) + uint64(block.OpCount)
		if end > uint64(len(artifact.ops)) {
			return Evaluation{}, fmt.Errorf("equation: compiled block range is invalid")
		}
		for operation := block.OpStart; operation < uint32(end); operation++ {
			op := artifact.ops[operation]
			equation, buildErr := artifact.bindOperation(op, entry, frame)
			if buildErr != nil {
				return Evaluation{}, buildErr
			}
			binding, found := e.vm.registry.resolve(equation)
			if !found {
				return Evaluation{}, fmt.Errorf("equation: no contract-bound kernel for %s", equation.Target.Name)
			}
			frame.view.reset(closure, equation.Guards)
			result, executeErr := binding.Kernel.Execute(equation, Partition{closure: closure, guards: equation.Guards, shared: &frame.view})
			if executeErr != nil {
				return Evaluation{}, fmt.Errorf("equation: transaction %s: %w", equation.Target.Name, executeErr)
			}
			if !result.Complete {
				return Evaluation{}, fmt.Errorf("equation: transaction %s: %w", equation.Target.Name, ErrIncompleteTransaction)
			}
			if binding.Verify != nil {
				if verifyErr := binding.Verify(result.Access); verifyErr != nil {
					return Evaluation{}, fmt.Errorf("equation: transaction %s access audit: %w", equation.Target.Name, verifyErr)
				}
			}
			closure, err = joinClosure(closure, stampClosure(result.Closure, equation.Guards))
			if err != nil {
				return Evaluation{}, fmt.Errorf("equation: transaction %s output: %w", equation.Target.Name, err)
			}
			frame.cells[operation] = operation + 1
		}
	}
	return Evaluation{Closure: closure, Transactions: len(artifact.ops)}, nil
}

// EvaluateCompiled is the direct AcyclicVM adapter for callers which already
// own a worker scratch.  It avoids creating another evaluator object in a hot
// loop while preserving the same implementation used by the differential gate.
func (vm *AcyclicVM) EvaluateCompiled(artifact CompiledArtifact, entry EntryBinding, scratch *EvaluatorScratch) (Evaluation, error) {
	return (&FastEvaluator{vm: vm}).Evaluate(artifact, entry, scratch)
}

func (a CompiledArtifact) maxFrameWidths() (operands, guards, dependencies int) {
	for _, op := range a.ops {
		if int(op.OperandCount) > operands {
			operands = int(op.OperandCount)
		}
		if int(op.GuardCount) > guards {
			guards = int(op.GuardCount)
		}
		if int(op.DependencyCount) > dependencies {
			dependencies = int(op.DependencyCount)
		}
	}
	return operands, guards, dependencies
}

func (a CompiledArtifact) validCompiledPlan() error {
	if !a.id.Valid() || !a.body.Valid() || a.entryName == "" || len(a.ops) == 0 || len(a.blocks) == 0 ||
		a.layout.OperationCount != uint32(len(a.ops)) || a.layout.OperandCount != uint32(len(a.operands)) || a.layout.GuardCount != uint32(len(a.guards)) {
		return fmt.Errorf("equation: invalid compiled artifact")
	}
	nextOperation := uint32(0)
	for _, block := range a.blocks {
		if block.OpCount == 0 || block.OpStart != nextOperation || uint64(block.OpStart)+uint64(block.OpCount) > uint64(len(a.ops)) {
			return fmt.Errorf("equation: invalid compiled block layout")
		}
		nextOperation += block.OpCount
	}
	if nextOperation != uint32(len(a.ops)) {
		return fmt.Errorf("equation: compiled blocks do not cover every operation")
	}
	for _, op := range a.ops {
		if uint64(op.OperandStart)+uint64(op.OperandCount) > uint64(len(a.operands)) ||
			uint64(op.GuardStart)+uint64(op.GuardCount) > uint64(len(a.guards)) ||
			uint64(op.DependencyStart)+uint64(op.DependencyCount) > uint64(len(a.deps)) {
			return fmt.Errorf("equation: compiled operation range is invalid")
		}
	}
	return nil
}

func (a CompiledArtifact) bindOperation(op CompiledOp, entry EntryBinding, frame *compiledEvaluatorFrame) (BoundEquation, error) {
	operandCount, guardCount, dependencyCount := int(op.OperandCount), int(op.GuardCount), int(op.DependencyCount)
	if operandCount > len(frame.operands) || guardCount > len(frame.guards) || dependencyCount > len(frame.dependencies) {
		return BoundEquation{}, ErrEvaluatorScratchOverflow
	}
	operands := frame.operands[:operandCount]
	for index, descriptor := range a.operands[op.OperandStart : op.OperandStart+op.OperandCount] {
		value, ok := a.arenaBytes(descriptor.ValueOffset, descriptor.ValueLength)
		if !ok {
			return BoundEquation{}, fmt.Errorf("equation: compiled operand range is invalid")
		}
		if descriptor.Kind == OperandEntryProjection {
			value = entry.Value
		} else if descriptor.Kind != OperandCanonicalConstant {
			return BoundEquation{}, fmt.Errorf("equation: compiled operand kind %d is not executable", descriptor.Kind)
		}
		role, ok := a.arenaString(descriptor.RoleOffset, descriptor.RoleLength)
		if !ok || role == "" {
			return BoundEquation{}, fmt.Errorf("equation: compiled operand role is invalid")
		}
		operands[index] = BoundOperand{Role: OperandRole(role), Value: value}
	}
	guards := frame.guards[:guardCount]
	for index, descriptor := range a.guards[op.GuardStart : op.GuardStart+op.GuardCount] {
		encoding, ok := a.arenaBytes(descriptor.offset, descriptor.length)
		if !ok {
			return BoundEquation{}, fmt.Errorf("equation: compiled guard range is invalid")
		}
		guards[index] = Guard{Body: a.body, Encoding: encoding}
	}
	dependencies := frame.dependencies[:dependencyCount]
	for index, dependency := range a.deps[op.DependencyStart : op.DependencyStart+op.DependencyCount] {
		if dependency >= uint32(len(a.ops)) {
			return BoundEquation{}, fmt.Errorf("equation: compiled dependency index is invalid")
		}
		name, ok := a.arenaString(a.ops[dependency].TargetOffset, a.ops[dependency].TargetLength)
		if !ok || name == "" {
			return BoundEquation{}, fmt.Errorf("equation: compiled dependency target is invalid")
		}
		dependencies[index] = Coordinate{Body: a.body, Name: name}
	}
	targetName, targetOK := a.arenaString(op.TargetOffset, op.TargetLength)
	kernelID, kernelOK := a.arenaString(op.KernelOffset, op.KernelLength)
	kind, kindOK := a.arenaString(op.OccurrenceOffset, op.OccurrenceLength)
	if !targetOK || !kernelOK || !kindOK || targetName == "" || kernelID == "" || kind == "" || !op.ContractID.Valid() {
		return BoundEquation{}, fmt.Errorf("equation: malformed compiled operation")
	}
	return BoundEquation{
		Target:       Coordinate{Body: a.body, Name: targetName},
		Dependencies: dependencies,
		Occurrence:   Occurrence{Kind: kind, ContractID: op.ContractID},
		KernelID:     kernelID,
		Guards:       guards,
		Operands:     operands,
	}, nil
}

// arenaBytes returns an immutable plan slice for internal execution.  Public
// plan accessors still copy; only this executor can observe the retained arena.
func (a CompiledArtifact) arenaBytes(offset, length uint32) ([]byte, bool) {
	end := uint64(offset) + uint64(length)
	if end > uint64(len(a.bytes)) {
		return nil, false
	}
	return a.bytes[offset:end], true
}

func (a CompiledArtifact) arenaString(offset, length uint32) (string, bool) {
	if _, ok := a.arenaBytes(offset, length); !ok {
		return "", false
	}
	value, ok := a.text[compiledRange{offset: offset, length: length}]
	return value, ok
}
