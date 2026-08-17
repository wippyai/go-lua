package source

import (
	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// sourceTermBits is a bounded dense membership plane. It is allocated only
// after the first order pass has proved every Count and row shape, and it is
// discarded when this preflight returns. A bitset keeps the hostile case
// linear without allowing one source Term to reserve a Go object of its own.
type sourceTermBits []uint64

func newSourceTermBits(count uint32) sourceTermBits {
	words := (uint64(count) + 63) / 64
	return make(sourceTermBits, int(words))
}

func (bits sourceTermBits) mark(ordinal uint32) bool {
	if ordinal == 0 {
		return true
	}
	index := uint64(ordinal - 1)
	word := index >> 6
	if word >= uint64(len(bits)) {
		return true
	}
	mask := uint64(1) << (index & 63)
	if bits[word]&mask != 0 {
		return true
	}
	bits[word] |= mask
	return false
}

type sourceOrderScratch struct {
	direct      [keyspace.FamilyCount]sourceTermBits
	cells       sourceTermBits
	faultOwners []keyspace.Term
}

func newSourceOrderScratch(counts [keyspace.FamilyCount]uint32) sourceOrderScratch {
	var scratch sourceOrderScratch
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if sourceDirectFamily(family) {
			scratch.direct[family] = newSourceTermBits(counts[family])
		}
	}
	scratch.cells = newSourceTermBits(counts[keyspace.FamilyCell])
	scratch.faultOwners = make([]keyspace.Term, int(counts[keyspace.FamilyControlFault]))
	return scratch
}

// preflightSourceOrder first proves all Count arities and row shapes on a
// copied Reader. Only after that allocation-free pass does it allocate the
// bounded dense scratch used by the second pass for all uniqueness and owner
// relations. Both passes are linear in the order section.
func preflightSourceOrder(reader *framing.Reader, counts [keyspace.FamilyCount]uint32) ([]keyspace.Term, error) {
	if reader == nil {
		return nil, framing.ErrMalformed
	}
	probe := *reader
	if err := walkSourceOrder(&probe, counts, nil); err != nil {
		return nil, err
	}
	scratch := newSourceOrderScratch(counts)
	if err := walkSourceOrder(reader, counts, &scratch); err != nil {
		return nil, err
	}
	for _, owner := range scratch.faultOwners {
		if owner == 0 {
			return nil, framing.ErrMalformed
		}
	}
	return scratch.faultOwners, nil
}

func walkSourceOrder(reader *framing.Reader, counts [keyspace.FamilyCount]uint32, scratch *sourceOrderScratch) error {
	if reader == nil {
		return framing.ErrMalformed
	}
	if err := readSourceRangeTag(reader, keyspace.FamilyBody); err != nil {
		return err
	}
	bodyCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return err
	}
	if uint32(bodyCount) != counts[keyspace.FamilyBody] {
		return framing.ErrMalformed
	}
	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return err
		}
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyIndex+1))
		for termIndex := 0; termIndex < rowCount; termIndex++ {
			term, err := readBoundTerm(reader, counts, keyspace.FamilyInvalid, false)
			if err != nil {
				return err
			}
			family := keyspace.TermFamily(term)
			if !sourceDirectFamily(family) {
				return framing.ErrMalformed
			}
			if scratch == nil {
				continue
			}
			if scratch.direct[family].mark(keyspace.TermOrdinal(term)) {
				return framing.ErrMalformed
			}
			if family == keyspace.FamilyControlFault {
				ordinal := keyspace.TermOrdinal(term)
				if ordinal == 0 || uint64(ordinal) > uint64(len(scratch.faultOwners)) || scratch.faultOwners[ordinal-1] != 0 {
					return framing.ErrMalformed
				}
				scratch.faultOwners[ordinal-1] = body
			}
		}
	}

	if err := readSourceRangeTag(reader, keyspace.FamilyBind); err != nil {
		return err
	}
	bindCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return err
	}
	if uint32(bindCount) != counts[keyspace.FamilyBind] {
		return framing.ErrMalformed
	}
	for bindIndex := 0; bindIndex < bindCount; bindIndex++ {
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return err
		}
		for cellIndex := 0; cellIndex < rowCount; cellIndex++ {
			cell, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
			if err != nil {
				return err
			}
			if scratch != nil && scratch.cells.mark(keyspace.TermOrdinal(cell)) {
				return framing.ErrMalformed
			}
		}
	}

	if err := readSourceRangeTag(reader, keyspace.FamilyFunction); err != nil {
		return err
	}
	functionCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return err
	}
	if uint32(functionCount) != counts[keyspace.FamilyFunction] {
		return framing.ErrMalformed
	}
	for functionIndex := 0; functionIndex < functionCount; functionIndex++ {
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return err
		}
		for formalIndex := 0; formalIndex < rowCount; formalIndex++ {
			formal, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
			if err != nil {
				return err
			}
			if scratch != nil && scratch.cells.mark(keyspace.TermOrdinal(formal)) {
				return framing.ErrMalformed
			}
		}
	}
	return nil
}

// readSourceOrder returns the direct Body owner of each authored ControlFault
// ordinal. Keeping this as a dense slice (rather than a map) mirrors the
// Input's canonical ordinal storage and lets the fault section cross-check
// owner provenance without importing or invoking Build.
func readSourceOrder(reader *framing.Reader, input *Input, counts [keyspace.FamilyCount]uint32) ([]keyspace.Term, error) {
	if input == nil {
		return nil, framing.ErrMalformed
	}
	faultOwners := make([]keyspace.Term, int(counts[keyspace.FamilyControlFault]))
	var seen [keyspace.FamilyCount][]bool
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if counts[family] != 0 {
			seen[family] = make([]bool, int(counts[family]))
		}
	}
	seenCells := make([]bool, int(counts[keyspace.FamilyCell]))

	if err := readSourceRangeTag(reader, keyspace.FamilyBody); err != nil {
		return nil, err
	}
	bodyCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return nil, err
	}
	if uint32(bodyCount) != counts[keyspace.FamilyBody] {
		return nil, framing.ErrMalformed
	}
	input.Bodies = make([]BodySource, bodyCount)
	for index := range input.Bodies {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
		terms, err := readDirectTerms(reader, counts, seen, faultOwners, body)
		if err != nil {
			return nil, err
		}
		input.Bodies[index] = BodySource{Body: body, Terms: terms}
	}

	if err := readSourceRangeTag(reader, keyspace.FamilyBind); err != nil {
		return nil, err
	}
	bindCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return nil, err
	}
	if uint32(bindCount) != counts[keyspace.FamilyBind] {
		return nil, framing.ErrMalformed
	}
	input.Binds = make([]BindCells, bindCount)
	for index := range input.Binds {
		bind := keyspace.MakeTerm(keyspace.FamilyBind, uint32(index+1))
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return nil, err
		}
		cells := make([]keyspace.Term, rowCount)
		for at := range cells {
			cell, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
			if err != nil {
				return nil, err
			}
			ordinal := keyspace.TermOrdinal(cell) - 1
			if seenCells[ordinal] {
				return nil, framing.ErrMalformed
			}
			seenCells[ordinal] = true
			cells[at] = cell
		}
		input.Binds[index] = BindCells{Bind: bind, Cells: cells}
	}

	if err := readSourceRangeTag(reader, keyspace.FamilyFunction); err != nil {
		return nil, err
	}
	functionCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return nil, err
	}
	if uint32(functionCount) != counts[keyspace.FamilyFunction] {
		return nil, framing.ErrMalformed
	}
	input.Functions = make([]FunctionFormals, functionCount)
	for index := range input.Functions {
		function := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return nil, err
		}
		formals := make([]keyspace.Term, rowCount)
		for at := range formals {
			formal, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
			if err != nil {
				return nil, err
			}
			ordinal := keyspace.TermOrdinal(formal) - 1
			if seenCells[ordinal] {
				return nil, framing.ErrMalformed
			}
			seenCells[ordinal] = true
			formals[at] = formal
		}
		input.Functions[index] = FunctionFormals{Function: function, Formals: formals}
	}
	return faultOwners, nil
}

func readSourceRangeTag(reader *framing.Reader, family keyspace.Family) error {
	tag, err := reader.Uint()
	if err != nil {
		return err
	}
	if tag != uint64(family) {
		return framing.ErrMalformed
	}
	return nil
}
