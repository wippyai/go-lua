package summary

import (
	"fmt"
	"math"
	"reflect"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// PayloadDigest returns a deterministic content digest for a summary payload.
// Callers that own normalized summaries can use NormalizedPayloadDigest to avoid
// a defensive clone/normalization pass.
func PayloadDigest(reg *axis.Registry, s Summary) Digest {
	return NormalizedPayloadDigest(reg, Normalize(reg, s))
}

// NormalizedPayloadDigest returns a deterministic content digest for a summary
// payload already normalized under reg.
func NormalizedPayloadDigest(reg *axis.Registry, s Summary) Digest {
	w := summaryDigestWriter{h: internalhash.NewWriter(), reg: reg}
	w.writeString("summary-payload-v1")
	w.writeReflect(reflect.ValueOf(s))
	return Digest(w.h.Sum64())
}

type summaryDigestWriter struct {
	h   internalhash.Writer
	reg *axis.Registry
}

func (w *summaryDigestWriter) writeRaw(value string) {
	_, _ = w.h.WriteString(value)
}

func (w *summaryDigestWriter) writeByte(value byte) {
	_ = w.h.WriteByte(value)
}

func (w *summaryDigestWriter) writeRawInt(value int64) {
	w.h.WriteIntDecimal(value)
}

func (w *summaryDigestWriter) writeRawUint(value uint64) {
	w.h.WriteUintDecimal(value)
}

func (w *summaryDigestWriter) writeString(value string) {
	w.writeRaw("s:")
	w.writeRawInt(int64(len(value)))
	w.writeByte(':')
	w.writeRaw(value)
	w.writeByte(';')
}

func (w *summaryDigestWriter) writeProduct(value product.Value) {
	w.writeRaw("product-presence:")
	w.writeRawInt(int64(product.PresenceOf(value)))
	w.writeByte(';')
	if t, ok := typevalue.TypeOf(w.reg, value); ok {
		w.writeRaw("product-type:")
		w.writeRawUint(typ.EqualityHash(t))
		w.writeByte(':')
		w.writeString(t.String())
		return
	}
	w.writeRaw("product-fallback:")
	w.writeRawUint(product.Hash(w.reg, value))
	w.writeByte(';')
}

func (w *summaryDigestWriter) writeReflect(v reflect.Value) {
	if !v.IsValid() {
		w.writeString("<invalid>")
		return
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			w.writeString(v.Type().String() + ":<nil>")
			return
		}
		w.writeString("interface:" + v.Type().String())
		w.writeReflect(v.Elem())
		return
	}
	if v.CanInterface() {
		if value, ok := v.Interface().(product.Value); ok {
			w.writeProduct(value)
			return
		}
		if value, ok := v.Interface().(typ.Type); ok {
			if value == nil {
				w.writeString("type:<nil>")
			} else {
				w.writeRaw("type:")
				w.writeRawUint(typ.EqualityHash(value))
				w.writeByte(':')
				w.writeString(value.String())
			}
			return
		}
		if value, ok := v.Interface().(pathdom.Path); ok {
			w.writeString("path:" + string(value.Key()))
			return
		}
		if value, ok := v.Interface().(heapidentity.TableObject); ok {
			w.writeTableObject(value)
			return
		}
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			w.writeString(v.Type().String() + ":<nil>")
			return
		}
		w.writeString("ptr:" + v.Type().String())
		w.writeReflect(v.Elem())
	case reflect.Struct:
		w.writeString("struct:" + v.Type().String())
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			w.writeString("field:" + field.Name)
			w.writeReflect(v.Field(i))
		}
	case reflect.Slice, reflect.Array:
		w.writeString("seq:" + v.Type().String())
		w.writeRaw("len:")
		w.writeRawInt(int64(v.Len()))
		w.writeByte(';')
		for i := 0; i < v.Len(); i++ {
			w.writeReflect(v.Index(i))
		}
	case reflect.Map:
		w.writeMap(v)
	case reflect.String:
		w.writeString(v.String())
	case reflect.Bool:
		w.writeRaw("bool:")
		w.h.WriteBool(v.Bool())
		w.writeByte(';')
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		w.writeRaw("int:")
		w.writeRawInt(v.Int())
		w.writeByte(';')
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		w.writeRaw("uint:")
		w.writeRawUint(v.Uint())
		w.writeByte(';')
	case reflect.Float32, reflect.Float64:
		w.writeRaw("float:")
		w.h.WriteUintHex(math.Float64bits(v.Float()))
		w.writeByte(';')
	case reflect.Invalid:
		w.writeString("<invalid>")
	default:
		if v.CanInterface() {
			w.writeString(fmt.Sprintf("%s:%v", v.Type(), v.Interface()))
		} else {
			w.writeString(v.Type().String() + ":<unexported>")
		}
	}
}

func (w *summaryDigestWriter) writeMap(v reflect.Value) {
	w.writeString("map:" + v.Type().String())
	if v.IsNil() {
		w.writeRaw("len:-1;")
		return
	}
	type entry struct {
		digest uint64
		key    reflect.Value
		value  reflect.Value
	}
	keys := v.MapKeys()
	entries := make([]entry, 0, len(keys))
	for _, key := range keys {
		keyWriter := summaryDigestWriter{h: internalhash.NewWriter(), reg: w.reg}
		keyWriter.writeReflect(key)
		entries = append(entries, entry{digest: keyWriter.h.Sum64(), key: key, value: v.MapIndex(key)})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].digest != entries[j].digest {
			return entries[i].digest < entries[j].digest
		}
		return fmt.Sprint(entries[i].key.Interface()) < fmt.Sprint(entries[j].key.Interface())
	})
	w.writeRaw("len:")
	w.writeRawInt(int64(len(entries)))
	w.writeByte(';')
	for _, entry := range entries {
		w.writeReflect(entry.key)
		w.writeReflect(entry.value)
	}
}

func (w *summaryDigestWriter) writeTableObject(object heapidentity.TableObject) {
	w.writeString("heapidentity.TableObject")
	w.writeString("root")
	w.writeReflect(reflect.ValueOf(object.Root()))
	w.writeString("static-members")
	w.writeReflect(reflect.ValueOf(object.StaticMembers()))
	w.writeString("dynamic-index-facts")
	w.writeReflect(reflect.ValueOf(object.DynamicIndexFacts()))
}
