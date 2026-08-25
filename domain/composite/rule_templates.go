package composite

import (
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	callactivation "github.com/wippyai/go-lua/domain/call/activation"
	callactivationprogram "github.com/wippyai/go-lua/domain/call/activation/program"
	calldispatch "github.com/wippyai/go-lua/domain/call/dispatch"
	calldispatchprogram "github.com/wippyai/go-lua/domain/call/dispatch/program"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	callsitebody "github.com/wippyai/go-lua/domain/effect/callsite/body"
	callsitebodyprogram "github.com/wippyai/go-lua/domain/effect/callsite/body/program"
	callsiteopaque "github.com/wippyai/go-lua/domain/effect/callsite/opaque"
	callsiteopaqueprogram "github.com/wippyai/go-lua/domain/effect/callsite/opaque/program"
	callsiteselected "github.com/wippyai/go-lua/domain/effect/callsite/selected"
	callsiteselectedprogram "github.com/wippyai/go-lua/domain/effect/callsite/selected/program"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	heapclosed "github.com/wippyai/go-lua/domain/heap/allocation/closed"
	heapclosedprogram "github.com/wippyai/go-lua/domain/heap/allocation/closed/program"
	heapempty "github.com/wippyai/go-lua/domain/heap/allocation/empty"
	heapingress "github.com/wippyai/go-lua/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/domain/heap/bootstrap"
	contextowner "github.com/wippyai/go-lua/domain/heap/context/owner"
	heapformalfreeze "github.com/wippyai/go-lua/domain/heap/formalfreeze"
	heapfreezeprogram "github.com/wippyai/go-lua/domain/heap/formalfreeze/program"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	heappublicationfreeze "github.com/wippyai/go-lua/domain/heap/publicationfreeze"
	heappublicationfreezeprogram "github.com/wippyai/go-lua/domain/heap/publicationfreeze/program"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	packsource "github.com/wippyai/go-lua/domain/pack/source"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementcapture "github.com/wippyai/go-lua/domain/placement/capture"
	placementcontainment "github.com/wippyai/go-lua/domain/placement/containment"
	placementformal "github.com/wippyai/go-lua/domain/placement/formal"
	placementformalprogram "github.com/wippyai/go-lua/domain/placement/formal/program"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	placementpublicationescape "github.com/wippyai/go-lua/domain/placement/publicationescape"
	placementreturnescape "github.com/wippyai/go-lua/domain/placement/returnescape"
	placementreturnescapeprogram "github.com/wippyai/go-lua/domain/placement/returnescape/program"
	placementstore "github.com/wippyai/go-lua/domain/placement/store"
	placementstoreprogram "github.com/wippyai/go-lua/domain/placement/store/program"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
	placementtransfer "github.com/wippyai/go-lua/domain/placement/transfer"
	placementtransferprogram "github.com/wippyai/go-lua/domain/placement/transfer/program"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
	statictransfer "github.com/wippyai/go-lua/domain/static/transfer"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueallocation "github.com/wippyai/go-lua/domain/value/allocation"
	valueallocationprogram "github.com/wippyai/go-lua/domain/value/allocation/program"
	valuearithmeticprogram "github.com/wippyai/go-lua/domain/value/arithmetic/program"
	valuebodyresult "github.com/wippyai/go-lua/domain/value/bodyresult"
	valuebodyresultprogram "github.com/wippyai/go-lua/domain/value/bodyresult/program"
	valuebootstrap "github.com/wippyai/go-lua/domain/value/bootstrap"
	valueequalityprogram "github.com/wippyai/go-lua/domain/value/equality/program"
	valueexactfold "github.com/wippyai/go-lua/domain/value/execution/exactfold"
	valuefreshresult "github.com/wippyai/go-lua/domain/value/freshresult"
	valuefreshresultprogram "github.com/wippyai/go-lua/domain/value/freshresult/program"
	valuemoduleload "github.com/wippyai/go-lua/domain/value/moduleload"
	valuemoduleloadprogram "github.com/wippyai/go-lua/domain/value/moduleload/program"
	valueorderprogram "github.com/wippyai/go-lua/domain/value/order/program"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	valuerefinementprogram "github.com/wippyai/go-lua/domain/value/refinement/program"
	valueresultalias "github.com/wippyai/go-lua/domain/value/resultalias"
	valueresultaliasprogram "github.com/wippyai/go-lua/domain/value/resultalias/program"
	valueruntimekind "github.com/wippyai/go-lua/domain/value/runtimekind"
	valueruntimekindprogram "github.com/wippyai/go-lua/domain/value/runtimekind/program"
	valuesource "github.com/wippyai/go-lua/domain/value/source"
	valuetransfer "github.com/wippyai/go-lua/domain/value/transfer"
)

type Principals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	StaticTypePrincipal() *staticowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
	HeapPrincipal() *heapowner.SchemaFragment
	ContextPrincipal() *contextowner.SchemaFragment
	PlacementPrincipal() *placementowner.SchemaFragment
	EvidencePrincipal() *placementsuspension.EvidenceFactorFragment
	PackPrincipal() *packowner.SchemaFragment
	EffectPrincipal() *effectowner.SchemaFragment
}

type Authorities interface {
	ValueAuthority() *valueowner.HotOwner
	StaticTypeAuthority() *staticowner.HotOwner
	CallAuthority() *callowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	ContextAuthority() *contextowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	EvidenceAuthority() *placementsuspension.EvidenceOwner
	PackAuthority() *packowner.HotOwner
	EffectAuthority() *effectowner.HotOwner
	ValueSchema() *valuedomain.Schema
	HeapSchema() heapdomain.Schema
	PlacementSchema() placementdomain.Schema
	PackSchema() *packdomain.Schema
	Topology() *heapindex.Topology
	KeySelection() *keymatch.SelectorProjection
	Allocations() *allocationcatalog.Catalog
	TargetContract() *contract.Contract
}

// RuleTemplates is the single schema composition registration for executable
// rules. It returns data-only catalog entries and the typed compose passes that
// bind each entry exactly once to its domain implementation.
func RuleTemplates[P Principals, A Authorities]() ([]*rule.Template, []RuleContributor[P, A], bool) {
	admitted, contributors, _, ok := ruleTemplateWiring[P, A]()
	return admitted, contributors, ok
}

// RuleTemplateRefusal is the named first refusal of the rule catalog wiring.
// It is what a reader needs when the catalog does not admit: which rule, and
// which half of the row disagreed with the other.
func RuleTemplateRefusal[P Principals, A Authorities]() RuleWiringFailure {
	_, _, failure, _ := ruleTemplateWiring[P, A]()
	return failure
}

func ruleTemplateWiring[P Principals, A Authorities]() ([]*rule.Template, []RuleContributor[P, A], RuleWiringFailure, bool) {
	var admitted []*rule.Template
	var contributors []RuleContributor[P, A]
	failure := RuleWiringFailure{}
	add := func(entry *rule.Template, contributor RuleContributor[P, A], ok bool) {
		refusal := contributor.complete(entry)
		if !ok && refusal == WiringAdmitted {
			refusal = WiringTemplateAbsent
		}
		if refusal != WiringAdmitted {
			if !failure.Available() {
				failure = RuleWiringFailure{Refusal: refusal}
				if entry != nil {
					failure.Rule = entry.Key()
				}
			}
			return
		}
		admitted = append(admitted, entry)
		contributors = append(contributors, contributor)
	}

	add(WireGeneratedRule[P, A](valuesource.RuleEntry()))
	add(WireGeneratedRule[P, A](packsource.RuleEntry()))
	add(WireGeneratedRule[P, A](heapingress.RuleEntry()))
	// The constructor result is a Program whose fold reads nothing and whose
	// publication is a transformed carry, so it installs the family the engine
	// has no generic builder for.
	add(WireGeneratedRuleWithFamily[P, A](valueallocationprogram.RuleEntry(), valueallocation.InstallFamily[A]))
	// The empty constructor is a Program whose fold is a transformed carry, so
	// it installs the family the engine has no generic builder for.
	add(WireGeneratedRuleWithFamily[P, A](heapempty.RuleEntry(), heapempty.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](heapclosedprogram.RuleEntry(), heapclosed.InstallFamily[A]))
	add(WireRule(heapindex.RawGetEntry[P, A](), heapindex.DeclareRawGet[P], heapindex.RegisterRawGet, nil, heapindex.BindRawGet[A], nil, nil, nil))
	add(WireRule(heapindex.RawSetEntry[P, A](), heapindex.DeclareRawSet[P], heapindex.RegisterRawSet, nil, heapindex.BindRawSet[A], nil, nil, nil))
	add(WireGeneratedRuleWithFamily[P, A](calldispatchprogram.RuleEntry(), calldispatch.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](callsiteselectedprogram.RuleEntry(), callsiteselected.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](callsiteopaqueprogram.RuleEntry(), callsiteopaque.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](callsitebodyprogram.RuleEntry(), callsitebody.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](callactivationprogram.RuleEntry(), callactivation.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](valueruntimekindprogram.RuleEntry(), valueruntimekind.InstallFamily[A]))
	add(WireGeneratedRule[P, A](valuebootstrap.RuleEntry()))
	add(WireGeneratedRule[P, A](heapbootstrap.RuleEntry()))
	add(WireGeneratedRule[P, A](valuetransfer.RuleEntry()))
	add(WireGeneratedRuleWithFamily[P, A](valuearithmeticprogram.RuleEntry(), valueexactfold.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](valueequalityprogram.RuleEntry(), valueexactfold.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](valueorderprogram.RuleEntry(), valueexactfold.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](valuerefinementprogram.RuleEntry(), valueexactfold.InstallFamily[A]))
	// Placement displacement consumers follow the value/call/heap producers
	// whose sealed facts they read. ReturnEscape is the generated dependent
	// family: its authored route relation is installed once through the
	// rule's own family claimant, exactly as Store's.
	add(WireGeneratedRuleWithFamily[P, A](placementreturnescapeprogram.RuleEntry(), placementreturnescape.InstallFamily[A]))
	add(WireRule(placementcapture.RuleEntry[P, A](), placementcapture.DeclareRule[P], placementcapture.RegisterRule, nil, placementcapture.BindRule[A], placementcapture.FinalizeRule[A], nil, nil))
	// Formal ownership is the generated dependent Formal family. Its authored
	// route relation is installed once through the rule's own family claimant,
	// exactly as Transfer's.
	add(WireGeneratedRuleWithFamily[P, A](placementformalprogram.RuleEntry(), placementformal.InstallFamily[A]))
	// Containment is the singleton declarative rule expanded at every mounted point.
	add(WireRule(placementcontainment.RuleEntry[P, A](), placementcontainment.DeclareRule[P], placementcontainment.RegisterRule, nil, placementcontainment.BindRule[A], placementcontainment.FinalizeRule[A], placementcontainment.OccurrenceCatalog, nil))
	// Storage lifetime is the generated dependent Store family. Its authored
	// route relation is installed once through the rule's own family claimant.
	add(WireGeneratedRuleWithFamily[P, A](placementstoreprogram.RuleEntry(), placementstore.InstallFamily[A]))
	// Suspension consumes neutral Program liveness rows through the
	// owner-fenced Value/Placement bridge.
	add(WireRule(placementsuspension.RuleEntry[P, A](), placementsuspension.DeclareRule[P], placementsuspension.RegisterRule, nil, placementsuspension.BindRule[A], placementsuspension.FinalizeRule[A], placementsuspension.OccurrenceCatalog, nil))
	// Suspension evidence is a separate Link producer with its own typed
	// Heap-aligned Factor receipt. Keeping it adjacent to the class producer
	// makes the ordered projection's source explicit.
	add(WireRule(placementsuspension.EvidenceRuleEntry[P, A](), placementsuspension.DeclareEvidenceRule[P], placementsuspension.RegisterEvidenceRule, nil, placementsuspension.BindEvidenceRule[A], placementsuspension.FinalizeEvidenceRule[A], placementsuspension.EvidenceOccurrenceCatalog, nil))
	// Module-load result projection has exact Call/Value reads and writes an
	// existing mounted Program CallResultValue coordinate.
	add(WireGeneratedRuleWithFamily[P, A](valuemoduleloadprogram.RuleEntry(), valuemoduleload.InstallFamily[A]))
	// Formal freeze is a terminal mounted Heap transition over the existing
	// call-effect cut. Its authored route relation is installed once through
	// the rule's own family claimant.
	add(WireGeneratedRuleWithFamily[P, A](heapfreezeprogram.RuleEntry(), heapformalfreeze.InstallFamily[A]))
	// Publication escape is the terminal mounted Placement consumer.
	add(WireRule(placementpublicationescape.RuleEntry[P, A](), placementpublicationescape.DeclareRule[P], placementpublicationescape.RegisterRule, nil, placementpublicationescape.BindRule[A], nil, nil, nil))
	// Publication FreezeSeal is the terminal mounted Heap consumer and follows
	// the Placement publication demand it witnesses.
	add(WireGeneratedRuleWithFamily[P, A](heappublicationfreezeprogram.RuleEntry(), heappublicationfreeze.InstallFamily[A]))
	// Target-transfer is appended to preserve established rule ordinals. It is
	// a mounted invocation consumer: Call and Pack actuals are read, Target is
	// joined through Link's exact Contract, and only Placement is written. Its
	// authored route relation is installed once through the rule's own family
	// claimant, exactly as ReturnEscape's.
	add(WireGeneratedRuleWithFamily[P, A](placementtransferprogram.RuleEntry(), placementtransfer.InstallFamily[A]))
	// The two Target result consumers are appended to preserve established
	// rule ordinals. ResultAlias is a mounted consumer of the selected Call
	// and the mounted actual it aliases; fresh-result is the Link producer
	// that hands a mounted call result the Heap fresh root Value it allocates,
	// enumerated from Value's own admitted fresh-result directory.
	add(WireGeneratedRuleWithFamily[P, A](valueresultaliasprogram.RuleEntry(), valueresultalias.InstallFamily[A]))
	add(WireGeneratedRuleWithFamily[P, A](valuefreshresultprogram.RuleEntry(), valuefreshresult.InstallFamily[A]))
	// Body-result is the Value-owned executable-body counterpart to the two
	// Target result consumers above. It consumes selected Call body targets and
	// Value's sealed ReturnBoundary relation; no caller reconstructs Program
	// return geometry.
	add(WireGeneratedRuleWithFamily[P, A](valuebodyresultprogram.RuleEntry(), valuebodyresult.InstallFamily[A]))
	// Static typed-fact transfer is the identity copy of TypeFact along
	// Value's sealed StorageTransfer. It is appended so established rule
	// ordinals stay fixed. The composition derives its generated slot from
	// the one declaration-owned plan catalog.
	add(WireGeneratedRule[P, A](statictransfer.RuleEntry()))

	return admitted, contributors, failure, !failure.Available() && len(admitted) > 0
}
