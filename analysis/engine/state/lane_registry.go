package state

var defaultLaneCatalog = LaneCatalog{
	specs: []laneSpec{
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
	},
}
