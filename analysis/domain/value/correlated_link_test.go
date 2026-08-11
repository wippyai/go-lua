package value

import "testing"

// Return boundaries are Value-owned Program operands, not Link rows. This
// keeps the returned Pack in the existing Value coordinate system while
// making the source relation independently enumerable from Program.
func TestReturnBoundaryUsesDirectProgramReturn(t *testing.T) {
	schema, source := correlatedFixture(t, `
local x = 1
return x
`, false)
	foreign, _ := correlatedFixture(t, `return 2`, false)
	found := 0
	for shardIndex := 0; shardIndex < source.Project().Mounts().Count(); shardIndex++ {
		shard, ok := source.Project().Mounts().At(shardIndex)
		if !ok {
			t.Fatalf("ShardAt(%d)", shardIndex)
		}
		program, ok := source.Project().Mounts().Program(shard)
		if !ok || program == nil {
			t.Fatalf("Program(%v)", shard)
		}
		returns := program.Flow().Authored().Control().Returns()
		for returnIndex := 0; returnIndex < returns.Count(); returnIndex++ {
			term, ok := returns.At(returnIndex)
			if !ok || !program.Flow().Executable().Contains(term) {
				continue
			}
			boundary, ok := schema.ReturnBoundary(shard, term)
			if !ok || !schema.OwnsReturnBoundary(boundary) || foreign.OwnsReturnBoundary(boundary) {
				t.Fatalf("ReturnBoundary(%v,%d) owner fence", shard, term)
			}
			id, ok := boundary.ID()
			if !ok || !id.Available() {
				t.Fatalf("ReturnBoundary(%v,%d) identity", shard, term)
			}
			values, ok := boundary.Values()
			if !ok || !values.Valid() {
				t.Fatalf("ReturnBoundary(%v,%d) Value coordinate", shard, term)
			}
			found++
		}
	}
	if found == 0 {
		t.Fatal("fixture omitted executable return")
	}
}
