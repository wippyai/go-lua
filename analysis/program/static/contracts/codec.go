package contracts

import (
	"github.com/wippyai/go-lua/analysis/program/internal/wire"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	functionWireMin = wire.UintWireMin * 3
	callWireMin     = wire.UintWireMin
)

func typeParamFamily(family keyspace.Family) bool { return family == keyspace.FamilyTypeParam }

// WriteContent emits the dense static sidecars for opaque Flow Function and
// Call identities. It encodes semantic sequences, never column offsets.
func WriteContent(writer *framing.Writer, table Table) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writer.Count(uint64(table.function.Count())); err != nil {
		return err
	}
	for _, row := range table.function.All() {
		if err := wire.WriteTermSpan(writer, table.terms, row.TypeParams); err != nil {
			return err
		}
		if err := writer.Bool(row.ReturnsKnown); err != nil {
			return err
		}
		if err := wire.WriteTermSpan(writer, table.terms, row.Returns); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.call.Count())); err != nil {
		return err
	}
	for _, row := range table.call.All() {
		if err := wire.WriteTermSpan(writer, table.terms, row.TypeArguments); err != nil {
			return err
		}
	}
	return nil
}

// Scan validates and consumes one Contracts vertical without allocating row
// slices. It is the allocation-free preflight half of Decode.
func Scan(reader *framing.Reader) error {
	_, err := decode(reader, false)
	return err
}

// Decode consumes one Contracts vertical and returns owned authored rows.
func Decode(reader *framing.Reader) (Input, error) {
	return decode(reader, true)
}

func decode(reader *framing.Reader, retain bool) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}
	var input Input
	count, err := wire.Count(reader, functionWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Function = make([]FunctionContract, count)
	}
	for index := 0; index < count; index++ {
		typeParams, _, err := wire.TermSequence(reader, 0, retain, typeParamFamily)
		if err != nil {
			return Input{}, err
		}
		returnsKnown, err := wire.Bool(reader)
		if err != nil {
			return Input{}, err
		}
		returns, returnCount, err := wire.TermSequence(reader, 0, retain, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		if !returnsKnown && returnCount != 0 {
			return Input{}, framing.ErrMalformed
		}
		if retain {
			input.Function[index] = FunctionContract{
				TypeParams: typeParams, ReturnsKnown: returnsKnown, Returns: returns,
			}
		}
	}

	count, err = wire.Count(reader, callWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Call = make([]CallContract, count)
	}
	for index := 0; index < count; index++ {
		typeArguments, _, err := wire.TermSequence(reader, 0, retain, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Call[index] = CallContract{TypeArguments: typeArguments}
		}
	}
	return input, nil
}
