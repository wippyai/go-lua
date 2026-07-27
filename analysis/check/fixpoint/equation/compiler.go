package equation

import (
	"errors"
	"fmt"
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
	"resource", "entry", "publication", "expression", "eval-node",
	"claim",
}

var ErrUnimplementedLowering = errors.New("equation: lowering is not implemented")

// Draft is Equation's pre-lowering constructor view. Terms and guards were
// sealed by the source program before this compiler was called.
type Draft = Equation

func validDraft(d Draft) error {
	probe := canonicalEquationSlices(d)
	probe.KernelID = "draft"
	return probe.valid()
}

// Source is the immutable relation-program projection.  A source is a walk
// result, rather than a second relation syntax or a transfer interpreter.
type Source struct{ Drafts []Draft }

// Compiler binds each frozen occurrence kind directly to its existing
// canonical transfer kernel. An empty binding is fail-closed.
type Compiler struct{ kernels map[string]string }

// Skeleton returns the complete frame with a fail-closed hook for every kind.
func Skeleton() *Compiler {
	kernels := make(map[string]string, len(FrozenKinds))
	for _, kind := range FrozenKinds {
		kernels[kind] = ""
	}
	return &Compiler{kernels: kernels}
}

// With returns a copied compiler with exactly one kernel binding replaced.
func (c *Compiler) With(kind, kernelID string) (*Compiler, error) {
	if c == nil || kernelID == "" {
		return nil, fmt.Errorf("equation: invalid lowering replacement %q", kind)
	}
	if _, known := c.kernels[kind]; !known {
		return nil, fmt.Errorf("equation: invalid lowering replacement %q", kind)
	}
	kernels := make(map[string]string, len(c.kernels))
	for key, value := range c.kernels {
		kernels[key] = value
	}
	kernels[kind] = kernelID
	return &Compiler{kernels: kernels}, nil
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
		if err := validDraft(draft); err != nil {
			return Artifact{}, fmt.Errorf("equation: draft %d: %w", index, err)
		}
		kernelID, present := c.kernels[draft.Occurrence.Kind]
		if !present {
			return Artifact{}, fmt.Errorf("equation: draft %d has unknown kind %q", index, draft.Occurrence.Kind)
		}
		if kernelID == "" {
			return Artifact{}, fmt.Errorf("equation: lower %s at %s: %w", draft.Occurrence.Kind, draft.Target.Name, ErrUnimplementedLowering)
		}
		lowered := canonicalEquationSlices(Equation{Target: draft.Target, Entry: draft.Entry, Guards: draft.Guards, Dependencies: draft.Dependencies, Occurrence: draft.Occurrence, Operands: draft.Operands, KernelID: kernelID})
		equations = append(equations, lowered)
	}
	artifact := Artifact{Equations: equations}
	if artifact.CanonicalBytes() == nil {
		return Artifact{}, fmt.Errorf("equation: non-canonical lowering artifact")
	}
	return artifact, nil
}
