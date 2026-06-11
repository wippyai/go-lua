package projection

import (
	"fmt"
	"strings"

	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/identity"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

type StepKind uint8

const (
	StepField StepKind = iota + 1
	StepCallableReturn
	StepGenericArg
	StepInstantiateGeneric
)

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
	return identity.TypeEquals(a.Type, b.Type)
}
