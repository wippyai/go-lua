package front

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

var (
	// ErrWireAbsent is returned only by required decodes of an empty transport
	// slot. It is distinct from a present wire whose JSON is malformed.
	ErrWireAbsent = errors.New("front: wire absent")
	// ErrWireMalformed identifies a present transport value that cannot be
	// decoded as the schema selected by its owning role or fact family.
	ErrWireMalformed = errors.New("front: malformed wire")
)

const (
	branchPredicatePrefix = "front/branch-predicate/v1/"
	branchEvidencePrefix  = "front/branch-evidence/v1/"
	branchDiffPrefix      = "front/branch-diff/v1/"
	branchChainPrefix     = "front/branch-chain/v1/"
	moduleProviderPrefix  = "provider/module/v1/"
)

// BranchPredicateWire is the closed branch predicate vocabulary. The front
// owns both its schema and codec; consumers must not reconstruct either from
// the JSON representation.
type BranchPredicateWire struct {
	Kind           string              `json:"kind"`
	Path           string              `json:"path,omitempty"`
	OtherPath      string              `json:"other_path,omitempty"`
	PathShape      BranchChainPathWire `json:"path_shape,omitempty"`
	OtherPathShape BranchChainPathWire `json:"other_path_shape,omitempty"`
	TypeName       string              `json:"type_name,omitempty"`
	Literal        string              `json:"literal,omitempty"`
	LenFloor       int64               `json:"len_floor,omitempty"`
	NumFloor       int64               `json:"num_floor,omitempty"`
	NumCeil        int64               `json:"num_ceil,omitempty"`
	HasNumCeil     bool                `json:"has_num_ceil,omitempty"`
	NumCeilNegated bool                `json:"num_ceil_negated,omitempty"`
	Modulus        int64               `json:"modulus,omitempty"`
	Residue        int64               `json:"residue,omitempty"`
	Negated        bool                `json:"negated,omitempty"`
}

// BranchDiffWire is one normalized difference-logic descriptor:
//
//	CoHi*HiPath + CoHi2*Hi2Path - LoPath <= C
type BranchDiffWire struct {
	CoHi     int64  `json:"co_hi"`
	HiPath   string `json:"hi_path"`
	HiIsLen  bool   `json:"hi_is_len,omitempty"`
	CoHi2    int64  `json:"co_hi2,omitempty"`
	Hi2Path  string `json:"hi2_path,omitempty"`
	Hi2IsLen bool   `json:"hi2_is_len,omitempty"`
	HasHi2   bool   `json:"has_hi2,omitempty"`
	LoPath   string `json:"lo_path"`
	LoIsLen  bool   `json:"lo_is_len,omitempty"`
	C        int64  `json:"c,omitempty"`
	Edge     bool   `json:"edge,omitempty"`
}

// BranchChainPathWire is the structural path publication needed by consumers
// of one authored if/elseif chain. Parent is populated only when FinalField
// names the path's final static field; consumers never recover that relation
// by trimming a display string.
type BranchChainPathWire struct {
	Key           string `json:"key"`
	Display       string `json:"display"`
	ParentKey     string `json:"parent_key,omitempty"`
	ParentDisplay string `json:"parent_display,omitempty"`
	FinalField    string `json:"final_field,omitempty"`
}

// BranchChainCheckWire couples the already-normalized predicate with the
// structural path metadata that its compact predicate wire intentionally
// omits.
type BranchChainCheckWire struct {
	Predicate     BranchPredicateWire `json:"predicate"`
	Path          BranchChainPathWire `json:"path"`
	OtherPath     BranchChainPathWire `json:"other_path,omitempty"`
	LiteralTarget string              `json:"literal_target,omitempty"`
}

// BranchChainWire publishes complete authored chain topology on every branch
// equation. The branch position makes the set self-validating after equation
// canonicalization has discarded source order.
type BranchChainWire struct {
	ID       uint32                 `json:"id"`
	Position uint32                 `json:"position"`
	Count    uint32                 `json:"count"`
	HasElse  bool                   `json:"has_else,omitempty"`
	HeadSpan wir.Span               `json:"head_span"`
	Checks   []BranchChainCheckWire `json:"checks"`
}

// ModuleProviderWire binds a provider to one resolved require module and
// structural member suffix.
type ModuleProviderWire struct {
	Module string `json:"module"`
	Suffix string `json:"suffix,omitempty"`
}

// ValidateDraftWires checks an externally assembled compilation. Compilations
// returned by this front carry a private post-construction certificate and pay
// no second decode cost at the engine boundary.
func (compilation Compilation) ValidateDraftWires() error {
	if compilation.draftWiresValidated {
		return nil
	}
	return ValidateDraftWires(compilation.Artifact)
}

// ValidateDraftWires is the admission fence for the front-owned wire families.
// An unrelated operand is absent semantics; an operand that carries one of
// these tags but cannot decode is a malformed artifact and must stop before
// any engine consumer can reinterpret it as absence.
func ValidateDraftWires(artifact equation.Artifact) error {
	for _, operation := range artifact.Equations {
		for _, operand := range operation.Operands {
			encoded := operand.Term.Encoding
			_, predicatePresent, predicateErr := DecodeBranchPredicateWire(encoded)
			if predicateErr != nil {
				return fmt.Errorf("front: operation %q role %q: %w", operation.Target.Name, operand.Role, predicateErr)
			}
			_, _, _, evidencePresent, evidenceErr := DecodeBranchEvidenceWire(encoded)
			if evidenceErr != nil {
				return fmt.Errorf("front: operation %q role %q: %w", operation.Target.Name, operand.Role, evidenceErr)
			}
			_, differencePresent, differenceErr := DecodeBranchDiffWire(encoded)
			if differenceErr != nil {
				return fmt.Errorf("front: operation %q role %q: %w", operation.Target.Name, operand.Role, differenceErr)
			}
			_, _, providerErr := DecodeModuleProviderWire(encoded)
			if providerErr != nil {
				return fmt.Errorf("front: operation %q role %q: %w", operation.Target.Name, operand.Role, providerErr)
			}
			_, chainPresent, chainErr := DecodeBranchChainWire(encoded)
			if chainErr != nil {
				return fmt.Errorf("front: operation %q role %q: %w", operation.Target.Name, operand.Role, chainErr)
			}
			switch {
			case operand.Role.Wire() == "predicate" && !predicatePresent:
				return fmt.Errorf("front: operation %q predicate role has no predicate wire", operation.Target.Name)
			case operand.Role.InFamily(equation.RoleFamilyDifference) && !differencePresent:
				return fmt.Errorf("front: operation %q difference role %q has no difference wire", operation.Target.Name, operand.Role)
			case operand.Role.InFamily(equation.RoleFamilyImplied) && !evidencePresent:
				return fmt.Errorf("front: operation %q implied role %q has no evidence wire", operation.Target.Name, operand.Role)
			case operand.Role.Wire() == "branch-chain" && !chainPresent:
				return fmt.Errorf("front: operation %q branch-chain role has no chain wire", operation.Target.Name)
			}
		}
	}
	return nil
}

// EncodeBranchChainWire is the sole constructor for authored chain topology.
func EncodeBranchChainWire(wire BranchChainWire) ([]byte, error) {
	return encodePrefixedWire(wire, branchChainPrefix, "branch chain", validateBranchChainWire)
}

// DecodeBranchChainWire distinguishes an unrelated operand from a malformed
// chain publication.
func DecodeBranchChainWire(encoded []byte) (wire BranchChainWire, present bool, err error) {
	return decodePrefixedWire(encoded, branchChainPrefix, "branch chain", validateBranchChainWire)
}

func validateBranchChainWire(wire BranchChainWire) error {
	if wire.ID == 0 || wire.Count == 0 || wire.Position >= wire.Count || !wire.HeadSpan.Valid() || len(wire.Checks) == 0 {
		return fmt.Errorf("front: malformed branch chain identity")
	}
	for _, check := range wire.Checks {
		if err := validateBranchPredicateWire(check.Predicate); err != nil {
			return fmt.Errorf("front: branch chain predicate: %w", err)
		}
		if err := validateBranchChainPath(check.Path, check.Predicate.Path); err != nil {
			return err
		}
		if check.Predicate.OtherPath != "" {
			if err := validateBranchChainPath(check.OtherPath, check.Predicate.OtherPath); err != nil {
				return err
			}
		}
		literal := check.Predicate.Kind == "literal-equal" || check.Predicate.Kind == "literal-not"
		if literal {
			if target, ok := shapefact.DecodeTarget([]byte(check.LiteralTarget)); !ok || target == nil {
				return fmt.Errorf("front: literal branch chain check has no typed target")
			}
		} else if check.LiteralTarget != "" {
			return fmt.Errorf("front: non-literal branch chain check has a literal target")
		}
	}
	return nil
}

func validateBranchChainPath(path BranchChainPathWire, predicateKey string) error {
	if path.Key == "" || path.Display == "" || path.Key != predicateKey {
		return fmt.Errorf("front: malformed branch chain path")
	}
	if path.FinalField == "" {
		if path.ParentKey != "" || path.ParentDisplay != "" {
			return fmt.Errorf("front: root branch chain path has a parent")
		}
		return nil
	}
	if path.ParentKey == "" || path.ParentDisplay == "" {
		return fmt.Errorf("front: field branch chain path has no parent")
	}
	return nil
}

// DecodeWireJSON is the single JSON admission primitive for analysis wires.
// Empty bytes mean that the selected transport slot is absent. Any non-empty
// bytes are present, and malformed JSON is returned as an error rather than
// being reinterpreted by a consumer as absence.
func DecodeWireJSON(encoded []byte, destination any) (present bool, err error) {
	if len(encoded) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return true, fmt.Errorf("%w: %v", ErrWireMalformed, err)
	}
	return true, nil
}

// DecodeRequiredWireJSON is used after a role, key, or fact lookup has already
// established presence. Both absence and malformed content remain explicit
// errors, so a consumer can fail closed without collapsing the two cases.
func DecodeRequiredWireJSON(encoded []byte, destination any) error {
	present, err := DecodeWireJSON(encoded, destination)
	if err != nil {
		return err
	}
	if !present {
		return ErrWireAbsent
	}
	return nil
}

func decodeDraftJSON(encoded []byte, destination any) error {
	present, err := DecodeWireJSON(encoded, destination)
	if err != nil {
		return err
	}
	if !present {
		return ErrWireAbsent
	}
	return nil
}

func encodePrefixedWire[T any](wire T, prefix, label string, validate func(T) error) ([]byte, error) {
	if err := validate(wire); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("front: encode %s: %w", label, err)
	}
	return append([]byte(prefix), encoded...), nil
}

func decodePrefixedWire[T any](encoded []byte, prefix, label string, validate func(T) error) (wire T, present bool, err error) {
	if !bytes.HasPrefix(encoded, []byte(prefix)) {
		return wire, false, nil
	}
	if err := decodeDraftJSON(encoded[len(prefix):], &wire); err != nil {
		return wire, true, fmt.Errorf("front: decode %s: %w", label, err)
	}
	if err := validate(wire); err != nil {
		return wire, true, err
	}
	return wire, true, nil
}

// EncodeBranchPredicateWire is the sole constructor of predicate wire bytes.
func EncodeBranchPredicateWire(wire BranchPredicateWire) ([]byte, error) {
	return encodePrefixedWire(wire, branchPredicatePrefix, "branch predicate", validateBranchPredicateWire)
}

// DecodeBranchPredicateWire distinguishes an unrelated value (present=false)
// from a malformed predicate value (present=true, err!=nil).
func DecodeBranchPredicateWire(encoded []byte) (wire BranchPredicateWire, present bool, err error) {
	return decodePrefixedWire(encoded, branchPredicatePrefix, "branch predicate", validateBranchPredicateWire)
}

func validateBranchPredicateWire(wire BranchPredicateWire) error {
	requiresPath := true
	requiresOther := false
	switch wire.Kind {
	case "truthy", "falsy", "nil", "not-nil", "len-ge", "num-ge", "num-le", "frozen-table", "mod-residue":
	case "literal-equal", "literal-not":
		if wire.Literal == "" {
			return fmt.Errorf("front: literal branch predicate has no literal")
		}
	case "path-equal", "path-not", "index-in-range":
		requiresOther = true
	case "type-equal", "type-not":
		if wire.TypeName == "" && wire.OtherPath == "" {
			return fmt.Errorf("front: type branch predicate has neither type name nor other path")
		}
	default:
		return fmt.Errorf("front: unknown branch predicate kind %q", wire.Kind)
	}
	if requiresPath && wire.Path == "" {
		return fmt.Errorf("front: branch predicate %q has no path", wire.Kind)
	}
	if requiresOther && wire.OtherPath == "" {
		return fmt.Errorf("front: branch predicate %q has no other path", wire.Kind)
	}
	if wire.PathShape.Key != "" {
		if err := validateBranchChainPath(wire.PathShape, wire.Path); err != nil {
			return err
		}
	}
	if wire.OtherPathShape.Key != "" {
		if err := validateBranchChainPath(wire.OtherPathShape, wire.OtherPath); err != nil {
			return err
		}
	}
	if (wire.Kind == "path-equal" || wire.Kind == "path-not") &&
		(wire.PathShape.Key == "" || wire.OtherPathShape.Key == "") {
		return fmt.Errorf("front: path relation has no structural path identity")
	}
	return nil
}

// EncodeBranchEvidenceWire is the sole constructor of a predicate plus its
// branch-edge polarity envelope.
func EncodeBranchEvidenceWire(wire BranchPredicateWire, edge, polarity bool) ([]byte, error) {
	predicate, err := EncodeBranchPredicateWire(wire)
	if err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("%s%t/%t/", branchEvidencePrefix, edge, polarity)
	return append([]byte(prefix), predicate...), nil
}

// DecodeBranchEvidenceWire distinguishes a non-evidence value from a malformed
// evidence envelope and returns the enclosed predicate through the same codec.
func DecodeBranchEvidenceWire(encoded []byte) (wire BranchPredicateWire, edge, polarity, present bool, err error) {
	if !bytes.HasPrefix(encoded, []byte(branchEvidencePrefix)) {
		return BranchPredicateWire{}, false, false, false, nil
	}
	rest := encoded[len(branchEvidencePrefix):]
	first := bytes.IndexByte(rest, '/')
	if first < 0 {
		return BranchPredicateWire{}, false, false, true, fmt.Errorf("front: malformed branch evidence edge")
	}
	second := bytes.IndexByte(rest[first+1:], '/')
	if second < 0 {
		return BranchPredicateWire{}, false, false, true, fmt.Errorf("front: malformed branch evidence polarity")
	}
	second += first + 1
	parseBool := func(value []byte) (bool, error) {
		switch string(value) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("invalid boolean %q", value)
		}
	}
	edge, err = parseBool(rest[:first])
	if err != nil {
		return BranchPredicateWire{}, false, false, true, fmt.Errorf("front: branch evidence edge: %w", err)
	}
	polarity, err = parseBool(rest[first+1 : second])
	if err != nil {
		return BranchPredicateWire{}, false, false, true, fmt.Errorf("front: branch evidence polarity: %w", err)
	}
	wire, predicatePresent, err := DecodeBranchPredicateWire(rest[second+1:])
	if err != nil {
		return BranchPredicateWire{}, false, false, true, err
	}
	if !predicatePresent {
		return BranchPredicateWire{}, false, false, true, fmt.Errorf("front: branch evidence has no predicate")
	}
	return wire, edge, polarity, true, nil
}

// EncodeBranchDiffWire is the sole constructor of difference wire bytes.
func EncodeBranchDiffWire(wire BranchDiffWire) ([]byte, error) {
	return encodePrefixedWire(wire, branchDiffPrefix, "branch difference", validateBranchDiffWire)
}

// DecodeBranchDiffWire distinguishes absence from a malformed descriptor.
func DecodeBranchDiffWire(encoded []byte) (wire BranchDiffWire, present bool, err error) {
	return decodePrefixedWire(encoded, branchDiffPrefix, "branch difference", validateBranchDiffWire)
}

func validateBranchDiffWire(wire BranchDiffWire) error {
	if wire.HiPath == "" || wire.LoPath == "" || (wire.HasHi2 && wire.Hi2Path == "") {
		return fmt.Errorf("front: branch difference has an empty operand")
	}
	if !wire.HasHi2 && (wire.Hi2Path != "" || wire.CoHi2 != 0 || wire.Hi2IsLen) {
		return fmt.Errorf("front: branch difference has a second operand without its presence tag")
	}
	return nil
}

// EncodeModuleProviderWire is the sole constructor of a module provider wire.
func EncodeModuleProviderWire(wire ModuleProviderWire) ([]byte, error) {
	if wire.Module == "" {
		return nil, fmt.Errorf("front: module provider has no module")
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("front: encode module provider: %w", err)
	}
	return []byte(moduleProviderPrefix + base64.RawURLEncoding.EncodeToString(encoded)), nil
}

// DecodeModuleProviderWire distinguishes unrelated providers from malformed
// module-provider bytes and rejects non-canonical JSON spellings.
func DecodeModuleProviderWire(encoded []byte) (wire ModuleProviderWire, present bool, err error) {
	if !bytes.HasPrefix(encoded, []byte(moduleProviderPrefix)) {
		return ModuleProviderWire{}, false, nil
	}
	payload := encoded[len(moduleProviderPrefix):]
	wired, decodeErr := base64.RawURLEncoding.DecodeString(string(payload))
	if decodeErr != nil {
		return ModuleProviderWire{}, true, fmt.Errorf("front: decode module provider base64: %w", decodeErr)
	}
	if err := decodeDraftJSON(wired, &wire); err != nil {
		return ModuleProviderWire{}, true, fmt.Errorf("front: decode module provider JSON: %w", err)
	}
	if wire.Module == "" {
		return ModuleProviderWire{}, true, fmt.Errorf("front: module provider has no module")
	}
	canonical, marshalErr := json.Marshal(wire)
	if marshalErr != nil {
		return ModuleProviderWire{}, true, fmt.Errorf("front: canonicalize module provider: %w", marshalErr)
	}
	if !bytes.Equal(canonical, wired) {
		return ModuleProviderWire{}, true, fmt.Errorf("front: non-canonical module provider")
	}
	return wire, true, nil
}
