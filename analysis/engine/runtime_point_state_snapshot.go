package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/internal/canonical"
)

const (
	pointStateDenominatorDomain = "engine/point-state-snapshot-denominator"
	pointStateRowDomain         = "engine/point-state-snapshot-row"
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

// sealPointUniverse freezes the complete point-state key universe. Explicit
// engine-only construction keeps its historical graph-point keys. An artifact
// plan instead mints one context-qualified key per compact StateOrdinal, so
// two occurrences of the same graph Point in different contexts are distinct
// publication rows rather than an implicit duplicate refusal. The denominator
// is structural and is minted once for this schema.
func sealPointUniverse(runtime *solverRuntime, schema identity.ContentID) ([]composition.Key, identity.ContentID, bool) {
	if runtime == nil || runtime.graph == nil || !schema.Available() || len(runtime.activePoints) != runtime.graph.PointCount() {
		if runtime == nil || runtime.graph == nil || !schema.Available() || !runtime.artifactBacked {
			return nil, identity.ContentID{}, false
		}
	}
	pointCount := runtime.graph.PointCount()
	if runtime.artifactBacked {
		pointCount = runtime.stateCount()
		if pointCount == 0 || len(runtime.activeStates) != pointCount || runtime.executionPlan == nil {
			return nil, identity.ContentID{}, false
		}
	}
	pointKeys := make([]composition.Key, pointCount)
	seen := make(map[composition.Key]struct{}, len(pointKeys))
	for index := range pointKeys {
		var key composition.Key
		if runtime.artifactBacked {
			var keyOK bool
			key, keyOK = artifactStatePointKey(runtime, index)
			if !keyOK {
				return nil, identity.ContentID{}, false
			}
		} else {
			point, ok := runtime.graph.PointAt(schedule.Node(index))
			if !ok || !point.Key().Available() {
				return nil, identity.ContentID{}, false
			}
			key = point.Key()
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, identity.ContentID{}, false
		}
		pointKeys[index] = key
		seen[key] = struct{}{}
	}
	denominator := pointStateDenominatorIdentity(schema)
	if len(pointKeys) == 0 || !denominator.Available() {
		return nil, identity.ContentID{}, false
	}
	return pointKeys, denominator, true
}

// artifactStatePointKey is the publication adapter for one exact compact
// state. It retains the graph point only as metadata in the framed key; the
// StateOrdinal and (when mounted) exact ContextID are part of the row identity.
func artifactStatePointKey(runtime *solverRuntime, state int) (composition.Key, bool) {
	if runtime == nil || !runtime.artifactBacked || runtime.executionPlan == nil || state < 0 || state >= runtime.stateCount() {
		return composition.Key{}, false
	}
	point, pointIndex, _, pointOK := runtime.graphPointAtState(state)
	pointKey := point.Key()
	if !pointOK || pointIndex < 0 || !pointKey.Available() {
		return composition.Key{}, false
	}
	cell, cellOK := runtime.executionPlan.StateAt(contextfiber.StateOrdinal(state))
	if !cellOK {
		return composition.Key{}, false
	}
	compositionID := runtime.graph.CompositionID()
	if !compositionID.Available() {
		return composition.Key{}, false
	}
	contextID := identity.ContentID{}
	if contextOrdinal, mounted := cell.ContextOrdinal(); mounted {
		contextID, _ = runtime.executionPlan.Layout().ContextID(contextOrdinal)
	}
	key, keyOK := framedCompositionKey(pointStateRowDomain, pointStateIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(compositionID[:]) == nil &&
			writer.Uint(uint64(state)) == nil &&
			writer.Bytes(pointKey.ID[:]) == nil &&
			writer.Uint(pointKey.Version) == nil &&
			writer.Bytes(contextID[:]) == nil
	})
	return key, keyOK
}

// collectActivePointRows collects only solve-local active Point values. Its
// complete key universe and denominator are owned by the sealed publication
// plan; this map is needed only when a fresh builder first declares the point
// column.
func collectActivePointRows(plan *solvedPublicationPlan, epoch *executorEpoch) (map[composition.Key]carrier.PointState, bool) {
	if plan == nil || epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || len(plan.pointKeys) == 0 || len(epoch.points) != epoch.runtime.stateCount() {
		return nil, false
	}
	if epoch.runtime.artifactBacked {
		if len(plan.pointKeys) != epoch.runtime.stateCount() || epoch.runtime.executionPlan == nil {
			return nil, false
		}
	} else if len(plan.pointKeys) != epoch.runtime.graph.PointCount() {
		return nil, false
	}
	rows := make(map[composition.Key]carrier.PointState, len(plan.pointKeys))
	for state := range epoch.points {
		if !epoch.runtime.activeState(state) {
			continue
		}
		keyIndex := state
		if !epoch.runtime.artifactBacked {
			_, pointIndex, _, pointOK := epoch.runtime.graphPointAtState(state)
			if !pointOK || pointIndex < 0 || pointIndex >= len(plan.pointKeys) {
				return nil, false
			}
			keyIndex = pointIndex
		}
		if keyIndex < 0 || keyIndex >= len(plan.pointKeys) {
			return nil, false
		}
		key := plan.pointKeys[keyIndex]
		if !key.Available() || !epoch.points[state].Valid() {
			return nil, false
		}
		if _, present := rows[key]; present {
			return nil, false
		}
		rows[key] = epoch.points[state]
	}
	return rows, len(rows) != 0
}

func declarePointColumnFromPlan(plan *solvedPublicationPlan, builder *snapshot.Builder, rows map[composition.Key]carrier.PointState) (snapshot.Axis[composition.Key, carrier.PointState], bool) {
	if plan == nil || builder == nil || !plan.pointAxis.Available() || !plan.pointDenominator.Available() || len(plan.pointKeys) == 0 || len(rows) == 0 || len(rows) > len(plan.pointKeys) || !plan.pointWrite.Available() {
		return snapshot.Axis[composition.Key, carrier.PointState]{}, false
	}
	err := PublishColumn(plan.pointWrite, builder, snapshot.Content[composition.Key, carrier.PointState]{
		Rows:        rows,
		Denominator: plan.pointDenominator,
		Members:     plan.pointKeys,
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
	if publication == nil || epoch == nil || epoch.runtime == nil || epoch.runtime.publication != publication.plan || publication.plan == nil || !publication.plan.pointAxis.Available() || point < 0 || point >= len(epoch.points) {
		return composition.Key{}, false
	}
	keyIndex := point
	if !epoch.runtime.artifactBacked {
		_, pointIndex, _, pointOK := epoch.runtime.graphPointAtState(point)
		if !pointOK || pointIndex < 0 || pointIndex >= len(publication.plan.pointKeys) {
			return composition.Key{}, false
		}
		keyIndex = pointIndex
	}
	if keyIndex < 0 || keyIndex >= len(publication.plan.pointKeys) {
		return composition.Key{}, false
	}
	key := publication.plan.pointKeys[keyIndex]
	if !key.Available() {
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
