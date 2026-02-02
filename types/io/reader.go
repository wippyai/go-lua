package io

import (
	"bytes"
	"io"
	"math"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

type typeReader struct {
	r   *bytes.Reader
	err error
}

func (r *typeReader) readByte() byte {
	if r.err != nil {
		return 0
	}

	b, err := r.r.ReadByte()
	if err != nil {
		r.err = err
		return 0
	}

	return b
}

func (r *typeReader) readUint32() uint32 {
	if r.err != nil {
		return 0
	}

	var v uint32
	r.err = binaryRead(r.r, &v)

	return v
}

func (r *typeReader) readUint64() uint64 {
	if r.err != nil {
		return 0
	}

	var v uint64
	r.err = binaryRead64(r.r, &v)

	return v
}

func (r *typeReader) readString() string {
	if r.err != nil {
		return ""
	}

	length := r.readUint32()
	if length == 0 {
		return ""
	}

	if length > maxSliceLen {
		r.err = ErrCorruptedData
		return ""
	}

	data := make([]byte, length)
	_, r.err = io.ReadFull(r.r, data)

	return string(data)
}

func (r *typeReader) readBool() bool {
	return r.readByte() == 1
}

func (r *typeReader) checkSliceLen(n uint32) bool {
	if n > maxSliceLen {
		r.err = ErrCorruptedData
		return false
	}

	return true
}

func (r *typeReader) readType() typ.Type {
	if r.err != nil {
		return nil
	}

	k := byteToKind(r.readByte())

	switch k {
	case kind.Nil:
		return typ.Nil
	case kind.Boolean:
		return typ.Boolean
	case kind.Number:
		return typ.Number
	case kind.Integer:
		return typ.Integer
	case kind.String:
		return typ.String
	case kind.Any:
		return typ.Any
	case kind.Unknown:
		return typ.Unknown
	case kind.Never:
		return typ.Never
	case kind.Self:
		return typ.Self

	case kind.Optional:
		return typ.NewOptional(r.readType())

	case kind.Union:
		count := r.readUint32()
		if !r.checkSliceLen(count) {
			return nil
		}

		members := make([]typ.Type, count)
		for i := uint32(0); i < count; i++ {
			members[i] = r.readType()
		}

		return typ.NewUnion(members...)

	case kind.Intersection:
		count := r.readUint32()
		if !r.checkSliceLen(count) {
			return nil
		}

		members := make([]typ.Type, count)
		for i := uint32(0); i < count; i++ {
			members[i] = r.readType()
		}

		return typ.NewIntersection(members...)

	case kind.Tuple:
		count := r.readUint32()
		if !r.checkSliceLen(count) {
			return nil
		}

		elems := make([]typ.Type, count)
		for i := uint32(0); i < count; i++ {
			elems[i] = r.readType()
		}

		return typ.NewTuple(elems...)

	case kind.Function:
		paramCount := r.readUint32()
		if !r.checkSliceLen(paramCount) {
			return nil
		}

		fb := typ.Func()

		for i := uint32(0); i < paramCount; i++ {
			name := r.readString()
			pType := r.readType()

			if r.err != nil {
				return nil
			}

			optional := r.readBool()
			if optional {
				fb.OptParam(name, pType)
			} else {
				fb.Param(name, pType)
			}
		}

		if r.readBool() {
			fb.Variadic(r.readType())
		}

		retCount := r.readUint32()
		if !r.checkSliceLen(retCount) {
			return nil
		}

		returns := make([]typ.Type, retCount)
		for i := uint32(0); i < retCount; i++ {
			returns[i] = r.readType()

			if r.err != nil {
				return nil
			}
		}

		fb.Returns(returns...)

		eff := r.readEffectRow()
		fb.Effects(eff)

		if r.readBool() {
			spec := r.readContractSpec()
			if spec != nil {
				fb.Spec(spec)
			}
		}

		if refinement := r.readFunctionEffect(); refinement != nil {
			fb.WithRefinement(refinement)
		}

		return fb.Build()

	case kind.Array:
		return typ.NewArray(r.readType())

	case kind.Map:
		key := r.readType()
		value := r.readType()

		return typ.NewMap(key, value)

	case kind.Record:
		fieldCount := r.readUint32()
		if !r.checkSliceLen(fieldCount) {
			return nil
		}

		rb := typ.NewRecord()

		for i := uint32(0); i < fieldCount; i++ {
			name := r.readString()
			fType := r.readType()

			if r.err != nil {
				return nil
			}

			optional := r.readBool()
			readonly := r.readBool()

			switch {
			case readonly:
				rb.ReadonlyField(name, fType)
			case optional:
				rb.OptField(name, fType)
			default:
				rb.Field(name, fType)
			}
		}

		rec := rb.Build()
		if r.readBool() {
			rec.Metatable = r.readType()
		}

		return rec

	case kind.Generic:
		name := r.readString()

		paramCount := r.readUint32()
		if !r.checkSliceLen(paramCount) {
			return nil
		}

		params := make([]*typ.TypeParam, paramCount)
		for i := uint32(0); i < paramCount; i++ {
			params[i] = r.readTypeParam()
		}

		var body typ.Type
		if r.readBool() {
			body = r.readType()
		}

		return typ.NewGeneric(name, params, body)

	case kind.Instantiated:
		var generic *typ.Generic

		if r.readBool() {
			if g := r.readType(); g != nil {
				if gen, ok := g.(*typ.Generic); ok {
					generic = gen
				}
			}
		}

		argCount := r.readUint32()
		if !r.checkSliceLen(argCount) {
			return nil
		}

		args := make([]typ.Type, argCount)
		for i := uint32(0); i < argCount; i++ {
			args[i] = r.readType()
		}

		if generic == nil {
			return nil
		}
		return typ.Instantiate(generic, args...)

	case kind.TypeParam:
		name := r.readString()

		var constr typ.Type
		if r.readBool() {
			constr = r.readType()
		}

		return typ.NewTypeParam(name, constr)

	case kind.TypeVar:
		return typ.NewTypeVar(int(r.readUint32()))

	case kind.Literal:
		return r.readLiteral()

	case kind.Ref:
		module := r.readString()
		name := r.readString()

		return typ.NewRef(module, name)

	case kind.Alias:
		name := r.readString()
		target := r.readType()

		return typ.NewAlias(name, target)

	case kind.Meta:
		return typ.NewMeta(r.readType())

	case kind.Platform:
		return typ.NewPlatform(r.readString())

	case kind.Sum, kind.Interface, kind.Refined, kind.FieldAccess, kind.IndexAccess, kind.Recursive:
		r.err = ErrUnknownType
		return nil
	}

	r.err = ErrUnknownType
	return nil
}

func (r *typeReader) readTypeParam() *typ.TypeParam {
	name := r.readString()

	var constr typ.Type
	if r.readBool() {
		constr = r.readType()
	}

	return typ.NewTypeParam(name, constr)
}

func (r *typeReader) readLiteral() *typ.Literal {
	base := byteToKind(r.readByte())
	switch base {
	case kind.Boolean:
		return typ.LiteralBool(r.readBool())
	case kind.Integer:
		return typ.LiteralInt(int64(r.readUint64()))
	case kind.Number:
		return typ.LiteralNumber(math.Float64frombits(r.readUint64()))
	case kind.String:
		return typ.LiteralString(r.readString())
	case kind.Nil, kind.Any, kind.Unknown, kind.Never, kind.Optional, kind.Union,
		kind.Intersection, kind.Tuple, kind.Function, kind.Array, kind.Map, kind.Record,
		kind.Sum, kind.Interface, kind.Alias, kind.Generic, kind.Instantiated, kind.Platform,
		kind.Literal, kind.Self, kind.Ref, kind.Meta, kind.TypeParam, kind.TypeVar,
		kind.Refined, kind.FieldAccess, kind.IndexAccess, kind.Recursive:
		r.err = ErrInvalidType
		return nil
	}

	r.err = ErrInvalidType
	return nil
}

func (r *typeReader) readPath() constraint.Path {
	root := r.readString()
	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return constraint.Path{}
	}

	segs := make([]constraint.Segment, 0, count)

	for i := uint32(0); i < count; i++ {
		segKind := constraint.SegmentKind(r.readByte())
		seg := constraint.Segment{Kind: segKind}

		switch segKind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			seg.Name = r.readString()
		case constraint.SegmentIndexInt:
			seg.Index = int(int32(r.readUint32()))
		}

		segs = append(segs, seg)
	}

	return constraint.Path{Root: root, Segments: segs}
}

func (r *typeReader) readTypeKey() narrow.TypeKey {
	k := narrow.TypeKeyKind(r.readByte())
	switch k {
	case narrow.TypeKeyBuiltin:
		return narrow.TypeKey{Kind: k, Name: r.readString()}
	case narrow.TypeKeyHash:
		return narrow.TypeKey{Kind: k, Hash: r.readUint64()}
	case narrow.TypeKeyInvalid:
		return narrow.TypeKey{}
	}

	return narrow.TypeKey{}
}

func (r *typeReader) readCallbackSpec() *contract.CallbackSpec {
	if !r.readBool() {
		return nil
	}

	index := int(int32(r.readUint32()))

	cb := &contract.CallbackSpec{
		InputSource:    effect.ParamRef{Index: index},
		ReturnsBoolean: r.readBool(),
		Cardinality:    contract.Cardinality(r.readUint32()),
		Pure:           r.readBool(),
	}

	envCount := r.readUint32()
	if envCount > 0 {
		if !r.checkSliceLen(envCount) {
			return cb
		}

		cb.EnvOverlay = make(map[string]typ.Type, envCount)

		for i := uint32(0); i < envCount; i++ {
			name := r.readString()
			cb.EnvOverlay[name] = r.readType()
		}
	}

	return cb
}

func (r *typeReader) readContractSpec() *contract.Spec {
	spec := contract.NewSpec()
	spec.Requires = r.readCondition()
	spec.Ensures = r.readCondition()
	spec.ExprRequires = r.readExprCompares()
	spec.ExprEnsures = r.readExprCompares()
	spec.Effects = r.readEffectRow()
	cbCount := r.readUint32()

	if !r.checkSliceLen(cbCount) {
		return spec
	}

	for i := uint32(0); i < cbCount; i++ {
		key := int(r.readUint32())

		if spec.Callbacks == nil {
			spec.Callbacks = make(map[int]*contract.CallbackSpec)
		}

		if cb := r.readCallbackSpec(); cb != nil {
			spec.Callbacks[key] = cb
		}
	}

	spec.Return = r.readReturnSpec()

	return spec
}

func (r *typeReader) readReturnSpec() *contract.ReturnSpec {
	if !r.readBool() {
		return nil
	}

	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return nil
	}

	rs := &contract.ReturnSpec{
		Cases: make([]contract.ReturnCase, count),
	}
	for i := uint32(0); i < count; i++ {
		rs.Cases[i] = contract.ReturnCase{
			When: r.readCondition(),
			Type: r.readType(),
		}
	}

	if r.readBool() {
		rs.Default = r.readType()
	}

	return rs
}

func (r *typeReader) readEffectRow() effect.Row {
	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return effect.Empty
	}

	labels := make([]effect.Label, 0, count)

	for i := uint32(0); i < count; i++ {
		if label := r.readEffectLabel(); label != nil {
			labels = append(labels, label)
		}
	}

	var tail *effect.Var
	if r.readBool() {
		tail = &effect.Var{Name: r.readString()}
	}

	return effect.Row{Labels: labels, Tail: tail}
}

func (r *typeReader) readEffectLabel() effect.Label {
	key := r.readString()
	if key == "" {
		return nil
	}

	codec, ok := effect.Lookup(key)
	if !ok {
		return nil
	}

	label, _ := codec.Decode(&readerAdapter{r})

	return label
}

type readerAdapter struct{ r *typeReader }

func (a *readerAdapter) ReadByte() (byte, error) {
	return a.r.readByte(), a.r.err
}

func (a *readerAdapter) ReadInt32() (int32, error) {
	return int32(a.r.readUint32()), a.r.err
}

func (a *readerAdapter) ReadString() (string, error) {
	return a.r.readString(), a.r.err
}

func (a *readerAdapter) ReadType() (any, error) {
	return a.r.readType(), a.r.err
}
