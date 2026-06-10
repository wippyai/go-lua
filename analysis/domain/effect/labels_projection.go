package effect

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/lua/access"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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

// ApplyTypeProjection applies projection steps to source.
func ApplyTypeProjection(source typ.Type, projection TypeProjection) (typ.Type, bool) {
	current := source
	for _, step := range projection.Steps {
		switch step.Kind {
		case TypeProjectionField:
			next, ok := access.Field(current, step.Field)
			if !ok {
				return nil, false
			}
			current = next
		case TypeProjectionCallableReturn:
			next, ok := access.CallableReturn(current)
			if !ok {
				return nil, false
			}
			current = next
		case TypeProjectionGenericArg:
			if step.Index < 0 {
				return nil, false
			}
			inst, ok := unwrap.Alias(current).(*typ.Instantiated)
			if !ok || inst == nil || step.Index >= len(inst.TypeArgs) || inst.TypeArgs[step.Index] == nil {
				return nil, false
			}
			current = inst.TypeArgs[step.Index]
		case TypeProjectionInstantiateGeneric:
			g, ok := unwrap.Alias(step.Type).(*typ.Generic)
			if !ok || g == nil || len(g.TypeParams) != 1 || current == nil {
				return nil, false
			}

			payload := current
			if meta, ok := unwrap.Alias(payload).(*typ.Meta); ok && meta != nil && meta.Of != nil {
				payload = meta.Of
			}
			if payload == nil {
				return nil, false
			}
			if constraint := g.TypeParams[0].Constraint; constraint != nil && !subtype.IsSubtype(payload, constraint) {
				return nil, false
			}
			current = typ.Instantiate(g, payload)
		default:
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
