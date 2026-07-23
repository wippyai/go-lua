package equation

import (
	"errors"
	"fmt"
	"sort"
)

// FrozenKinds is duplicated deliberately as stable wire vocabulary, not as an
// import of transformer.  Transformer owns the catalog; this list lets the
// lowerer frame reject a missing hook before it could silently skip a family.
var FrozenKinds = []string{
	"apply", "path-replacement", "dynamic-index-read", "path-invalidation", "index-mutation",
	"allocation-template", "object-materialization", "environment-write",
	"channel-select", "branch-relations", "call-results", "presence-implications",
	"loop-control", "generic-for", "root-assignment", "covariant-exposure",
	"contribution", "external-call", "outcome", "nonreturning", "definition",
	"resource", "entry", "publication", "expression",
	"claim",
}

var ErrUnimplementedLowering = errors.New("equation: lowering is not implemented")

// Draft is the relation-program projection passed to a family lowerer.  It
// deliberately has no State field.  Terms and guards were sealed by the
// source program before this compiler was called.
type Draft struct {
	Target       Coordinate
	Entry        EntryParameter
	Guards       []Guard
	Dependencies []Coordinate
	Occurrence   Occurrence
	Operands     []Operand
}

func (d Draft) valid() error {
	probe := Equation{Target: d.Target, Entry: d.Entry, Guards: d.Guards, Dependencies: d.Dependencies, Occurrence: d.Occurrence, Operands: d.Operands, KernelID: "draft"}
	return probe.valid()
}

// Source is the immutable relation-program projection.  A source is a walk
// result, rather than a second relation syntax or a transfer interpreter.
type Source struct{ Drafts []Draft }

// Lowerer binds one already-existing transfer kernel.  It must not implement
// a transfer itself: KernelID identifies the source-owned canonical kernel.
type Lowerer interface{ Lower(Draft) (Equation, error) }

type LowererFunc func(Draft) (Equation, error)

func (f LowererFunc) Lower(d Draft) (Equation, error) { return f(d) }

// Compiler has one explicit hook for every frozen catalog kind.  A nil hook
// is invalid; unimplemented hooks fail rather than allowing an occurrence to
// disappear from a parameterized artifact.
type Compiler struct{ hooks map[string]Lowerer }

func NewCompiler(hooks map[string]Lowerer) (*Compiler, error) {
	if len(hooks) != len(FrozenKinds) {
		return nil, fmt.Errorf("equation: lowering hook count %d, want %d", len(hooks), len(FrozenKinds))
	}
	copy := make(map[string]Lowerer, len(hooks))
	for _, kind := range FrozenKinds {
		hook, present := hooks[kind]
		if !present || hook == nil {
			return nil, fmt.Errorf("equation: missing lowering hook %q", kind)
		}
		copy[kind] = hook
	}
	for kind := range hooks {
		if !knownKind(kind) {
			return nil, fmt.Errorf("equation: unknown lowering hook %q", kind)
		}
	}
	return &Compiler{hooks: copy}, nil
}

func knownKind(kind string) bool {
	for _, candidate := range FrozenKinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

// Skeleton returns the complete frame with a fail-closed hook for every kind.
func Skeleton() *Compiler {
	hooks := make(map[string]Lowerer, len(FrozenKinds))
	for _, kind := range FrozenKinds {
		kind := kind
		hooks[kind] = LowererFunc(func(Draft) (Equation, error) { return Equation{}, fmt.Errorf("%w: %s", ErrUnimplementedLowering, kind) })
	}
	compiler, err := NewCompiler(hooks)
	if err != nil {
		panic(err)
	}
	return compiler
}

// With returns a copied compiler with exactly one hook replaced.
func (c *Compiler) With(kind string, hook Lowerer) (*Compiler, error) {
	if c == nil || !knownKind(kind) || hook == nil {
		return nil, fmt.Errorf("equation: invalid lowering replacement %q", kind)
	}
	hooks := make(map[string]Lowerer, len(c.hooks))
	for key, value := range c.hooks {
		hooks[key] = value
	}
	hooks[kind] = hook
	return NewCompiler(hooks)
}

// Compile walks every sealed relation occurrence once and emits only complete
// lowered equations.  A transaction either yields its complete equation or no
// artifact at all; in particular, it cannot publish a predecessor equation.
func (c *Compiler) Compile(source Source) (Artifact, error) {
	if c == nil {
		return Artifact{}, fmt.Errorf("equation: nil compiler")
	}
	equations := make([]Equation, 0, len(source.Drafts))
	for index, draft := range source.Drafts {
		if err := draft.valid(); err != nil {
			return Artifact{}, fmt.Errorf("equation: draft %d: %w", index, err)
		}
		hook, present := c.hooks[draft.Occurrence.Kind]
		if !present {
			return Artifact{}, fmt.Errorf("equation: draft %d has unknown kind %q", index, draft.Occurrence.Kind)
		}
		lowered, err := hook.Lower(draft)
		if err != nil {
			return Artifact{}, fmt.Errorf("equation: lower %s at %s: %w", draft.Occurrence.Kind, draft.Target.Name, err)
		}
		if lowered.Target != draft.Target || lowered.Entry != draft.Entry || lowered.Occurrence != draft.Occurrence || !sameCoordinates(lowered.Dependencies, draft.Dependencies) || lowered.KernelID == "" {
			return Artifact{}, fmt.Errorf("equation: lowering %q changed occurrence identity or omitted its kernel", draft.Occurrence.Kind)
		}
		if _, err := canonicalEquation(lowered); err != nil {
			return Artifact{}, fmt.Errorf("equation: lowered %q: %w", draft.Occurrence.Kind, err)
		}
		equations = append(equations, lowered)
	}
	artifact := Artifact{Equations: equations}
	if artifact.CanonicalBytes() == nil {
		return Artifact{}, fmt.Errorf("equation: non-canonical lowering artifact")
	}
	return artifact, nil
}

func sameCoordinates(left, right []Coordinate) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]Coordinate(nil), left...)
	right = append([]Coordinate(nil), right...)
	sort.Slice(left, func(i, j int) bool { return left[i].less(left[j]) })
	sort.Slice(right, func(i, j int) bool { return right[i].less(right[j]) })
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// BindExistingKernel creates a mechanical lowering hook.  The only semantic
// choice is the pre-existing kernel identifier supplied by the owner; this
// helper has no evaluation logic.
func BindExistingKernel(kernelID string) Lowerer {
	return LowererFunc(func(draft Draft) (Equation, error) {
		if kernelID == "" {
			return Equation{}, fmt.Errorf("equation: empty canonical kernel binding")
		}
		return Equation{Target: draft.Target, Entry: draft.Entry, Guards: append([]Guard(nil), draft.Guards...), Dependencies: append([]Coordinate(nil), draft.Dependencies...), Occurrence: draft.Occurrence, Operands: append([]Operand(nil), draft.Operands...), KernelID: kernelID}, nil
	})
}
