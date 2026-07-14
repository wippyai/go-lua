package state

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
)

// ErrFingerprintCoverage reports that a registered semantic lane has no
// deterministic fingerprint implementation. Reuse callers must fail closed on
// this error: silently omitting a lane aliases distinct solver inputs.
var ErrFingerprintCoverage = errors.New("state: incomplete semantic fingerprint coverage")

// ErrFingerprintKeySpace reports a key that cannot be rendered through the
// prepared body's structural keyspace.
var ErrFingerprintKeySpace = errors.New("state: semantic fingerprint keyspace mismatch")

// FingerprintConfig supplies the semantic environment needed to fingerprint a
// State. Nil Lanes selects the default inventory; a non-nil empty slice selects
// no lanes, matching DomainWithOptionalLanes.
type FingerprintConfig struct {
	Context  context.Context
	Registry *axis.Registry
	// KeySpace must be the run-local owner of every structural key in State.
	// Callers crossing analysis boundaries must rekey State before fingerprinting,
	// matching the solver's existing state/keyspace ownership precondition.
	KeySpace *keyspace.KeySpace
	Lanes    []LaneID
}

// SemanticFingerprint returns a deterministic digest of every selected State
// lane. Lane encoders are owned by the same laneSpec registrations as lattice
// equality, so inventory growth cannot silently bypass the reuse identity.
// State must already be rekeyed into config.KeySpace. Invalid/out-of-range
// keys fail closed, but dense IDs cannot prove provenance when two KeySpaces
// happen to assign the same IDs.
//
// TODO: add a run-local keyspace-owner token to State so same-ID foreign keys
// can be rejected directly instead of relying on the solver boundary contract.
func SemanticFingerprint(config FingerprintConfig, st State) (uint64, error) {
	if config.Registry == nil {
		return 0, fmt.Errorf("state: semantic fingerprint requires registry")
	}
	if config.Context != nil {
		if err := config.Context.Err(); err != nil {
			return 0, err
		}
	}
	specs := defaultLaneCatalog.specs
	if config.Lanes != nil {
		var err error
		specs, err = defaultLaneCatalog.selectSpecs(NewLaneSet(config.Lanes...))
		if err != nil {
			return 0, err
		}
	}
	var bits laneMask
	for _, spec := range specs {
		bits |= spec.bit
	}
	scope := scopedLaneMask(bits)
	if !st.canonical || !stateHasLaneMask(st, scope) {
		domain := domainFromLaneSpecs(config.Registry, specs, defaultLaneCatalog.specs)
		st = domain.Join(domain.Bottom(), st)
	}
	w := newFingerprintWriter(config)
	w.string("schema", "go-lua.state-semantic-fingerprint/v1")
	for _, spec := range specs {
		if spec.fingerprint == nil {
			return 0, fmt.Errorf("%w: lane %q", ErrFingerprintCoverage, spec.id)
		}
		w.string("lane", string(spec.id))
		spec.fingerprint(w, st)
		if err := w.err(); err != nil {
			return 0, err
		}
	}
	return w.sum64(), nil
}

// fingerprintWriter is the lane-facing canonical encoder. It deliberately
// exposes only framed deterministic primitives; lane implementations own their
// semantic traversal and ordering, while process-local map iteration and
// keyspace IDs never enter the digest.
type fingerprintWriter struct {
	h            internalhash.Writer
	ctx          context.Context
	reg          *axis.Registry
	keys         *keyspace.KeySpace
	keyEncodings map[keyspace.Key]string
	steps        uint64
	errVal       error
}

func newFingerprintWriter(config FingerprintConfig) *fingerprintWriter {
	return &fingerprintWriter{
		h:            internalhash.NewWriter(),
		ctx:          config.Context,
		reg:          config.Registry,
		keys:         config.KeySpace,
		keyEncodings: make(map[keyspace.Key]string),
	}
}

func (w *fingerprintWriter) sum64() uint64 { return w.h.Sum64() }

func (w *fingerprintWriter) err() error {
	if w.errVal != nil {
		return w.errVal
	}
	if w.ctx != nil {
		return w.ctx.Err()
	}
	return nil
}

func (w *fingerprintWriter) checkpoint() bool {
	if w.errVal != nil {
		return false
	}
	w.steps++
	if w.steps%64 != 0 || w.ctx == nil {
		return true
	}
	if err := w.ctx.Err(); err != nil {
		w.errVal = err
		return false
	}
	return true
}

func (w *fingerprintWriter) raw(value string) {
	if !w.checkpoint() {
		return
	}
	_, _ = w.h.WriteString(value)
}

func (w *fingerprintWriter) string(label, value string) {
	w.raw(label)
	w.raw(":s:")
	w.h.WriteIntDecimal(int64(len(value)))
	_ = w.h.WriteByte(':')
	w.raw(value)
	_ = w.h.WriteByte(';')
}

func (w *fingerprintWriter) bool(label string, value bool) {
	w.raw(label)
	w.raw(":b:")
	w.h.WriteBool(value)
	_ = w.h.WriteByte(';')
}

func (w *fingerprintWriter) int64(label string, value int64) {
	w.raw(label)
	w.raw(":i:")
	w.h.WriteIntDecimal(value)
	_ = w.h.WriteByte(';')
}

func (w *fingerprintWriter) uint64(label string, value uint64) {
	w.raw(label)
	w.raw(":u:")
	w.h.WriteUintDecimal(value)
	_ = w.h.WriteByte(';')
}

func (w *fingerprintWriter) identity(label string, id identity.ID) {
	w.string(label+":kind", id.Kind)
	w.string(label+":site", id.Site)
	w.uint64(label+":index", id.Index)
}

func (w *fingerprintWriter) pathKey(label string, key keyspace.Key) {
	w.string(label, keyspaceEncoding(w, key))
}

func (w *fingerprintWriter) product(label string, value product.Value) {
	w.uint64(label+":hash", product.Hash(w.reg, value))
	w.int64(label+":shape", int64(product.ShapeOf(value)))
}

// keyspaceEncoding is a lossless structural encoding of a key. Display path
// strings are deliberately unsuitable here: a field named "a.b" and the two
// fields "a", "b" have the same display spelling. Dense root/segment IDs are
// also unsuitable because they are local to one KeySpace.
func keyspaceEncoding(w *fingerprintWriter, key keyspace.Key) string {
	if encoded, ok := w.keyEncodings[key]; ok {
		return encoded
	}
	if !w.checkpoint() {
		return ""
	}
	if w.keys == nil {
		w.errVal = fmt.Errorf("%w: missing keyspace", ErrFingerprintKeySpace)
		return ""
	}
	if key.Kind == keyspace.KindInvalid {
		w.errVal = fmt.Errorf("%w: invalid key", ErrFingerprintKeySpace)
		return ""
	}
	segments, ok := w.keys.SegmentsView(key)
	if !ok {
		w.errVal = fmt.Errorf("%w: key does not decode in supplied keyspace", ErrFingerprintKeySpace)
		return ""
	}

	root := ""
	switch key.Kind {
	case keyspace.KindResolverSym:
		if key.Sym == 0 || key.Ver == 0 || key.Root != 0 || key.Canon {
			w.errVal = fmt.Errorf("%w: malformed resolver key", ErrFingerprintKeySpace)
			return ""
		}
	case keyspace.KindUnversionedSym, keyspace.KindStableSym:
		if key.Sym == 0 || key.Ver != 0 || key.Root != 0 || key.Canon {
			w.errVal = fmt.Errorf("%w: malformed symbol key", ErrFingerprintKeySpace)
			return ""
		}
	case keyspace.KindPlaceholder, keyspace.KindRetSlot:
		if key.Sym != 0 || key.Ver != 0 {
			w.errVal = fmt.Errorf("%w: malformed indexed-root key", ErrFingerprintKeySpace)
			return ""
		}
		root = strconv.FormatUint(uint64(key.Root), 10)
	case keyspace.KindNamed:
		if key.Sym != 0 || key.Ver != 0 || key.Root == 0 {
			w.errVal = fmt.Errorf("%w: malformed named-root key", ErrFingerprintKeySpace)
			return ""
		}
		path, pathOK := w.keys.StatePath(key)
		if !pathOK || path.Root == "" {
			w.errVal = fmt.Errorf("%w: named root does not decode", ErrFingerprintKeySpace)
			return ""
		}
		root = path.Root
	case keyspace.KindRootlessSuffix:
		if key.Sym != 0 || key.Ver != 0 || key.Root != 0 || key.Canon || len(segments) == 0 {
			w.errVal = fmt.Errorf("%w: malformed rootless key", ErrFingerprintKeySpace)
			return ""
		}
	default:
		w.errVal = fmt.Errorf("%w: unknown key kind %d", ErrFingerprintKeySpace, key.Kind)
		return ""
	}

	var encoded strings.Builder
	encodedSize := 96 + len(root)
	for _, segment := range segments {
		encodedSize += 40 + len(segment.Name)
	}
	encoded.Grow(encodedSize)
	appendFingerprintField(&encoded, strconv.FormatUint(uint64(key.Kind), 10))
	appendFingerprintField(&encoded, strconv.FormatUint(uint64(key.Sym), 10))
	appendFingerprintField(&encoded, strconv.FormatUint(uint64(key.Ver), 10))
	appendFingerprintField(&encoded, root)
	appendFingerprintField(&encoded, strconv.FormatBool(key.Canon))
	appendFingerprintField(&encoded, strconv.Itoa(len(segments)))
	for _, segment := range segments {
		appendFingerprintField(&encoded, strconv.FormatUint(uint64(segment.Kind), 10))
		appendFingerprintField(&encoded, segment.Name)
		appendFingerprintField(&encoded, strconv.Itoa(segment.Index))
	}
	out := encoded.String()
	w.keyEncodings[key] = out
	return out
}

func appendFingerprintField(out *strings.Builder, value string) {
	out.WriteString(strconv.Itoa(len(value)))
	out.WriteByte(':')
	out.WriteString(value)
	out.WriteByte(';')
}
