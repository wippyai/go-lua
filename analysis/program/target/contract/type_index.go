package contract

import (
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
)

// Types returns the immutable Target-owned qualified type directory. The
// returned value has private storage and exposes only exact lookup and dense
// canonical enumeration.
func (c *Contract) Types() typeindex.Table {
	if c == nil {
		return typeindex.Table{}
	}
	return c.types
}
