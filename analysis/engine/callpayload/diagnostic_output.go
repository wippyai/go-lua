package callpayload

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/factmap"
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

// DiagnosticOutput is the canonical recursive call-boundary diagnostic lane.
// It is a component of the same tuple fixpoint as the abstract State, not a
// separately evaluated summary. The obligation lanes are contravariant must
// constraints: keys are accumulated and colliding values meet, with a missing
// key denoting Top/no constraint. Exposures are covariant may facts and union.
//
// The containing State supplies reachability/bottom. Consequently the zero
// DiagnosticOutput is both the empty diagnostic payload and the payload of an
// unknown suspension certification; callers must not join it as an unreachable
// identity without first consulting the State component.
type DiagnosticOutput struct {
	SuspensionKnown bool
	MaySuspend      bool

	ParamObligations []CallParamObligation
	PathObligations  []CallPathObligation
	ParamExposures   []CallParamExposure
}

// DiagnosticOutputLattice is the one canonical recursive-diagnostic algebra.
// Reachability lives in the containing tuple's care set, so the reachable
// least element is the certified quiet value rather than Go's zero value.
// The carrier has no finitely representable greatest element (exposures are a
// may set), and its forward consumers require no Meet; those optional lattice
// operations are therefore deliberately nil.
func DiagnosticOutputLattice(reg *axis.Registry) lattice.Lattice[DiagnosticOutput] {
	return lattice.Lattice[DiagnosticOutput]{
		Bottom:   func() DiagnosticOutput { return DiagnosticOutput{SuspensionKnown: true} },
		Equal:    func(left, right DiagnosticOutput) bool { return left.Equal(reg, right) },
		Same:     func(left, right DiagnosticOutput) bool { return left.RepresentationEqual(reg, right) },
		LessOrEq: func(left, right DiagnosticOutput) bool { return left.LessOrEq(reg, right) },
		Join:     func(left, right DiagnosticOutput) DiagnosticOutput { return left.Join(reg, right) },
		Widen:    func(left, right DiagnosticOutput) DiagnosticOutput { return left.Join(reg, right) },
		Narrow: func(left, right DiagnosticOutput) DiagnosticOutput {
			return right.Normalize(reg)
		},
	}
}

// CertifiedIdentity reports the reachable diagnostic identity. It is not the
// zero value: zero carries unknown suspension certification and may only be
// treated as unreachable identity by the containing State lattice.
func (d DiagnosticOutput) CertifiedIdentity() bool {
	return d.SuspensionKnown && !d.MaySuspend && d.noFacts()
}

func (d DiagnosticOutput) noFacts() bool {
	return len(d.ParamObligations) == 0 && len(d.PathObligations) == 0 && len(d.ParamExposures) == 0
}

// DiagnosticOutputFromCallOutcome extracts and canonicalizes the descriptor-
// owned recursive diagnostic fields of outcome.
func DiagnosticOutputFromCallOutcome(reg *axis.Registry, outcome CallOutcome) DiagnosticOutput {
	return (DiagnosticOutput{
		SuspensionKnown:  outcome.SuspensionKnown,
		MaySuspend:       outcome.MaySuspend,
		ParamObligations: outcome.ParamObligations,
		PathObligations:  outcome.PathObligations,
		ParamExposures:   outcome.ParamExposures,
	}).Normalize(reg)
}

// ApplyTo replaces the descriptor-owned recursive diagnostic fields in
// outcome with detached canonical storage.
func (d DiagnosticOutput) ApplyTo(reg *axis.Registry, outcome *CallOutcome) {
	if outcome == nil {
		return
	}
	d = d.Normalize(reg)
	outcome.SuspensionKnown = d.SuspensionKnown
	outcome.MaySuspend = d.MaySuspend
	outcome.ParamObligations = publishParamObligations(reg, d.ParamObligations)
	outcome.PathObligations = publishPathObligations(reg, d.PathObligations)
	outcome.ParamExposures = d.ParamExposures
}

// Empty reports whether d carries no certified suspension or diagnostic fact.
func (d DiagnosticOutput) Empty() bool {
	return !d.SuspensionKnown && !d.MaySuspend && len(d.ParamObligations) == 0 &&
		len(d.PathObligations) == 0 && len(d.ParamExposures) == 0
}

// Clone returns detached immutable-publication storage.
func (d DiagnosticOutput) Clone() DiagnosticOutput {
	out := d
	out.ParamObligations = append([]CallParamObligation(nil), d.ParamObligations...)
	out.PathObligations = clonePathObligations(d.PathObligations)
	out.ParamExposures = cloneParamExposures(d.ParamExposures)
	return out
}

// Normalize returns d in deterministic, deduplicated canonical form.
func (d DiagnosticOutput) Normalize(reg *axis.Registry) DiagnosticOutput {
	if reg == nil {
		return DiagnosticOutput{}
	}
	if d.noFacts() {
		return d
	}
	out := DiagnosticOutput{SuspensionKnown: d.SuspensionKnown, MaySuspend: d.MaySuspend}
	out.ParamObligations = normalizeParamObligations(reg, d.ParamObligations)
	out.PathObligations = normalizePathObligations(reg, d.PathObligations)
	out.ParamExposures = normalizeParamExposures(reg, d.ParamExposures)
	return out
}

// Valid reports whether d is a well-formed payload in reg's value universe.
// Validity is independent of canonical ordering; Normalize establishes the
// unique spelling retained by the fixpoint.
func (d DiagnosticOutput) Valid(reg *axis.Registry) bool {
	if reg == nil {
		return false
	}
	for _, obligation := range d.ParamObligations {
		if obligation.ParamIndex < 0 || !product.BelongsToRegistry(reg, obligation.Value) ||
			!validParamObligationOrigin(obligation.Origin) {
			return false
		}
	}
	for _, obligation := range d.PathObligations {
		if obligation.Path.IsEmpty() || !product.BelongsToRegistry(reg, obligation.Value) {
			return false
		}
	}
	for _, exposure := range d.ParamExposures {
		if exposure.Source.IsEmpty() || !validExposureKind(exposure.Kind) ||
			!product.BelongsToRegistry(reg, exposure.Contract) {
			return false
		}
	}
	return true
}

// Equal reports semantic equality after canonicalization.
func (d DiagnosticOutput) Equal(reg *axis.Registry, other DiagnosticOutput) bool {
	if d.SuspensionKnown != other.SuspensionKnown || d.MaySuspend != other.MaySuspend {
		return false
	}
	if d.noFacts() && other.noFacts() {
		return true
	}
	return paramObligationMap(reg).Equal(d.ParamObligations, other.ParamObligations) &&
		pathObligationMap(reg).Equal(d.PathObligations, other.PathObligations) &&
		paramExposureSet(reg).Equal(d.ParamExposures, other.ParamExposures)
}

// RepresentationEqual proves exact reusable immutable representation after
// canonical ordering. Semantic equality alone is insufficient because two
// product values may be lattice-equal while retaining different spellings.
func (d DiagnosticOutput) RepresentationEqual(reg *axis.Registry, other DiagnosticOutput) bool {
	if reg == nil {
		return false
	}
	d, other = d.Normalize(reg), other.Normalize(reg)
	if !d.Equal(reg, other) {
		return false
	}
	domain := product.Domain(reg)
	for index := range d.ParamObligations {
		if !domain.Same(d.ParamObligations[index].Value, other.ParamObligations[index].Value) {
			return false
		}
	}
	for index := range d.PathObligations {
		if !domain.Same(d.PathObligations[index].Value, other.PathObligations[index].Value) {
			return false
		}
	}
	for index := range d.ParamExposures {
		if !domain.Same(d.ParamExposures[index].Contract, other.ParamExposures[index].Contract) {
			return false
		}
	}
	return true
}

// LessOrEq reports the canonical diagnostic order. Obligation values are
// reversed because a narrower requirement is stronger; exposure membership and
// MaySuspend are ordinary may facts, while certification is reversed because
// losing a certificate is conservative.
func (d DiagnosticOutput) LessOrEq(reg *axis.Registry, other DiagnosticOutput) bool {
	if d.CertifiedIdentity() {
		return true
	}
	if d.SuspensionKnown && !other.SuspensionKnown {
		// A certified value is below an unknown value.
	} else if !d.SuspensionKnown && other.SuspensionKnown {
		return false
	}
	if d.MaySuspend && !other.MaySuspend {
		return false
	}
	if d.noFacts() && other.noFacts() {
		return true
	}
	if !paramObligationMap(reg).LessOrEq(d.ParamObligations, other.ParamObligations) {
		return false
	}
	if !pathObligationMap(reg).LessOrEq(d.PathObligations, other.PathObligations) {
		return false
	}
	return paramExposureSet(reg).LessOrEq(d.ParamExposures, other.ParamExposures)
}

// Join returns the least upper bound for two reachable diagnostic tuples. The
// containing State lattice, not this method, handles unreachable operands.
func (d DiagnosticOutput) Join(reg *axis.Registry, other DiagnosticOutput) DiagnosticOutput {
	if d.CertifiedIdentity() {
		return other.Normalize(reg)
	}
	if other.CertifiedIdentity() {
		return d.Normalize(reg)
	}
	if d.noFacts() && other.noFacts() {
		return DiagnosticOutput{
			SuspensionKnown: d.SuspensionKnown && other.SuspensionKnown,
			MaySuspend:      d.MaySuspend || other.MaySuspend,
		}
	}
	return DiagnosticOutput{
		SuspensionKnown:  d.SuspensionKnown && other.SuspensionKnown,
		MaySuspend:       d.MaySuspend || other.MaySuspend,
		ParamObligations: joinParamObligations(reg, d.ParamObligations, other.ParamObligations),
		PathObligations:  joinPathObligations(reg, d.PathObligations, other.PathObligations),
		ParamExposures:   paramExposureSet(reg).Join(d.ParamExposures, other.ParamExposures),
	}
}

// Fingerprint returns a deterministic semantic fingerprint in existing
// CallOutcome descriptor order. It is an accelerator, not equality authority.
func (d DiagnosticOutput) Fingerprint(reg *axis.Registry) uint64 {
	d = d.Normalize(reg)
	if d.noFacts() {
		// Four exact no-fact spellings. These constants are stable
		// accelerators; structural equality remains bucket authority.
		fingerprint := uint64(0x6b9df6f4f2b79a31)
		if d.SuspensionKnown {
			fingerprint ^= 0x9e3779b97f4a7c15
		}
		if d.MaySuspend {
			fingerprint ^= 0xc2b2ae3d27d4eb4f
		}
		return fingerprint
	}
	w := internalhash.NewWriter()
	_, _ = w.WriteString("callpayload.DiagnosticOutput/v1")
	for _, role := range diagnosticOutputFieldRoles {
		_, _ = w.WriteString(role.FieldName)
		_ = w.WriteByte(0)
		switch role.FieldName {
		case "SuspensionKnown":
			w.WriteBool(d.SuspensionKnown)
		case "MaySuspend":
			w.WriteBool(d.MaySuspend)
		case "ParamObligations":
			fingerprintParamObligations(&w, d.ParamObligations)
		case "PathObligations":
			fingerprintPathObligations(&w, d.PathObligations)
		case "ParamExposures":
			fingerprintParamExposures(&w, d.ParamExposures)
		}
	}
	return w.Sum64()
}

// DiagnosticOutputFieldRoles returns the recursive diagnostic fields in the
// canonical CallOutcome descriptor order. The returned slice is a copy.
func DiagnosticOutputFieldRoles() []CallOutcomeFieldRole {
	out := make([]CallOutcomeFieldRole, len(diagnosticOutputFieldRoles))
	copy(out, diagnosticOutputFieldRoles)
	return out
}

var diagnosticOutputFieldRoles = deriveDiagnosticOutputFieldRoles()

func deriveDiagnosticOutputFieldRoles() []CallOutcomeFieldRole {
	wanted := map[string]struct{}{
		"SuspensionKnown": {}, "MaySuspend": {}, "ParamObligations": {},
		"PathObligations": {}, "ParamExposures": {},
	}
	roles := CallOutcomeFieldRoles()
	out := make([]CallOutcomeFieldRole, 0, len(wanted))
	for _, role := range roles {
		if _, ok := wanted[role.FieldName]; ok {
			out = append(out, role)
			delete(wanted, role.FieldName)
		}
	}
	if len(wanted) != 0 {
		panic("callpayload: diagnostic output field has no CallOutcome descriptor")
	}
	return out
}

type paramObligationKey struct {
	Index            int
	Origin           CallParamObligationOrigin
	SignatureSurface bool
}

type paramObligationFactMap = factmap.Map[paramObligationKey, CallParamObligation, product.Value]
type pathObligationFactMap = factmap.Map[pathdom.PathKey, CallPathObligation, product.Value]
type paramExposureFactSet = factset.Set[paramExposureKey, CallParamExposure]

var paramObligationFactMaps registrycache.Cache[paramObligationFactMap]
var pathObligationFactMaps registrycache.Cache[pathObligationFactMap]
var paramExposureFactSets registrycache.Cache[paramExposureFactSet]

type paramExposureKey struct {
	Path     pathdom.PathKey
	Kind     factflow.CovariantExposureKind
	Contract product.Value
}

func paramObligationKeyOf(o CallParamObligation) paramObligationKey {
	return paramObligationKey{Index: o.ParamIndex, Origin: o.Origin, SignatureSurface: o.SignatureSurface}
}

func normalizeParamObligations(reg *axis.Registry, in []CallParamObligation) []CallParamObligation {
	return paramObligationMap(reg).Normalize(in)
}

func joinParamObligations(reg *axis.Registry, left, right []CallParamObligation) []CallParamObligation {
	return paramObligationMap(reg).Join(left, right)
}

func paramObligationMap(reg *axis.Registry) paramObligationFactMap {
	return paramObligationFactMaps.GetFor(reg, newParamObligationMap)
}

func newParamObligationMap(reg *axis.Registry) paramObligationFactMap {
	return paramObligationFactMap{
		Key:   paramObligationKeyOf,
		Value: func(f CallParamObligation) product.Value { return f.Value },
		WithValue: func(f CallParamObligation, value product.Value) CallParamObligation {
			f.Value = value
			return f
		},
		Less: paramObligationLess,
		Valid: func(f CallParamObligation) bool {
			return f.ParamIndex >= 0 && validParamObligationOrigin(f.Origin) && product.BelongsToRegistry(reg, f.Value)
		},
		Domain: obligationValueLattice(reg),
	}
}

func paramObligationLess(a, b CallParamObligation) bool {
	if a.ParamIndex != b.ParamIndex {
		return a.ParamIndex < b.ParamIndex
	}
	if a.SignatureSurface != b.SignatureSurface {
		return !a.SignatureSurface
	}
	return paramOriginLess(a.Origin, b.Origin)
}

func paramOriginLess(a, b CallParamObligationOrigin) bool {
	if a.HasOrigin != b.HasOrigin {
		return !a.HasOrigin
	}
	if a.ReceiverParam != b.ReceiverParam {
		return a.ReceiverParam < b.ReceiverParam
	}
	if a.ReceiverPath != b.ReceiverPath {
		return a.ReceiverPath < b.ReceiverPath
	}
	if a.Member != b.Member {
		return segmentLess(a.Member, b.Member)
	}
	if a.ArgParam != b.ArgParam {
		return a.ArgParam < b.ArgParam
	}
	if a.MemberParamIndex != b.MemberParamIndex {
		return a.MemberParamIndex < b.MemberParamIndex
	}
	if a.SubjectLabel != b.SubjectLabel {
		return a.SubjectLabel < b.SubjectLabel
	}
	return a.ProviderLabel < b.ProviderLabel
}

func validParamObligationOrigin(origin CallParamObligationOrigin) bool {
	if !origin.HasOrigin {
		return true
	}
	return origin.ReceiverParam >= 0 && origin.ArgParam >= 0 && origin.MemberParamIndex >= 0 && validSegment(origin.Member)
}

func normalizePathObligations(reg *axis.Registry, in []CallPathObligation) []CallPathObligation {
	return pathObligationMap(reg).Normalize(in)
}

func joinPathObligations(reg *axis.Registry, left, right []CallPathObligation) []CallPathObligation {
	return pathObligationMap(reg).Join(left, right)
}

func pathObligationMap(reg *axis.Registry) pathObligationFactMap {
	return pathObligationFactMaps.GetFor(reg, newPathObligationMap)
}

func newPathObligationMap(reg *axis.Registry) pathObligationFactMap {
	return pathObligationFactMap{
		Key:   func(f CallPathObligation) pathdom.PathKey { return f.Path.Key() },
		Value: func(f CallPathObligation) product.Value { return f.Value },
		WithValue: func(f CallPathObligation, value product.Value) CallPathObligation {
			f.Value = value
			return f
		},
		Less:  func(a, b CallPathObligation) bool { return a.Path.Less(b.Path) },
		Valid: func(f CallPathObligation) bool { return !f.Path.IsEmpty() && product.BelongsToRegistry(reg, f.Value) },
		CloneFact: func(f CallPathObligation) CallPathObligation {
			f.Path = f.Path.Clone()
			return f
		},
		Domain: obligationValueLattice(reg),
	}
}

func obligationValueLattice(reg *axis.Registry) lattice.Lattice[product.Value] {
	return lattice.Lattice[product.Value]{
		Bottom: func() product.Value { return product.Top() },
		Top:    func() product.Value { return product.Bottom(reg) },
		Equal:  func(a, b product.Value) bool { return product.Equal(reg, a, b) },
		Same:   func(a, b product.Value) bool { return a == b },
		LessOrEq: func(a, b product.Value) bool {
			return product.LessOrEq(reg, b, a)
		},
		Join:  func(a, b product.Value) product.Value { return product.Meet(reg, a, b) },
		Meet:  func(a, b product.Value) product.Value { return product.Join(reg, a, b) },
		Widen: func(a, b product.Value) product.Value { return product.Meet(reg, a, b) },
	}
}

func normalizeParamExposures(reg *axis.Registry, in []CallParamExposure) []CallParamExposure {
	return paramExposureSet(reg).Normalize(in)
}

func paramExposureSet(reg *axis.Registry) paramExposureFactSet {
	return paramExposureFactSets.GetFor(reg, newParamExposureSet)
}

func newParamExposureSet(reg *axis.Registry) paramExposureFactSet {
	return paramExposureFactSet{
		Key: func(f CallParamExposure) paramExposureKey {
			return paramExposureKey{Path: f.Source.Key(), Kind: f.Kind, Contract: f.Contract}
		},
		EqualFact: func(a, b CallParamExposure) bool { return paramExposureEqual(reg, a, b) },
		Less:      paramExposureLess,
		Valid: func(f CallParamExposure) bool {
			return !f.Source.IsEmpty() && validExposureKind(f.Kind) && product.BelongsToRegistry(reg, f.Contract)
		},
		CloneFact: func(f CallParamExposure) CallParamExposure {
			f.Source = f.Source.Clone()
			return f
		},
	}
}

func paramExposureEqual(reg *axis.Registry, a, b CallParamExposure) bool {
	return a.Source.Equal(b.Source) && a.Kind == b.Kind && product.Equal(reg, a.Contract, b.Contract)
}

func paramExposureLess(a, b CallParamExposure) bool {
	if !a.Source.Equal(b.Source) {
		return a.Source.Less(b.Source)
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return product.CanonicalHash(a.Contract) < product.CanonicalHash(b.Contract)
}

func clonePathObligations(in []CallPathObligation) []CallPathObligation {
	out := append([]CallPathObligation(nil), in...)
	for i := range out {
		out[i].Path = out[i].Path.Clone()
	}
	return out
}

func cloneParamExposures(in []CallParamExposure) []CallParamExposure {
	out := append([]CallParamExposure(nil), in...)
	for i := range out {
		out[i].Source = out[i].Source.Clone()
	}
	return out
}

func publishParamObligations(reg *axis.Registry, in []CallParamObligation) []CallParamObligation {
	out := make([]CallParamObligation, 0, len(in))
	for _, fact := range in {
		if usefulPublishedObligation(reg, fact.Value) {
			out = append(out, fact)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func publishPathObligations(reg *axis.Registry, in []CallPathObligation) []CallPathObligation {
	out := make([]CallPathObligation, 0, len(in))
	for _, fact := range in {
		if usefulPublishedObligation(reg, fact.Value) {
			fact.Path = fact.Path.Clone()
			out = append(out, fact)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func usefulPublishedObligation(reg *axis.Registry, value product.Value) bool {
	return reg != nil && product.BelongsToRegistry(reg, value) &&
		!product.Equal(reg, value, product.Top()) &&
		!product.Equal(reg, value, product.Bottom(reg))
}

func fingerprintParamObligations(w *internalhash.Writer, in []CallParamObligation) {
	for _, fact := range in {
		w.WriteIntDecimal(int64(fact.ParamIndex))
		w.WriteBool(fact.SignatureSurface)
		fingerprintOrigin(w, fact.Origin)
		w.WriteUintHex(product.CanonicalHash(fact.Value))
		_ = w.WriteByte(0xff)
	}
}

func fingerprintOrigin(w *internalhash.Writer, o CallParamObligationOrigin) {
	w.WriteBool(o.HasOrigin)
	w.WriteIntDecimal(int64(o.ReceiverParam))
	_, _ = w.WriteString(string(o.ReceiverPath))
	w.WriteUintDecimal(uint64(o.Member.Kind))
	_, _ = w.WriteString(o.Member.Name)
	w.WriteIntDecimal(int64(o.Member.Index))
	w.WriteIntDecimal(int64(o.ArgParam))
	w.WriteIntDecimal(int64(o.MemberParamIndex))
	_, _ = w.WriteString(o.SubjectLabel)
	_ = w.WriteByte(0)
	_, _ = w.WriteString(o.ProviderLabel)
}

func fingerprintPathObligations(w *internalhash.Writer, in []CallPathObligation) {
	for _, fact := range in {
		_, _ = w.WriteString(string(fact.Path.Key()))
		w.WriteUintHex(product.CanonicalHash(fact.Value))
		_ = w.WriteByte(0xff)
	}
}

func fingerprintParamExposures(w *internalhash.Writer, in []CallParamExposure) {
	for _, fact := range in {
		_, _ = w.WriteString(string(fact.Source.Key()))
		w.WriteUintDecimal(uint64(fact.Kind))
		w.WriteUintHex(product.CanonicalHash(fact.Contract))
		_ = w.WriteByte(0xff)
	}
}

func validExposureKind(kind factflow.CovariantExposureKind) bool {
	return kind == factflow.CovariantExposureRecord || kind == factflow.CovariantExposureArray
}

func validSegment(seg segment.Segment) bool {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name != ""
	case segment.SegmentIndexInt:
		return true
	default:
		return false
	}
}

func segmentLess(a, b segment.Segment) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.Index < b.Index
}
