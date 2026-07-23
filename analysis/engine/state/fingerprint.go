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

// ErrUncanonicalizedAllocationTemplate reports that a lexical allocation
// template reached a semantic lattice boundary. Templates are relation-local
// syntax and must be substituted by their linked call-frame lens before any
// Join/Widen can observe them.
var ErrUncanonicalizedAllocationTemplate = errors.New("state: uncanonicalized allocation template")

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
	// Workspace reuses only structural key encodings and typed ordering
	// scratch across a serial solve. It never caches a semantic digest or an
	// equality decision; every fingerprint still traverses and hashes its
	// complete registered lane inventory.
	Workspace *FingerprintWorkspace
	// keyEncodings is shared only by one solve-scoped product fingerprint
	// session. Ordinary callers retain an isolated per-call cache.
	keyEncodings map[keyspace.Key]string
	// scratch is shared only by a serial, solve-scoped fingerprint session. It
	// changes allocation behavior, never ordering or the encoded byte stream.
	scratch *fingerprintScratch
}

// FingerprintWorkspace is a serial, solve-scoped allocation workspace bound
// to exactly one structural KeySpace. It is safe to retain across transaction
// rollback because its contents are pure encodings and scratch buffers, not
// published semantic results.
type FingerprintWorkspace struct {
	keys         *keyspace.KeySpace
	keyEncodings map[keyspace.Key]string
	scratch      fingerprintScratch
}

// NewFingerprintWorkspace seals one solve-local fingerprint workspace.
func NewFingerprintWorkspace(keys *keyspace.KeySpace) (*FingerprintWorkspace, error) {
	if keys == nil || !keys.Valid() {
		return nil, fmt.Errorf("%w: workspace requires a valid keyspace", ErrFingerprintKeySpace)
	}
	return &FingerprintWorkspace{keys: keys, keyEncodings: make(map[keyspace.Key]string)}, nil
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
	w := newFingerprintWriter(config)
	w.string("schema", "go-lua.state-semantic-fingerprint/v2")
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
	scratch      *fingerprintScratch
}

func newFingerprintWriter(config FingerprintConfig) *fingerprintWriter {
	keyEncodings := config.keyEncodings
	scratch := config.scratch
	workspaceErr := error(nil)
	if config.Workspace != nil {
		if config.KeySpace == nil || config.Workspace.keys != config.KeySpace {
			workspaceErr = fmt.Errorf("%w: workspace belongs to another keyspace", ErrFingerprintKeySpace)
		} else {
			keyEncodings = config.Workspace.keyEncodings
			scratch = &config.Workspace.scratch
		}
	}
	if keyEncodings == nil {
		keyEncodings = make(map[keyspace.Key]string)
	}
	if scratch == nil {
		scratch = &fingerprintScratch{}
	}
	out := &fingerprintWriter{
		h: internalhash.NewWriter(), ctx: config.Context, reg: config.Registry,
		keys: config.KeySpace, keyEncodings: keyEncodings, scratch: scratch,
	}
	out.errVal = workspaceErr
	return out
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

func (w *fingerprintWriter) identityTerm(label string, term identity.Term) {
	w.uint64(label+":term-kind", uint64(term.Kind()))
	if id, ok := term.Concrete(); ok {
		w.string(label+":kind", id.Kind)
		w.string(label+":site", id.Site)
		w.uint64(label+":index", id.Index)
		return
	}
	if formal, ok := term.Formal(); ok {
		owner := formal.Schema().Owner()
		w.string(label+":formal-owner", string(owner[:]))
		w.uint64(label+":formal-ordinal", uint64(formal.Schema().Ordinal()))
		w.uint64(label+":formal-vocabulary", uint64(formal.Vocabulary()))
		return
	}
	if allocation, ok := term.Allocation(); ok {
		owner := allocation.Owner()
		w.string(label+":allocation-owner", string(owner[:]))
		w.uint64(label+":allocation", uint64(allocation.AllocationOrdinal()))
		w.uint64(label+":allocation-object", uint64(allocation.ObjectOrdinal()))
	}
}

func (w *fingerprintWriter) pathKey(label string, key keyspace.Key) {
	w.string(label, keyspaceEncoding(w, key))
}

func (w *fingerprintWriter) product(label string, value product.Value) {
	w.uint64(label+":hash", product.Hash(w.reg, value))
	w.int64(label+":shape", int64(product.ShapeOf(value)))
}

// keyspaceEncoding is a lossless structural encoding of a key. All namespace
// interpretation, including private boundary roots, belongs to KeySpace's
// FreezeKey decoder. This consumer records only the solve-independent scalar
// snapshot and therefore cannot drift when KeySpace adds an internal kind.
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
	frozen, err := keyspace.FreezeKey(w.ctx, w.keys, key)
	if err != nil {
		w.errVal = fmt.Errorf("%w: %v", ErrFingerprintKeySpace, err)
		return ""
	}

	var encoded strings.Builder
	encodedSize := 128 + len(frozen.NamedRoot())
	for index := 0; index < frozen.SegmentLen(); index++ {
		segment := frozen.SegmentAt(index)
		encodedSize += 40 + len(segment.Name)
	}
	encoded.Grow(encodedSize)
	appendFingerprintUint(&encoded, uint64(frozen.Kind()))
	appendFingerprintUint(&encoded, uint64(frozen.Symbol()))
	appendFingerprintUint(&encoded, uint64(frozen.Version()))
	appendFingerprintUint(&encoded, uint64(frozen.RootIndex()))
	appendFingerprintField(&encoded, frozen.NamedRoot())
	appendFingerprintBool(&encoded, frozen.Canonical())
	appendFingerprintUint(&encoded, uint64(frozen.SegmentLen()))
	for index := 0; index < frozen.SegmentLen(); index++ {
		segment := frozen.SegmentAt(index)
		appendFingerprintUint(&encoded, uint64(segment.Kind))
		appendFingerprintField(&encoded, segment.Name)
		appendFingerprintInt(&encoded, int64(segment.Index))
	}
	out := encoded.String()
	w.keyEncodings[key] = out
	return out
}

func appendFingerprintUint(out *strings.Builder, value uint64) {
	appendFingerprintField(out, strconv.FormatUint(value, 10))
}

func appendFingerprintInt(out *strings.Builder, value int64) {
	appendFingerprintField(out, strconv.FormatInt(value, 10))
}

func appendFingerprintBool(out *strings.Builder, value bool) {
	if value {
		appendFingerprintField(out, "1")
		return
	}
	appendFingerprintField(out, "0")
}

func appendFingerprintField(out *strings.Builder, value string) {
	out.WriteString(strconv.Itoa(len(value)))
	out.WriteByte(':')
	out.WriteString(value)
	out.WriteByte(';')
}
