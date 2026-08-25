package operators

import (
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/internal/wire"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	typeOfWireMin      = wire.UintWireMin * 2
	keyOfWireMin       = wire.UintWireMin
	indexAccessWireMin = wire.UintWireMin * 2
	conditionalWireMin = wire.UintWireMin * 4
)

// WriteContent emits the canonical authored operator rows. Record framing is
// owned by the enclosing Static stream; this function owns only this vertical's
// fields and their order.
func WriteContent(writer *framing.Writer, table Table) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writeTypeOf(writer, table.typeOf); err != nil {
		return err
	}
	if err := writeKeyOf(writer, table.keyOf); err != nil {
		return err
	}
	if err := writeIndexAccess(writer, table.indexAccess); err != nil {
		return err
	}
	return writeConditional(writer, table.conditional)
}

func writeTypeOf(writer *framing.Writer, table rows.Table[TypeOf]) error {
	if err := writer.Count(uint64(table.Count())); err != nil {
		return err
	}
	for _, row := range table.All() {
		if err := writer.Uint(uint64(row.Scope)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Operand)); err != nil {
			return err
		}
	}
	return nil
}
func writeKeyOf(writer *framing.Writer, table rows.Table[KeyOf]) error {
	if err := writer.Count(uint64(table.Count())); err != nil {
		return err
	}
	for _, row := range table.All() {
		if err := writer.Uint(uint64(row.Inner)); err != nil {
			return err
		}
	}
	return nil
}
func writeIndexAccess(writer *framing.Writer, table rows.Table[IndexAccess]) error {
	if err := writer.Count(uint64(table.Count())); err != nil {
		return err
	}
	for _, row := range table.All() {
		if err := writer.Uint(uint64(row.Object)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Index)); err != nil {
			return err
		}
	}
	return nil
}
func writeConditional(writer *framing.Writer, table rows.Table[Conditional]) error {
	if err := writer.Count(uint64(table.Count())); err != nil {
		return err
	}
	for _, row := range table.All() {
		if err := writer.Uint(uint64(row.Check)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Extends)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Then)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Else)); err != nil {
			return err
		}
	}
	return nil
}

// Scan validates and consumes one operator vertical without allocating row
// slices. It is the allocation-free preflight half of Decode.
func Scan(reader *framing.Reader) error {
	if reader == nil {
		return framing.ErrMalformed
	}
	_, err := decode(reader, false)
	return err
}

// Decode consumes one operator vertical and returns owned input rows.
func Decode(reader *framing.Reader) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}
	return decode(reader, true)
}

func decode(reader *framing.Reader, retain bool) (Input, error) {
	var input Input
	count, err := wire.Count(reader, typeOfWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.TypeOf = make([]TypeOf, count)
	}
	for index := 0; index < count; index++ {
		scope, err := wire.ConstrainedTerm(reader, staticrole.ScopeHandleFamily)
		if err != nil {
			return Input{}, err
		}
		operand, err := wire.ConstrainedTerm(reader, flowrole.ValueOccurrenceFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.TypeOf[index] = TypeOf{Scope: scope, Operand: operand}
		}
	}

	count, err = wire.Count(reader, keyOfWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.KeyOf = make([]KeyOf, count)
	}
	for index := 0; index < count; index++ {
		inner, err := wire.ConstrainedTerm(reader, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.KeyOf[index] = KeyOf{Inner: inner}
		}
	}

	count, err = wire.Count(reader, indexAccessWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.IndexAccess = make([]IndexAccess, count)
	}
	for index := 0; index < count; index++ {
		object, err := wire.ConstrainedTerm(reader, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		indexTerm, err := wire.ConstrainedTerm(reader, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.IndexAccess[index] = IndexAccess{Object: object, Index: indexTerm}
		}
	}

	count, err = wire.Count(reader, conditionalWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Conditional = make([]Conditional, count)
	}
	for index := 0; index < count; index++ {
		check, err := wire.ConstrainedTerm(reader, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		extends, err := wire.ConstrainedTerm(reader, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		thenTerm, err := wire.ConstrainedTerm(reader, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		elseTerm, err := wire.ConstrainedTerm(reader, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Conditional[index] = Conditional{Check: check, Extends: extends, Then: thenTerm, Else: elseTerm}
		}
	}
	return input, nil
}
