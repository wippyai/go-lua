package testfixture

import (
	"context"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

// ScopedRequireOperation is the builtin module-loader declaration every Link
// the analyzer mounts carries. Call seals one scoped loader seed per mount and
// Value resolves the require result slot while sealing, so a hand-authored
// fixture target declares this operation rather than a Link no composition can
// mount.
func ScopedRequireOperation() (vocabulary.OperationSpec, error) {
	path, pathErr := domaincontract.Encode(context.Background(), typ.String, nil)
	if pathErr != nil {
		return vocabulary.OperationSpec{}, pathErr
	}
	result, resultErr := domaincontract.Encode(context.Background(), typ.Any, nil)
	if resultErr != nil {
		return vocabulary.OperationSpec{}, resultErr
	}
	return vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{path}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{result}, Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}, nil
}
