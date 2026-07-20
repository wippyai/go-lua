package callboundary

// Clone returns a detached copy of f. Product and domain values are immutable;
// path segment slices are the only nested mutable storage inside fact records.
func (f NormalReturnFacts) Clone() NormalReturnFacts {
	out := NormalReturnFacts{
		PathRefinements:          cloneNormalReturnSlice(f.PathRefinements),
		PersistentPathWrites:     cloneNormalReturnSlice(f.PersistentPathWrites),
		PathStaticMembers:        cloneNormalReturnSlice(f.PathStaticMembers),
		PathStaticMemberDeltas:   cloneNormalReturnSlice(f.PathStaticMemberDeltas),
		PathInvalidations:        cloneNormalReturnSlice(f.PathInvalidations),
		DynamicIndexFacts:        cloneNormalReturnSlice(f.DynamicIndexFacts),
		KeyMemberships:           cloneNormalReturnSlice(f.KeyMemberships),
		DynamicValueKeys:         cloneNormalReturnSlice(f.DynamicValueKeys),
		DynamicAllValues:         cloneNormalReturnSlice(f.DynamicAllValues),
		BranchProofs:             cloneNormalReturnSlice(f.BranchProofs),
		PathPresenceImplications: cloneNormalReturnSlice(f.PathPresenceImplications),
		ChannelSelects:           cloneNormalReturnSlice(f.ChannelSelects),
		FrozenTables:             cloneNormalReturnSlice(f.FrozenTables),
		EffectDeltas:             cloneNormalReturnSlice(f.EffectDeltas),
		EscapeEvents:             cloneNormalReturnSlice(f.EscapeEvents),
		StoreRelations:           cloneNormalReturnSlice(f.StoreRelations),
		LifecycleFacts:           cloneNormalReturnSlice(f.LifecycleFacts),
		NumFloors:                cloneNormalReturnSlice(f.NumFloors),
		NumCeils:                 cloneNormalReturnSlice(f.NumCeils),
		RelConstraints:           cloneNormalReturnSlice(f.RelConstraints),
	}
	for i := range out.PathRefinements {
		out.PathRefinements[i].Path = out.PathRefinements[i].Path.Clone()
	}
	for i := range out.PersistentPathWrites {
		out.PersistentPathWrites[i].Path = out.PersistentPathWrites[i].Path.Clone()
	}
	for i := range out.PathStaticMembers {
		out.PathStaticMembers[i].Path = out.PathStaticMembers[i].Path.Clone()
	}
	for i := range out.PathStaticMemberDeltas {
		out.PathStaticMemberDeltas[i].Path = out.PathStaticMemberDeltas[i].Path.Clone()
	}
	for i := range out.PathInvalidations {
		out.PathInvalidations[i].Path = out.PathInvalidations[i].Path.Clone()
	}
	for i := range out.DynamicIndexFacts {
		out.DynamicIndexFacts[i].Table = out.DynamicIndexFacts[i].Table.Clone()
		out.DynamicIndexFacts[i].KeyPath = out.DynamicIndexFacts[i].KeyPath.Clone()
		out.DynamicIndexFacts[i].ValuePath = out.DynamicIndexFacts[i].ValuePath.Clone()
	}
	for i := range out.KeyMemberships {
		out.KeyMemberships[i].Key = out.KeyMemberships[i].Key.Clone()
		out.KeyMemberships[i].Table = out.KeyMemberships[i].Table.Clone()
	}
	for i := range out.DynamicValueKeys {
		out.DynamicValueKeys[i].Container = out.DynamicValueKeys[i].Container.Clone()
		out.DynamicValueKeys[i].Table = out.DynamicValueKeys[i].Table.Clone()
	}
	for i := range out.DynamicAllValues {
		out.DynamicAllValues[i].Container = out.DynamicAllValues[i].Container.Clone()
		out.DynamicAllValues[i].Table = out.DynamicAllValues[i].Table.Clone()
	}
	for i := range out.BranchProofs {
		out.BranchProofs[i].Path = out.BranchProofs[i].Path.Clone()
		out.BranchProofs[i].Other = out.BranchProofs[i].Other.Clone()
	}
	for i := range out.PathPresenceImplications {
		out.PathPresenceImplications[i].Trigger = out.PathPresenceImplications[i].Trigger.Clone()
		out.PathPresenceImplications[i].Target = out.PathPresenceImplications[i].Target.Clone()
	}
	for i := range out.ChannelSelects {
		out.ChannelSelects[i].Result = out.ChannelSelects[i].Result.Clone()
		out.ChannelSelects[i].Case = out.ChannelSelects[i].Case.Clone()
	}
	for i := range out.FrozenTables {
		out.FrozenTables[i].Target = out.FrozenTables[i].Target.Clone()
	}
	for i := range out.EffectDeltas {
		out.EffectDeltas[i].Target = out.EffectDeltas[i].Target.Clone()
	}
	for i := range out.EscapeEvents {
		out.EscapeEvents[i].Target = out.EscapeEvents[i].Target.Clone()
	}
	for i := range out.StoreRelations {
		out.StoreRelations[i].Source = out.StoreRelations[i].Source.Clone()
		out.StoreRelations[i].Into = out.StoreRelations[i].Into.Clone()
	}
	for i := range out.LifecycleFacts {
		out.LifecycleFacts[i].Target = out.LifecycleFacts[i].Target.Clone()
	}
	for i := range out.NumFloors {
		out.NumFloors[i].Path = out.NumFloors[i].Path.Clone()
	}
	for i := range out.NumCeils {
		out.NumCeils[i].Path = out.NumCeils[i].Path.Clone()
	}
	for i := range out.RelConstraints {
		out.RelConstraints[i].A.Path = out.RelConstraints[i].A.Path.Clone()
		out.RelConstraints[i].B.Path = out.RelConstraints[i].B.Path.Clone()
		out.RelConstraints[i].C.Path = out.RelConstraints[i].C.Path.Clone()
	}
	return out
}

func cloneNormalReturnSlice[T any](in []T) []T { return append([]T(nil), in...) }
