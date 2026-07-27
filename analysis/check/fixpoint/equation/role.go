package equation

import (
	"fmt"
	"strconv"
	"strings"
)

// OperandRole is the declared role vocabulary carried by equation operands.
// Its representation is deliberately private: role text is a wire spelling,
// not a value another package may cast into semantic identity. Producers use
// the fixed values or the constructors below; artifact decoders use
// ParseOperandRole and fail closed on an empty spelling.
type OperandRole struct{ spelling string }

// OperandRoleFamily is a declared family whose members use the existing
// "<family>-<suffix>" canonical spelling.
type OperandRoleFamily struct{ id uint8 }

var (
	RoleFamilyArgument               = OperandRoleFamily{id: 1}
	RoleFamilyArgumentDisplay        = OperandRoleFamily{id: 2}
	RoleFamilyCapture                = OperandRoleFamily{id: 3}
	RoleFamilyCase                   = OperandRoleFamily{id: 4}
	RoleFamilyCaseDisplay            = OperandRoleFamily{id: 5}
	RoleFamilyDeclaredReturn         = OperandRoleFamily{id: 6}
	RoleFamilyDeclaredRoot           = OperandRoleFamily{id: 7}
	RoleFamilyDeclaredType           = OperandRoleFamily{id: 8}
	RoleFamilyDensityRelation        = OperandRoleFamily{id: 9}
	RoleFamilyDisplay                = OperandRoleFamily{id: 10}
	RoleFamilyDifference             = OperandRoleFamily{id: 11}
	RoleFamilyImplied                = OperandRoleFamily{id: 12}
	RoleFamilyMember                 = OperandRoleFamily{id: 13}
	RoleFamilyMonotoneFloor          = OperandRoleFamily{id: 14}
	RoleFamilyNative                 = OperandRoleFamily{id: 15}
	RoleFamilyNativeCaptureRoot      = OperandRoleFamily{id: 16}
	RoleFamilyNativeCaptureTransport = OperandRoleFamily{id: 17}
	RoleFamilyNativeHostGlobal       = OperandRoleFamily{id: 18}
	RoleFamilyNativeListChild        = OperandRoleFamily{id: 19}
	RoleFamilyNativeLengthEvent      = OperandRoleFamily{id: 20}
	RoleFamilyNativeLoopArm          = OperandRoleFamily{id: 21}
	RoleFamilyNativeLoopBoundDisplay = OperandRoleFamily{id: 22}
	RoleFamilyNativeLoopBoundLimit   = OperandRoleFamily{id: 23}
	RoleFamilyNativeLoopBoundPath    = OperandRoleFamily{id: 24}
	RoleFamilyPayloadType            = OperandRoleFamily{id: 25}
	RoleFamilyResult                 = OperandRoleFamily{id: 26}
	RoleFamilyReturnDisplay          = OperandRoleFamily{id: 27}
	RoleFamilyReturnValue            = OperandRoleFamily{id: 28}
	RoleFamilyShortCircuit           = OperandRoleFamily{id: 29}
	RoleFamilySuspensionLive         = OperandRoleFamily{id: 30}
	RoleFamilyTarget                 = OperandRoleFamily{id: 31}
	RoleFamilyTypeArgument           = OperandRoleFamily{id: 32}
	RoleFamilyValue                  = OperandRoleFamily{id: 33}
	RoleFamilyValueDisplay           = OperandRoleFamily{id: 34}
)

var operandRoleFamilyNames = [...]string{
	"",
	"argument",
	"argument-display",
	"capture",
	"case",
	"case-display",
	"declared-return",
	"declared-root",
	"declared-type",
	"density-relation",
	"display",
	"difference",
	"implied",
	"member",
	"monotone-floor",
	"native",
	"native-capture-root",
	"native-capture-transport",
	"native-host-global",
	"native-list-child",
	"native-length-event",
	"native-loop-arm",
	"native-loop-bound-display",
	"native-loop-bound-limit",
	"native-loop-bound-path",
	"payload-type",
	"result",
	"return-display",
	"return-value",
	"short-circuit",
	"suspension-live",
	"target",
	"type-argument",
	"value",
	"value-display",
}

var (
	RoleAllocationResult     = mustOperandRole("allocation-result")
	RoleApplication          = mustOperandRole("application")
	RoleArgumentSpread       = mustOperandRole("argument-spread")
	RoleBranchChain          = mustOperandRole("branch-chain")
	RoleBoundCallee          = mustOperandRole("bound-callee")
	RoleCallee               = mustOperandRole("callee")
	RoleCalleeDisplay        = mustOperandRole("callee-display")
	RoleCondition            = mustOperandRole("condition")
	RoleContainer            = mustOperandRole("container")
	RoleDefault              = mustOperandRole("default")
	RoleDisplay              = mustOperandRole("display")
	RoleEntry                = mustOperandRole("entry")
	RoleGlobalCallee         = mustOperandRole("global-callee-binding")
	RoleKind                 = mustOperandRole("kind")
	RoleNativeTopologyDrafts = mustOperandRole("native-topology-drafts")
	RoleOperation            = mustOperandRole("operation")
	RolePredicate            = mustOperandRole("predicate")
	RolePredicateDisplay     = mustOperandRole("predicate-display")
	RoleProvider             = mustOperandRole("provider")
	RoleReceiver             = mustOperandRole("receiver")
	RoleResult               = mustOperandRole("result")
	RoleResultArity          = mustOperandRole("result-arity")
	RoleResultDisplay        = mustOperandRole("result-display")
	RoleResultSpread         = mustOperandRole("result-spread")
	RoleTarget               = mustOperandRole("target")
	RoleValue                = mustOperandRole("value")
)

func (family OperandRoleFamily) name() (string, bool) {
	if int(family.id) <= 0 || int(family.id) >= len(operandRoleFamilyNames) {
		return "", false
	}
	name := operandRoleFamilyNames[family.id]
	return name, name != ""
}

// ParseOperandRole is the wire admission boundary for role spellings.
func ParseOperandRole(spelling string) (OperandRole, bool) {
	if spelling == "" || strings.ContainsAny(spelling, "/\x00") {
		return OperandRole{}, false
	}
	return OperandRole{spelling: spelling}, true
}

// MustOperandRole constructs a statically declared producer role. Dynamic
// families should prefer IndexedRole and SuffixedRole.
func MustOperandRole(spelling string) OperandRole {
	return mustOperandRole(spelling)
}

func mustOperandRole(spelling string) OperandRole {
	role, ok := ParseOperandRole(spelling)
	if !ok {
		panic("equation: invalid operand role")
	}
	return role
}

// IndexedRole constructs the canonical fixed-width member of a declared
// indexed role family.
func IndexedRole(family OperandRoleFamily, index int) OperandRole {
	name, ok := family.name()
	if !ok || index < 0 {
		return OperandRole{}
	}
	return mustOperandRole(fmt.Sprintf("%s-%08d", name, index))
}

// SuffixedRole constructs a non-indexed member of a declared family. Empty
// suffixes and separator-bearing suffixes are refused so callers cannot mint a
// second family grammar through this escape hatch.
func SuffixedRole(family OperandRoleFamily, suffix string) OperandRole {
	name, ok := family.name()
	if !ok || suffix == "" || strings.Contains(suffix, "/") {
		return OperandRole{}
	}
	return mustOperandRole(name + "-" + suffix)
}

// Valid reports whether the role came through an owner constructor.
func (role OperandRole) Valid() bool { return role.spelling != "" }

func (role OperandRole) Wire() string { return role.spelling }

// Suffix returns the family-owned suffix. Parsing is centralized here; callers
// choose the exact declared family rather than hand-matching text.
func (role OperandRole) Suffix(family OperandRoleFamily) (string, bool) {
	name, ok := family.name()
	if !ok {
		return "", false
	}
	return strings.CutPrefix(role.spelling, name+"-")
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
	return strings.HasSuffix(role.Wire(), "-display")
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
