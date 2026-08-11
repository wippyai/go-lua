package static

import (
	"bytes"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
)

// sealClassOrder gives the finite Class rows a construction-order-independent
// dense presentation. Direct mutual subtyping is not an equivalence relation;
// semantic equality is
// derived later from extensional principal coverage over the sealed universe.
func (s *ClassSet) sealClassOrder(runtime *typeauthority.Runtime) error {
	if s == nil || runtime == nil || len(s.rows) == 0 || s.rows[0].kind != ClassAnyValue {
		return errors.New("static: ClassSet order source unavailable")
	}
	order := make([]int, len(s.rows)-1)
	for index := 1; index < len(s.rows); index++ {
		order[index-1] = index
		row := s.rows[index]
		switch row.kind {
		case ClassConcrete:
			if !runtime.Equal(row.inner, row.inner) || len(row.encoded) == 0 {
				return errors.New("static: malformed concrete Class row")
			}
		case ClassOpaque:
			if len(row.encoded) == 0 {
				return errors.New("static: malformed opaque Class row")
			}
		default:
			return errors.New("static: invalid ClassSet order member")
		}
	}
	sort.Slice(order, func(left, right int) bool {
		first, second := s.rows[order[left]], s.rows[order[right]]
		if first.kind != second.kind {
			return first.kind < second.kind
		}
		return bytes.Compare(first.encoded, second.encoded) < 0
	})

	oldToNew := make([]uint32, len(s.rows))
	rows := make([]classRow, len(s.rows))
	rows[0] = s.rows[0]
	for next, old := range order {
		index := uint32(next + 1)
		oldToNew[old] = index
		rows[index] = s.rows[old]
	}
	remap := func(class Class) (Class, error) {
		if class.owner != s || uint64(class.index) >= uint64(len(oldToNew)) ||
			(class.index != 0 && oldToNew[class.index] == 0) {
			return Class{}, errors.New("static: foreign ClassSet order member")
		}
		return Class{owner: s, index: oldToNew[class.index]}, nil
	}
	for key, class := range s.byStatic {
		mapped, err := remap(class)
		if err != nil {
			return err
		}
		s.byStatic[key] = mapped
	}
	for key, class := range s.byTarget {
		mapped, err := remap(class)
		if err != nil {
			return err
		}
		s.byTarget[key] = mapped
	}
	nilClass, err := remap(s.nil)
	if err != nil {
		return err
	}
	s.nil = nilClass
	s.rows = rows
	return nil
}
