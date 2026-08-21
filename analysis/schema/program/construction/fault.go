// Package construction owns the schema-declared refusal value for Program
// construction. It is deliberately separate from the compiler package: a
// compiler sequences a fault but does not define its vocabulary.
package construction

import "github.com/wippyai/go-lua/analysis/schema"

// Fault is one construction refusal identified by its generated semantic row
// family, declared issue key, and stable coordinates within that family. A
// zero value means that no refusal was issued.
type Fault struct {
	family schema.EntryID
	issue  schema.Key
	row    int
	subrow int
}

// New issues one refusal against a generated schema row family. A negative
// coordinate denotes the whole family rather than an invalid position.
func New(family schema.EntryID, issue schema.Key, row, subrow int) Fault {
	if !family.Available() || !issue.Available() || row < -1 || subrow < -1 {
		return Fault{}
	}
	return Fault{family: family, issue: issue, row: row, subrow: subrow}
}

// Available reports whether this value is an issued construction refusal.
func (fault Fault) Available() bool {
	return fault.family.Available() && fault.issue.Available() && fault.row >= -1 && fault.subrow >= -1
}

func (fault Fault) Family() schema.EntryID { return fault.family }
func (fault Fault) Issue() schema.Key      { return fault.issue }
func (fault Fault) Row() (int, bool) {
	return fault.row, fault.Available() && fault.row >= 0
}
func (fault Fault) Subrow() (int, bool) {
	return fault.subrow, fault.Available() && fault.subrow >= 0
}

// Module issue keys are schema vocabulary, not compiler-local reasons.
const (
	IssueModuleUnavailable  schema.Key = "program.module.unavailable"
	IssueModuleImport       schema.Key = "program.module.import"
	IssueModuleRequest      schema.Key = "program.module.request"
	IssueModuleEntry        schema.Key = "program.module.entry"
	IssueModuleRootCell     schema.Key = "program.module.root-cell"
	IssueModuleRootFunction schema.Key = "program.module.root-function"
	IssueModuleMember       schema.Key = "program.module.member"
)
