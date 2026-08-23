package typeindex

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/internal/framing"
)

// Encode writes the complete canonical qualified-type row named by typ. The
// exact name and neutral declaration are both part of Target content identity.
func Encode(writer *framing.Writer, table *Table, typ vocabulary.Type) error {
	if table == nil {
		return errors.New("target/typeindex: unavailable table")
	}
	name, ok := table.Name(typ)
	if !ok {
		return errors.New("target/typeindex: malformed type handle")
	}
	declaration, ok := table.Declaration(typ)
	if !ok || !declaration.Available() {
		return errors.New("target/typeindex: unavailable type declaration")
	}
	if err := writer.String(name); err != nil {
		return err
	}
	return encodeDeclaration(writer, declaration)
}

func encodeDeclaration(writer *framing.Writer, declaration schematype.Type) error {
	primitive, primitiveOK := declaration.Primitive()
	if err := writer.Bool(primitiveOK); err != nil {
		return err
	}
	if primitiveOK {
		if err := writer.Uint(uint64(primitive)); err != nil {
			return err
		}
	} else if err := writer.Uint(0); err != nil {
		return err
	}
	if err := writer.Uint(uint64(declaration.ExternalFormals())); err != nil {
		return err
	}
	return writer.Bytes(declaration.Bytes())
}
