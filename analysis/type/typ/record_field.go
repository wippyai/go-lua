package typ

import "github.com/wippyai/go-lua/analysis/type/annotation"

// Field represents a record field with name, type, optionality, and mutability.
type Field struct {
	Name     string
	Type     Type
	Optional bool // True if field may be absent (nil access returns nil)
	Readonly bool // True if field cannot be reassigned
}
