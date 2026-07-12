package primitive

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/semantic/transaction"
)

type Builder struct {
	programs   map[string]ProgramDescriptor
	intrinsics map[string]IntrinsicDescriptor
	bindings   map[string]NativeBinding
	coverage   []Coverage
	sealed     bool
}

func NewBuilder() *Builder {
	return &Builder{
		programs:   make(map[string]ProgramDescriptor),
		intrinsics: make(map[string]IntrinsicDescriptor),
		bindings:   make(map[string]NativeBinding),
	}
}

func (b *Builder) AddProgram(program ProgramDescriptor) error {
	if err := b.mutable(); err != nil {
		return err
	}
	if err := validateID("program", program.ID); err != nil {
		return err
	}
	if program.SchemaVersion == 0 {
		return invalid("program "+program.ID, errors.New("schema version must be non-zero"))
	}
	if len(program.Steps) == 0 {
		return invalid("program "+program.ID, errors.New("steps must not be empty"))
	}
	if _, duplicate := b.programs[program.ID]; duplicate {
		return invalid("program", fmt.Errorf("duplicate id %q", program.ID))
	}
	for index, step := range program.Steps {
		switch step.kind {
		case StepTransaction:
			if len(step.transaction.CanonicalBytes()) == 0 {
				return invalid("program "+program.ID, fmt.Errorf("step %d has an empty transaction", index))
			}
		case StepIntrinsicCall:
			if err := validateID("intrinsic call", step.intrinsic.ID); err != nil || step.intrinsic.SchemaVersion == 0 {
				return invalid("program "+program.ID, fmt.Errorf("step %d has an invalid intrinsic call", index))
			}
		default:
			return invalid("program "+program.ID, fmt.Errorf("step %d has unknown kind %d", index, step.kind))
		}
	}
	b.programs[program.ID] = program.copy()
	return nil
}

func (b *Builder) AddIntrinsic(descriptor IntrinsicDescriptor) error {
	if err := b.mutable(); err != nil {
		return err
	}
	if err := validateID("intrinsic", descriptor.ID); err != nil {
		return err
	}
	if descriptor.SchemaVersion == 0 {
		return invalid("intrinsic "+descriptor.ID, errors.New("schema version must be non-zero"))
	}
	if _, duplicate := b.intrinsics[descriptor.ID]; duplicate {
		return invalid("intrinsic", fmt.Errorf("duplicate id %q", descriptor.ID))
	}
	b.intrinsics[descriptor.ID] = descriptor
	return nil
}

func (b *Builder) BindIntrinsic(binding NativeBinding) error {
	if err := b.mutable(); err != nil {
		return err
	}
	if binding.id == "" || binding.schemaVersion == 0 || binding.implementationID == "" || binding.invoke == nil {
		return invalid("native binding", errors.New("binding was not constructed by NewNativeBinding"))
	}
	if _, duplicate := b.bindings[binding.id]; duplicate {
		return invalid("native binding", fmt.Errorf("duplicate id %q", binding.id))
	}
	b.bindings[binding.id] = binding
	return nil
}

func (b *Builder) AddCoverage(coverage Coverage) error {
	if err := b.mutable(); err != nil {
		return err
	}
	if err := validateID("coverage program", coverage.ProgramID); err != nil {
		return err
	}
	if coverage.LeafID == "" {
		return invalid("coverage", errors.New("leaf id must not be empty"))
	}
	if !coverage.Role.valid() {
		return invalid("coverage", fmt.Errorf("invalid role %d", coverage.Role))
	}
	b.coverage = append(b.coverage, coverage)
	return nil
}

type Registry struct {
	programs     []ProgramDescriptor
	programByID  map[string]ProgramDescriptor
	intrinsics   []IntrinsicDescriptor
	bindings     map[string]NativeBinding
	coverage     []Coverage
	capabilities map[string][]transaction.Capability
	leaves       map[string][]Leaf
	canonical    []byte
	digest       [sha256.Size]byte
}

func (b *Builder) Seal() (Registry, error) {
	if err := b.mutable(); err != nil {
		return Registry{}, err
	}
	b.sealed = true

	programs := make([]ProgramDescriptor, 0, len(b.programs))
	for _, program := range b.programs {
		programs = append(programs, program.copy())
	}
	sort.Slice(programs, func(left, right int) bool { return programs[left].ID < programs[right].ID })
	intrinsics := make([]IntrinsicDescriptor, 0, len(b.intrinsics))
	for _, descriptor := range b.intrinsics {
		intrinsics = append(intrinsics, descriptor)
	}
	sort.Slice(intrinsics, func(left, right int) bool { return intrinsics[left].ID < intrinsics[right].ID })

	for _, descriptor := range intrinsics {
		binding, ok := b.bindings[descriptor.ID]
		if !ok {
			return Registry{}, invalid("seal", fmt.Errorf("intrinsic %q has no native binding", descriptor.ID))
		}
		if binding.schemaVersion != descriptor.SchemaVersion {
			return Registry{}, invalid("seal", fmt.Errorf("intrinsic %q schema version %d has binding version %d", descriptor.ID, descriptor.SchemaVersion, binding.schemaVersion))
		}
	}
	for id := range b.bindings {
		if _, ok := b.intrinsics[id]; !ok {
			return Registry{}, invalid("seal", fmt.Errorf("native binding %q has no intrinsic descriptor", id))
		}
	}
	for _, program := range programs {
		for index, step := range program.Steps {
			if step.kind != StepIntrinsicCall {
				continue
			}
			descriptor, ok := b.intrinsics[step.intrinsic.ID]
			if !ok {
				return Registry{}, invalid("seal", fmt.Errorf("program %q step %d references missing intrinsic %q", program.ID, index, step.intrinsic.ID))
			}
			if descriptor.SchemaVersion != step.intrinsic.SchemaVersion {
				return Registry{}, invalid("seal", fmt.Errorf("program %q step %d intrinsic %q has schema version %d, want %d", program.ID, index, descriptor.ID, step.intrinsic.SchemaVersion, descriptor.SchemaVersion))
			}
		}
	}

	capabilities := make(map[string][]transaction.Capability, len(programs))
	leaves := make(map[string][]Leaf, len(programs))
	for _, program := range programs {
		byID := make(map[string]transaction.Capability)
		leafByID := make(map[string]Leaf)
		for _, step := range program.Steps {
			if step.kind != StepTransaction {
				continue
			}
			for _, capability := range step.transaction.Capabilities() {
				if previous, exists := byID[capability.ID]; exists && previous.Kind != capability.Kind {
					return Registry{}, invalid("seal", fmt.Errorf("program %q capability %q has conflicting kinds", program.ID, capability.ID))
				}
				byID[capability.ID] = capability
			}
			for _, slot := range step.transaction.Slots() {
				leaf := leafForSlot(slot)
				if previous, exists := leafByID[leaf.ID]; exists && previous != leaf {
					return Registry{}, invalid("seal", fmt.Errorf("program %q leaf %q has conflicting descriptors", program.ID, leaf.ID))
				}
				leafByID[leaf.ID] = leaf
			}
		}
		derived := make([]transaction.Capability, 0, len(byID))
		for _, capability := range byID {
			derived = append(derived, capability)
		}
		sort.Slice(derived, func(left, right int) bool { return derived[left].ID < derived[right].ID })
		capabilities[program.ID] = derived
		derivedLeaves := make([]Leaf, 0, len(leafByID))
		for _, leaf := range leafByID {
			derivedLeaves = append(derivedLeaves, leaf)
		}
		sort.Slice(derivedLeaves, func(left, right int) bool { return derivedLeaves[left].ID < derivedLeaves[right].ID })
		leaves[program.ID] = derivedLeaves
	}

	coverage := append([]Coverage(nil), b.coverage...)
	sort.Slice(coverage, func(left, right int) bool {
		if coverage[left].ProgramID != coverage[right].ProgramID {
			return coverage[left].ProgramID < coverage[right].ProgramID
		}
		if coverage[left].LeafID != coverage[right].LeafID {
			return coverage[left].LeafID < coverage[right].LeafID
		}
		return coverage[left].Role < coverage[right].Role
	})
	if err := verifyCoverage(programs, leaves, coverage); err != nil {
		return Registry{}, err
	}

	registry := Registry{
		programs:     programs,
		programByID:  make(map[string]ProgramDescriptor, len(programs)),
		intrinsics:   intrinsics,
		bindings:     make(map[string]NativeBinding, len(b.bindings)),
		coverage:     coverage,
		capabilities: capabilities,
		leaves:       leaves,
	}
	for _, program := range programs {
		registry.programByID[program.ID] = program.copy()
	}
	for id, binding := range b.bindings {
		registry.bindings[id] = binding
	}
	canonical, err := encodeCanonical(registry)
	if err != nil {
		return Registry{}, err
	}
	registry.canonical = canonical
	registry.digest = sha256.Sum256(canonical)
	return registry, nil
}

func verifyCoverage(programs []ProgramDescriptor, leaves map[string][]Leaf, coverage []Coverage) error {
	expected := make(map[string]struct{})
	for _, program := range programs {
		for _, leaf := range leaves[program.ID] {
			for _, role := range requiredCoverageRoles {
				expected[coverageKey(program.ID, leaf.ID, role)] = struct{}{}
			}
		}
	}
	seen := make(map[string]struct{}, len(coverage))
	for _, row := range coverage {
		key := coverageKey(row.ProgramID, row.LeafID, row.Role)
		if _, duplicate := seen[key]; duplicate {
			return invalid("coverage", fmt.Errorf("duplicate row for program %q leaf %q role %d", row.ProgramID, row.LeafID, row.Role))
		}
		seen[key] = struct{}{}
		if _, ok := expected[key]; !ok {
			return invalid("coverage", fmt.Errorf("row for program %q leaf %q role %d is not verifier-derived", row.ProgramID, row.LeafID, row.Role))
		}
	}
	for key := range expected {
		if _, ok := seen[key]; !ok {
			return invalid("coverage", fmt.Errorf("missing row %q", key))
		}
	}
	return nil
}

func coverageKey(programID, leafID string, role CoverageRole) string {
	return fmt.Sprintf("%s/%s/%d", programID, leafID, role)
}

func (b *Builder) mutable() error {
	if b == nil || b.programs == nil {
		return invalid("builder", errors.New("nil or zero builder"))
	}
	if b.sealed {
		return ErrSealed
	}
	return nil
}

func (r Registry) Programs() []ProgramDescriptor {
	out := make([]ProgramDescriptor, len(r.programs))
	for index, program := range r.programs {
		out[index] = program.copy()
	}
	return out
}

func (r Registry) Program(id string) (ProgramDescriptor, bool) {
	program, ok := r.programByID[id]
	return program.copy(), ok
}

func (r Registry) Capabilities(programID string) []transaction.Capability {
	return append([]transaction.Capability(nil), r.capabilities[programID]...)
}

func (r Registry) Leaves(programID string) []Leaf {
	return append([]Leaf(nil), r.leaves[programID]...)
}

func (r Registry) CanonicalBytes() []byte    { return append([]byte(nil), r.canonical...) }
func (r Registry) Digest() [sha256.Size]byte { return r.digest }

// InvokeIntrinsic resolves both direct concrete execution and circuit
// specialization through the same sealed native binding.
func (r Registry) InvokeIntrinsic(call IntrinsicCall, input NativeInput) (NativeOutput, error) {
	binding, ok := r.bindings[call.ID]
	if !ok || binding.schemaVersion != call.SchemaVersion {
		return NativeOutput{}, invalid("invoke", fmt.Errorf("intrinsic %q version %d is not sealed", call.ID, call.SchemaVersion))
	}
	input.CallPayload = append([]byte(nil), call.Payload...)
	input.Payload = append([]byte(nil), input.Payload...)
	output, err := binding.invoke(input)
	if err != nil {
		return NativeOutput{}, err
	}
	output.Payload = append([]byte(nil), output.Payload...)
	return output, nil
}

func (r Registry) Equal(other Registry) bool {
	return bytes.Equal(r.canonical, other.canonical)
}
