// Package familyroster is the composition roster of emitted rule families:
// the one ordered list that says which rule declarations have their execution
// family generated, and which file each one is generated into.
//
// It exists so the family emitter selects its targets from a registry rather
// than from a switch it holds itself, exactly as memberroster does for the
// axis member generator. A rule whose execution family is emitted appears here
// as one line; nothing registers itself, and a rule that is absent is a rule
// whose family is still authored.
package familyroster

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule/emit"
	"github.com/wippyai/go-lua/analysis/schema/rule/emitlaw"
	calldispatchprogram "github.com/wippyai/go-lua/domain/call/dispatch/program"
	callsitebodyprogram "github.com/wippyai/go-lua/domain/effect/callsite/body/program"
	callsiteopaqueprogram "github.com/wippyai/go-lua/domain/effect/callsite/opaque/program"
	callsiteselectedprogram "github.com/wippyai/go-lua/domain/effect/callsite/selected/program"
	heapempty "github.com/wippyai/go-lua/domain/heap/allocation/empty"
	freezeprogram "github.com/wippyai/go-lua/domain/heap/formalfreeze/program"
	returnprogram "github.com/wippyai/go-lua/domain/placement/returnescape/program"
	storeprogram "github.com/wippyai/go-lua/domain/placement/store/program"
	valueallocationprogram "github.com/wippyai/go-lua/domain/value/allocation/program"
	freshresultprogram "github.com/wippyai/go-lua/domain/value/freshresult/program"
	valuemoduleloadprogram "github.com/wippyai/go-lua/domain/value/moduleload/program"
	refinementprogram "github.com/wippyai/go-lua/domain/value/refinement/program"
	valueresultaliasprogram "github.com/wippyai/go-lua/domain/value/resultalias/program"
	valueruntimekindprogram "github.com/wippyai/go-lua/domain/value/runtimekind/program"
)

// GeneratedFileName is the one name an emitted family is written under. It is
// fixed rather than authored per rule so a package's generated half is
// identifiable without consulting this roster.
const GeneratedFileName = "generated_family.go"

// GeneratedConstructionLawFileName is the one name the laws of an emitted
// construction are written under. They sit beside the family rather than
// beside the declaration because what they hold - the order a derived member
// set answers in, and that it costs no allocation to answer - is a property of
// the generated code itself and is unreadable from the declaration package.
const GeneratedConstructionLawFileName = "generated_construction_law_test.go"

// Family is one emitted rule family: the declaration it is derived from and
// the repository-relative directory it is generated into.
type Family struct {
	Target    emit.Target
	Directory string
}

// Key is the rule key this family is emitted from.
func (family Family) Key() schema.Key { return family.Target.Spec.Key }

// Families returns the roster. Returning a fresh slice keeps the registry
// independent of callers that inspect or reorder it.
func Families() []Family {
	return []Family{
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/heap/allocation/empty",
				PackageName: "empty",
				Spec:        heapempty.RuleEntry(),
			},
			Directory: "domain/heap/allocation/empty",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/placement/store",
				PackageName: "store",
				Spec:        storeprogram.RuleEntry(),
			},
			Directory: "domain/placement/store",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/placement/returnescape",
				PackageName: "returnescape",
				Spec:        returnprogram.RuleEntry(),
			},
			Directory: "domain/placement/returnescape",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/call/dispatch",
				PackageName: "dispatch",
				Spec:        calldispatchprogram.RuleEntry(),
			},
			Directory: "domain/call/dispatch",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/effect/callsite/selected",
				PackageName: "selected",
				Spec:        callsiteselectedprogram.RuleEntry(),
			},
			Directory: "domain/effect/callsite/selected",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/effect/callsite/opaque",
				PackageName: "opaque",
				Spec:        callsiteopaqueprogram.RuleEntry(),
			},
			Directory: "domain/effect/callsite/opaque",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/effect/callsite/body",
				PackageName: "body",
				Spec:        callsitebodyprogram.RuleEntry(),
			},
			Directory: "domain/effect/callsite/body",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/moduleload",
				PackageName: "moduleload",
				Spec:        valuemoduleloadprogram.RuleEntry(),
			},
			Directory: "domain/value/moduleload",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/runtimekind",
				PackageName: "runtimekind",
				Spec:        valueruntimekindprogram.RuleEntry(),
			},
			Directory: "domain/value/runtimekind",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/resultalias",
				PackageName: "resultalias",
				Spec:        valueresultaliasprogram.RuleEntry(),
			},
			Directory: "domain/value/resultalias",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/freshresult",
				PackageName: "freshresult",
				Spec:        freshresultprogram.RuleEntry(),
			},
			Directory: "domain/value/freshresult",
		},
		{
			Target: emit.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/allocation",
				PackageName: "allocation",
				Spec:        valueallocationprogram.RuleEntry(),
			},
			Directory: "domain/value/allocation",
		},
	}
}

// GeneratedLawFileName is the one name an emitted structural law suite is
// written under, for the same reason GeneratedFileName is fixed: a package's
// generated laws are identifiable without consulting this roster.
const GeneratedLawFileName = "generated_law_test.go"

// Declaration is one rule whose structural law suite is emitted: the
// declaration package it is emitted into, and the accessors that package
// publishes.
//
// It is a separate roster row from Family because the two cutovers are
// separate. A rule can have its execution family emitted while its structural
// laws are still authored, and a rule whose family is authored can still owe
// the same structural laws - what a declaration is obliged to prove about
// itself does not depend on who wrote its executor.
type Declaration struct {
	Target    emitlaw.Target
	Directory string
}

// Key is the rule key this law suite is emitted from.
func (declaration Declaration) Key() schema.Key { return declaration.Target.Spec.Key }

// Declarations returns the roster of rules whose structural law suite is
// emitted. Returning a fresh slice keeps the registry independent of callers
// that inspect or reorder it.
func Declarations() []Declaration {
	return []Declaration{
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/placement/store/program",
				PackageName: "program",
				Declaration: "Storage",
				Entry:       "RuleEntry",
				Spec:        storeprogram.RuleEntry(),
			},
			Directory: "domain/placement/store/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/heap/formalfreeze/program",
				PackageName: "program",
				Declaration: "FormalFreeze",
				Entry:       "RuleEntry",
				Spec:        freezeprogram.RuleEntry(),
			},
			Directory: "domain/heap/formalfreeze/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/placement/returnescape/program",
				PackageName: "program",
				Declaration: "ReturnEscape",
				Entry:       "RuleEntry",
				Spec:        returnprogram.RuleEntry(),
			},
			Directory: "domain/placement/returnescape/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/refinement/program",
				PackageName: "program",
				Declaration: "PresenceRefinement",
				Entry:       "RuleEntry",
				Spec:        refinementprogram.RuleEntry(),
			},
			Directory: "domain/value/refinement/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/call/dispatch/program",
				PackageName: "program",
				Declaration: "Dispatch",
				Entry:       "RuleEntry",
				Spec:        calldispatchprogram.RuleEntry(),
			},
			Directory: "domain/call/dispatch/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/effect/callsite/selected/program",
				PackageName: "program",
				Declaration: "SelectedCallEffect",
				Entry:       "RuleEntry",
				Spec:        callsiteselectedprogram.RuleEntry(),
			},
			Directory: "domain/effect/callsite/selected/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/effect/callsite/opaque/program",
				PackageName: "program",
				Declaration: "OpaqueCallEffect",
				Entry:       "RuleEntry",
				Spec:        callsiteopaqueprogram.RuleEntry(),
			},
			Directory: "domain/effect/callsite/opaque/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/effect/callsite/body/program",
				PackageName: "program",
				Declaration: "BodyCallEffect",
				Entry:       "RuleEntry",
				Spec:        callsitebodyprogram.RuleEntry(),
			},
			Directory: "domain/effect/callsite/body/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/moduleload/program",
				PackageName: "program",
				Declaration: "ModuleLoadCallResult",
				Entry:       "RuleEntry",
				Spec:        valuemoduleloadprogram.RuleEntry(),
			},
			Directory: "domain/value/moduleload/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/runtimekind/program",
				PackageName: "program",
				Declaration: "RuntimeKindCallResult",
				Entry:       "RuleEntry",
				Spec:        valueruntimekindprogram.RuleEntry(),
			},
			Directory: "domain/value/runtimekind/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/freshresult/program",
				PackageName: "program",
				Declaration: "FreshResult",
				Entry:       "RuleEntry",
				Spec:        freshresultprogram.RuleEntry(),
			},
			Directory: "domain/value/freshresult/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/allocation/program",
				PackageName: "program",
				Declaration: "AllocationResult",
				Entry:       "RuleEntry",
				Spec:        valueallocationprogram.RuleEntry(),
			},
			Directory: "domain/value/allocation/program",
		},
		{
			Target: emitlaw.Target{
				PackagePath: "github.com/wippyai/go-lua/domain/value/resultalias/program",
				PackageName: "program",
				Declaration: "ResultAliasCallResult",
				Entry:       "RuleEntry",
				Spec:        valueresultaliasprogram.RuleEntry(),
			},
			Directory: "domain/value/resultalias/program",
		},
	}
}
