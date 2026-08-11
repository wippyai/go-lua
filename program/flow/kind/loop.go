package kind

// LoopKind is the closed authored Lua loop vocabulary. Its execution edges
// and exits are derived only by the whole-Flow finalizer.
type LoopKind uint8

const (
	LoopWhile LoopKind = iota + 1
	LoopRepeat
	LoopNumericFor
	LoopGenericFor
)
