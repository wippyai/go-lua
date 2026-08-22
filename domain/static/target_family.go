package static

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/type/targetfamily"
)

// admitTargetFamily installs the sealed Target class vocabulary into this
// ClassSet. Every concrete row was decoded and canonically encoded once, by
// the contract that owns the declaration, so this pass performs no decode, no
// clone, and no canonical encoding however many Links mount the target.
func (s *ClassSet) admitTargetFamily(target *contract.Contract) error {
	family, available := targetfamily.Of(target)
	if !available {
		return errors.New("static: Target carries no sealed class family")
	}
	if s == nil || s.types == nil {
		return errors.New("static: Target class family type authority unavailable")
	}
	for index := 0; index < family.Count(); index++ {
		value, member, available := family.At(index)
		if !available {
			return errors.New("static: malformed sealed Target class family")
		}
		if _, exists := s.byTarget[value]; exists {
			continue
		}
		if member < 0 {
			class, err := s.addTargetResidual(target, value)
			if err != nil {
				return err
			}
			s.byTarget[value] = class
			continue
		}
		canonicalID, input, memberOK := family.Member(member, s.types)
		if !memberOK {
			return errors.New("static: sealed Target class member unavailable")
		}
		class, err := s.addConcreteCanonical(canonicalID, input)
		if err != nil {
			return err
		}
		s.byTarget[value] = class
	}
	return nil
}

// addTargetResidual admits one scoped endpoint that retains operation formals.
// It is still a finite opaque class; Static does not manufacture a parallel
// Target-formal authority to decode it.
func (s *ClassSet) addTargetResidual(target *contract.Contract, value vocabulary.Type) (Class, error) {
	declaration, ok := target.Operations.TypeDeclaration(value)
	if !ok {
		return Class{}, errors.New("static: Target type declaration unavailable")
	}
	contractID := target.ContentID()
	digest := declaration.Digest()
	residual := make([]byte, 0, len(contractID)+8+len(digest))
	residual = append(residual, contractID[:]...)
	var ordinal [8]byte
	binary.BigEndian.PutUint64(ordinal[:], uint64(value))
	residual = append(residual, ordinal[:]...)
	residual = append(residual, digest[:]...)
	index, ordinalErr := denseOrdinal(len(s.rows))
	if ordinalErr != nil {
		return Class{}, fmt.Errorf("static: Target class handle: %w", ordinalErr)
	}
	class := Class{owner: s, index: index}
	s.rows = append(s.rows, classRow{kind: ClassOpaque, opaqueID: residual})
	return class, nil
}
