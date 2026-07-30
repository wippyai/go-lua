// Package bytecode serializes compiled Lua function prototypes for local cache
// storage. The format is internal to go-lua and only promises compatibility
// across matching bytecode versions.
package bytecode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	lua "github.com/wippyai/go-lua"
)

const (
	headerMagic = 0x4C554143 // "LUAC"
	version     = 3          // v3 removes analysis metadata from executable bytecode.
)

var (
	ErrInvalidHeader       = errors.New("invalid bytecode header")
	ErrVersionMismatch     = errors.New("bytecode version mismatch")
	ErrCorruptedBytecode   = errors.New("corrupted bytecode")
	ErrUnsupportedConstant = errors.New("unsupported bytecode constant")
)

// Dump serializes proto into the go-lua bytecode cache format.
func Dump(proto *lua.FunctionProto) ([]byte, error) {
	if proto == nil {
		return nil, ErrCorruptedBytecode
	}

	var buf bytes.Buffer
	w := &writer{w: &buf}
	w.writeUint32(headerMagic)
	w.writeByte(version)
	w.writeProto(proto)
	if w.err != nil {
		return nil, w.err
	}
	return buf.Bytes(), nil
}

// Undump deserializes a FunctionProto produced by Dump.
func Undump(data []byte) (*lua.FunctionProto, error) {
	r := &reader{r: bytes.NewReader(data)}
	magic := r.readUint32()
	if r.err != nil || magic != headerMagic {
		return nil, ErrInvalidHeader
	}
	if ver := r.readByte(); r.err != nil || ver != version {
		return nil, ErrVersionMismatch
	}

	proto, err := r.readProto()
	if err != nil {
		return nil, err
	}
	if r.r.Len() != 0 {
		return nil, ErrCorruptedBytecode
	}

	rebuildStringConstants(proto)
	return proto, nil
}

func rebuildStringConstants(proto *lua.FunctionProto) {
	if proto == nil {
		return
	}
	proto.RebuildStringConstants()
	for _, child := range proto.FunctionPrototypes {
		rebuildStringConstants(child)
	}
}

type writer struct {
	w   io.Writer
	err error
}

func (w *writer) writeByte(v byte) {
	if w.err != nil {
		return
	}
	_, w.err = w.w.Write([]byte{v})
}

func (w *writer) writeUint32(v uint32) {
	if w.err != nil {
		return
	}
	w.err = binary.Write(w.w, binary.LittleEndian, v)
}

func (w *writer) writeUint64(v uint64) {
	if w.err != nil {
		return
	}
	w.err = binary.Write(w.w, binary.LittleEndian, v)
}

func (w *writer) writeInt(v int) {
	w.writeUint64(uint64(v))
}

func (w *writer) writeBytes(data []byte) {
	w.writeUint32(uint32(len(data)))
	if w.err != nil || len(data) == 0 {
		return
	}
	_, w.err = w.w.Write(data)
}

func (w *writer) writeString(s string) {
	w.writeBytes([]byte(s))
}

func (w *writer) writeProto(proto *lua.FunctionProto) {
	if w.err != nil {
		return
	}
	if proto == nil {
		w.err = ErrCorruptedBytecode
		return
	}

	w.writeString(proto.SourceName)
	w.writeInt(proto.LineDefined)
	w.writeInt(proto.LastLineDefined)
	w.writeByte(proto.NumUpvalues)
	w.writeByte(proto.NumParameters)
	w.writeByte(proto.IsVarArg)
	w.writeByte(proto.NumUsedRegisters)

	w.writeUint32(uint32(len(proto.Code)))
	for _, op := range proto.Code {
		w.writeUint32(op)
	}

	w.writeUint32(uint32(len(proto.Constants)))
	for _, constant := range proto.Constants {
		w.writeConstant(constant)
	}

	w.writeUint32(uint32(len(proto.FunctionPrototypes)))
	for _, child := range proto.FunctionPrototypes {
		w.writeProto(child)
	}

	w.writeUint32(uint32(len(proto.DbgSourcePositions)))
	for _, pos := range proto.DbgSourcePositions {
		w.writeInt(pos)
	}

	w.writeUint32(uint32(len(proto.DbgLocals)))
	for _, local := range proto.DbgLocals {
		if local == nil {
			w.err = ErrCorruptedBytecode
			return
		}
		w.writeString(local.Name)
		w.writeInt(local.StartPc)
		w.writeInt(local.EndPc)
	}

	w.writeUint32(uint32(len(proto.DbgCalls)))
	for _, call := range proto.DbgCalls {
		w.writeString(call.Name)
		w.writeInt(call.Pc)
	}

	w.writeUint32(uint32(len(proto.DbgUpvalues)))
	for _, upvalue := range proto.DbgUpvalues {
		w.writeString(upvalue)
	}
}

func (w *writer) writeConstant(v lua.LValue) {
	if w.err != nil {
		return
	}
	switch val := v.(type) {
	case *lua.LNilType:
		w.writeByte(byte(lua.LTNil))
	case lua.LBool:
		w.writeByte(byte(lua.LTBool))
		if val {
			w.writeByte(1)
		} else {
			w.writeByte(0)
		}
	case lua.LNumber:
		w.writeByte(byte(lua.LTNumber))
		w.writeUint64(math.Float64bits(float64(val)))
	case lua.LInteger:
		w.writeByte(byte(lua.LTInteger))
		w.writeUint64(uint64(val))
	case lua.LString:
		w.writeByte(byte(lua.LTString))
		w.writeString(string(val))
	default:
		if v == nil {
			w.err = ErrCorruptedBytecode
			return
		}
		w.err = fmt.Errorf("%w: %s", ErrUnsupportedConstant, v.Type())
	}
}

type reader struct {
	r   *bytes.Reader
	err error
}

func (r *reader) readByte() byte {
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

func (r *reader) readUint32() uint32 {
	if r.err != nil {
		return 0
	}
	var v uint32
	r.err = binary.Read(r.r, binary.LittleEndian, &v)
	return v
}

func (r *reader) readUint64() uint64 {
	if r.err != nil {
		return 0
	}
	var v uint64
	r.err = binary.Read(r.r, binary.LittleEndian, &v)
	return v
}

func (r *reader) readInt() int {
	return int(r.readUint64())
}

func (r *reader) readBytes() []byte {
	if r.err != nil {
		return nil
	}
	length := r.readUint32()
	if r.err != nil || length == 0 {
		return nil
	}
	data := make([]byte, length)
	_, r.err = io.ReadFull(r.r, data)
	return data
}

func (r *reader) readString() string {
	return string(r.readBytes())
}

func (r *reader) readProto() (*lua.FunctionProto, error) {
	proto := &lua.FunctionProto{
		SourceName:       r.readString(),
		LineDefined:      r.readInt(),
		LastLineDefined:  r.readInt(),
		NumUpvalues:      r.readByte(),
		NumParameters:    r.readByte(),
		IsVarArg:         r.readByte(),
		NumUsedRegisters: r.readByte(),
	}
	if r.err != nil {
		return nil, ErrCorruptedBytecode
	}

	codeLen := r.readUint32()
	proto.Code = make([]uint32, codeLen)
	for i := range proto.Code {
		proto.Code[i] = r.readUint32()
	}

	constLen := r.readUint32()
	proto.Constants = make([]lua.LValue, constLen)
	for i := range proto.Constants {
		proto.Constants[i] = r.readConstant()
	}

	childLen := r.readUint32()
	proto.FunctionPrototypes = make([]*lua.FunctionProto, childLen)
	for i := range proto.FunctionPrototypes {
		child, err := r.readProto()
		if err != nil {
			return nil, err
		}
		proto.FunctionPrototypes[i] = child
	}

	posLen := r.readUint32()
	proto.DbgSourcePositions = make([]int, posLen)
	for i := range proto.DbgSourcePositions {
		proto.DbgSourcePositions[i] = r.readInt()
	}

	localLen := r.readUint32()
	proto.DbgLocals = make([]*lua.DbgLocalInfo, localLen)
	for i := range proto.DbgLocals {
		proto.DbgLocals[i] = &lua.DbgLocalInfo{
			Name:    r.readString(),
			StartPc: r.readInt(),
			EndPc:   r.readInt(),
		}
	}

	callLen := r.readUint32()
	proto.DbgCalls = make([]lua.DbgCall, callLen)
	for i := range proto.DbgCalls {
		proto.DbgCalls[i] = lua.DbgCall{
			Name: r.readString(),
			Pc:   r.readInt(),
		}
	}

	upvalueLen := r.readUint32()
	proto.DbgUpvalues = make([]string, upvalueLen)
	for i := range proto.DbgUpvalues {
		proto.DbgUpvalues[i] = r.readString()
	}

	if r.err != nil {
		return nil, ErrCorruptedBytecode
	}
	return proto, nil
}

func (r *reader) readConstant() lua.LValue {
	tag := lua.LValueType(r.readByte())
	if r.err != nil {
		return lua.LNil
	}
	switch tag {
	case lua.LTNil:
		return lua.LNil
	case lua.LTBool:
		if r.readByte() == 1 {
			return lua.LTrue
		}
		return lua.LFalse
	case lua.LTNumber:
		return lua.LNumber(math.Float64frombits(r.readUint64()))
	case lua.LTInteger:
		return lua.LInteger(int64(r.readUint64()))
	case lua.LTString:
		return lua.LString(r.readString())
	default:
		r.err = ErrCorruptedBytecode
		return lua.LNil
	}
}
