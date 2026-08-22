// Package construction owns the schema-declared refusal value for Program
// construction. It is deliberately separate from the compiler package: a
// compiler sequences a fault but does not define its vocabulary.
package construction

import (
	"github.com/wippyai/go-lua/analysis/schema"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
)

// Fault is one construction refusal identified by its generated semantic row
// family, declared issue key, and stable coordinates within that family. A
// zero value means that no refusal was issued.
type Fault struct {
	family programcatalog.Definition
	issue  schema.Key
	row    int
	subrow int
}

// New issues one refusal against a generated schema row family. A negative
// coordinate denotes the whole family rather than an invalid position.
func New(family programcatalog.Definition, issue schema.Key, row, subrow int) Fault {
	if !family.Valid() || !issue.Available() || row < -1 || subrow < -1 {
		return Fault{}
	}
	return Fault{family: family, issue: issue, row: row, subrow: subrow}
}

// Available reports whether this value is an issued construction refusal.
func (fault Fault) Available() bool {
	return fault.family.Valid() && fault.issue.Available() && fault.row >= -1 && fault.subrow >= -1
}

func (fault Fault) Family() programcatalog.Definition { return fault.family }
func (fault Fault) Issue() schema.Key                 { return fault.issue }
func (fault Fault) Row() (int, bool) {
	return fault.row, fault.Available() && fault.row >= 0
}
func (fault Fault) Subrow() (int, bool) {
	return fault.subrow, fault.Available() && fault.subrow >= 0
}

// Construction issue keys are schema vocabulary, not compiler-local reasons.
const (
	IssueModuleUnavailable             schema.Key = "program.module.unavailable"
	IssueModuleImport                  schema.Key = "program.module.import"
	IssueModuleRequest                 schema.Key = "program.module.request"
	IssueModuleEntry                   schema.Key = "program.module.entry"
	IssueModuleRootCell                schema.Key = "program.module.root-cell"
	IssueModuleRootFunction            schema.Key = "program.module.root-function"
	IssueModuleMember                  schema.Key = "program.module.member"
	IssueDiagnosticInvalidInput        schema.Key = "program.diagnostic.invalid-input"
	IssueDiagnosticUnavailable         schema.Key = "program.diagnostic.unavailable"
	IssueDiagnosticRouteUnavailable    schema.Key = "program.diagnostic.route-unavailable"
	IssueDiagnosticRouteGuard          schema.Key = "program.diagnostic.route-guard"
	IssueDiagnosticStorageRead         schema.Key = "program.diagnostic.storage-read"
	IssueDiagnosticCall                schema.Key = "program.diagnostic.call"
	IssueDiagnosticDuplicate           schema.Key = "program.diagnostic.duplicate"
	IssueExactScalarUnavailable        schema.Key = "program.exact-scalar.unavailable"
	IssueExactScalarValueSourceAppend  schema.Key = "program.exact-scalar.value-source-append"
	IssueExactScalarValues             schema.Key = "program.exact-scalar.values"
	IssueExactScalarStorageRead        schema.Key = "program.exact-scalar.storage-read"
	IssueExactScalarStorageBind        schema.Key = "program.exact-scalar.storage-bind"
	IssueBodyUnavailable               schema.Key = "program.body.unavailable"
	IssueBodyForeign                   schema.Key = "program.body.foreign"
	IssueBodyIdentity                  schema.Key = "program.body.identity"
	IssueBodyDuplicate                 schema.Key = "program.body.duplicate"
	IssueBodyRange                     schema.Key = "program.body.range"
	IssueOutcomeUnavailable            schema.Key = "program.outcome.unavailable"
	IssueOutcomeAttachment             schema.Key = "program.outcome.attachment"
	IssueOutcomeShape                  schema.Key = "program.outcome.shape"
	IssueOutcomeForeign                schema.Key = "program.outcome.foreign"
	IssueOutcomeIdentity               schema.Key = "program.outcome.identity"
	IssueOutcomeDuplicate              schema.Key = "program.outcome.duplicate"
	IssueOutcomeKind                   schema.Key = "program.outcome.kind"
	IssueOutcomeTarget                 schema.Key = "program.outcome.target"
	IssueOutcomePropagation            schema.Key = "program.outcome.propagation"
	IssueOutcomeReference              schema.Key = "program.outcome.reference"
	IssueOutcomeRange                  schema.Key = "program.outcome.range"
	IssueOutcomeReturn                 schema.Key = "program.outcome.return"
	IssueReturnValueUnavailable        schema.Key = "program.return-value.unavailable"
	IssueReturnValueReference          schema.Key = "program.return-value.reference"
	IssueCallTargetUnavailable         schema.Key = "program.call-target.unavailable"
	IssueCallTargetDuplicate           schema.Key = "program.call-target.duplicate"
	IssueHeapAllocationUnavailable     schema.Key = "program.heap-allocation.unavailable"
	IssueHeapAllocationDuplicate       schema.Key = "program.heap-allocation.duplicate"
	IssueHeapFieldUnavailable          schema.Key = "program.heap-field.unavailable"
	IssueHeapFieldDuplicate            schema.Key = "program.heap-field.duplicate"
	IssueLocalTransferUnavailable      schema.Key = "program.local-transfer.unavailable"
	IssueLocalTransferDuplicate        schema.Key = "program.local-transfer.duplicate"
	IssueLocalTransferWriteUnavailable schema.Key = "program.local-transfer-write.unavailable"
)
