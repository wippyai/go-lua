package exactkey

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// Encode writes one canonical exact-key atom. Payload spelling is owned by
// this directory; consumers provide only their issued handle.
func Encode(writer *framing.Writer, table *Table, key vocabulary.ExactKey) error {
	if table == nil {
		return errors.New("target/exactkey: unavailable table")
	}
	value, ok := table.Value(key)
	if !ok {
		return errors.New("target/exactkey: malformed exact key")
	}
	if err := writer.Uint(uint64(value.Kind)); err != nil {
		return err
	}
	switch value.Kind {
	case keyspace.LiteralBool:
		return writer.Bool(value.Bool)
	case keyspace.LiteralInteger:
		return writer.Uint(uint64(value.Integer))
	case keyspace.LiteralFloat:
		return writer.Uint(value.FloatBits)
	case keyspace.LiteralString:
		return writer.String(value.String)
	default:
		return errors.New("target/exactkey: malformed exact key kind")
	}
}
