package state

var defaultDomainLaneCatalog = LaneCatalog{
	specs: []stateLaneSpec{
		valuesDomainLane,
		pathEvidenceDomainLane,
		dynamicIndexDomainLane,
		heapTableIdentityDomainLane,
		frozenTablesDomainLane,
		effectDeltasDomainLane,
		escapeEventsDomainLane,
		channelSelectDomainLane,
		storeRelationsDomainLane,
		typestatesDomainLane,
		placementDomainLane,
		lenFloorsDomainLane,
		numFloorsDomainLane,
		diffRelationsDomainLane,
	},
}
