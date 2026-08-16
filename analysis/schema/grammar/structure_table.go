package grammar

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// structureSpecs is the authored analyzer structural vocabulary: the eight
// structural arms, the three bracket events, and the seven body outcomes.
//
// These are the three catalogs the analyzer today spells once per consumer.
// Declaring them here makes this table the one place a member is added, and
// the surface's density law is what lets a consumer's projection switch on the
// declared ordinals exhaustively. Position in each list is the member's
// ordinal, numbered from one, so the declaration order is the catalog.
func structureSpecs() []structure.Spec {
	var specs []structure.Spec
	declare := func(category structure.Category, members ...schema.Key) {
		for index, member := range members {
			specs = append(specs, structure.Spec{Key: member, Category: category, Ordinal: uint16(index + 1)})
		}
	}
	declare(structure.CategoryArm,
		"arm/local", "arm/resume", "arm/select-true", "arm/select-false",
		"arm/tail", "arm/throw", "arm/yield", "arm/cancel")
	declare(structure.CategoryEvent,
		"event/enter", "event/point", "event/exit")
	declare(structure.CategoryOutcome,
		"outcome/normal", "outcome/return", "outcome/throw", "outcome/break",
		"outcome/goto", "outcome/yield", "outcome/cancel")
	return specs
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
