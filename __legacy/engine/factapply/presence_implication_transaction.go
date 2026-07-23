package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ConcretePresenceImplicationCancellation selects the state published when a
// presence-implication barrier observes cancellation. Node transfers are one
// transaction and roll back to their immutable input. Edge transfers are
// already evolving a predecessor state and retain the last completed closure
// round.
type ConcretePresenceImplicationCancellation uint8

const (
	ConcretePresenceImplicationKeepEvolving ConcretePresenceImplicationCancellation = iota
	ConcretePresenceImplicationRollbackNode
)

// ConcretePresenceImplicationBarriers describes where a publication operation
// closes its facts. The default matches bulk boundary-fact publication. Direct
// path facts use the invalidation-aware form because making a root absent must
// not overtake consequences pending on one of its descendants.
type ConcretePresenceImplicationBarriers uint8

const (
	ConcretePresenceImplicationTrailingBarrier ConcretePresenceImplicationBarriers = iota
	ConcretePresenceImplicationDescendantInvalidationBarriers
)

// ConcretePresenceImplicationRequest describes one atomic publication phase.
// Publications are concrete implication facts, not individual consequence
// firings: Apply closes the whole set to a fixed point at the same barriers as
// the legacy applicator.
//
// Publications are batched behind one trailing barrier by default. With
// ConcretePresenceImplicationDescendantInvalidationBarriers, an implication
// that can make its target absent first closes all pending facts, then publishes
// and closes itself before the next fact. This preserves the invalidation
// ordering relied on by direct path evidence.
type ConcretePresenceImplicationRequest struct {
	Registry     *axis.Registry
	Resolver     *visibility.Resolver
	Point        cfg.Point
	Input        state.State
	Output       state.State
	Publications []pathevidence.PathPresenceImplication
	Token        *cancellation.Token
	Cancellation ConcretePresenceImplicationCancellation
	Barriers     ConcretePresenceImplicationBarriers
}

// ConcretePresenceImplicationResult is the state at the completed barrier, or
// the request's rollback state when Canceled is true.
type ConcretePresenceImplicationResult struct {
	Output   state.State
	Canceled bool
	Err      error
}

// ConcretePresenceImplicationExecutor publishes implication facts and closes
// their consequences. Its zero value is ready to use and carries no mutable
// semantic state, so a future operation-plan interpreter can share the same
// concrete kernel as the current applicator.
type ConcretePresenceImplicationExecutor struct{}

// Apply publishes the request and executes every required closure barrier.
func (*ConcretePresenceImplicationExecutor) Apply(req ConcretePresenceImplicationRequest) ConcretePresenceImplicationResult {
	valuesTop := false
	if req.Registry != nil {
		_, values := state.DecomposeValueLane(state.Domain(req.Registry), req.Output)
		valuesTop = values.Top
	}
	storage := &concretePresenceStorage{
		reg: req.Registry, resolver: req.Resolver, value: req.Output,
		reachable: req.Registry != nil && !state.IsBottom(req.Registry, req.Output),
		valuesTop: valuesTop,
	}
	// A transfer cannot manufacture reachability from the fixed-point Bottom.
	// Publications are semantic consequences of an executable point.
	if !storage.Reachable() {
		return ConcretePresenceImplicationResult{Output: req.Output}
	}
	canceled, err := applyPresenceImplicationStorage(req.Registry, req.Resolver, req.Point, storage, req.Publications, req.Token, req.Barriers)
	if err != nil {
		return ConcretePresenceImplicationResult{Output: req.Input, Err: err}
	}
	if canceled && req.Cancellation == ConcretePresenceImplicationRollbackNode {
		return ConcretePresenceImplicationResult{Output: req.Input, Canceled: true}
	}
	return ConcretePresenceImplicationResult{Output: storage.value, Canceled: canceled}
}

func applyPresenceImplicationStorage(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	storage presenceImplicationStorage,
	publications []pathevidence.PathPresenceImplication,
	token *cancellation.Token,
	barriers ConcretePresenceImplicationBarriers,
) (bool, error) {
	if reg == nil || resolver == nil || storage == nil || !storage.Reachable() {
		return false, nil
	}
	pending := false
	poll := cancellation.NewPoller(token, cancellation.EveryCheap)
	barrier := func() (bool, error) {
		if !pending {
			return false, nil
		}
		canceled, err := closePresenceImplications(reg, resolver, point, storage, token)
		pending = false
		return canceled, err
	}
	for _, implication := range publications {
		if poll.Poll() {
			return true, nil
		}
		if barriers == ConcretePresenceImplicationDescendantInvalidationBarriers && presenceImplicationTargetInvalidatesDescendants(implication) {
			if canceled, err := barrier(); canceled || err != nil {
				return canceled, err
			}
			if !storage.Reachable() {
				return false, nil
			}
			if _, ok := storage.AddImplication(implication); !ok {
				return false, state.ErrInvalidLaneFactor
			}
			pending = true
			if canceled, err := barrier(); canceled || err != nil {
				return canceled, err
			}
			if !storage.Reachable() {
				return false, nil
			}
			continue
		}
		if _, ok := storage.AddImplication(implication); !ok {
			return false, state.ErrInvalidLaneFactor
		}
		pending = true
	}
	if len(publications) == 0 {
		pending = true
	}
	return barrier()
}

// ApplyConcretePresenceImplications is the stateless convenience form.
func ApplyConcretePresenceImplications(req ConcretePresenceImplicationRequest) ConcretePresenceImplicationResult {
	return new(ConcretePresenceImplicationExecutor).Apply(req)
}

// closePresenceImplications computes the exact least fixed point of one sealed
// implication component. The sparse graph supplies a RAW/WAW component, so a
// changed row re-enqueues only this mathematical block; independent blocks are
// never scanned or aligned here. There is no iteration cap.
func closePresenceImplications(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	storage presenceImplicationStorage,
	token *cancellation.Token,
) (bool, error) {
	if reg == nil || resolver == nil || storage == nil || !storage.Reachable() {
		return false, nil
	}
	poll := cancellation.NewPoller(token, cancellation.EveryExpensive)
	if poll.Poll() {
		return true, nil
	}
	sameInventory := func(left, right []pathevidence.PathPresenceImplication) bool {
		if len(left) != len(right) {
			return false
		}
		for index := range left {
			if left[index] != right[index] {
				return false
			}
		}
		return true
	}
restart:
	implications, ok := storage.SnapshotImplications()
	if !ok {
		return false, state.ErrInvalidLaneFactor
	}
	components, ok := presenceImplicationSCCs(storage, resolver.KeySpace(), implications)
	if !ok {
		return false, state.ErrInvalidLaneFactor
	}
	for _, component := range components {
		rows := make([]pathevidence.PathPresenceImplication, len(component))
		for index, rowIndex := range component {
			rows[index] = implications[rowIndex]
		}
		access, accessErr := freezeConcretePresenceStorageAccess(resolver, point, storage, rows)
		if accessErr != nil {
			return false, accessErr
		}
		canceled, err := closePresenceImplicationRows(reg, access, storage, rows, token)
		if canceled || err != nil {
			return canceled, err
		}
		current, valid := storage.SnapshotImplications()
		if !valid {
			return false, state.ErrInvalidLaneFactor
		}
		if !sameInventory(current, implications) {
			goto restart
		}
	}
	return false, nil
}

// closePresenceImplicationRows executes one already-sealed SCC. It performs no
// topology discovery. Rows absent from the current leaf inventory are ignored;
// unrelated implication coordinates may remain present as read-only topology
// witnesses without entering this block's equation system.
func closePresenceImplicationRows(
	reg *axis.Registry,
	access presenceKeyAccess,
	storage presenceImplicationStorage,
	sealed []pathevidence.PathPresenceImplication,
	token *cancellation.Token,
) (bool, error) {
	poll := cancellation.NewPoller(token, cancellation.EveryExpensive)
	for storage.Reachable() {
		if poll.Poll() {
			return true, nil
		}
		changed, err := applyPresenceImplicationRowsRound(reg, access, storage, sealed)
		if err != nil || !changed {
			return false, err
		}
	}
	return false, nil
}

// applyPresenceImplicationRowsRound evaluates exactly one immutable trigger
// snapshot for one presealed row component. It deliberately does not discover
// inventory, schedule SCCs, restart after publication, or iterate to closure.
// The caller decides whether and how another round is scheduled.
func applyPresenceImplicationRowsRound(
	reg *axis.Registry,
	access presenceKeyAccess,
	storage presenceImplicationStorage,
	sealed []pathevidence.PathPresenceImplication,
) (bool, error) {
	if reg == nil || !access.valid() || storage == nil {
		return false, state.ErrInvalidLaneFactor
	}
	if !storage.Reachable() {
		return false, nil
	}
	round := presenceImplicationRound{}
	for _, row := range sealed {
		exists, ok := storage.HasImplication(row)
		if !ok {
			return false, state.ErrInvalidLaneFactor
		}
		if !exists {
			continue
		}
		triggered, err := pathPresenceImplicationTriggered(reg, access, storage, row)
		if err != nil {
			return false, err
		}
		if triggered {
			if err := accumulatePathPresenceImplicationTarget(reg, access, storage, row, &round); err != nil {
				return false, err
			}
		}
	}
	return applyPresenceImplicationRound(reg, storage, round)
}
