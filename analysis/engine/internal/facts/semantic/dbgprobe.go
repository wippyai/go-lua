package semantic

// dbgprobe.go carries temporary structural counters for the many-way
// contribution merge. The solve loop is single threaded, so the counters are
// plain fields.

// DbgSemanticCounters records the shape of the FDD product a many-way join
// walks: how many terminal cells it reaches and how many operands each cell
// actually folds.
type DbgSemanticCounters struct {
	MergeMany  uint64
	Cells      uint64
	CellPairs  uint64
	CellWidth  uint64
	MaxOperand uint64
}

var dbgSemantic DbgSemanticCounters

// DbgSemantic returns the accumulated many-way merge counters.
func DbgSemantic() DbgSemanticCounters { return dbgSemantic }

// DbgSemanticReset clears the accumulated many-way merge counters.
func DbgSemanticReset() { dbgSemantic = DbgSemanticCounters{} }
