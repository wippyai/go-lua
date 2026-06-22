package state

var defaultDomainLaneFactories = []stateLaneFactory{
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
}
