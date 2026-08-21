package composite

import (
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	callactivation "github.com/wippyai/go-lua/domain/call/activation"
	calldispatch "github.com/wippyai/go-lua/domain/call/dispatch"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	heapclosed "github.com/wippyai/go-lua/domain/heap/allocation/closed"
	heapempty "github.com/wippyai/go-lua/domain/heap/allocation/empty"
	heapingress "github.com/wippyai/go-lua/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/domain/heap/bootstrap"
	heapformalfreeze "github.com/wippyai/go-lua/domain/heap/formalfreeze"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	heappublicationfreeze "github.com/wippyai/go-lua/domain/heap/publicationfreeze"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	packsource "github.com/wippyai/go-lua/domain/pack/source"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementallocation "github.com/wippyai/go-lua/domain/placement/allocation"
	placementcapture "github.com/wippyai/go-lua/domain/placement/capture"
	placementcontainment "github.com/wippyai/go-lua/domain/placement/containment"
	placementformal "github.com/wippyai/go-lua/domain/placement/formal"
	placementfresh "github.com/wippyai/go-lua/domain/placement/fresh"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	placementpublicationescape "github.com/wippyai/go-lua/domain/placement/publicationescape"
	placementreturnescape "github.com/wippyai/go-lua/domain/placement/returnescape"
	placementstore "github.com/wippyai/go-lua/domain/placement/store"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueallocation "github.com/wippyai/go-lua/domain/value/allocation"
	valuearithmetic "github.com/wippyai/go-lua/domain/value/arithmetic"
	valuebootstrap "github.com/wippyai/go-lua/domain/value/bootstrap"
	valueequality "github.com/wippyai/go-lua/domain/value/equality"
	valuemoduleload "github.com/wippyai/go-lua/domain/value/moduleload"
	valueorder "github.com/wippyai/go-lua/domain/value/order"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	valuerefinement "github.com/wippyai/go-lua/domain/value/refinement"
	valueruntimekind "github.com/wippyai/go-lua/domain/value/runtimekind"
	valuesource "github.com/wippyai/go-lua/domain/value/source"
	valuetransfer "github.com/wippyai/go-lua/domain/value/transfer"
)

type Principals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
	HeapPrincipal() *heapowner.SchemaFragment
	PlacementPrincipal() *placementowner.SchemaFragment
	EvidencePrincipal() *placementsuspension.EvidenceFactorFragment
	PackPrincipal() *packowner.SchemaFragment
	EffectPrincipal() *effectowner.SchemaFragment
}

type Authorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	EvidenceAuthority() *placementsuspension.EvidenceOwner
	PackAuthority() *packowner.HotOwner
	EffectAuthority() *effectowner.HotOwner
	ValueSchema() *valuedomain.Schema
	HeapSchema() heapdomain.Schema
	PlacementSchema() placementdomain.Schema
	PackSchema() *packdomain.Schema
	Topology() *heapindex.Topology
	Allocations() *allocationcatalog.Catalog
	ActivationCatalog() *callactivation.TargetBatchCatalog
	TargetContract() *contract.Contract
}

func activationRule(hot *callactivation.HotRule) ActivationRule { return hot }

// RuleTemplates is the single schema composition registration for executable
// rules. It returns data-only catalog entries and the typed compose passes that
// bind each entry exactly once to its domain implementation.
func RuleTemplates[P Principals, A Authorities]() ([]*rule.Template, []RuleContributor[P, A], bool) {
	var admitted []*rule.Template
	var contributors []RuleContributor[P, A]
	rejected := false
	add := func(entry *rule.Template, contributor RuleContributor[P, A], ok bool) {
		if !ok || !contributor.complete(entry) {
			rejected = true
			return
		}
		admitted = append(admitted, entry)
		contributors = append(contributors, contributor)
	}

	add(WireRule(valuesource.RuleEntry[P, A](), valuesource.DeclareRule[P], valuesource.RegisterRule, nil, valuesource.BindRule[A], nil, nil, valuesource.SealProgramRule, nil))
	add(WireRule(packsource.RuleEntry[P, A](), packsource.DeclareRule[P], packsource.RegisterRule, nil, packsource.BindRule[A], nil, nil, packsource.SealProgramRule, nil))
	add(WireRule(heapingress.RuleEntry[P, A](), heapingress.DeclareRule[P], heapingress.RegisterRule, nil, heapingress.BindRule[A], heapingress.FinalizeRule[A], nil, heapingress.SealProgramRule, nil))
	add(WireRule(valueallocation.RuleEntry[P, A](), valueallocation.DeclareRule[P], valueallocation.RegisterRule, nil, valueallocation.BindRule[A], nil, nil, valueallocation.SealProgramRule, nil))
	add(WireRule(heapempty.RuleEntry[P, A](), heapempty.DeclareRule[P], heapempty.RegisterRule, nil, heapempty.BindRule[A], nil, nil, heapempty.SealProgramRule, nil))
	add(WireRule(heapclosed.RuleEntry[P, A](), heapclosed.DeclareRule[P], heapclosed.RegisterRule, nil, heapclosed.BindRule[A], nil, nil, heapclosed.SealProgramRule, nil))
	add(WireRule(heapindex.RawGetEntry[P, A](), heapindex.DeclareRawGet[P], heapindex.RegisterRawGet, nil, heapindex.BindRawGet[A], nil, nil, heapindex.SealRawGetProgramRule, nil))
	add(WireRule(heapindex.RawSetEntry[P, A](), heapindex.DeclareRawSet[P], heapindex.RegisterRawSet, nil, heapindex.BindRawSet[A], nil, nil, heapindex.SealRawSetProgramRule, nil))
	add(WireRule(calldispatch.RuleEntry[P, A](), calldispatch.DeclareRule[P], calldispatch.RegisterRule, nil, calldispatch.BindRule[A], calldispatch.FinalizeRule[A], nil, calldispatch.SealProgramRule, nil))
	add(WireRule(callsite.SelectedEntry[P, A](), callsite.DeclareSelected[P], callsite.RegisterSelected, nil, callsite.BindSelected[A], callsite.FinalizeSelected[A], nil, callsite.SealProgramRule, nil))
	add(WireRule(callsite.OpaqueEntry[P, A](), callsite.DeclareOpaque[P], callsite.RegisterOpaque, nil, callsite.BindOpaque[A], callsite.FinalizeOpaque[A], nil, callsite.SealProgramRule, nil))
	add(WireRule(callsite.BodyEntry[P, A](), callsite.DeclareBody[P], callsite.RegisterBody, nil, callsite.BindBody[A], callsite.FinalizeBody[A], nil, callsite.SealBodyProgramRule, nil))
	add(WireRule(callactivation.RuleEntry[P, A](), callactivation.DeclareRule[P], callactivation.RegisterRule, nil, callactivation.BindRule[A], nil, nil, nil, activationRule))
	add(WireRule(valueruntimekind.RuleEntry[P, A](), valueruntimekind.DeclareRule[P], valueruntimekind.RegisterRule, nil, valueruntimekind.BindRule[A], nil, nil, valueruntimekind.SealProgramRule, nil))
	add(WireRule(valuebootstrap.RuleEntry[P, A](), valuebootstrap.DeclareRule[P], valuebootstrap.RegisterRule, nil, valuebootstrap.BindRule[A], valuebootstrap.FinalizeRule[A], valuebootstrap.LinkCatalog, valuebootstrap.SealProgramRule, nil))
	add(WireRule(heapbootstrap.RuleEntry[P, A](), heapbootstrap.DeclareRule[P], heapbootstrap.RegisterRule, heapbootstrap.PairRule, heapbootstrap.BindRule[A], heapbootstrap.FinalizeRule[A], heapbootstrap.LinkCatalog, heapbootstrap.SealProgramRule, nil))
	add(WireRule(valuetransfer.RuleEntry[P, A](), valuetransfer.DeclareRule[P], valuetransfer.RegisterRule, nil, valuetransfer.BindRule[A], nil, nil, valuetransfer.SealProgramRule, nil))
	add(WireRule(valuearithmetic.RuleEntry[P, A](), valuearithmetic.DeclareRule[P], valuearithmetic.RegisterRule, nil, valuearithmetic.BindRule[A], nil, nil, valuearithmetic.SealProgramRule, nil))
	add(WireRule(valueequality.RuleEntry[P, A](), valueequality.DeclareRule[P], valueequality.RegisterRule, nil, valueequality.BindRule[A], nil, nil, valueequality.SealProgramRule, nil))
	add(WireRule(valueorder.RuleEntry[P, A](), valueorder.DeclareRule[P], valueorder.RegisterRule, nil, valueorder.BindRule[A], nil, nil, valueorder.SealProgramRule, nil))
	add(WireRule(valuerefinement.RuleEntry[P, A](), valuerefinement.DeclareRule[P], valuerefinement.RegisterRule, nil, valuerefinement.BindRule[A], nil, nil, valuerefinement.SealProgramRule, nil))
	add(WireRule(placementallocation.RuleEntry[P, A](), placementallocation.DeclareRule[P], placementallocation.RegisterRule, nil, placementallocation.BindRule[A], nil, nil, placementallocation.SealProgramRule, nil))
	// Keep newly admitted mounted rules at the end of the declaration list:
	// artifact role ordinals address every preceding slot directly.
	add(WireRule(placementreturnescape.RuleEntry[P, A](), placementreturnescape.DeclareRule[P], placementreturnescape.RegisterRule, nil, placementreturnescape.BindRule[A], nil, nil, placementreturnescape.SealProgramRule, nil))
	add(WireRule(placementcapture.RuleEntry[P, A](), placementcapture.DeclareRule[P], placementcapture.RegisterRule, nil, placementcapture.BindRule[A], placementcapture.FinalizeRule[A], nil, placementcapture.SealProgramRule, nil))
	add(WireRule(placementformal.RuleEntry[P, A](), placementformal.DeclareRule[P], placementformal.RegisterRule, nil, placementformal.BindRule[A], placementformal.FinalizeRule[A], nil, placementformal.SealProgramRule, nil))
	// Containment starts Placement's append-only Link tail.
	add(WireRule(placementcontainment.RuleEntry[P, A](), placementcontainment.DeclareRule[P], placementcontainment.RegisterRule, nil, placementcontainment.BindRule[A], placementcontainment.FinalizeRule[A], placementcontainment.LinkCatalog, placementcontainment.SealProgramRule, nil))
	// Storage lifetime is the final mounted Placement consumer. Appending this
	// declaration preserves every existing rule ordinal and keeps its neutral
	// Program/Value prerequisite explicit.
	add(WireRule(placementstore.RuleEntry[P, A](), placementstore.DeclareRule[P], placementstore.RegisterRule, nil, placementstore.BindRule[A], nil, nil, placementstore.SealProgramRule, nil))
	// Suspension consumes the neutral
	// Program liveness rows through the owner-fenced Value/Placement bridge;
	// its established ordinal remains fixed as later Link producers are added.
	add(WireRule(placementsuspension.RuleEntry[P, A](), placementsuspension.DeclareRule[P], placementsuspension.RegisterRule, nil, placementsuspension.BindRule[A], placementsuspension.FinalizeRule[A], placementsuspension.LinkCatalog, placementsuspension.SealProgramRule, nil))
	// Suspension evidence is a separate Link producer with its own typed
	// Heap-aligned Factor receipt. Keeping it adjacent to the class producer
	// makes the ordered projection's source explicit while preserving all
	// preceding rule ordinals.
	add(WireRule(placementsuspension.EvidenceRuleEntry[P, A](), placementsuspension.DeclareEvidenceRule[P], placementsuspension.RegisterEvidenceRule, nil, placementsuspension.BindEvidenceRule[A], placementsuspension.FinalizeEvidenceRule[A], placementsuspension.LinkEvidenceCatalog, placementsuspension.SealEvidenceProgramRule, nil))
	// Fresh roots are a separate zero-input Link denominator. Appending this
	// producer preserves every existing rule ordinal while keeping it disjoint
	// from the mounted Program-allocation seed above.
	add(WireRule(placementfresh.RuleEntry[P, A](), placementfresh.DeclareRule[P], placementfresh.RegisterRule, nil, placementfresh.BindRule[A], placementfresh.FinalizeRule[A], placementfresh.LinkCatalog, placementfresh.SealProgramRule, nil))
	// Module-load result projection is appended so every existing rule ordinal
	// remains stable. Its Call/Value reads are exact and its write is an
	// existing mounted Program CallResultValue coordinate.
	add(WireRule(valuemoduleload.RuleEntry[P, A](), valuemoduleload.DeclareRule[P], valuemoduleload.RegisterRule, nil, valuemoduleload.BindRule[A], nil, nil, valuemoduleload.SealProgramRule, nil))
	// Formal freeze is the terminal mounted Heap transition. It consumes the
	// existing call-effect cut and appends without renumbering any established
	// rule slot.
	add(WireRule(heapformalfreeze.RuleEntry[P, A](), heapformalfreeze.DeclareRule[P], heapformalfreeze.RegisterRule, nil, heapformalfreeze.BindRule[A], heapformalfreeze.FinalizeRule[A], nil, heapformalfreeze.SealProgramRule, nil))
	// Publication escape is the terminal mounted Placement consumer. Append
	// it so every established rule ordinal remains unchanged.
	add(WireRule(placementpublicationescape.RuleEntry[P, A](), placementpublicationescape.DeclareRule[P], placementpublicationescape.RegisterRule, nil, placementpublicationescape.BindRule[A], placementpublicationescape.FinalizeRule[A], nil, placementpublicationescape.SealProgramRule, nil))
	// Publication FreezeSeal is the terminal mounted Heap consumer. It is
	// appended after publication escape so no established rule ordinal moves.
	add(WireRule(heappublicationfreeze.RuleEntry[P, A](), heappublicationfreeze.DeclareRule[P], heappublicationfreeze.RegisterRule, nil, heappublicationfreeze.BindRule[A], heappublicationfreeze.FinalizeRule[A], nil, heappublicationfreeze.SealProgramRule, nil))

	return admitted, contributors, !rejected && len(admitted) > 0
}
