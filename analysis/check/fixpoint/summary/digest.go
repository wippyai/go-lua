package summary

import (
	"math"
	"reflect"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// NormalizedPayloadDigest returns a deterministic content digest for a summary
// payload. Its payload is normalized under reg and excludes metadata that
// summary equality deliberately ignores.
func NormalizedPayloadDigest(reg *axis.Registry, s Summary) Digest {
	s = Normalize(reg, s)
	// HeapKeySpace only re-interns heap facts for consumers. It is metadata and
	// is intentionally excluded by summary equality. Retain it on the writer so
	// solve-local dense keys can be encoded by structural spelling.
	heapKeySpace := s.HeapKeySpace
	s.HeapKeySpace = nil
	w := summaryDigestWriter{h: internalhash.NewWriter(), reg: reg, heapKeySpace: heapKeySpace}
	w.writeString("summary-payload-v1")
	w.writeReflect(reflect.ValueOf(s))
	return Digest(w.h.Sum64())
}

type summaryDigestWriter struct {
	h            internalhash.Writer
	reg          *axis.Registry
	heapKeySpace *keyspace.KeySpace
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
	// product.Hash is the canonical product encoding: it covers the normalized
	// shape, presence, and every registered semantic axis used by product.Equal.
	// Do not project a type from this value here; that would discard the other
	// axes and let the digest drift from summary equality.
	w.writeRaw("product:")
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
		if value, ok := v.Interface().(keyspace.Key); ok {
			w.writeKeySpaceKey(value)
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
		panic("summary digest: unsupported reflect kind " + v.Kind().String() + " for " + v.Type().String())
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
		keyWriter := summaryDigestWriter{h: internalhash.NewWriter(), reg: w.reg, heapKeySpace: w.heapKeySpace}
		keyWriter.writeReflect(key)
		entries = append(entries, entry{digest: keyWriter.h.Sum64(), key: key, value: v.MapIndex(key)})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].digest != entries[j].digest {
			return entries[i].digest < entries[j].digest
		}
		return compareDigestMapKeys(entries[i].key, entries[j].key) < 0
	})
	w.writeRaw("len:")
	w.writeRawInt(int64(len(entries)))
	w.writeByte(';')
	for _, entry := range entries {
		w.writeReflect(entry.key)
		w.writeReflect(entry.value)
	}
}

func (w *summaryDigestWriter) writeKeySpaceKey(key keyspace.Key) {
	w.writeString("keyspace.Key")
	if key.Kind == keyspace.KindInvalid {
		w.writeString("invalid")
		return
	}
	if w.heapKeySpace == nil {
		panic("summary digest: non-empty keyspace.Key has no producing HeapKeySpace")
	}
	spelling := w.heapKeySpace.FormatReadOnly(key)
	if spelling == "" {
		panic("summary digest: keyspace.Key is foreign to producing HeapKeySpace")
	}
	// Kind and the non-dense root dimensions preserve key namespaces whose
	// presentation spelling can overlap. Root and Segs themselves are omitted:
	// both are solve-local intern ids already represented by spelling.
	w.writeRaw("kind:")
	w.writeRawUint(uint64(key.Kind))
	w.writeByte(';')
	w.writeRaw("sym:")
	w.writeRawUint(uint64(key.Sym))
	w.writeByte(';')
	w.writeRaw("ver:")
	w.writeRawUint(uint64(key.Ver))
	w.writeByte(';')
	w.writeRaw("canon:")
	w.h.WriteBool(key.Canon)
	w.writeByte(';')
	w.writeString("spelling:" + string(spelling))
}

func compareDigestMapKeys(a, b reflect.Value) int {
	if a.Type() != b.Type() {
		return compareDigestStrings(a.Type().String(), b.Type().String())
	}
	switch a.Kind() {
	case reflect.Bool:
		if a.Bool() == b.Bool() {
			return 0
		}
		if !a.Bool() {
			return -1
		}
		return 1
	case reflect.String:
		return compareDigestStrings(a.String(), b.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return compareDigestInts(a.Int(), b.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return compareDigestUints(a.Uint(), b.Uint())
	case reflect.Array:
		for i := 0; i < a.Len(); i++ {
			if c := compareDigestMapKeys(a.Index(i), b.Index(i)); c != 0 {
				return c
			}
		}
		return 0
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if c := compareDigestMapKeys(a.Field(i), b.Field(i)); c != 0 {
				return c
			}
		}
		return 0
	case reflect.Interface:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() == b.IsNil() {
				return 0
			}
			if a.IsNil() {
				return -1
			}
			return 1
		}
		return compareDigestMapKeys(a.Elem(), b.Elem())
	default:
		panic("summary digest: unsupported map key kind " + a.Kind().String() + " for " + a.Type().String())
	}
}

func compareDigestStrings(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareDigestInts(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareDigestUints(a, b uint64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func (w *summaryDigestWriter) writeTableObject(object heapidentity.TableObject) {
	w.writeString("heapidentity.TableObject")
	w.writeString("bottom")
	w.writeReflect(reflect.ValueOf(object.IsBottom()))
	w.writeString("root")
	w.writeReflect(reflect.ValueOf(object.Root()))
	w.writeString("static-members")
	w.writeReflect(reflect.ValueOf(object.StaticMembers()))
	w.writeString("dynamic-index-facts")
	w.writeReflect(reflect.ValueOf(object.DynamicIndexFacts()))
	w.writeString("dynamic-index-facts-top")
	w.writeReflect(reflect.ValueOf(object.DynamicIndexFactsTop()))
	w.writeString("stable-shape")
	w.writeReflect(reflect.ValueOf(object.StableShape()))
	w.writeString("prefix-stable-shape")
	w.writeReflect(reflect.ValueOf(object.PrefixStableShape()))
}
