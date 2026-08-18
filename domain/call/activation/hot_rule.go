package activation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
)

// HotRule is Call activation's receipt-native callback. It retains only the
// sealed mounted target-batch identity catalog; Program geometry is consumed
// before this owner is bound.
type HotRule struct {
	owner          *callowner.HotOwner
	implementation *callowner.ActivationRuleImplementation
	transport      *engine.MountedActivationCandidateIssuer
	read           engine.Read[engine.OrderedCells[calldomain.Value]]
	catalog        *TargetBatchCatalog
	receiptsSealed bool
}

// BindHot attaches the one exact Call read and selector callback to its
// callback-free schema fragment. The catalog is issued by the sealed Program
// artifact/target-batch join; this binder neither accepts fabricable route
// rows nor reopens Program or Flow state.
func BindHot(fragment *SchemaFragment, owner *callowner.HotOwner, catalog *TargetBatchCatalog) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || !fragment.semantic.Available() || !fragment.admission.Available() ||
		owner == nil || owner.Algebra() == nil || !owner.Algebra().Valid() || !validHotCatalog(catalog, owner) {
		return nil, false
	}
	rule := &HotRule{owner: owner, catalog: catalog}
	implementation, read, bound := callowner.BindExactActivationRule(owner, fragment.slot, fragment.read, engine.HotActivationSpec{
		Admission: engine.AdmitActivationByTrustedTheorem(fragment.admission),
		Run:       rule.run,
	})
	if !bound || implementation == nil {
		return nil, false
	}
	rule.implementation, rule.read = implementation, read
	return rule, true
}

// Implementation returns Call's opaque activation issuer after verifying the
// exact sealed Binding.  No engine slot or callback is exposed.
func (rule *HotRule) Implementation() (*callowner.ActivationRuleImplementation, bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := callowner.ResolveActivationRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

// BindMountedTransport completes the one pre-seal activation bridge. This
// package owns the call plane's transport roster: the value, call, heap and
// pack lanes are imported into a mounted body and the effect lane is exported
// back out of it. It may run once only; the issuer itself is engine owned and
// accepts no caller-provided point or factor-edge rows.
func BindMountedTransport[V, C, H, P, E any](rule *HotRule, value engine.FactorRef[V], calls engine.FactorRef[C], heap engine.FactorRef[H], pack engine.FactorRef[P], effect engine.FactorRef[E]) bool {
	if rule == nil || rule.owner == nil || rule.implementation == nil || rule.transport != nil {
		return false
	}
	imports := []engine.AnyFactorRef{value.Any(), calls.Any(), heap.Any(), pack.Any()}
	issuer, ok := callowner.BindMountedActivationCandidateIssuer(rule.implementation, imports, effect.Any())
	if !ok || issuer == nil {
		return false
	}
	rule.transport = issuer
	return true
}

func (rule *HotRule) run(activation engine.Activation) bool {
	if rule == nil || rule.owner == nil || rule.owner.Algebra() == nil {
		return false
	}
	application, applicationOK := engine.ActivationApplication(activation)
	cells, readOK := engine.ActivationReadValue(activation, rule.read)
	if !applicationOK || !readOK || cells.Count() != 1 {
		return false
	}
	value, present, available := cells.At(0)
	if !available || !present {
		return available
	}
	return rule.visit(value, func(item route) bool {
		return engine.Activate(activation, application, item.target, item.endpoint)
	})
}

func (rule *HotRule) visit(value calldomain.Value, apply func(route) bool) bool {
	if rule == nil || rule.owner == nil || rule.owner.Algebra() == nil || !rule.catalog.valid() || apply == nil {
		return false
	}
	if value.IsTop() {
		for index := 0; index < rule.catalog.routeCount(); index++ {
			item, ok := rule.catalog.routeAt(index)
			if !ok || !apply(item) {
				return false
			}
		}
		return true
	}
	bodies := rule.owner.Algebra().Bodies()
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, ok := value.KnownTargetAt(index)
		if !ok {
			return false
		}
		body, bodyOK := target.Body()
		if !bodyOK {
			continue
		}
		bodyIndex, indexOK := bodies.Index(body)
		if !indexOK {
			return false
		}
		item, routeOK := rule.catalog.routeAt(bodyIndex)
		if !routeOK || !item.body.Same(body) || !apply(item) {
			return false
		}
	}
	return true
}

func validHotCatalog(catalog *TargetBatchCatalog, owner *callowner.HotOwner) bool {
	if catalog == nil || owner == nil || owner.Algebra() == nil || !catalog.valid() {
		return false
	}
	bodies := owner.Algebra().Bodies()
	if bodies.Count() != len(catalog.rows) {
		return false
	}
	for index := 0; index < bodies.Count(); index++ {
		body, bodyOK := bodies.At(index)
		item, itemOK := catalog.routeAt(index)
		if !bodyOK || !itemOK || !item.body.Same(body) {
			return false
		}
		bodyID, bodyIDOK := body.ContentID()
		moduleKey, moduleKeyOK := body.ModuleKey()
		bodyPath, bodyPathOK := body.BodyPath()
		programID, programIDOK := body.ProgramID()
		role, roleOK := body.RoleID()
		row := catalog.rows[index]
		if !bodyIDOK || !moduleKeyOK || !bodyPathOK || !programIDOK || !roleOK || row.moduleKey != moduleKey ||
			row.bodyID != bodyID || row.role != role || row.bodyPath != bodyPath || row.programID != programID {
			return false
		}
	}
	return true
}
