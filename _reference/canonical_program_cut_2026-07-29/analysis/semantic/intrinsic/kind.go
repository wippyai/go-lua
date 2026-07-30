package intrinsic

// Kind is an unforgeable semantic identity assigned by binding authority.
// Textual expression operators cannot manufacture one.
type Kind uint8

const (
	None Kind = iota
	LuaType
)

func (k Kind) Valid() bool { return k > None && k <= LuaType }
