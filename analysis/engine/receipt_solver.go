package engine

// Binding returns the exact SchemaBinding that owns this receipt assembly.
// The returned capability is useful only with receipt-native engine entry
// points; callers cannot inspect or forge its sealed cells.
func (assembly *ReceiptAssembly) Binding() *SchemaBinding {
	if assembly == nil || assembly.builder == nil || assembly.binding == nil {
		return nil
	}
	return assembly.binding
}

// CompileReceiptTopology attaches the sealed Factor vertical to one committed
// receipt graph and assembles the common runtime with zero semantic members
// and zero queries. Artifact structural rows therefore execute immediately;
// rule/query attachment remains a separate typed receipt phase.
func CompileReceiptTopology(binding *SchemaBinding, graph *ReceiptGraph) (*Solver, bool) {
	compilation, ok := BeginReceiptTopologyCompilation(binding, graph)
	if !ok || compilation == nil {
		return nil, false
	}
	solver, ok := compilation.Solver()
	if !ok || solver == nil {
		return nil, false
	}
	return solver, true
}

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
