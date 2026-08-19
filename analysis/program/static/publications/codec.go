package publications

import (
	"github.com/wippyai/go-lua/analysis/program/internal/wire"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// publicationWireMin is the row floor: assign, pair, and target.
const publicationWireMin = wire.UintWireMin * 3

// WriteContent emits the exact authored Assign-pair-to-TypeRef relation.
// Duplicate-detection state and any future export projection are absent.
func WriteContent(writer *framing.Writer, table Table) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writer.Count(uint64(table.publication.Count())); err != nil {
		return err
	}
	for _, row := range table.publication.All() {
		if err := writer.Uint(uint64(row.Assign)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Pair)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Target)); err != nil {
			return err
		}
	}
	return nil
}

// Scan validates and consumes one Publications vertical without allocating
// row slices. It is the allocation-free preflight half of Decode.
func Scan(reader *framing.Reader) error {
	_, err := decode(reader, false)
	return err
}

// Decode consumes one Publications vertical and returns owned authored rows.
func Decode(reader *framing.Reader) (Input, error) {
	return decode(reader, true)
}

func decode(reader *framing.Reader, retain bool) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}
	var input Input
	count, err := wire.Count(reader, publicationWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Type = make([]Publication, count)
	}
	for index := 0; index < count; index++ {
		assign, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		pair, err := wire.Uint32(reader)
		if err != nil {
			return Input{}, err
		}
		target, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		if keyspace.TermFamily(assign) != keyspace.FamilyAssign ||
			keyspace.TermFamily(target) != keyspace.FamilyTypeRef {
			return Input{}, framing.ErrMalformed
		}
		if retain {
			input.Type[index] = Publication{Assign: assign, Pair: pair, Target: target}
		}
	}
	return input, nil
}
