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
