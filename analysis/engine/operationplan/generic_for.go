package operationplan

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// GenericForSourceKind identifies the syntax-free iterator producer.
type GenericForSourceKind uint8

const (
	GenericForSourceUnknown GenericForSourceKind = iota
	GenericForSourceExpression
	GenericForSourceCall
)

// GenericForSource is the immutable operation-plan projection of the first
// iterator source.
type GenericForSource struct {
	Kind         GenericForSourceKind
	CallPoint    cfg.Point
	HasCallPoint bool
	RootPath     pathdom.Path
	HasRootPath  bool
}

// GenericForOperation is the complete neutral payload for one loop-variable
// binding. It belongs to the operation plan so every executor consumes the
// same immutable semantic operation.
type GenericForOperation struct {
	variableIndex   int
	target          symbol.ID
	firstTarget     symbol.ID
	source          GenericForSource
	sourceContracts []typ.Type
}

func NewGenericForOperation(variableIndex int, target, firstTarget symbol.ID, source GenericForSource, sourceContracts []typ.Type) (GenericForOperation, bool) {
	if variableIndex < 0 || target == 0 {
		return GenericForOperation{}, false
	}
	source.RootPath.Segments = append([]segment.Segment(nil), source.RootPath.Segments...)
	if source.HasCallPoint != (source.CallPoint != 0) || source.HasRootPath != !source.RootPath.IsEmpty() {
		return GenericForOperation{}, false
	}
	return GenericForOperation{variableIndex: variableIndex, target: target, firstTarget: firstTarget, source: source, sourceContracts: append([]typ.Type(nil), sourceContracts...)}, true
}

func (o GenericForOperation) VariableIndex() int     { return o.variableIndex }
func (o GenericForOperation) Target() symbol.ID      { return o.target }
func (o GenericForOperation) FirstTarget() symbol.ID { return o.firstTarget }
func (o GenericForOperation) Source() GenericForSource {
	out := o.source
	out.RootPath.Segments = append([]segment.Segment(nil), out.RootPath.Segments...)
	return out
}
func (o GenericForOperation) SourceContract(index int) (typ.Type, bool) {
	if index < 0 || index >= len(o.sourceContracts) || o.sourceContracts[index] == nil {
		return nil, false
	}
	return o.sourceContracts[index], true
}
func (o GenericForOperation) valid() bool { return o.variableIndex >= 0 && o.target != 0 }
func (o GenericForOperation) clone() GenericForOperation {
	out := o
	out.source = o.Source()
	out.sourceContracts = append([]typ.Type(nil), o.sourceContracts...)
	return out
}

func (o GenericForOperation) equal(other GenericForOperation) bool {
	if o.variableIndex != other.variableIndex || o.target != other.target || o.firstTarget != other.firstTarget ||
		o.source.Kind != other.source.Kind || o.source.CallPoint != other.source.CallPoint || o.source.HasCallPoint != other.source.HasCallPoint ||
		o.source.HasRootPath != other.source.HasRootPath || !o.source.RootPath.Equal(other.source.RootPath) || len(o.sourceContracts) != len(other.sourceContracts) {
		return false
	}
	for i := range o.sourceContracts {
		if !typ.TypeEquals(o.sourceContracts[i], other.sourceContracts[i]) {
			return false
		}
	}
	return true
}
