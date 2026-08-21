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

func TestAuthoredScalarQueriesRespectOwnerAndRepresentatives(t *testing.T) {
	project, boundary, spec := moduleFixture(t)
	spec.ModuleCacheAliases[0] = ModuleCacheAliasClassSpec{
		Actor: "actor", Instances: []string{"cache-main", "cache-main-alt"}, Representative: "cache-main",
	}
	spec.AnalysisRoots = append(spec.AnalysisRoots, AnalysisRootSpec{
		Name: "main-alt", Module: "main", Actor: "actor", Instance: "cache-main-alt",
	})
	draft, err := Build(Input{Project: project, Boundary: boundary, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}

	actors, cache := component.Actors(), component.Cache()
	actor, ok := actors.At(0)
	if !ok {
		t.Fatal("actor unavailable")
	}
	actorID, ok := actors.ID(actor)
	if !ok || !actorID.Available() {
		t.Fatal("actor ID unavailable")
	}
	foreignActor, _ := sealModuleFixture(t).Actors().At(0)
	if _, ok := actors.ID(foreignActor); ok {
		t.Fatal("foreign actor accepted")
	}

	for index, row := range component.authority.instances {
		instance := ModuleCacheInstance{component: component, ordinal: uint32(index + 1)}
		instanceID, ok := cache.InstanceID(instance)
		if !ok || !instanceID.Available() {
			t.Fatalf("instance %d ID unavailable", index)
		}
		representative, ok := cache.Representative(instance)
		if !ok {
			t.Fatalf("instance %d representative unavailable", index)
		}
		representativeID, ok := cache.InstanceID(representative)
		if !ok || !representativeID.Available() {
			t.Fatalf("instance %d representative ID unavailable", index)
		}
		if row.representative == uint32(index+1) && representative != instance {
			t.Fatalf("instance %d representative did not remain reflexive", index)
		}
		if row.representative != uint32(index+1) && representative == instance {
			t.Fatalf("instance %d nonrepresentative collapsed to itself", index)
		}
		if row.representative != uint32(index+1) && instanceID == representativeID {
			t.Fatalf("instance %d and representative share identity", index)
		}
	}
	foreign := ModuleCacheInstance{component: sealModuleFixture(t), ordinal: 1}
	if _, ok := cache.InstanceID(foreign); ok {
		t.Fatal("foreign cache instance accepted")
	}
	if _, ok := cache.Representative(foreign); ok {
		t.Fatal("foreign cache representative accepted")
	}
}
