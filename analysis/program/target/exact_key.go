package target

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
)

func (c *Contract) appendExactKeys(input []keyspace.LiteralValue) error {
	if _, err := checkedStoredRange("exact key table", len(c.exactKeys), len(input)); err != nil {
		return err
	}
	for _, value := range input {
		normalized, ok := scalar.Normalize(value)
		if !ok || normalized != value {
			return errors.New("target: malformed exact key")
		}
		c.exactKeys = append(c.exactKeys, value)
	}
	return nil
}
