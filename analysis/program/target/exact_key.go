package target

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// Exact-key coordinates are published from the one shared immutable owner.
// These methods retain Contract's historical read surface without retaining a
// second literal slice in Contract.
func (c *Contract) ExactKeyCount() int {
	if c == nil {
		return 0
	}
	return c.exactKeys.Count()
}

func (c *Contract) ExactKeyAt(index int) (vocabulary.ExactKey, bool) {
	if c == nil {
		return 0, false
	}
	return c.exactKeys.At(index)
}

func (c *Contract) ExactKeyValue(key vocabulary.ExactKey) (keyspace.LiteralValue, bool) {
	if c == nil {
		return keyspace.LiteralValue{}, false
	}
	return c.exactKeys.Value(key)
}

func encodeExactKey(writer *framing.Writer, c *Contract, key vocabulary.ExactKey) error {
	if c == nil {
		return errors.New("target: unavailable exact-key table")
	}
	return exactkey.Encode(writer, &c.exactKeys, key)
}

func exactKeyHandle(keys exactkey.Table, value keyspace.LiteralValue) (vocabulary.ExactKey, error) {
	key, ok := keys.Handle(value)
	if !ok || key == 0 {
		return 0, errors.New("target: unresolved exact key")
	}
	return key, nil
}
