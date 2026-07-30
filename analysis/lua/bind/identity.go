package bind

// ID identifies one lexical declaration within a binding result. Zero is
// unresolved. IDs from different results are never comparable.
type ID uint64

// Kind classifies how a lexical declaration was introduced.
type Kind uint8

const (
	Unknown Kind = iota
	Param
	Local
	Global
	Upvalue
	Function
)
