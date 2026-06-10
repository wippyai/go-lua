package effect

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/type/project"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type TypeProjection struct {
	Source ParamRef
	Steps  []TypeProjectionStep
}

func (TypeProjection) returnType() {}
func (p TypeProjection) String() string {
	if len(p.Steps) == 0 {
		return fmt.Sprintf("project_type(%s)", p.Source)
	}
	parts := make([]string, 0, len(p.Steps))
	for _, step := range p.Steps {
		parts = append(parts, step.String())
	}
	return fmt.Sprintf("project_type(%s.%s)", p.Source, strings.Join(parts, "."))
}

type TypeProjectionStepKind uint8

const (
	TypeProjectionField TypeProjectionStepKind = iota + 1
	TypeProjectionCallableReturn
	TypeProjectionGenericArg
	TypeProjectionInstantiateGeneric
)

type TypeProjectionStep struct {
	Kind  TypeProjectionStepKind
	Field string
	Index int
	Type  typ.Type
}

func ProjectField(name string) TypeProjectionStep {
	return TypeProjectionStep{Kind: TypeProjectionField, Field: name}
}

func ProjectCallableReturn() TypeProjectionStep {
	return TypeProjectionStep{Kind: TypeProjectionCallableReturn}
}

func ProjectGenericArg(index int) TypeProjectionStep {
	return TypeProjectionStep{Kind: TypeProjectionGenericArg, Index: index}
}

func ProjectInstantiateGeneric(generic typ.Type) TypeProjectionStep {
	return TypeProjectionStep{Kind: TypeProjectionInstantiateGeneric, Type: generic}
}

// ApplyTypeProjection applies projection steps to source using the pure type
// projection package.
func ApplyTypeProjection(source typ.Type, projection TypeProjection) (typ.Type, bool) {
	current := source
	for _, step := range projection.Steps {
		var ok bool
		switch step.Kind {
		case TypeProjectionField:
			current, ok = project.Field(current, step.Field)
		case TypeProjectionCallableReturn:
			current, ok = project.CallableReturn(current)
		case TypeProjectionGenericArg:
			current, ok = project.GenericArg(current, step.Index)
		case TypeProjectionInstantiateGeneric:
			current, ok = project.InstantiateGeneric(step.Type, current)
		default:
			return nil, false
		}
		if !ok {
			return nil, false
		}
	}
	return current, current != nil
}

func (s TypeProjectionStep) String() string {
	switch s.Kind {
	case TypeProjectionField:
		return s.Field
	case TypeProjectionCallableReturn:
		return "return"
	case TypeProjectionGenericArg:
		return fmt.Sprintf("arg[%d]", s.Index)
	case TypeProjectionInstantiateGeneric:
		return fmt.Sprintf("instantiate[%s]", typ.FormatShort(s.Type))
	default:
		return "unknown"
	}
}

func typeProjectionStepEquals(a, b TypeProjectionStep) bool {
	if a.Kind != b.Kind || a.Field != b.Field || a.Index != b.Index {
		return false
	}
	return typ.TypeEquals(a.Type, b.Type)
}
