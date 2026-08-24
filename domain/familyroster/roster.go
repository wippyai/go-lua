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
	heapempty "github.com/wippyai/go-lua/domain/heap/allocation/empty"
	storeprogram "github.com/wippyai/go-lua/domain/placement/store/program"
)

// GeneratedFileName is the one name an emitted family is written under. It is
// fixed rather than authored per rule so a package's generated half is
// identifiable without consulting this roster.
const GeneratedFileName = "generated_family.go"

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
	}
}
