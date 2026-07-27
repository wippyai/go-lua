package equation

import (
	"fmt"
	"strconv"
	"strings"
)

// OperandRole is the declared role vocabulary carried by equation operands.
// Its textual form remains wire-compatible with content-v1 artifacts, while
// family and index recovery is owned here instead of repeated by consumers.
type OperandRole string

// OperandRoleFamily is a declared family whose members use the existing
// "<family>-<suffix>" canonical spelling.
type OperandRoleFamily uint8

const (
	RoleFamilyArgument OperandRoleFamily = iota + 1
	RoleFamilyArgumentDisplay
	RoleFamilyCapture
	RoleFamilyCase
	RoleFamilyCaseDisplay
	RoleFamilyDeclaredReturn
	RoleFamilyDeclaredRoot
	RoleFamilyDeclaredType
	RoleFamilyDensityRelation
	RoleFamilyDisplay
	RoleFamilyDifference
	RoleFamilyImplied
	RoleFamilyMember
	RoleFamilyMonotoneFloor
	RoleFamilyNative
	RoleFamilyNativeCaptureRoot
	RoleFamilyNativeCaptureTransport
	RoleFamilyNativeHostGlobal
	RoleFamilyNativeListChild
	RoleFamilyNativeLengthEvent
	RoleFamilyNativeLoopArm
	RoleFamilyNativeLoopBoundDisplay
	RoleFamilyNativeLoopBoundLimit
	RoleFamilyNativeLoopBoundPath
	RoleFamilyPayloadType
	RoleFamilyResult
	RoleFamilyReturnDisplay
	RoleFamilyReturnValue
	RoleFamilyShortCircuit
	RoleFamilySuspensionLive
	RoleFamilyTarget
	RoleFamilyTypeArgument
	RoleFamilyValue
	RoleFamilyValueDisplay
)

var operandRoleFamilyNames = [...]string{
	RoleFamilyArgument:               "argument",
	RoleFamilyArgumentDisplay:        "argument-display",
	RoleFamilyCapture:                "capture",
	RoleFamilyCase:                   "case",
	RoleFamilyCaseDisplay:            "case-display",
	RoleFamilyDeclaredReturn:         "declared-return",
	RoleFamilyDeclaredRoot:           "declared-root",
	RoleFamilyDeclaredType:           "declared-type",
	RoleFamilyDensityRelation:        "density-relation",
	RoleFamilyDisplay:                "display",
	RoleFamilyDifference:             "difference",
	RoleFamilyImplied:                "implied",
	RoleFamilyMember:                 "member",
	RoleFamilyMonotoneFloor:          "monotone-floor",
	RoleFamilyNative:                 "native",
	RoleFamilyNativeCaptureRoot:      "native-capture-root",
	RoleFamilyNativeCaptureTransport: "native-capture-transport",
	RoleFamilyNativeHostGlobal:       "native-host-global",
	RoleFamilyNativeListChild:        "native-list-child",
	RoleFamilyNativeLengthEvent:      "native-length-event",
	RoleFamilyNativeLoopArm:          "native-loop-arm",
	RoleFamilyNativeLoopBoundDisplay: "native-loop-bound-display",
	RoleFamilyNativeLoopBoundLimit:   "native-loop-bound-limit",
	RoleFamilyNativeLoopBoundPath:    "native-loop-bound-path",
	RoleFamilyPayloadType:            "payload-type",
	RoleFamilyResult:                 "result",
	RoleFamilyReturnDisplay:          "return-display",
	RoleFamilyReturnValue:            "return-value",
	RoleFamilyShortCircuit:           "short-circuit",
	RoleFamilySuspensionLive:         "suspension-live",
	RoleFamilyTarget:                 "target",
	RoleFamilyTypeArgument:           "type-argument",
	RoleFamilyValue:                  "value",
	RoleFamilyValueDisplay:           "value-display",
}

const (
	RoleAllocationResult OperandRole = "allocation-result"
	RoleApplication      OperandRole = "application"
	RoleArgumentSpread   OperandRole = "argument-spread"
	RoleBranchChain      OperandRole = "branch-chain"
	RoleCallee           OperandRole = "callee"
	RoleCalleeDisplay    OperandRole = "callee-display"
	RoleCondition        OperandRole = "condition"
	RoleContainer        OperandRole = "container"
	RoleDefault          OperandRole = "default"
	RoleDisplay          OperandRole = "display"
	RoleEntry            OperandRole = "entry"
	RoleGlobalCallee     OperandRole = "global-callee-binding"
	RolePredicate        OperandRole = "predicate"
	RolePredicateDisplay OperandRole = "predicate-display"
	RoleProvider         OperandRole = "provider"
	RoleReceiver         OperandRole = "receiver"
	RoleResult           OperandRole = "result"
	RoleResultArity      OperandRole = "result-arity"
	RoleResultDisplay    OperandRole = "result-display"
	RoleResultSpread     OperandRole = "result-spread"
	RoleTarget           OperandRole = "target"
	RoleValue            OperandRole = "value"
)

func (family OperandRoleFamily) name() (string, bool) {
	if int(family) <= 0 || int(family) >= len(operandRoleFamilyNames) {
		return "", false
	}
	name := operandRoleFamilyNames[family]
	return name, name != ""
}

// IndexedRole constructs the canonical fixed-width member of a declared
// indexed role family.
func IndexedRole(family OperandRoleFamily, index int) OperandRole {
	name, ok := family.name()
	if !ok || index < 0 {
		return ""
	}
	return OperandRole(fmt.Sprintf("%s-%08d", name, index))
}

// SuffixedRole constructs a non-indexed member of a declared family. Empty
// suffixes and separator-bearing suffixes are refused so callers cannot mint a
// second family grammar through this escape hatch.
func SuffixedRole(family OperandRoleFamily, suffix string) OperandRole {
	name, ok := family.name()
	if !ok || suffix == "" || strings.Contains(suffix, "/") {
		return ""
	}
	return OperandRole(name + "-" + suffix)
}

// Subrole appends one closed component to an already-typed parent role.
func Subrole(parent OperandRole, suffix string) OperandRole {
	if parent == "" || suffix == "" || strings.ContainsAny(suffix, "/-") {
		return ""
	}
	return OperandRole(parent.String() + "-" + suffix)
}

func (role OperandRole) String() string { return string(role) }

// Suffix returns the family-owned suffix. Parsing is centralized here; callers
// choose the exact declared family rather than hand-matching text.
func (role OperandRole) Suffix(family OperandRoleFamily) (string, bool) {
	name, ok := family.name()
	if !ok {
		return "", false
	}
	return strings.CutPrefix(string(role), name+"-")
}

// InFamily reports membership in a declared role family.
func (role OperandRole) InFamily(family OperandRoleFamily) bool {
	_, ok := role.Suffix(family)
	return ok
}

// Component returns the suffix after one declared subrole component.
func (role OperandRole) Component(family OperandRoleFamily, component string) (string, bool) {
	suffix, ok := role.Suffix(family)
	if !ok || component == "" {
		return "", false
	}
	return strings.CutPrefix(suffix, component+"-")
}

// IsDisplay reports presentation-only roles. This classification is owned by
// the role type so a consumer never promotes display metadata by broad family
// matching.
func (role OperandRole) IsDisplay() bool {
	if role == RoleDisplay || role == RoleResultDisplay || role == RolePredicateDisplay || role == RoleCalleeDisplay {
		return true
	}
	if _, ok := role.Index(RoleFamilyArgumentDisplay); ok {
		return true
	}
	if _, ok := role.Index(RoleFamilyCaseDisplay); ok {
		return true
	}
	if _, ok := role.Index(RoleFamilyReturnDisplay); ok {
		return true
	}
	if _, ok := role.Index(RoleFamilyValueDisplay); ok {
		return true
	}
	return strings.HasSuffix(role.String(), "-display")
}

// Index returns a decimal indexed-family member. It intentionally accepts
// legacy non-padded test artifacts; producers use IndexedRole for canonical
// construction.
func (role OperandRole) Index(family OperandRoleFamily) (int, bool) {
	suffix, ok := role.Suffix(family)
	if !ok || suffix == "" {
		return 0, false
	}
	index, err := strconv.Atoi(suffix)
	return index, err == nil && index >= 0
}

// FixedIndex returns an indexed member only when its decimal suffix has the
// required canonical width.
func (role OperandRole) FixedIndex(family OperandRoleFamily, width int) (int, bool) {
	suffix, ok := role.Suffix(family)
	if !ok || len(suffix) != width {
		return 0, false
	}
	return role.Index(family)
}

// SemanticResultRole can only be obtained from OperandRole.SemanticResult.
// Display, arity, and spread metadata have no construction path into this
// semantic type.
type SemanticResultRole struct{ role OperandRole }

func (role SemanticResultRole) OperandRole() OperandRole { return role.role }

func (role SemanticResultRole) Index() (int, bool) {
	return role.role.Index(RoleFamilyResult)
}

// SemanticResult classifies only a scalar result role or an indexed result
// slot. In particular result-display is structurally outside the return type.
func (role OperandRole) SemanticResult() (SemanticResultRole, bool) {
	if role == RoleResult {
		return SemanticResultRole{role: role}, true
	}
	if _, ok := role.Index(RoleFamilyResult); ok {
		return SemanticResultRole{role: role}, true
	}
	return SemanticResultRole{}, false
}
