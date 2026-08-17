package module

import "testing"

func TestModuleQueriesResolveShardRootRanges(t *testing.T) {
	component := sealModuleFixture(t)
	shard, ok := component.authority.project.Mounts().At(0)
	if !ok {
		t.Fatal("module fixture mount unavailable")
	}
	count := component.Roots().ForShardCount(shard)
	if count == 0 {
		t.Fatal("root range was empty for mounted shard")
	}
	for index := 0; index < count; index++ {
		root, ok := component.Roots().ForShardAt(shard, index)
		if !ok {
			t.Fatalf("root range row %d unavailable", index)
		}
		if _, _, _, ok := component.Roots().Mapping(root); !ok {
			t.Fatalf("root range row %d has no mapping", index)
		}
	}
	if _, ok := component.Roots().ForShardAt(shard, component.Roots().Count()); ok {
		t.Fatal("root range accepted its upper bound")
	}
}
