package composite

import (
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/schema"
	denominatorpublication "github.com/wippyai/go-lua/analysis/schema/denominator/publication"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	callactivation "github.com/wippyai/go-lua/domain/call/activation"
	calldispatchprogram "github.com/wippyai/go-lua/domain/call/dispatch/program"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	callquery "github.com/wippyai/go-lua/domain/call/query"
	"github.com/wippyai/go-lua/domain/constraint"
	callsitebodyprogram "github.com/wippyai/go-lua/domain/effect/callsite/body/program"
	callsiteopaqueprogram "github.com/wippyai/go-lua/domain/effect/callsite/opaque/program"
	callsiteselectedprogram "github.com/wippyai/go-lua/domain/effect/callsite/selected/program"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	effectpublication "github.com/wippyai/go-lua/domain/effect/publication"
	executionowner "github.com/wippyai/go-lua/domain/execution/owner"
	heapclosed "github.com/wippyai/go-lua/domain/heap/allocation/closed"
	heapempty "github.com/wippyai/go-lua/domain/heap/allocation/empty"
	heapingress "github.com/wippyai/go-lua/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/domain/heap/bootstrap"
	contextowner "github.com/wippyai/go-lua/domain/heap/context/owner"
	heapfreezeprogram "github.com/wippyai/go-lua/domain/heap/formalfreeze/program"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	heappublicationfreeze "github.com/wippyai/go-lua/domain/heap/publicationfreeze"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	packsource "github.com/wippyai/go-lua/domain/pack/source"
	placementcapture "github.com/wippyai/go-lua/domain/placement/capture"
	placementcontainment "github.com/wippyai/go-lua/domain/placement/containment"
	placementformal "github.com/wippyai/go-lua/domain/placement/formal"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	placementpublicationescape "github.com/wippyai/go-lua/domain/placement/publicationescape"
	placementquery "github.com/wippyai/go-lua/domain/placement/query"
	placementreturnescapeprogram "github.com/wippyai/go-lua/domain/placement/returnescape/program"
	placementstoreprogram "github.com/wippyai/go-lua/domain/placement/store/program"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
	placementtransfer "github.com/wippyai/go-lua/domain/placement/transfer"
	"github.com/wippyai/go-lua/domain/runtimekind"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
	statictransfer "github.com/wippyai/go-lua/domain/static/transfer"
	typedomain "github.com/wippyai/go-lua/domain/type"
	valueallocation "github.com/wippyai/go-lua/domain/value/allocation"
	valuearithmeticprogram "github.com/wippyai/go-lua/domain/value/arithmetic/program"
	valuebodyresult "github.com/wippyai/go-lua/domain/value/bodyresult"
	valuebootstrap "github.com/wippyai/go-lua/domain/value/bootstrap"
	valueequalityprogram "github.com/wippyai/go-lua/domain/value/equality/program"
	valuefreshresult "github.com/wippyai/go-lua/domain/value/freshresult/program"
	valuemoduleloadprogram "github.com/wippyai/go-lua/domain/value/moduleload/program"
	valueorderprogram "github.com/wippyai/go-lua/domain/value/order/program"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	valuerefinementprogram "github.com/wippyai/go-lua/domain/value/refinement/program"
	valueresultalias "github.com/wippyai/go-lua/domain/value/resultalias"
	valueruntimekind "github.com/wippyai/go-lua/domain/value/runtimekind"
	valuesource "github.com/wippyai/go-lua/domain/value/source"
	valuetransfer "github.com/wippyai/go-lua/domain/value/transfer"
)

// analyzerVocabulary is the analyzer's own contribution to the structural
// vocabulary: the eight structural arms, the three bracket events, and the
// seven body outcomes.
//
// Position in each list is the member's ordinal, numbered from one, so the
// declaration order is the catalog, and the artifact and ingress spellings are
// pinned to these positions by law. Those foreign ordinals are compiled at load
// rather than serialized, so what the pin holds is the agreement between the
// spellings; the ordinals are authored here so a reader sees the position the
// pin names.
//
// Each member declares the name it renders as. A consumer that needs the name
// reads the declaration instead of taking the key apart or keeping a switch of
// its own.
//
// Arms and events are projected whole. Outcomes carry the accepted property
// the body-exit projection reads: Break and Goto conclude a body inside its own
// function, so they contribute no transfer exit.
func analyzerVocabulary() []structure.Spec {
	var specs []structure.Spec
	type member struct {
		key      schema.Key
		spelling string
		accepted bool
	}
	declare := func(category structure.Category, members ...member) {
		for index, declared := range members {
			specs = append(specs, structure.Spec{
				Key:      declared.key,
				Category: category,
				Ordinal:  uint16(index + 1),
				Spelling: declared.spelling,
				Accepted: declared.accepted,
			})
		}
	}
	declare(structure.CategoryArm,
		member{"arm/local", "local", true},
		member{"arm/resume", "resume", true},
		member{"arm/select-true", "select-true", true},
		member{"arm/select-false", "select-false", true},
		member{"arm/tail", "tail", true},
		member{"arm/throw", "throw", true},
		member{"arm/yield", "yield", true},
		member{"arm/cancel", "cancel", true})
	declare(structure.CategoryEvent,
		member{"event/enter", "enter", true},
		member{"event/point", "point", true},
		member{"event/exit", "exit", true})
	declare(structure.CategoryOutcome,
		member{"outcome/normal", "normal", true},
		member{"outcome/return", "return", true},
		member{"outcome/throw", "throw", true},
		member{"outcome/break", "break", false},
		member{"outcome/goto", "goto", false},
		member{"outcome/yield", "yield", true},
		member{"outcome/cancel", "cancel", true})
	return specs
}

// occurrenceVocabulary is the analyzer's contribution of the compiled
// occurrence geometry: the occurrence families a program artifact carries, the
// placement forms a subscription takes, the operand shapes a subscription
// requires those rows to carry, the operand polarities an issued occurrence
// reads, and the execution cuts it is placed at.
//
// A rule declares which occurrence families issue it, in which form, so this is
// the vocabulary those declarations resolve against. Position in each list is
// the member's ordinal, and the artifact's own spellings are pinned to these
// positions by law.
func occurrenceVocabulary() []structure.Spec {
	var specs []structure.Spec
	declare := func(category structure.Category, spellings ...string) {
		for index, spelling := range spellings {
			specs = append(specs, structure.Spec{
				Key:      schema.Key(occurrenceCategoryPrefix(category) + spelling),
				Category: category,
				Ordinal:  uint16(index + 1),
				Spelling: spelling,
				Accepted: true,
			})
		}
	}
	declare(structure.CategoryOccurrenceKind,
		"point-attachment", "values", "values-member", "values-tail", "value-source",
		"storage-read", "storage-bind", "storage-bind-transfer", "storage-assignment", "storage-write",
		"index-read", "index-write", "allocation", "allocation-field", "call",
		"call-activation", "call-boundary", "call-arm", "call-argument", "call-type-argument",
		"unary", "select",
		"value-claim", "binary-arithmetic", "binary-equality", "binary-order",
		"binary-presence-refinement", "return-boundary", "formal-entry", "operation-predicate-refinement",
		"binary-concat")
	return specs
}

// occurrenceCategoryPrefix is the authored key namespace of one occurrence
// vocabulary. A member's key is its category's namespace and its spelling, so a
// declaration reads as the one name it is, and two vocabularies that share a
// spelling still name two members.
func occurrenceCategoryPrefix(category structure.Category) string {
	switch category {
	case structure.CategoryOccurrenceKind:
		return "occurrence/"
	default:
		return ""
	}
}

// semanticRoleVocabulary is the analyzer's semantic role catalog, aggregated
// from the domains that own the roles. Each axis owner contributes the identity
// its coordinate space is bound under and the forms its schema is declared
// with, and each rule owner the two or three forms its rules are identified
// by. An engine-published axis contributes its identity alone: it declares no
// schema, so it declares none of the forms one is declared with.
//
// The order is the declaration order of the axis and rule tables, so a reader
// following one table reads the same sequence in the other. Position carries no
// identity here: a member of this vocabulary is only ever resolved by key, and
// the identity it resolves to is derived from its declared spelling.
func semanticRoleVocabulary() []structure.Spec {
	contributions := [][]structure.Spec{
		runtimekind.BehaviorStructureSpecs(),
		valueowner.StructureSpecs(),
		staticowner.StructureSpecs(),
		packowner.StructureSpecs(),
		heapowner.StructureSpecs(),
		contextowner.StructureSpecs(),
		placementowner.StructureSpecs(),
		placementquery.StructureSpecs(),
		callowner.StructureSpecs(),
		callquery.StructureSpecs(),
		effectowner.StructureSpecs(),
		executionowner.StructureSpecs(),
		valuesource.StructureSpecs(),
		packsource.StructureSpecs(),
		heapingress.StructureSpecs(),
		valueallocation.StructureSpecs(),
		heapempty.StructureSpecs(),
		heapclosed.StructureSpecs(),
		heapindex.StructureSpecs(),
		calldispatchprogram.StructureSpecs(),
		callsitebodyprogram.StructureSpecs(),
		callsiteselectedprogram.StructureSpecs(),
		callsiteopaqueprogram.StructureSpecs(),
		callactivation.StructureSpecs(),
		valuebootstrap.StructureSpecs(),
		heapbootstrap.StructureSpecs(),
		valuetransfer.StructureSpecs(),
		valuearithmeticprogram.StructureSpecs(),
		valueequalityprogram.StructureSpecs(),
		valueorderprogram.StructureSpecs(),
		valuerefinementprogram.StructureSpecs(),
		valueruntimekind.StructureSpecs(),
		denominatorpublication.StructureSpecs(),
		effectpublication.StructureSpecs(),
		typedomain.ChannelSelectStructureSpecs(),
		selectapply.StructureSpecs(),
		programmount.StructureSpecs(),
		modulecomposition.StructureSpecs(),
		placementreturnescapeprogram.StructureSpecs(),
		placementcapture.StructureSpecs(),
		placementformal.StructureSpecs(),
		placementcontainment.StructureSpecs(),
		placementstoreprogram.StructureSpecs(),
		placementsuspension.StructureSpecs(),
		valuemoduleloadprogram.StructureSpecs(),
		heapfreezeprogram.StructureSpecs(),
		placementpublicationescape.StructureSpecs(),
		heappublicationfreeze.StructureSpecs(),
		placementtransfer.StructureSpecs(),
		valueresultalias.StructureSpecs(),
		valuefreshresult.StructureSpecs(),
		valuebodyresult.StructureSpecs(),
		statictransfer.StructureSpecs(),
	}
	var specs []structure.Spec
	for _, contribution := range contributions {
		specs = append(specs, contribution...)
	}
	return specs
}

// structureContributions is the ordered set of contributions the structural
// vocabulary is hosted from. A category is hosted rather than owned: the
// runtime family vocabulary is declared by the domain that owns the families,
// the expression form vocabulary by the domain that owns the grammar, the
// publication vocabularies beside the diagnostic rows that name them, and a
// category a later domain adds rows to arrives as one more contribution here.
// This table states membership and order alone, and the surface numbers the
// aggregate.
func structureContributions() [][]structure.Spec {
	return [][]structure.Spec{
		analyzerVocabulary(),
		occurrenceVocabulary(),
		runtimekind.StructureSpecs(),
		constraint.StructureSpecs(),
		diagnosticVocabulary(),
		structure.NativePublicationSpecs(),
		structure.PublicationPlaneSpecs(),
		structure.PublicationEffectSpecs(),
		structure.ReductionOutcomeSpecs(),
		semanticRoleVocabulary(),
		observationRoleVocabulary(),
		queryRoleVocabulary(),
	}
}

// structureSpecs is the flattened authored inventory, in the order the surface
// numbers it.
func structureSpecs() []structure.Spec {
	var specs []structure.Spec
	for _, contribution := range structureContributions() {
		specs = append(specs, contribution...)
	}
	return specs
}

// StructureVocabulary is the sealed structural vocabulary the composition hands
// to the boundaries that read it. It is the only way a consumer reaches the
// arm, event, and outcome catalogs.
func StructureVocabulary(compilation Compilation) (structure.Table, bool) {
	state := compilation.catalog
	if state == nil {
		return structure.Table{}, false
	}
	return state.structure, state.structureOK
}

// structureEntries admits the contributed inventory. The surface numbers each
// category across the contributions, so a row a contributor authors out of
// position, and a row it authors incompletely, leave the table unavailable
// rather than half declared.
func structureEntries() ([]*structure.Entry, bool) {
	return structure.Collect(structureContributions()...)
}
