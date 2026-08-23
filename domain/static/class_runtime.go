package static

import (
	"errors"

	"github.com/wippyai/go-lua/domain/type/authority"
)

// sealRuntime transfers the complete evaluated structural denominator into
// typeauthority exactly once. ClassSet keeps only its Pack carrier after this
// call; no runtime reflection graph or mutable bridge points back into Static.
func (s *ClassSet) sealRuntime() (*typeauthority.Runtime, error) {
	if s == nil || s.types == nil {
		return nil, errors.New("static: Runtime seal source unavailable")
	}
	inputs := make([]typeauthority.RuntimeInput, 0, len(s.rows))
	classByInput := make([]uint32, 0, len(s.rows))
	for index := 1; index < len(s.rows); index++ {
		row := s.rows[index]
		if row.kind != ClassConcrete {
			continue
		}
		input := row.input
		inputs = append(inputs, input)
		classByInput = append(classByInput, uint32(index))
	}

	runtime, mapped, err := typeauthority.SealRuntime(s.types, inputs)
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
	for value, class := range s.byTarget {
		if class.owner != s || class.index == 0 || uint64(class.index) >= uint64(len(s.rows)) {
			return nil, errors.New("static: malformed Target Runtime class projection")
		}
		row := s.rows[class.index]
		if row.kind != ClassConcrete {
			continue
		}
		if _, owned := runtime.Index(row.inner); !owned {
			return nil, errors.New("static: Target Runtime projection unavailable")
		}
		s.targetRuntime[value] = row.inner
	}
	return runtime, nil
}

func (s *ClassSet) sealRuntimeClassProjection() error {
	if s == nil || s.runtime == nil || len(s.rows) == 0 || s.classByRuntime != nil || s.runtimeByClass != nil {
		return errors.New("static: Runtime/Class projection source unavailable")
	}
	s.runtimeByClass = make([]typeauthority.RuntimeInner, len(s.rows))
	s.classByRuntime = make(map[typeauthority.RuntimeInner]Class, len(s.rows))
	for index, row := range s.rows {
		if row.kind != ClassConcrete {
			continue
		}
		if !s.runtime.Closed(row.inner) {
			return errors.New("static: concrete Class lacks closed Runtime row")
		}
		class := Class{owner: s, index: uint32(index)}
		if _, duplicate := s.classByRuntime[row.inner]; duplicate {
			return errors.New("static: duplicate Runtime/Class projection")
		}
		s.runtimeByClass[index] = row.inner
		s.classByRuntime[row.inner] = class
	}
	return nil
}

// ClassForRuntime returns the exact sealed Static class for one closed Runtime
// row. Derived union descriptors have no Runtime row and cannot enter here.
func (s *ClassSet) ClassForRuntime(inner typeauthority.RuntimeInner) (Class, bool) {
	if s == nil || s.runtime == nil || !s.runtime.Closed(inner) {
		return Class{}, false
	}
	class, ok := s.classByRuntime[inner]
	return class, ok && s.owns(class)
}

// RuntimeForClass returns the exact structural row for one sealed concrete
// class. AnyValue, opaque, and derived union classes intentionally have no
// single structural row.
func (s *ClassSet) RuntimeForClass(class Class) (typeauthority.RuntimeInner, bool) {
	if !s.owns(class) || class.descriptor != nil || class.index == 0 || uint64(class.index) >= uint64(len(s.runtimeByClass)) {
		return typeauthority.RuntimeInner{}, false
	}
	inner := s.runtimeByClass[class.index]
	return inner, s.runtime != nil && s.runtime.Closed(inner)
}
