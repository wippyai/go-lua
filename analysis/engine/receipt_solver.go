package engine

// BeginReceiptTopologyCompilation opens the typed attachment transaction for
// a committed receipt graph. Callers may attach Rule and Query receipts before
// terminally calling ReceiptCompilation.Solver.
func BeginReceiptTopologyCompilation(binding *SchemaBinding, graph *ReceiptGraph) (*ReceiptCompilation, bool) {
	if binding == nil || graph == nil || !binding.Sealed() || !graph.valid() || graph.state != binding.state {
		return nil, false
	}
	compilation, ok := compileReceiptFactors(binding, graph.graph)
	if !ok || compilation == nil {
		return nil, false
	}
	return &ReceiptCompilation{inner: compilation, graph: graph}, true
}
