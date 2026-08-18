package source

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// preflightSourceSpellings proves the complete debug-spelling section on a
// copied Reader before the allocation/fill pass. Cell rows are dense; Call
// rows are sparse but strictly ordered and must carry a nonempty authored
// spelling when present.
func preflightSourceSpellings(reader *framing.Reader, counts [keyspace.FamilyCount]uint32) error {
	if reader == nil {
		return framing.ErrMalformed
	}
	cellCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 3)
	if err != nil {
		return err
	}
	if uint32(cellCount) != counts[keyspace.FamilyCell] {
		return framing.ErrMalformed
	}
	for index := 0; index < cellCount; index++ {
		cell, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
		if err != nil {
			return err
		}
		if cell != keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1)) {
			return framing.ErrMalformed
		}
		if _, err := sourceStringBytes(reader); err != nil {
			return err
		}
	}
	callCount, err := sourceCount(reader, uint64(counts[keyspace.FamilyCall]), 3)
	if err != nil {
		return err
	}
	var previous keyspace.Term
	for index := 0; index < callCount; index++ {
		call, err := readBoundTerm(reader, counts, keyspace.FamilyCall, false)
		if err != nil {
			return err
		}
		name, err := sourceStringBytes(reader)
		if err != nil {
			return err
		}
		if len(name) == 0 || previous != 0 && call <= previous {
			return framing.ErrMalformed
		}
		previous = call
	}
	return nil
}

func readSourceSpellings(reader *framing.Reader, input *Input, counts [keyspace.FamilyCount]uint32) error {
	if reader == nil || input == nil {
		return framing.ErrMalformed
	}
	cellCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 3)
	if err != nil {
		return err
	}
	if uint32(cellCount) != counts[keyspace.FamilyCell] {
		return framing.ErrMalformed
	}
	input.CellSpellings = make([]CellSpelling, cellCount)
	for index := range input.CellSpellings {
		cell, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
		if err != nil {
			return err
		}
		if cell != keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1)) {
			return framing.ErrMalformed
		}
		name, err := sourceString(reader)
		if err != nil {
			return err
		}
		input.CellSpellings[index] = CellSpelling{Cell: cell, Name: name}
	}
	callCount, err := sourceCount(reader, uint64(counts[keyspace.FamilyCall]), 3)
	if err != nil {
		return err
	}
	input.CallSpellings = make([]CallSpelling, callCount)
	var previous keyspace.Term
	for index := range input.CallSpellings {
		call, err := readBoundTerm(reader, counts, keyspace.FamilyCall, false)
		if err != nil {
			return err
		}
		name, err := sourceString(reader)
		if err != nil {
			return err
		}
		if name == "" || previous != 0 && call <= previous {
			return framing.ErrMalformed
		}
		input.CallSpellings[index] = CallSpelling{Call: call, Name: name}
		previous = call
	}
	return nil
}
