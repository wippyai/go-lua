package state

var defaultLaneCatalog = newLaneCatalog([]laneSpec{
	valuesLaneSpec,
	pathEvidenceLaneSpec,
	dynamicIndexLaneSpec,
	heapTableIdentityLaneSpec,
	frozenTablesLaneSpec,
	effectDeltasLaneSpec,
	escapeEventsLaneSpec,
	channelSelectLaneSpec,
	storeRelationsLaneSpec,
	typestatesLaneSpec,
	placementLaneSpec,
	lenFloorsLaneSpec,
	numFloorsLaneSpec,
	diffRelationsLaneSpec,
})

var (
	laneValuesBit            = defaultLaneCatalog.mustLaneBit(LaneValues)
	lanePathEvidenceBit      = defaultLaneCatalog.mustLaneBit(LanePathEvidence)
	laneDynamicIndexBit      = defaultLaneCatalog.mustLaneBit(LaneDynamicIndex)
	laneHeapTableIdentityBit = defaultLaneCatalog.mustLaneBit(LaneHeapTableIdentity)
	laneFrozenTablesBit      = defaultLaneCatalog.mustLaneBit(LaneFrozenTables)
	laneEffectDeltasBit      = defaultLaneCatalog.mustLaneBit(LaneEffectDeltas)
	laneEscapeEventsBit      = defaultLaneCatalog.mustLaneBit(LaneEscapeEvents)
	laneChannelSelectBit     = defaultLaneCatalog.mustLaneBit(LaneChannelSelect)
	laneStoreRelationsBit    = defaultLaneCatalog.mustLaneBit(LaneStoreRelations)
	laneTypestatesBit        = defaultLaneCatalog.mustLaneBit(LaneTypestates)
	lanePlacementBit         = defaultLaneCatalog.mustLaneBit(LanePlacement)
	laneLenFloorsBit         = defaultLaneCatalog.mustLaneBit(LaneLenFloors)
	laneNumFloorsBit         = defaultLaneCatalog.mustLaneBit(LaneNumFloors)
	laneDiffRelationsBit     = defaultLaneCatalog.mustLaneBit(LaneDiffRelations)
)
