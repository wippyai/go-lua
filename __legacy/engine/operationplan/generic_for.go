package operationplan

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
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

func (s GenericForSource) clone() GenericForSource {
	s.RootPath.Segments = append([]segment.Segment(nil), s.RootPath.Segments...)
	return s
}

func (s GenericForSource) valid() bool {
	return s.HasCallPoint == (s.CallPoint != 0) && s.HasRootPath == !s.RootPath.IsEmpty()
}

// GenericForOperation is the complete neutral payload for one loop-variable
// binding. It belongs to the operation plan so every executor consumes the
// same immutable semantic operation.
type GenericForOperation struct {
	variableIndex    int
	target           symbol.ID
	firstTarget      symbol.ID
	protocolSources  []GenericForSource
	sourceContracts  []typ.Type
	iterator         iteration.Iterator
	hasIterator      bool
	callableIterator bool
}

func NewGenericForOperation(variableIndex int, target, firstTarget symbol.ID, protocolSources []GenericForSource, sourceContracts []typ.Type) (GenericForOperation, bool) {
	if variableIndex < 0 || target == 0 {
		return GenericForOperation{}, false
	}
	sources := make([]GenericForSource, len(protocolSources))
	for i, source := range protocolSources {
		if !source.valid() {
			return GenericForOperation{}, false
		}
		sources[i] = source.clone()
	}
	return GenericForOperation{variableIndex: variableIndex, target: target, firstTarget: firstTarget, protocolSources: sources, sourceContracts: append([]typ.Type(nil), sourceContracts...)}, true
}

func (o GenericForOperation) VariableIndex() int       { return o.variableIndex }
func (o GenericForOperation) Target() symbol.ID        { return o.target }
func (o GenericForOperation) FirstTarget() symbol.ID   { return o.firstTarget }
func (o GenericForOperation) ProtocolSourceCount() int { return len(o.protocolSources) }
func (o GenericForOperation) ProtocolSource(index int) (GenericForSource, bool) {
	if index < 0 || index >= len(o.protocolSources) {
		return GenericForSource{}, false
	}
	return o.protocolSources[index].clone(), true
}
func (o GenericForOperation) SourceContract(index int) (typ.Type, bool) {
	if index < 0 || index >= len(o.sourceContracts) || o.sourceContracts[index] == nil {
		return nil, false
	}
	return o.sourceContracts[index], true
}
func (o GenericForOperation) WithIterator(iterator iteration.Iterator) GenericForOperation {
	if iterator.Kind != iteration.IterateIndexed && iterator.Kind != iteration.IterateKeyed {
		return o
	}
	o.iterator, o.hasIterator, o.callableIterator = iterator, true, false
	return o
}
func (o GenericForOperation) Iterator() (iteration.Iterator, bool) { return o.iterator, o.hasIterator }

// WithCallableIterator records that the canonical call descriptor returns the
// iterator function itself, rather than describing a collection projection.
func (o GenericForOperation) WithCallableIterator() GenericForOperation {
	o.iterator, o.hasIterator, o.callableIterator = iteration.Iterator{}, false, true
	return o
}
func (o GenericForOperation) CallableIterator() bool { return o.callableIterator }
func (o GenericForOperation) valid() bool            { return o.variableIndex >= 0 && o.target != 0 }
func (o GenericForOperation) clone() GenericForOperation {
	out := o
	out.protocolSources = make([]GenericForSource, len(o.protocolSources))
	for i, source := range o.protocolSources {
		out.protocolSources[i] = source.clone()
	}
	out.sourceContracts = append([]typ.Type(nil), o.sourceContracts...)
	return out
}

func (o GenericForOperation) equal(other GenericForOperation) bool {
	if o.variableIndex != other.variableIndex || o.target != other.target || o.firstTarget != other.firstTarget ||
		len(o.protocolSources) != len(other.protocolSources) || len(o.sourceContracts) != len(other.sourceContracts) {
		return false
	}
	if o.iterator != other.iterator || o.hasIterator != other.hasIterator || o.callableIterator != other.callableIterator {
		return false
	}
	for i, source := range o.protocolSources {
		otherSource := other.protocolSources[i]
		if source.Kind != otherSource.Kind || source.CallPoint != otherSource.CallPoint || source.HasCallPoint != otherSource.HasCallPoint ||
			source.HasRootPath != otherSource.HasRootPath || !source.RootPath.Equal(otherSource.RootPath) {
			return false
		}
	}
	for i := range o.sourceContracts {
		if !typ.TypeEquals(o.sourceContracts[i], other.sourceContracts[i]) {
			return false
		}
	}
	return true
}
