package region

// Node is one raw transport row in a Boolean DAG. References use the common
// terminal/node convention: 0 is False, 1 is True, and a value >= 2 names
// the row at Nodes[value-2]. NewRegion copies transport rows before sealing.
type Node struct {
	Atom Atom
	Low  uint32
	High uint32
}
