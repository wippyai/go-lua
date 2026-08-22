package typecontract

import (
	"context"

	targetcontract "github.com/wippyai/go-lua/analysis/program/target/contract"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/targetfamily"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// SealColumn seals the class vocabulary of one Target as its semantic column.
//
// Every sealed target already names this adapter as the authority that reads
// its declarations, so it is also the authority that can read all of them
// once, at seal, instead of once per Link. The order and the identity of the
// vocabulary belong to targetfamily; only the reading belongs here.
func (Semantics) SealColumn(operations operationvalue.Core) (targetcontract.SealedColumn, error) {
	return targetfamily.SealColumn(operations, func(declaration schematype.Type) (typ.Type, bool) {
		decoded, err := Decode(context.Background(), declaration, nil)
		return decoded, err == nil && decoded != nil
	})
}
