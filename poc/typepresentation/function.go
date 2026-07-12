// Package typepresentation prototypes a split between semantic function types
// and source-owned presentation labels. It is intentionally not wired into the
// analyzer yet.
package typepresentation

import (
	"hash/fnv"
	"slices"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Param is one function parameter. Label is presentation-only. Receiver is the
// calling-convention bit that the current typ.Param.Name == "self" convention
// conflates with presentation.
type Param struct {
	Label    string
	Type     typ.Type
	Optional bool
	Receiver bool
}

// Function owns one canonical semantic type plus immutable source labels.
// Semantic Params contain only "self" for receiver-consuming positions and an
// empty label otherwise. Labels never enter semantic equality or hashing.
type Function struct {
	semantic *typ.Function
	labels   []string
	receiver []bool
	hash     uint64
}

// NewFunction constructs semantic identity and presentation once at the
// immutable type boundary. Nested recursive and generic parameter/result types
// remain shared; no later type-witness walk is required.
func NewFunction(typeParams []*typ.TypeParam, params []Param, variadic typ.Type, returns []typ.Type) Function {
	b := typ.Func().ReserveParams(len(params))
	for _, typeParam := range typeParams {
		b.TypeParamRef(typeParam)
	}
	labels := make([]string, len(params))
	receivers := make([]bool, len(params))
	for i, param := range params {
		labels[i] = param.Label
		receivers[i] = param.Receiver
		semanticName := ""
		if param.Receiver {
			semanticName = "self"
		}
		if param.Optional {
			b.OptParam(semanticName, param.Type)
		} else {
			b.Param(semanticName, param.Type)
		}
	}
	if variadic != nil {
		b.Variadic(variadic)
	}
	b.Returns(returns...)
	semantic := b.Build()
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte("presented-function-v1:"))
	var semanticHash [8]byte
	for i := range semanticHash {
		semanticHash[i] = byte(semantic.Hash() >> (8 * i))
	}
	_, _ = hasher.Write(semanticHash[:])
	for _, receiver := range receivers {
		if receiver {
			_, _ = hasher.Write([]byte{1})
		} else {
			_, _ = hasher.Write([]byte{0})
		}
	}
	return Function{semantic: semantic, labels: labels, receiver: receivers, hash: hasher.Sum64()}
}

func (f Function) Semantic() *typ.Function { return f.semantic }
func (f Function) Labels() []string        { return slices.Clone(f.labels) }
func (f Function) Hash() uint64            { return f.hash }

func (f Function) Equal(other Function) bool {
	return f.hash == other.hash && slices.Equal(f.receiver, other.receiver) && typ.TypeEquals(f.semantic, other.semantic)
}

// Label returns presentation metadata without exposing mutable backing storage.
func (f Function) Label(index int) (string, bool) {
	if index < 0 || index >= len(f.labels) {
		return "", false
	}
	return f.labels[index], true
}
