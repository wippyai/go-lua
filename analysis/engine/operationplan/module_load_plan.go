package operationplan

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// WithModuleLoads attaches immutable module-load producers. Equal operations
// are interned once with full semantic equality as the digest collision check.
func (p *Plan) WithModuleLoads(input map[cfg.Point]ModuleLoadOperation) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.moduleLoadRefs = make([]uint32, len(p.rows))
	out.moduleLoads = make([]ModuleLoadOperation, 0, len(input))
	out.moduleLoadTables = make([]ModuleLoadExportTable, 0, 1)
	buckets := make(map[ModuleLoadContentID][]uint32, len(input))
	tableBuckets := make(map[ModuleLoadExportTableContentID][]uint32, 1)
	for rawPoint := 0; rawPoint < len(p.rows); rawPoint++ {
		point := cfg.Point(rawPoint)
		op, ok := input[point]
		if !ok || !op.valid() {
			continue
		}
		var tableRef uint32
		for _, candidate := range tableBuckets[op.table.ContentID()] {
			if out.moduleLoadTables[candidate-1].equal(op.table) {
				tableRef = candidate
				break
			}
		}
		if tableRef == 0 {
			out.moduleLoadTables = append(out.moduleLoadTables, op.table)
			tableRef = uint32(len(out.moduleLoadTables))
			tableBuckets[op.table.ContentID()] = append(tableBuckets[op.table.ContentID()], tableRef)
		}
		op.table = out.moduleLoadTables[tableRef-1]
		var ref uint32
		for _, candidate := range buckets[op.ContentID()] {
			if out.moduleLoads[candidate-1].equal(op) {
				ref = candidate
				break
			}
		}
		if ref == 0 {
			out.moduleLoads = append(out.moduleLoads, op.clone())
			ref = uint32(len(out.moduleLoads))
			buckets[op.ContentID()] = append(buckets[op.ContentID()], ref)
		}
		out.moduleLoadRefs[rawPoint] = ref
	}
	return &out
}

// ModuleLoadOperation returns a detached producer descriptor for point.
func (p *Plan) ModuleLoadOperation(point cfg.Point) (ModuleLoadOperation, bool) {
	if p == nil || uint64(point) >= uint64(len(p.moduleLoadRefs)) {
		return ModuleLoadOperation{}, false
	}
	ref := p.moduleLoadRefs[point]
	if ref == 0 || int(ref) > len(p.moduleLoads) {
		return ModuleLoadOperation{}, false
	}
	return p.moduleLoads[ref-1].clone(), true
}
