package state

var defaultDomainLaneCatalog = LaneCatalog{
	factories: []stateLaneFactory{
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
