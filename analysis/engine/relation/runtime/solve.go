package runtime

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/eval/delta"
	"github.com/wippyai/go-lua/analysis/engine/relation/eval/step"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/relation/solve/fixpoint"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Solve runs the one serial coordinator for an exact mounted execution.
// Planning, physical-node selection, dependency wake tables, and widening
// authority have already been sealed by the preceding layers.  This function
// only composes those authorities:
//
//  1. seed the complete initial root once;
//  2. redeem one authenticated work item and evaluate its exact entry;
//  3. advance the root only through a Publish settlement carrying an
//     authenticated database delta;
//  4. seed the exact successor delta through the queue's sealed wake index;
//  5. stop when the queue is empty.
//
// No callback, fallback scan, copied dependency graph, caller-supplied
// widening flag, or concrete domain value enters this loop.  A malformed or
// foreign authority refuses the whole solve rather than being compensated by
// a rescan or fabricated root.
func Solve(mounted witness.Mounted, base database.Version, view geometry.Geometry) (result terminal.Result, solved bool) {
	if !mounted.Available() || !base.Available() || !view.ValidFor(mounted) {
		return terminal.Result{}, false
	}
	execution := mounted.Arrangement().Execution()
	queue, ok := fixpoint.New(execution, mounted)
	if !ok {
		return terminal.Result{}, false
	}
	catalog, ok := applicationCatalog(mounted)
	if !ok {
		return terminal.Result{}, false
	}
	initial, ok := fixpoint.Full(base)
	if !ok || !queue.SeedFull(initial) {
		return terminal.Result{}, false
	}

	current := base
	var evaluations, publications uint64
	for {
		work, ok := queue.Next()
		if !ok {
			break
		}
		root := work.Root()
		observed, ok := observedRoot(root)
		if !ok || !observed.Same(current) {
			// Queue normally enforces this invariant.  Keep the coordinator's
			// boundary strict as well: a stale or foreign Work root must never
			// be silently replaced by the coordinator's current version.
			return terminal.Result{}, false
		}
		entry, ok := queue.Entry(work)
		if !ok || !entry.Available() {
			return terminal.Result{}, false
		}
		switch root.Mode() {
		case fixpoint.FullRoot:
			version, rootOK := root.FullVersion()
			if !rootOK || !version.Same(current) {
				return terminal.Result{}, false
			}
			value, evaluateOK := redeemFull(mounted, root, entry, current, view)
			if !evaluateOK || !validResultEntry(entry, value.Available(), value.Dependency(), value.Expression(), value.Node(), value.Kind()) {
				return terminal.Result{}, false
			}
			evaluations++
			updated, observeOK := observeApplications(catalog, observed, entry.Dependency(), value.Applications())
			if !observeOK {
				return terminal.Result{}, false
			}
			catalog = updated
			if value.Kind() != algebra.KindPublish {
				continue
			}
			if !advanceSettlements(&current, &publications, &queue, value.Settlements()) {
				return terminal.Result{}, false
			}
		case fixpoint.LaterRoot:
			inputDelta, rootOK := root.Delta()
			if !rootOK || !inputDelta.Available() || !inputDelta.Next().Same(current) {
				return terminal.Result{}, false
			}
			value, evaluateOK := redeemLater(mounted, root, entry, current, view)
			if !evaluateOK || !validResultEntry(entry, value.Available(), value.Dependency(), value.Expression(), value.Node(), value.Kind()) {
				return terminal.Result{}, false
			}
			evaluations++
			if value.Kind() != algebra.KindPublish {
				if !value.Next().Same(current) {
					return terminal.Result{}, false
				}
			}
			updated, observeOK := observeApplications(catalog, observed, entry.Dependency(), value.Applications())
			if !observeOK {
				return terminal.Result{}, false
			}
			catalog = updated
			if value.Kind() != algebra.KindPublish {
				continue
			}
			if !advanceSettlements(&current, &publications, &queue, value.Settlements()) || !value.Next().Same(current) {
				return terminal.Result{}, false
			}
		default:
			return terminal.Result{}, false
		}
	}

	result, solved = terminal.New(current, evaluations, publications, catalog)
	return result, solved
}

// applicationCatalog admits exactly the mounted schema observation catalogue.
// It is intentionally derived once from the mount; runtime never reconstructs
// observations from schedule nodes or admits a caller-supplied descriptor.
func applicationCatalog(mounted witness.Mounted) (terminal.Catalog, bool) {
	if !mounted.Available() {
		return terminal.Catalog{}, false
	}
	return terminal.NewCatalog(mounted.Observations())
}

// observeApplications transfers only authenticated Apply result extents to
// terminal state. Every evaluation first clears the declared keys for its
// dependency, so an omitted operation cannot leave stale output behind.
// Results for operations not declared under this dependency are ignored;
// duplicate occurrences of one declared key are refused.
func observeApplications(catalog terminal.Catalog, root database.Version, dependency model.DependencyID, results []apply.Results) (terminal.Catalog, bool) {
	if !catalog.Available() || !root.Available() || !dependency.Available() {
		return terminal.Catalog{}, false
	}
	updated, ok := catalog.ClearDependency(dependency)
	if !ok {
		return terminal.Catalog{}, false
	}
	if len(results) == 0 {
		return updated, updated.CompleteDependency(dependency)
	}
	for _, result := range results {
		if !result.Available() {
			return terminal.Catalog{}, false
		}
		operation := result.Operation()
		if !updated.Declared(dependency, operation) {
			continue
		}
		if _, duplicate := updated.Lookup(dependency, operation); duplicate {
			return terminal.Catalog{}, false
		}
		application, applicationOK := terminal.NewApplication(root, dependency, operation, result)
		if !applicationOK {
			return terminal.Catalog{}, false
		}
		updated, ok = updated.Replace(application)
		if !ok {
			return terminal.Catalog{}, false
		}
	}
	if !updated.CompleteDependency(dependency) {
		return terminal.Catalog{}, false
	}
	return updated, true
}

// observedRoot projects the exact committed version a Work item must see.
// Full roots carry that version directly; Later roots carry a successor
// delta, whose Next is the only version visible to the differential session.
func observedRoot(root fixpoint.Root) (database.Version, bool) {
	if !root.Available() {
		return database.Version{}, false
	}
	switch root.Mode() {
	case fixpoint.FullRoot:
		return root.FullVersion()
	case fixpoint.LaterRoot:
		deltaValue, ok := root.Delta()
		if !ok || !deltaValue.Available() {
			return database.Version{}, false
		}
		return deltaValue.Next(), deltaValue.Next().Available()
	default:
		return database.Version{}, false
	}
}

func validResultEntry(entry arrangement.ScheduleEntry, available bool, dependency model.DependencyID, expression model.ExpressionID, node identity.ContentID, kind algebra.Kind) bool {
	return entry.Available() && available && dependency == entry.Dependency() && expression == entry.Expression() && node == entry.Node().Digest() && kind == entry.Node().Kind()
}

// redeemFull is the only full evaluator dispatch.  It is kept typed so no
// public or private result adapter can make a Later value look like a full
// result.
func redeemFull(mounted witness.Mounted, root fixpoint.Root, entry arrangement.ScheduleEntry, current database.Version, view geometry.Geometry) (step.Result, bool) {
	if !mounted.Available() || !root.Available() || !entry.Available() || !current.Available() || !view.ValidFor(mounted) {
		return step.Result{}, false
	}
	if observed, ok := observedRoot(root); !ok || !observed.Same(current) {
		return step.Result{}, false
	}
	if root.Mode() != fixpoint.FullRoot {
		return step.Result{}, false
	}
	version, ok := root.FullVersion()
	if !ok || !version.Same(current) {
		return step.Result{}, false
	}
	session, ok := step.New(mounted, version, view)
	if !ok || !session.Available() {
		return step.Result{}, false
	}
	value, ok := session.Evaluate(entry)
	return value, ok && value.Available()
}

// redeemLater is the only differential evaluator dispatch.  A Full root is
// rejected before delta.New, so it cannot be replayed through a later path.
func redeemLater(mounted witness.Mounted, root fixpoint.Root, entry arrangement.ScheduleEntry, current database.Version, view geometry.Geometry) (delta.Result, bool) {
	if !mounted.Available() || !root.Available() || !entry.Available() || !current.Available() || !view.ValidFor(mounted) {
		return delta.Result{}, false
	}
	if root.Mode() != fixpoint.LaterRoot {
		return delta.Result{}, false
	}
	inputDelta, ok := root.Delta()
	if !ok || !inputDelta.Available() || !inputDelta.Next().Same(current) {
		return delta.Result{}, false
	}
	session, ok := delta.New(mounted, root, view)
	if !ok || !session.Available() {
		return delta.Result{}, false
	}
	value, ok := session.Evaluate(entry)
	if !ok || !value.Available() || !value.InputDelta().Available() || !value.InputDelta().Base().Same(inputDelta.Base()) || !value.InputDelta().Next().Same(inputDelta.Next()) || !value.Base().Same(inputDelta.Base()) || !value.Successor().Same(inputDelta.Next()) || !value.Next().Available() {
		return delta.Result{}, false
	}
	return value, true
}

func advanceSettlements(current *database.Version, publications *uint64, queue *fixpoint.Queue, settlements []publish.Settlement) bool {
	if current == nil || publications == nil || queue == nil || settlements == nil {
		return false
	}
	for _, settlement := range settlements {
		if !settlement.Available() || !settlement.Base().Same(*current) || !settlement.Next().Available() {
			return false
		}
		deltaValue, changed := settlement.Delta()
		if !changed {
			if !settlement.Next().Same(*current) {
				return false
			}
			continue
		}
		if !deltaValue.Available() || !deltaValue.Base().Same(*current) || !deltaValue.Next().Same(settlement.Next()) {
			return false
		}
		next := settlement.Next()
		later, ok := fixpoint.Later(deltaValue)
		if !ok || !queue.SeedLater(later) {
			return false
		}
		*current = next
		(*publications)++
	}
	return true
}
