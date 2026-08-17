package projection

import (
	"fmt"
	"strings"

	typeformat "github.com/wippyai/go-lua/analysis/domain/type/format"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

type Projection struct {
	Steps []Step
}

func (p Projection) String() string {
	if len(p.Steps) == 0 {
		return ""
	}
	parts := make([]string, 0, len(p.Steps))
	for _, step := range p.Steps {
		parts = append(parts, step.String())
	}
	return strings.Join(parts, ".")
}

func Equal(a, b Projection) bool {
	if len(a.Steps) != len(b.Steps) {
		return false
	}
	for i := range a.Steps {
		if !stepEqual(a.Steps[i], b.Steps[i]) {
			return false
		}
	}
	return true
}

// StepKind is one member of the closed vocabulary of projection steps this
// package owns. The ordinals are dense from StepField and are this package's
// own numbering. They are not a wire format: a serializer that needs a stable
// external spelling declares one against this catalog rather than writing the
// ordinal.
type StepKind uint8

const (
	StepField StepKind = iota + 1
	StepCallableReturn
	StepGenericArg
	StepInstantiateGeneric
	stepKindLimit
)

// StepKindCount is the size of the closed vocabulary. The ordinals are dense
// from StepField, so a consumer indexes by kind without a lookup.
const StepKindCount = int(stepKindLimit) - 1

// Valid reports membership in the closed vocabulary. The zero kind is the
// unset field of a Step and is not a member.
func (kind StepKind) Valid() bool {
	return kind >= StepField && kind < stepKindLimit
}

// StepKinds is the vocabulary catalog in ordinal order. It is the one
// enumeration of the steps this package owns, so a consumer that visits,
// serializes, or declares every step projects it instead of restating the
// member list. The catalog is returned by value and costs no allocation to
// range over.
func StepKinds() [StepKindCount]StepKind {
	return [StepKindCount]StepKind{
		StepField, StepCallableReturn, StepGenericArg, StepInstantiateGeneric,
	}
}

type Step struct {
	Kind  StepKind
	Field string
	Index int
	Type  typ.Type
}

func Field(name string) Step {
	return Step{Kind: StepField, Field: name}
}

func CallableReturn() Step {
	return Step{Kind: StepCallableReturn}
}

func GenericArg(index int) Step {
	return Step{Kind: StepGenericArg, Index: index}
}

func InstantiateGeneric(generic typ.Type) Step {
	return Step{Kind: StepInstantiateGeneric, Type: generic}
}

func (s Step) String() string {
	switch s.Kind {
	case StepField:
		return s.Field
	case StepCallableReturn:
		return "return"
	case StepGenericArg:
		return fmt.Sprintf("arg[%d]", s.Index)
	case StepInstantiateGeneric:
		return fmt.Sprintf("instantiate[%s]", typeformat.Short(s.Type))
	default:
		return "unknown"
	}
}

func stepEqual(a, b Step) bool {
	if a.Kind != b.Kind || a.Field != b.Field || a.Index != b.Index {
		return false
	}
	return typ.TypeEquals(a.Type, b.Type)
}
