package kind

// CellRole is the closed lexical-storage definition vocabulary. Every sealed
// Cell has exactly one role; roles are derived from typed relations rather
// than supplied as mutable flags.
type CellRole uint8

const (
	CellGlobal CellRole = iota + 1
	CellLocal
	CellFormal
	CellFunctionVararg
	CellLoop
	CellCapture
	CellChunkVararg
)
