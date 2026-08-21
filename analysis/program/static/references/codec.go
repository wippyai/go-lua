package references

import (
	"github.com/wippyai/go-lua/analysis/program/internal/wire"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

// referenceWireMin is the row floor: disposition, target, root, and the two
// key-sequence counts with at least one source key.
const referenceWireMin = wire.UintWireMin * 6

// WriteContent emits authored spelling and binder disposition. Column offsets
// are implementation-only; the encoded paths are the exact authored sequences.
func WriteContent(writer *framing.Writer, table Table) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writer.Count(uint64(table.ref.Count())); err != nil {
		return err
	}
	for _, row := range table.ref.All() {
		if err := writer.Uint(uint64(row.Resolution)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Target)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Root)); err != nil {
			return err
		}
		if err := wire.WriteKeySpan(writer, table.keys, row.Source); err != nil {
			return err
		}
		if err := wire.WriteKeySpan(writer, table.keys, row.Canonical); err != nil {
			return err
		}
	}
	return nil
}

// Scan validates and consumes one References vertical without allocating row
// slices. It is the allocation-free preflight half of Decode.
func Scan(reader *framing.Reader) error {
	_, err := decode(reader, false)
	return err
}

// Decode consumes one References vertical and returns owned authored rows.
func Decode(reader *framing.Reader) (Input, error) {
	return decode(reader, true)
}

func decode(reader *framing.Reader, retain bool) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}
	var input Input
	count, err := wire.Count(reader, referenceWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.TypeRef = make([]TypeRef, count)
	}
	for index := 0; index < count; index++ {
		resolution, err := wire.Enum(reader, uint64(CanonicalPath))
		if err != nil {
			return Input{}, err
		}
		target, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		root, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		sourceKeys, sourceCount, err := wire.KeySequence(reader, 1, retain)
		if err != nil {
			return Input{}, err
		}
		canonicalKeys, canonicalCount, err := wire.KeySequence(reader, 0, retain)
		if err != nil {
			return Input{}, err
		}
		switch Resolution(resolution) {
		case Unresolved:
			if target != 0 || canonicalCount != 0 {
				return Input{}, framing.ErrMalformed
			}
		case Declaration:
			if !staticrole.TypeReferenceTargetFamily(keyspace.TermFamily(target)) || canonicalCount != 0 {
				return Input{}, framing.ErrMalformed
			}
		case CanonicalPath:
			if target != 0 || canonicalCount == 0 {
				return Input{}, framing.ErrMalformed
			}
		default:
			return Input{}, framing.ErrMalformed
		}
		if sourceCount == 1 && root != 0 {
			return Input{}, framing.ErrMalformed
		}
		if sourceCount > 1 && keyspace.TermFamily(root) != keyspace.FamilyCell {
			return Input{}, framing.ErrMalformed
		}
		if retain {
			input.TypeRef[index] = TypeRef{
				Resolution: Resolution(resolution), Target: target, Root: root,
				Source: sourceKeys, Canonical: canonicalKeys,
			}
		}
	}
	return input, nil
}
