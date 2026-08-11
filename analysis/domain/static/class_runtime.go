package static

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
)

// sealRuntime transfers the complete evaluated structural denominator into
// typeauthority exactly once. ClassSet keeps only its Pack carrier after this
// call; no runtime reflection graph or mutable bridge points back into Static.
func (s *ClassSet) sealRuntime() (*typeauthority.Runtime, error) {
	if s == nil || s.authority == nil || s.authority.source == nil || s.authority.types == nil {
		return nil, errors.New("static: Runtime seal source unavailable")
	}
	inputs := make([]typeauthority.RuntimeInput, 0, len(s.rows))
	inputByClass := make(map[uint32]typeauthority.RuntimeInput, len(s.rows))
	classByInput := make([]uint32, 0, len(s.rows))
	for index := 1; index < len(s.rows); index++ {
		row := s.rows[index]
		if row.kind != ClassConcrete {
			continue
		}
		input, ok := s.authority.types.RuntimeInput(row.encoded)
		if !ok {
			return nil, fmt.Errorf("static: concrete class %d lacks canonical Runtime input", index)
		}
		inputs = append(inputs, input)
		inputByClass[uint32(index)] = input
		classByInput = append(classByInput, uint32(index))
	}

	runtime, mapped, err := typeauthority.SealRuntime(s.authority.types, inputs)
	if err != nil {
		return nil, err
	}
	if runtime == nil || len(mapped) != len(classByInput) {
		return nil, errors.New("static: malformed Runtime seal projection")
	}
	seenInner := make(map[typeauthority.RuntimeInner]struct{}, len(mapped))
	for index, inner := range mapped {
		classIndex := classByInput[index]
		if uint64(classIndex) >= uint64(len(s.rows)) {
			return nil, errors.New("static: Runtime input class out of range")
		}
		s.rows[classIndex].inner = inner
		if _, duplicate := seenInner[inner]; duplicate {
			return nil, errors.New("static: duplicate concrete class escaped canonical row admission")
		}
		seenInner[inner] = struct{}{}
	}
	if err := s.sealTypeValueOccurrences(runtime); err != nil {
		return nil, err
	}
	return runtime, nil
}

// sealTypeValueOccurrences is Static's sole occurrence grounding pass.  It
// joins an existing executable Boundary Value to the already-contextual
// Static result and, only for a concrete result, to Runtime's structural row.
func (s *ClassSet) sealTypeValueOccurrences(runtime *typeauthority.Runtime) error {
	if s == nil || s.authority == nil || s.authority.source == nil || runtime == nil {
		return errors.New("static: TypeValue occurrence source unavailable")
	}
	a := s.authority
	boundary := a.source.Boundary()
	if boundary == nil {
		return errors.New("static: TypeValue Boundary unavailable")
	}
	mounts := a.source.Project().Mounts()
	rows := make([]typeValueRow, 0)
	seen := make(map[linkboundary.Value]struct{})
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, ok := mounts.At(mountIndex)
		if !ok {
			return errors.New("static: malformed TypeValue mount")
		}
		p, ok := mounts.Program(shard)
		if !ok || p == nil {
			return errors.New("static: unavailable TypeValue Program")
		}
		resolver, ok := a.source.Static().Namespaces().ResolverForShard(shard)
		if !ok {
			return errors.New("static: unavailable TypeValue resolver")
		}
		namespace, ok := a.source.Static().Namespaces().ResolverContentID(resolver)
		if !ok {
			return errors.New("static: unavailable TypeValue namespace")
		}
		typeValues := p.Flow().Authored().TypeValues()
		for index := 0; index < typeValues.Count(); index++ {
			term, ok := typeValues.At(index)
			if !ok {
				return errors.New("static: malformed TypeValue source")
			}
			if _, ok := typeValues.Get(term); !ok {
				return errors.New("static: unavailable TypeValue source")
			}
			if !p.Flow().Executable().Contains(term) {
				continue
			}
			value, ok := boundary.Values().Of(shard, term)
			if !ok {
				return errors.New("static: unavailable TypeValue Boundary Value")
			}
			if _, duplicate := seen[value]; duplicate {
				return errors.New("static: duplicate TypeValue Boundary Value")
			}
			target, ok := p.Static().Operands().TypeValues().Target(term)
			if !ok {
				return errors.New("static: unavailable TypeValue target")
			}
			selector, ok := a.types.Find(p.ContentID(), target)
			if !ok {
				return errors.New("static: unavailable TypeValue selector")
			}
			reference, ok := a.types.Ref(selector)
			if !ok {
				return errors.New("static: unavailable TypeValue reference")
			}
			coordinate, ok := a.coordinateFor(coordinateKey{reference: reference, namespace: namespace})
			if !ok {
				return errors.New("static: unavailable TypeValue coordinate")
			}
			result, ok := a.Result(coordinate)
			if !ok {
				return errors.New("static: unavailable TypeValue result")
			}
			name, root, ok := staticTypeValueIdentity(a.source, shard, p, term, target)
			if !ok {
				return errors.New("static: unavailable TypeValue identity")
			}
			row := typeValueRow{value: value, name: name, root: root}
			inner, exact, dispositionErr := s.typeValueExactInner(result, runtime)
			if dispositionErr != nil {
				return dispositionErr
			}
			row.inner, row.exact = inner, exact
			row.id, ok = staticTypeValueRowID(a.source, boundary.Values(), runtime, result, row)
			if !ok {
				return errors.New("static: unavailable TypeValue row identity")
			}
			seen[value] = struct{}{}
			rows = append(rows, row)
		}
	}
	a.typeValues = rows
	return nil
}

// typeValueExactInner classifies an already-grounded Static result. Bottom
// and Invalid are lawful explicit Other outcomes; all structural authority
// still flows exclusively through Static's Class image.
func (s *ClassSet) typeValueExactInner(result Value, runtime *typeauthority.Runtime) (typeauthority.RuntimeInner, bool, error) {
	if s == nil || s.authority == nil || runtime == nil || !s.authority.Owns(result) {
		return typeauthority.RuntimeInner{}, false, errors.New("static: foreign TypeValue result")
	}
	kind, valid := result.Kind()
	if !valid {
		return typeauthority.RuntimeInner{}, false, errors.New("static: malformed TypeValue result")
	}
	if kind == KindBottom || kind == KindInvalid {
		return typeauthority.RuntimeInner{}, false, nil
	}
	class, classified := s.ClassForStatic(result)
	if !classified {
		return typeauthority.RuntimeInner{}, false, errors.New("static: unavailable TypeValue class")
	}
	if class.descriptor != nil || class.index >= uint32(len(s.rows)) || s.rows[class.index].kind != ClassConcrete {
		return typeauthority.RuntimeInner{}, false, nil
	}
	inner := s.rows[class.index].inner
	if !runtime.Equal(inner, inner) {
		return typeauthority.RuntimeInner{}, false, errors.New("static: unavailable TypeValue Runtime inner")
	}
	return inner, true, nil
}
