package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/internal/canonical"
)

const (
	pointStateDenominatorDomain = "engine/point-state-snapshot-denominator"
	pointStateIdentityVersion   = 1
)

func pointStateAxis(schema identity.ContentID) snapshot.Axis[composition.Key, carrier.PointState] {
	return snapshot.Axis[composition.Key, carrier.PointState]{SchemaID: schema, Slot: solvedPointSlot}
}

func pointStateDenominatorIdentity(schema identity.ContentID) identity.ContentID {
	if !schema.Available() {
		return identity.ContentID{}
	}
	return framedContentID(pointStateDenominatorDomain, pointStateIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(schema[:]) == nil
	})
}

// sealPointUniverse freezes the complete graph Point key universe. The dense
// pointKeys vector lets publication writes address a Point without asking the
// graph again, while pointIndex proves the key belongs to exactly one Point.
// Initial publication may leave an inactive key absent, but the denominator
// already proves that absence so a later widened epoch can fill the same row.
// The denominator identity is minted once for this schema.
func sealPointUniverse(runtime *solverRuntime, schema identity.ContentID) ([]composition.Key, []composition.Key, map[composition.Key]int, identity.ContentID, bool) {
	if runtime == nil || runtime.graph == nil || !schema.Available() || len(runtime.activePoints) != runtime.graph.PointCount() {
		return nil, nil, nil, identity.ContentID{}, false
	}
	pointKeys := make([]composition.Key, runtime.graph.PointCount())
	pointMembers := make([]composition.Key, 0, runtime.graph.PointCount())
	pointIndex := make(map[composition.Key]int)
	for index := range pointKeys {
		point, ok := runtime.graph.PointAt(schedule.Node(index))
		if !ok || !point.Key().Available() {
			return nil, nil, nil, identity.ContentID{}, false
		}
		key := point.Key()
		if _, duplicate := pointIndex[key]; duplicate {
			return nil, nil, nil, identity.ContentID{}, false
		}
		pointKeys[index] = key
		pointIndex[key] = index
		pointMembers = append(pointMembers, key)
	}
	denominator := pointStateDenominatorIdentity(schema)
	if len(pointMembers) == 0 || !denominator.Available() {
		return nil, nil, nil, identity.ContentID{}, false
	}
	return pointMembers, pointKeys, pointIndex, denominator, true
}

// collectActivePointRows collects only solve-local active Point values. Its
// complete key universe and denominator are owned by the sealed publication
// plan; this map is needed only when a fresh builder first declares the point
// column.
func collectActivePointRows(plan *solvedPublicationPlan, epoch *executorEpoch) (map[composition.Key]carrier.PointState, bool) {
	if plan == nil || epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || len(plan.pointKeys) != epoch.runtime.graph.PointCount() || len(plan.pointMembers) == 0 || len(epoch.points) != len(plan.pointKeys) {
		return nil, false
	}
	rows := make(map[composition.Key]carrier.PointState, len(plan.pointMembers))
	for index, key := range plan.pointKeys {
		if !key.Available() {
			continue
		}
		if index >= len(epoch.runtime.activePoints) {
			return nil, false
		}
		if !epoch.runtime.activePoints[index] {
			continue
		}
		if index >= len(epoch.points) || !epoch.points[index].Valid() {
			return nil, false
		}
		if pointIndex, indexed := plan.pointIndex[key]; !indexed || pointIndex != index {
			return nil, false
		}
		rows[key] = epoch.points[index]
	}
	return rows, len(rows) != 0
}

func declarePointColumnFromPlan(plan *solvedPublicationPlan, builder *snapshot.Builder, rows map[composition.Key]carrier.PointState) (snapshot.Axis[composition.Key, carrier.PointState], bool) {
	if plan == nil || builder == nil || !plan.pointAxis.Available() || !plan.pointDenominator.Available() || len(plan.pointMembers) == 0 || len(rows) == 0 || len(rows) > len(plan.pointMembers) || !plan.pointWrite.Available() {
		return snapshot.Axis[composition.Key, carrier.PointState]{}, false
	}
	err := PublishColumn(plan.pointWrite, builder, snapshot.Content[composition.Key, carrier.PointState]{
		Rows:        rows,
		Denominator: plan.pointDenominator,
		Members:     plan.pointMembers,
	})
	return plan.pointAxis, err == nil
}

func (publication *solvedPublication) readPoint(epoch *executorEpoch, point int) (carrier.PointState, bool) {
	if publication == nil || epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || !publication.pointAxis.Available() {
		return carrier.PointState{}, false
	}
	key, covered := publication.pointKey(epoch, point)
	if !covered {
		return carrier.PointState{}, false
	}
	value, status := snapshot.ReadOverlay(&publication.builder, publication.pointAxis, key)
	return value, status == snapshot.ReadHit && value.Valid()
}

func (publication *solvedPublication) writePoint(epoch *executorEpoch, point int, held carrier.PointState) bool {
	if publication == nil || epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || !publication.pointAxis.Available() || !publication.pointWrite.Available() {
		return false
	}
	key, covered := publication.pointKey(epoch, point)
	if !covered {
		return false
	}
	if held.Valid() {
		return PublishRow(publication.pointWrite, &publication.builder, key, held) == nil
	}
	return WithdrawRow(publication.pointWrite, &publication.builder, key) == nil
}

func (publication *solvedPublication) pointKey(epoch *executorEpoch, point int) (composition.Key, bool) {
	if publication == nil || epoch == nil || epoch.runtime == nil || epoch.runtime.publication != publication.plan || publication.plan == nil || !publication.plan.pointAxis.Available() || point < 0 || point >= len(publication.plan.pointKeys) {
		return composition.Key{}, false
	}
	key := publication.plan.pointKeys[point]
	if !key.Available() {
		return composition.Key{}, false
	}
	if indexed, ok := publication.plan.pointIndex[key]; !ok || indexed != point {
		return composition.Key{}, false
	}
	_, status := snapshot.ReadOverlay(&publication.builder, publication.pointAxis, key)
	if status != snapshot.ReadHit && status != snapshot.ReadProvenAbsent {
		return composition.Key{}, false
	}
	return key, true
}

func readPointState(published snapshot.Snapshot, key composition.Key) (carrier.PointState, bool) {
	if !published.Published() || !key.Available() {
		return carrier.PointState{}, false
	}
	value, status := snapshot.Read(&published, pointStateAxis(published.Schema()), key)
	return value, status == snapshot.ReadHit && value.Valid()
}
