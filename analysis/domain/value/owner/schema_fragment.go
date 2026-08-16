package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
)

// SchemaFragment is Value's callback-free cold Factor surface. It owns only
// the Factor slot, its exact forms, and Value's one summary form; Link-bound
// coordinates, lattice callbacks, and executable behavior remain outside it.
type SchemaFragment struct {
	slot        *engine.FactorSlot[value.Value]
	ref         engine.FactorRef[value.Value]
	exactRead   engine.SchemaReadForm[value.Value]
	summaryRead engine.SchemaReadForm[value.Value]
	exactWrite  engine.SchemaWriteForm[value.Value]
}

func (fragment *SchemaFragment) Ref() engine.FactorRef[value.Value] {
	if fragment == nil {
		return engine.FactorRef[value.Value]{}
	}
	return fragment.ref
}
func (fragment *SchemaFragment) ExactRead() engine.SchemaReadForm[value.Value] {
	if fragment == nil {
		return engine.SchemaReadForm[value.Value]{}
	}
	return fragment.exactRead
}
func (fragment *SchemaFragment) SummaryRead() engine.SchemaReadForm[value.Value] {
	if fragment == nil {
		return engine.SchemaReadForm[value.Value]{}
	}
	return fragment.summaryRead
}
func (fragment *SchemaFragment) ExactWrite() engine.SchemaWriteForm[value.Value] {
	if fragment == nil {
		return engine.SchemaWriteForm[value.Value]{}
	}
	return fragment.exactWrite
}

// DeclareSchema records Value's global Factor shape in a callback-free schema
// builder. The supplied keys are the sole semantic authority for this row.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, summarySemantic engine.SemanticKey) (*SchemaFragment, bool) {
	if builder == nil || !semantic.Available() || !summarySemantic.Available() || semantic == summarySemantic {
		return nil, false
	}
	slot, ok := engine.NewFactorSlot[value.Value](builder, semantic)
	if !ok {
		return nil, false
	}
	read, ok := slot.ExactRead()
	if !ok {
		return nil, false
	}
	summary, ok := slot.SummaryRead(summarySemantic)
	if !ok {
		return nil, false
	}
	write, ok := slot.ExactWrite()
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, ref: slot.Ref(), exactRead: read, summaryRead: summary, exactWrite: write}, true
}
