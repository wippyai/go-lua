package summary

import (
	"fmt"
	"hash"
	"hash/fnv"
	"math"
	"reflect"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
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
	w := summaryDigestWriter{h: fnv.New64a(), reg: reg}
	w.writeString("summary-payload-v1")
	w.writeReflect(reflect.ValueOf(s))
	return Digest(w.h.Sum64())
}

type summaryDigestWriter struct {
	h   hash.Hash64
	reg *axis.Registry
}

func (w *summaryDigestWriter) writeString(value string) {
	fmt.Fprintf(w.h, "s:%d:", len(value))
	_, _ = w.h.Write([]byte(value))
	_, _ = w.h.Write([]byte(";"))
}

func (w *summaryDigestWriter) writeProduct(value product.Value) {
	fmt.Fprintf(w.h, "product-presence:%d;", product.PresenceOf(value))
	if t, ok := typevalue.TypeOf(w.reg, value); ok {
		fmt.Fprintf(w.h, "product-type:%d:", typ.EqualityHash(t))
		w.writeString(t.String())
		return
	}
	fmt.Fprintf(w.h, "product-fallback:%d;", product.Hash(w.reg, value))
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
				fmt.Fprintf(w.h, "type:%d:", typ.EqualityHash(value))
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
		fmt.Fprintf(w.h, "len:%d;", v.Len())
		for i := 0; i < v.Len(); i++ {
			w.writeReflect(v.Index(i))
		}
	case reflect.Map:
		w.writeMap(v)
	case reflect.String:
		w.writeString(v.String())
	case reflect.Bool:
		fmt.Fprintf(w.h, "bool:%t;", v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fmt.Fprintf(w.h, "int:%d;", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		fmt.Fprintf(w.h, "uint:%d;", v.Uint())
	case reflect.Float32, reflect.Float64:
		fmt.Fprintf(w.h, "float:%x;", math.Float64bits(v.Float()))
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
		fmt.Fprintf(w.h, "len:-1;")
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
		keyDigest := fnv.New64a()
		keyWriter := summaryDigestWriter{h: keyDigest, reg: w.reg}
		keyWriter.writeReflect(key)
		entries = append(entries, entry{digest: keyDigest.Sum64(), key: key, value: v.MapIndex(key)})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].digest != entries[j].digest {
			return entries[i].digest < entries[j].digest
		}
		return fmt.Sprint(entries[i].key.Interface()) < fmt.Sprint(entries[j].key.Interface())
	})
	fmt.Fprintf(w.h, "len:%d;", len(entries))
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
