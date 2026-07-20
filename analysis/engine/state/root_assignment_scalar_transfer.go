package state

import (
	"fmt"
	"sort"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

// RootAssignmentNumBound is one optional exact scalar bound. Its private
// presence bit prevents absence from being confused with a legitimate zero.
type RootAssignmentNumBound struct {
	value       int64
	sourceState pathaddr.StateKey
	source      keyspace.Key
	present     bool
	exact       bool
}

func NewRootAssignmentNumBound(value int64) RootAssignmentNumBound {
	return RootAssignmentNumBound{value: value, present: true, exact: true}
}

// NewRootAssignmentNumBoundSource constructs one affine bound copied from the
// point-entry lane and shifted by offset.
func NewRootAssignmentNumBoundSource(source pathaddr.StateKey, offset int64) (RootAssignmentNumBound, error) {
	if source == "" {
		return RootAssignmentNumBound{}, fmt.Errorf("state: invalid root-assignment numeric source")
	}
	return RootAssignmentNumBound{value: offset, sourceState: source, present: true}, nil
}

// RootAssignmentScalarTransferConfig is the representation-independent N4
// sidecar delta computed once from frozen facts. Target and UserSource are
// resolver StateKeys; Seal resolves them into the transaction keyspace.
type RootAssignmentScalarTransferConfig struct {
	Keys       *keyspace.KeySpace
	Target     pathaddr.StateKey
	UserSource pathaddr.StateKey
	NumFloor   RootAssignmentNumBound
	NumCeil    RootAssignmentNumBound
}

// RootAssignmentScalarTransfer is immutable and contains no State. Numeric,
// relational, and user-defined axes consume it through their registered lane
// laws, so product cardinality does not alter orchestration.
type RootAssignmentScalarTransfer struct {
	keys          *keyspace.KeySpace
	target        keyspace.Key
	targetState   pathaddr.StateKey
	userSource    keyspace.Key
	hasUserSource bool
	numFloor      RootAssignmentNumBound
	numCeil       RootAssignmentNumBound
	userRuntime   userlattice.Runtime
	sealed        bool
}

func SealRootAssignmentScalarTransfer(config RootAssignmentScalarTransferConfig) (RootAssignmentScalarTransfer, error) {
	if config.Keys == nil || !config.Keys.Valid() || config.Target == "" {
		return RootAssignmentScalarTransfer{}, fmt.Errorf("state: invalid root-assignment scalar transfer")
	}
	target, ok := config.Keys.InternStateKey(config.Target)
	if !ok || target.Kind == keyspace.KindInvalid {
		return RootAssignmentScalarTransfer{}, fmt.Errorf("state: unresolved root-assignment scalar target")
	}
	transfer := RootAssignmentScalarTransfer{
		keys: config.Keys, target: target, targetState: config.Target,
		numFloor: config.NumFloor, numCeil: config.NumCeil, sealed: true,
	}
	resolveBound := func(name string, bound *RootAssignmentNumBound) error {
		if !bound.present || bound.exact {
			return nil
		}
		source, sourceOK := config.Keys.InternStateKey(bound.sourceState)
		if !sourceOK || source.Kind == keyspace.KindInvalid {
			return fmt.Errorf("state: unresolved root-assignment numeric %s source", name)
		}
		bound.source = source
		return nil
	}
	if err := resolveBound("floor", &transfer.numFloor); err != nil {
		return RootAssignmentScalarTransfer{}, err
	}
	if err := resolveBound("ceil", &transfer.numCeil); err != nil {
		return RootAssignmentScalarTransfer{}, err
	}
	if config.UserSource != "" {
		source, sourceOK := config.Keys.InternStateKey(config.UserSource)
		if !sourceOK || source.Kind == keyspace.KindInvalid {
			return RootAssignmentScalarTransfer{}, fmt.Errorf("state: unresolved root-assignment user source")
		}
		transfer.userSource, transfer.hasUserSource = source, true
	}
	return transfer, nil
}

type RootAssignmentScalarFactorTransaction struct {
	seal     *productDomainSeal
	transfer RootAssignmentScalarTransfer
}

func (d ProductDomain) OwnsRootAssignmentScalarFactorTransaction(transaction RootAssignmentScalarFactorTransaction) bool {
	return d.Valid() && transaction.seal == d.seal && transaction.transfer.sealed
}

func (d ProductDomain) SealRootAssignmentScalarTransfer(transfer RootAssignmentScalarTransfer) (RootAssignmentScalarFactorTransaction, error) {
	if !d.Valid() || !transfer.sealed || transfer.keys == nil || !transfer.keys.Valid() {
		return RootAssignmentScalarFactorTransaction{}, fmt.Errorf("state: invalid root-assignment scalar factor transaction")
	}
	transfer.userRuntime = userlattice.RuntimeFor(d.reg)
	return RootAssignmentScalarFactorTransaction{seal: d.seal, transfer: transfer}, nil
}

// ApplyRootAssignmentScalarTransfer uses exactly the same registered laws as
// factor execution and exists as the concrete adapter/differential oracle.
func (d ProductDomain) ApplyRootAssignmentScalarTransfer(transaction RootAssignmentScalarFactorTransaction, pointEntry, current State) (State, error) {
	if !d.Valid() || transaction.seal != d.seal || !transaction.transfer.sealed {
		return State{}, fmt.Errorf("state: foreign root-assignment scalar factor transaction")
	}
	pointEntry = d.Normalize(pointEntry)
	out := d.Normalize(current)
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		runtime.rootAssignment.applyScalarState(&out, pointEntry, transaction.transfer)
		for coordinateIndex := range runtime.coordinates {
			coordinate := runtime.coordinates[coordinateIndex]
			policy := coordinate.ops.rootAssignment.scalarTransfer
			if policy.kind != coordinateScalarTransferParticipant {
				continue
			}
			pointPayload := runtime.ops.extract(pointEntry)
			currentPayload := runtime.ops.extract(out)
			pointSkeleton, pointEntries, err := coordinate.ops.decompose(pointPayload, transaction.transfer.keys)
			if err != nil {
				return State{}, err
			}
			currentSkeleton, currentEntries, err := coordinate.ops.decompose(currentPayload, transaction.transfer.keys)
			if err != nil {
				return State{}, err
			}
			inventory := make([]coordinateKeyPayload, len(currentEntries))
			for i := range currentEntries {
				inventory[i] = currentEntries[i].key
			}
			demands, present := policy.demand(inventory, transaction.transfer)
			if !present {
				continue
			}
			for _, demand := range demands {
				currentScalar, found := coordinateEntryScalar(coordinate.ops, currentEntries, demand.target)
				if !found {
					currentScalar, err = coordinate.ops.defaultScalar(currentSkeleton, demand.target)
					if err != nil {
						return State{}, err
					}
				}
				var pointSource coordinateScalarPayload
				if demand.hasSource {
					pointSource, found = coordinateEntryScalar(coordinate.ops, pointEntries, demand.source)
					if !found {
						pointSource, err = coordinate.ops.defaultScalar(pointSkeleton, demand.source)
						if err != nil {
							return State{}, err
						}
					}
				}
				nextSkeleton, nextScalar, ok := policy.apply(currentSkeleton, currentScalar, pointSource, demand.hasSource, transaction.transfer)
				if !ok {
					return State{}, ErrInvalidLaneFactor
				}
				currentSkeleton = nextSkeleton
				currentEntries = replaceCoordinateEntryScalar(coordinate.ops, currentEntries, demand.target, nextScalar, transaction.transfer.keys)
			}
			next, err := coordinate.ops.replace(currentPayload, transaction.transfer.keys, currentSkeleton, currentEntries)
			if err != nil {
				return State{}, err
			}
			runtime.ops.install(&out, next)
		}
	}
	return out, nil
}

func coordinateEntryScalar(ops coordinateFamilyOps, entries []coordinateEntry, key coordinateKeyPayload) (coordinateScalarPayload, bool) {
	for _, entry := range entries {
		if ops.keyEqual(entry.key, key) {
			return entry.scalar, true
		}
	}
	return nil, false
}

func replaceCoordinateEntryScalar(ops coordinateFamilyOps, entries []coordinateEntry, key coordinateKeyPayload, scalar coordinateScalarPayload, keys *keyspace.KeySpace) []coordinateEntry {
	out := append([]coordinateEntry(nil), entries...)
	for i := range out {
		if ops.keyEqual(out[i].key, key) {
			out[i].scalar = scalar
			return out
		}
	}
	out = append(out, coordinateEntry{key: key, scalar: scalar})
	sort.Slice(out, func(i, j int) bool { return ops.keyLess(out[i].key, out[j].key, keys) })
	return out
}

// RootAssignmentScalarLanes returns the exact independently executable factor
// inventory whose registered scalar law may change a component.
func (d ProductDomain) RootAssignmentScalarLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, 4)
	for index := range d.factorLanes {
		if d.factorLanes[index].rootAssignment.scalar {
			out = append(out, d.factorLanes[index].lane)
		}
	}
	return out
}

func (d ProductDomain) ApplyRootAssignmentScalarFactor(
	transaction RootAssignmentScalarFactorTransaction,
	pointEntry, current LaneFactor,
) (LaneFactor, error) {
	if !d.Valid() || transaction.seal != d.seal || !transaction.transfer.sealed {
		return LaneFactor{}, fmt.Errorf("state: foreign root-assignment scalar factor transaction")
	}
	pointRuntime, err := d.validateFactor(pointEntry)
	if err != nil {
		return LaneFactor{}, err
	}
	currentRuntime, err := d.validateFactor(current)
	if err != nil || pointRuntime.lane != currentRuntime.lane {
		return LaneFactor{}, fmt.Errorf("%w: root-assignment scalar lane mismatch", ErrInvalidLaneFactor)
	}
	next, changed := currentRuntime.rootAssignment.applyScalarFactor(pointEntry.payload, current.payload, transaction.transfer)
	if !changed {
		return current, nil
	}
	return LaneFactor{lane: currentRuntime.lane, payload: next}, nil
}

func addRootAssignmentInt64(left, right int64) (int64, bool) {
	const maxInt64 = int64(^uint64(0) >> 1)
	const minInt64 = -maxInt64 - 1
	if right > 0 && left > maxInt64-right || right < 0 && left < minInt64-right {
		return 0, false
	}
	return left + right, true
}

func applyRootAssignmentUserScalar(pointEntry, current userLatticeLane, transfer RootAssignmentScalarTransfer) (userLatticeLane, bool) {
	next := current
	changed := false
	for index := 0; index < transfer.userRuntime.Len(); index++ {
		axis := transfer.userRuntime.AxisAt(index)
		value := axis.Bottom()
		if transfer.hasUserSource {
			value = axis.Assign(pointEntry.read(axis, transfer.userSource))
		}
		var wrote bool
		next, wrote = next.write(axis, transfer.target, value)
		changed = changed || wrote
	}
	return next, changed
}
