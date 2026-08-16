package subtype

import "github.com/wippyai/go-lua/analysis/domain/type/typ"

func (c *checker) canWidenTo(narrow, wide typ.Type) bool {
	return c.prove(widenOf(narrow, wide))
}
