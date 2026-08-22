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
	return runtime, nil
}
