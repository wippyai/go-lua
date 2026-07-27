package shapefact

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const (
	scalarPrefix         = "scalar/"
	scalarBoolPrefix     = "scalar/bool/"
	scalarNumberPrefix   = "scalar/number/"
	scalarStringPrefix   = "scalar/string/"
	scalarFunctionPrefix = "scalar/function/"
	scalarClaimPrefix    = "scalar/claim/"
	memberMissingPrefix  = "shape/member-missing/v1/"
)

// Canonical fixed payload spellings are exported for consumers that compare
// or transport an already-decoded sentinel. Parameterized spellings must be
// built with ScalarValue or ClaimValue so no other package owns wire assembly.
const (
	ScalarTopWire                   = "scalar/top"
	ScalarNilWire                   = "scalar/nil"
	ScalarBooleanWire               = "scalar/boolean"
	ScalarTrueWire                  = "scalar/bool/true"
	ScalarFalseWire                 = "scalar/bool/false"
	ScalarTableWire                 = "scalar/table"
	ScalarFunctionWire              = "scalar/function"
	ScalarOptionalNilComparisonWire = "scalar/bool/optional-nil-comparison"
	ScalarExternalCallbackAnyWire   = "scalar/external-callback-any"
)

var (
	scalarTrueValue  = []byte(ScalarTrueWire)
	scalarFalseValue = []byte(ScalarFalseWire)
)

// PayloadForm is the closed top-level value-payload dialect. A form is
// accepted only when its envelope belongs to the declared vocabulary;
// kind-specific semantic projections fail closed on malformed content.
type PayloadForm uint8

const (
	PayloadInvalid PayloadForm = iota
	PayloadScalar
	PayloadShapeTable
	PayloadShapeTarget
	PayloadClaim
	PayloadMemberMissing
)

// ScalarKind is the declared scalar and sentinel vocabulary carried in value
// payloads. Identity-bearing sentinels retain their suffix in Scalar.Data.
type ScalarKind uint8

const (
	ScalarInvalid ScalarKind = iota
	ScalarTop
	ScalarNil
	ScalarBoolean
	ScalarBool
	ScalarNumber
	ScalarString
	ScalarTable
	ScalarFunction
	ScalarOptionalNilComparison
	ScalarExternalCallbackAny
	ScalarChannel
	ScalarChannelEntry
	ScalarChannelSummary
	ScalarDeclaration
	ScalarProvider
	ScalarResource
)

// Scalar is one scalar form. Data aliases the input and contains only the
// kind-specific suffix: a number spelling, quoted string, precise function
// envelope, or sentinel identity.
type Scalar struct {
	Kind ScalarKind
	Data []byte
	Bool bool
}

type scalarForm struct {
	kind        ScalarKind
	wire        string
	dataBearing bool
	emptyWire   string
}

var scalarForms = [...]scalarForm{
	{ScalarTop, ScalarTopWire, false, ScalarTopWire},
	{ScalarNil, ScalarNilWire, false, ScalarNilWire},
	{ScalarBoolean, ScalarBooleanWire, false, ScalarBooleanWire},
	{ScalarNumber, scalarNumberPrefix, true, ""},
	{ScalarString, scalarStringPrefix, true, ""},
	{ScalarTable, ScalarTableWire, false, ScalarTableWire},
	{ScalarFunction, scalarFunctionPrefix, true, ScalarFunctionWire},
	{ScalarOptionalNilComparison, ScalarOptionalNilComparisonWire, false, ScalarOptionalNilComparisonWire},
	{ScalarExternalCallbackAny, ScalarExternalCallbackAnyWire, false, ScalarExternalCallbackAnyWire},
	{ScalarChannel, "scalar/channel/", true, ""},
	{ScalarChannelEntry, "scalar/channel-entry/", true, ""},
	{ScalarChannelSummary, "scalar/channel-summary/", true, ""},
	{ScalarDeclaration, "scalar/declaration/", true, ""},
	{ScalarProvider, "scalar/provider/", true, ""},
	{ScalarResource, "scalar/resource/", true, ""},
}

// BooleanText returns the suffix of the scalar/bool/ dialect. Besides exact
// booleans, that dialect contains the optional-nil comparison sentinel.
func (scalar Scalar) BooleanText() (string, bool) {
	switch scalar.Kind {
	case ScalarBool:
		return strconv.FormatBool(scalar.Bool), true
	case ScalarOptionalNilComparison:
		return "optional-nil-comparison", true
	default:
		return "", false
	}
}

// Claim is a value refinement. Target aliases the complete target spelling
// following claim-kind/N/, including claim-type/ when the front published it.
type Claim struct {
	Kind   wir.ClaimKind
	Target []byte
}

// Payload is the typed sum for every fact value owned by this codec.
// encoded aliases the immutable input and exists solely to preserve its exact
// wire spelling when Encode is used for a decode/encode round trip.
type Payload struct {
	Form    PayloadForm
	Scalar  Scalar
	Table   Table
	Target  typ.Type
	Claim   Claim
	encoded []byte
}

// Decode recognizes exactly one declared value-payload form. Unknown scalar
// and shape prefixes fail closed instead of being treated as future variants.
func Decode(value []byte) (Payload, bool) {
	switch {
	case bytes.HasPrefix(value, []byte(tablePrefix)):
		table, ok := DecodeTable(value)
		if !ok {
			return Payload{}, false
		}
		return Payload{Form: PayloadShapeTable, Table: table, encoded: value}, true
	case bytes.HasPrefix(value, []byte(targetPrefix)):
		target, ok := DecodeTarget(value)
		if !ok {
			return Payload{}, false
		}
		return Payload{Form: PayloadShapeTarget, Target: target, encoded: value}, true
	case bytes.HasPrefix(value, []byte(memberMissingPrefix)):
		target, ok := DecodeTarget(value[len(memberMissingPrefix):])
		if !ok {
			return Payload{}, false
		}
		return Payload{Form: PayloadMemberMissing, Target: target, encoded: value}, true
	case bytes.HasPrefix(value, []byte(scalarClaimPrefix)):
		return decodeClaim(value)
	default:
		scalar, ok := decodeScalar(value)
		if !ok {
			return Payload{}, false
		}
		return Payload{Form: PayloadScalar, Scalar: scalar, encoded: value}, true
	}
}

func decodeClaim(value []byte) (Payload, bool) {
	rest := value[len(scalarClaimPrefix):]
	const claimKindPrefix = "claim-kind/"
	if !bytes.HasPrefix(rest, []byte(claimKindPrefix)) || len(rest) <= len(claimKindPrefix)+2 {
		return Payload{}, false
	}
	digit := rest[len(claimKindPrefix)]
	if digit < '1' || digit > '4' || rest[len(claimKindPrefix)+1] != '/' {
		return Payload{}, false
	}
	target := rest[len(claimKindPrefix)+2:]
	if len(target) == 0 {
		return Payload{}, false
	}
	return Payload{
		Form:    PayloadClaim,
		Claim:   Claim{Kind: wir.ClaimKind(digit - '0'), Target: target},
		encoded: value,
	}, true
}

func decodeScalar(value []byte) (Scalar, bool) {
	if bytes.Equal(value, scalarTrueValue) {
		return Scalar{Kind: ScalarBool, Bool: true}, true
	}
	if bytes.Equal(value, scalarFalseValue) {
		return Scalar{Kind: ScalarBool}, true
	}
	for _, form := range scalarForms {
		if form.emptyWire != "" && bytes.Equal(value, []byte(form.emptyWire)) {
			return Scalar{Kind: form.kind}, true
		}
		if form.dataBearing && len(value) > len(form.wire) && bytes.HasPrefix(value, []byte(form.wire)) {
			return Scalar{Kind: form.kind, Data: value[len(form.wire):]}, true
		}
	}
	return Scalar{}, false
}

// Encode returns the canonical wire form. A decoded payload preserves its
// original bytes exactly; a constructed payload is checked by decoding the
// generated spelling before it is returned.
func Encode(payload Payload) ([]byte, bool) {
	if len(payload.encoded) != 0 {
		decoded, ok := Decode(payload.encoded)
		if !ok || decoded.Form != payload.Form {
			return nil, false
		}
		return append([]byte(nil), payload.encoded...), true
	}
	var encoded []byte
	switch payload.Form {
	case PayloadShapeTable:
		return EncodeTable(payload.Table)
	case PayloadShapeTarget:
		return EncodeTarget(payload.Target)
	case PayloadMemberMissing:
		target, ok := EncodeTarget(payload.Target)
		if !ok {
			return nil, false
		}
		encoded = append([]byte(memberMissingPrefix), target...)
	case PayloadClaim:
		if payload.Claim.Kind < wir.ClaimCast || payload.Claim.Kind > wir.ClaimAssertsPredicate || len(payload.Claim.Target) == 0 {
			return nil, false
		}
		encoded = append(encoded, scalarClaimPrefix...)
		encoded = append(encoded, "claim-kind/"...)
		encoded = append(encoded, byte('0'+payload.Claim.Kind), '/')
		encoded = append(encoded, payload.Claim.Target...)
	case PayloadScalar:
		encoded = encodeScalar(payload.Scalar)
	default:
		return nil, false
	}
	if len(encoded) == 0 {
		return nil, false
	}
	_, ok := Decode(encoded)
	return encoded, ok
}

func encodeScalar(scalar Scalar) []byte {
	if scalar.Kind == ScalarBool {
		return []byte("scalar/bool/" + strconv.FormatBool(scalar.Bool))
	}
	form, ok := scalarFormForKind(scalar.Kind)
	if !ok || len(scalar.Data) == 0 && form.emptyWire == "" {
		return nil
	}
	if !form.dataBearing || len(scalar.Data) == 0 {
		return []byte(form.emptyWire)
	}
	return append([]byte(form.wire), scalar.Data...)
}

func scalarFormForKind(kind ScalarKind) (scalarForm, bool) {
	for _, form := range scalarForms {
		if form.kind == kind {
			return form, true
		}
	}
	return scalarForm{}, false
}

// ScalarValue is the sole constructor for a parameterized scalar wire value.
// Invalid scalar data fails closed as nil rather than emitting an undeclared
// payload spelling.
func ScalarValue(scalar Scalar) []byte {
	encoded, ok := Encode(Payload{Form: PayloadScalar, Scalar: scalar})
	if !ok {
		return nil
	}
	return encoded
}

// ScalarValueString is the string projection for string-valued carrier fields.
// Empty means the scalar was not encodable.
func ScalarValueString(scalar Scalar) string {
	return string(ScalarValue(scalar))
}

// BooleanValue returns an immutable canonical boolean payload. The shared
// bytes follow the same immutable-input contract as decoded Payload data and
// keep branch publication allocation-free.
func BooleanValue(value bool) []byte {
	if value {
		return scalarTrueValue
	}
	return scalarFalseValue
}

func BooleanValueString(value bool) string {
	if value {
		return ScalarTrueWire
	}
	return ScalarFalseWire
}

// ScalarTextValue constructs a data-bearing scalar directly from its textual
// suffix. Kinds without a textual suffix, and empty required suffixes, fail
// closed. The string form serves carrier fields without a []byte round trip.
func ScalarTextValue(kind ScalarKind, data string) []byte {
	value := ScalarTextValueString(kind, data)
	if value == "" {
		return nil
	}
	return []byte(value)
}

func ScalarTextValueString(kind ScalarKind, data string) string {
	form, ok := scalarFormForKind(kind)
	if !ok || !form.dataBearing || data == "" && form.emptyWire == "" {
		return ""
	}
	if data == "" {
		return form.emptyWire
	}
	return form.wire + data
}

// BooleanTextValue constructs the exact boolean scalar carried by branch
// protocols. Any text other than "true" or "false" fails closed.
func BooleanTextValue(text string) []byte {
	switch text {
	case "true":
		return scalarTrueValue
	case "false":
		return scalarFalseValue
	default:
		return nil
	}
}

// ClaimValue is the sole constructor for a claim wire value. Invalid claim
// kinds or empty targets fail closed as nil.
func ClaimValue(claim Claim) []byte {
	if claim.Kind < wir.ClaimCast || claim.Kind > wir.ClaimAssertsPredicate || len(claim.Target) == 0 {
		return nil
	}
	encoded := make([]byte, 0, len(scalarClaimPrefix)+len("claim-kind/N/")+len(claim.Target))
	encoded = append(encoded, scalarClaimPrefix...)
	encoded = append(encoded, "claim-kind/"...)
	encoded = append(encoded, byte('0'+claim.Kind), '/')
	encoded = append(encoded, claim.Target...)
	return encoded
}

// IsScalar reports whether value is any declared scalar, claim, or scalar
// sentinel form. Shape payloads are deliberately excluded.
func IsScalar(value []byte) bool {
	if !bytes.HasPrefix(value, []byte(scalarPrefix)) {
		return false
	}
	payload, ok := Decode(value)
	return ok && (payload.Form == PayloadScalar || payload.Form == PayloadClaim)
}

func DecodeScalar(value []byte) (Scalar, bool) {
	payload, ok := Decode(value)
	return payload.Scalar, ok && payload.Form == PayloadScalar
}

func IsScalarKind(value []byte, scalarKind ScalarKind) bool {
	scalar, ok := DecodeScalar(value)
	return ok && scalar.Kind == scalarKind
}

func DecodeScalarKind(value []byte, scalarKind ScalarKind) (Scalar, bool) {
	scalar, ok := DecodeScalar(value)
	return scalar, ok && scalar.Kind == scalarKind
}

func IsForm(value []byte, form PayloadForm) bool {
	payload, ok := Decode(value)
	return ok && payload.Form == form
}

func DecodeClaim(value []byte) (Claim, bool) {
	payload, ok := Decode(value)
	return payload.Claim, ok && payload.Form == PayloadClaim
}

// WitnessType returns the type whose value set this payload witnesses. Top,
// claims, abstract table/function sentinels, and table shapes carry no such
// scalar witness.
func (payload Payload) WitnessType() (typ.Type, bool) {
	if payload.Form == PayloadShapeTarget && payload.Target != nil {
		return payload.Target, true
	}
	if payload.Form != PayloadScalar {
		return nil, false
	}
	switch payload.Scalar.Kind {
	case ScalarNil:
		return typ.Nil, true
	case ScalarBoolean:
		return typ.Boolean, true
	case ScalarBool:
		if payload.Scalar.Bool {
			return typ.True, true
		}
		return typ.False, true
	case ScalarNumber:
		text := string(payload.Scalar.Data)
		if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
			return typ.LiteralInt(integer), true
		}
		if number, err := strconv.ParseFloat(text, 64); err == nil {
			return typ.LiteralNumber(number), true
		}
	case ScalarString:
		text, err := strconv.Unquote(string(payload.Scalar.Data))
		if err == nil {
			return typ.LiteralString(text), true
		}
	}
	return nil, false
}

func DecodeWitnessType(value []byte) (typ.Type, bool) {
	if bytes.HasPrefix(value, []byte(targetPrefix)) {
		target, ok := DecodeTarget(value)
		return target, ok && target != nil
	}
	scalar, ok := DecodeScalar(value)
	if !ok {
		return nil, false
	}
	return (Payload{Form: PayloadScalar, Scalar: scalar}).WitnessType()
}

// DecodeExactWitnessType projects only a concrete scalar value or a closed
// target publication. Abstract boolean is a witness set, not one runtime
// value, so expression and literal consumers must leave it undecided.
func DecodeExactWitnessType(value []byte) (typ.Type, bool) {
	if scalar, ok := DecodeScalar(value); ok && scalar.Kind == ScalarBoolean {
		return nil, false
	}
	return DecodeWitnessType(value)
}

// DecodeLiteralType projects the literal-predicate subset. Predicates publish
// only exact strings, booleans, and integer spellings; broader witnesses and
// floating-point spellings are deliberately not admitted by this projection.
func DecodeLiteralType(value []byte) (typ.Type, bool) {
	scalar, ok := DecodeScalar(value)
	if !ok {
		return nil, false
	}
	switch scalar.Kind {
	case ScalarString:
		text, err := strconv.Unquote(string(scalar.Data))
		if err == nil {
			return typ.LiteralString(text), true
		}
	case ScalarBool:
		if scalar.Bool {
			return typ.True, true
		}
		return typ.False, true
	case ScalarNumber:
		integer, err := strconv.ParseInt(string(scalar.Data), 10, 64)
		if err == nil {
			return typ.LiteralInt(integer), true
		}
	}
	return nil, false
}

// FunctionType decodes the canonical contract carried by a precise function
// scalar. Display-only signature fields are not a second type authority.
func (payload Payload) FunctionType() (typ.Type, bool) {
	if payload.Form != PayloadScalar || payload.Scalar.Kind != ScalarFunction || len(payload.Scalar.Data) == 0 {
		return nil, false
	}
	wire, err := base64.RawURLEncoding.DecodeString(string(payload.Scalar.Data))
	if err != nil {
		return nil, false
	}
	var envelope struct {
		Canonical string `json:"canonical,omitempty"`
	}
	if json.Unmarshal(wire, &envelope) != nil || envelope.Canonical == "" {
		return nil, false
	}
	canonical, err := base64.RawURLEncoding.DecodeString(envelope.Canonical)
	if err != nil {
		return nil, false
	}
	function, err := typ.DecodeCanonicalStructural(context.Background(), canonical)
	if err != nil || function == nil {
		return nil, false
	}
	if _, ok := unwrap.Alias(function).(*typ.Function); !ok || unwrap.Alias(function).Kind() != kind.Function {
		return nil, false
	}
	return function, true
}

func EncodeMemberMissing(target typ.Type) ([]byte, bool) {
	return Encode(Payload{Form: PayloadMemberMissing, Target: target})
}

func DecodeMemberMissing(value []byte) (typ.Type, bool) {
	payload, ok := Decode(value)
	return payload.Target, ok && payload.Form == PayloadMemberMissing
}
