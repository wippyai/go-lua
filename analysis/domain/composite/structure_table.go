package composite

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/constraint"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// structureSpecs is the authored analyzer structural vocabulary: the eight
// structural arms, the three bracket events, the seven body outcomes, the eight
// Lua runtime families, and the ten symbolic expression forms.
//
// Declaring them here makes this table the one place a member is added, and
// the surface's density law is what lets a consumer's projection switch on the
// declared ordinals exhaustively. Position in each list is the member's
// ordinal, numbered from one, so the declaration order is the catalog, and the
// ordinals are the artifact's serialized ABI ordinals, which this declaration
// adopts.
//
// Arms and events are projected whole. Outcomes carry the accepted property
// the body-exit projection reads: Break and Goto conclude a body inside its own
// function, so they contribute no transfer exit.
func structureSpecs() []structure.Spec {
	var specs []structure.Spec
	declare := func(category structure.Category, members ...schema.Key) {
		for index, member := range members {
			specs = append(specs, structure.Spec{Key: member, Category: category, Ordinal: uint16(index + 1), Accepted: true})
		}
	}
	declare(structure.CategoryArm,
		"arm/local", "arm/resume", "arm/select-true", "arm/select-false",
		"arm/tail", "arm/throw", "arm/yield", "arm/cancel")
	declare(structure.CategoryEvent,
		"event/enter", "event/point", "event/exit")
	outcomes := []struct {
		key      schema.Key
		accepted bool
	}{
		{"outcome/normal", true},
		{"outcome/return", true},
		{"outcome/throw", true},
		{"outcome/break", false},
		{"outcome/goto", false},
		{"outcome/yield", true},
		{"outcome/cancel", true},
	}
	for index, outcome := range outcomes {
		specs = append(specs, structure.Spec{Key: outcome.key, Category: structure.CategoryOutcome, Ordinal: uint16(index + 1), Accepted: outcome.accepted})
	}
	// The runtime family vocabulary is declared by the domain that owns the
	// families, because its ordinals are that domain's own Kind constants. This
	// table states membership and order alone.
	specs = append(specs, runtimekind.StructureSpecs()...)
	// The expression form vocabulary is declared by the domain that owns the
	// grammar, because its ordinals are that grammar's own closed enumeration.
	// This table states membership and order alone.
	specs = append(specs, constraint.StructureSpecs()...)
	return specs
}

// structuralVocabulary holds the one projection of the sealed structural table. The
// declaration is sealed once, so its projection is built once and handed out
// by value; a consumer receives the catalog rather than restating it.
var structuralVocabulary struct {
	once  sync.Once
	table structure.Table
	ok    bool
}

// StructureVocabulary is the sealed structural vocabulary the composition hands
// to the boundaries that read it. It is the only way a consumer reaches the
// arm, event, and outcome catalogs.
func StructureVocabulary() (structure.Table, bool) {
	structuralVocabulary.once.Do(func() {
		sealed, failure := Table()
		if failure.Available() || sealed == nil {
			return
		}
		view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
		if !viewOK {
			return
		}
		structuralVocabulary.table, structuralVocabulary.ok = structure.NewTable(view)
	})
	return structuralVocabulary.table, structuralVocabulary.ok
}

// structureEntries admits the authored inventory. A rejected row leaves the
// table unavailable rather than half declared.
func structureEntries() ([]*structure.Entry, bool) {
	specs := structureSpecs()
	entries := make([]*structure.Entry, 0, len(specs))
	for _, spec := range specs {
		entry, ok := structure.New(spec)
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	return entries, true
}
