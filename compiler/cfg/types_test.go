package cfg

import (
	"testing"

	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// TestVersion_IsZero tests Version zero check.
func TestVersion_IsZero(t *testing.T) {
	zero := Version{}
	if !zero.IsZero() {
		t.Error("Zero version should be zero")
	}

	zeroID := Version{Root: "x", ID: 0}
	if !zeroID.IsZero() {
		t.Error("Version with ID=0 should be zero")
	}

	nonZero := Version{Root: "x", ID: 1}
	if nonZero.IsZero() {
		t.Error("Non-zero version should not be zero")
	}
}

// TestNodeInfo_Kind tests NodeInfo Kind() method.
func TestNodeInfo_Kind(t *testing.T) {
	tests := []struct {
		name string
		info NodeInfo
		want basecfg.NodeKind
	}{
		{"AssignInfo", &AssignInfo{}, basecfg.NodeAssign},
		{"CallInfo", &CallInfo{}, basecfg.NodeCall},
		{"ReturnInfo", &ReturnInfo{}, basecfg.NodeReturn},
		{"BranchInfo", &BranchInfo{}, basecfg.NodeBranch},
		{"TypeDefInfo", &TypeDefInfo{}, basecfg.NodeTypeDef},
		{"FuncDefInfo", &FuncDefInfo{}, basecfg.NodeAssign},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.Kind(); got != tt.want {
				t.Errorf("%s.Kind() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestTargetKind_Values tests TargetKind constants.
func TestTargetKind_Values(t *testing.T) {
	if TargetIdent != 0 {
		t.Error("TargetIdent should be 0")
	}
	if TargetField != 1 {
		t.Error("TargetField should be 1")
	}
	if TargetIndex != 2 {
		t.Error("TargetIndex should be 2")
	}
}

// TestFuncDefTargetKind_Values tests FuncDefTargetKind constants.
func TestFuncDefTargetKind_Values(t *testing.T) {
	if FuncDefGlobal != 0 {
		t.Error("FuncDefGlobal should be 0")
	}
	if FuncDefField != 1 {
		t.Error("FuncDefField should be 1")
	}
	if FuncDefMethod != 2 {
		t.Error("FuncDefMethod should be 2")
	}
}

// TestAssignTarget_Fields tests AssignTarget field access.
func TestAssignTarget_Fields(t *testing.T) {
	target := AssignTarget{
		Kind:      TargetField,
		Name:      "",
		BaseName:  "obj",
		FieldPath: []string{"a", "b"},
	}

	if target.Kind != TargetField {
		t.Error("Kind should be TargetField")
	}
	if target.BaseName != "obj" {
		t.Error("BaseName should be obj")
	}
	if len(target.FieldPath) != 2 {
		t.Error("FieldPath should have 2 elements")
	}
}

// TestAssignInfo_Fields tests AssignInfo fields.
func TestAssignInfo_Fields(t *testing.T) {
	info := &AssignInfo{
		IsLocal: true,
		Targets: []AssignTarget{
			{Kind: TargetIdent, Name: "x"},
		},
		SourceNames:     []string{"y"},
		TargetVersions:  []Version{{Root: "x", ID: 1}},
		TypeAnnotations: nil,
	}

	if !info.IsLocal {
		t.Error("IsLocal should be true")
	}
	if len(info.Targets) != 1 {
		t.Error("Should have 1 target")
	}
	if info.Targets[0].Name != "x" {
		t.Error("Target name should be x")
	}
}

// TestCallInfo_Fields tests CallInfo fields.
func TestCallInfo_Fields(t *testing.T) {
	info := &CallInfo{
		CalleeName:    "print",
		Method:        "",
		ReceiverName:  "",
		IsStmt:        true,
		IsTypeCheck:   false,
		TypeCheckName: "",
		ArgNames:      []string{"a", ""},
	}

	if info.CalleeName != "print" {
		t.Error("CalleeName should be print")
	}
	if !info.IsStmt {
		t.Error("IsStmt should be true")
	}
	if info.IsTypeCheck {
		t.Error("IsTypeCheck should be false")
	}
}

// TestReturnInfo_Fields tests ReturnInfo fields.
func TestReturnInfo_Fields(t *testing.T) {
	info := &ReturnInfo{
		Names:   []string{"x", ""},
		Symbols: []basecfg.SymbolID{1, 0},
	}

	if len(info.Names) != 2 {
		t.Error("Should have 2 names")
	}
	if info.Names[0] != "x" {
		t.Error("First name should be x")
	}
}

// TestBranchInfo_Fields tests BranchInfo fields.
func TestBranchInfo_Fields(t *testing.T) {
	info := &BranchInfo{
		CondVar:   "x",
		CondCheck: basecfg.CondCheck{Kind: basecfg.CheckNil},
	}

	if info.CondVar != "x" {
		t.Error("CondVar should be x")
	}
	if info.CondCheck.Kind != basecfg.CheckNil {
		t.Error("CondCheck.Kind should be CheckNil")
	}
}

// TestTypeDefInfo_Fields tests TypeDefInfo fields.
func TestTypeDefInfo_Fields(t *testing.T) {
	info := &TypeDefInfo{
		Name: "MyType",
		TypeParams: []TypeParamInfo{
			{Name: "T"},
		},
	}

	if info.Name != "MyType" {
		t.Error("Name should be MyType")
	}
	if len(info.TypeParams) != 1 {
		t.Error("Should have 1 type param")
	}
}

// TestFuncDefInfo_Fields tests FuncDefInfo fields.
func TestFuncDefInfo_Fields(t *testing.T) {
	info := &FuncDefInfo{
		TargetKind: FuncDefMethod,
		Name:       "doIt",
		IsMethod:   true,
	}

	if info.TargetKind != FuncDefMethod {
		t.Error("TargetKind should be FuncDefMethod")
	}
	if info.Name != "doIt" {
		t.Error("Name should be doIt")
	}
	if !info.IsMethod {
		t.Error("IsMethod should be true")
	}
}

// TestPhiInfo_Fields tests PhiInfo fields.
func TestPhiInfo_Fields(t *testing.T) {
	info := &PhiInfo{
		Target: Version{Root: "x", ID: 3},
		Operands: []PhiOperand{
			{From: 1, Version: Version{Root: "x", ID: 1}},
			{From: 2, Version: Version{Root: "x", ID: 2}},
		},
	}

	if info.Target.Root != "x" || info.Target.ID != 3 {
		t.Error("Target should be x@3")
	}
	if len(info.Operands) != 2 {
		t.Error("Should have 2 operands")
	}
}

// TestPhiOperand_Fields tests PhiOperand fields.
func TestPhiOperand_Fields(t *testing.T) {
	op := PhiOperand{
		From:    basecfg.Point(5),
		Version: Version{Root: "y", ID: 2},
	}

	if op.From != 5 {
		t.Error("From should be 5")
	}
	if op.Version.Root != "y" || op.Version.ID != 2 {
		t.Error("Version should be y@2")
	}
}

// TestNestedFunc_Fields tests NestedFunc fields.
func TestNestedFunc_Fields(t *testing.T) {
	nf := NestedFunc{
		Point: basecfg.Point(10),
		Func:  nil,
	}

	if nf.Point != 10 {
		t.Error("Point should be 10")
	}
}

// TestNumericForInfo_Fields tests NumericForInfo fields.
func TestNumericForInfo_Fields(t *testing.T) {
	info := &NumericForInfo{
		VarName: "i",
	}

	if info.VarName != "i" {
		t.Error("VarName should be i")
	}
}

// TestTypeParamInfo_Fields tests TypeParamInfo fields.
func TestTypeParamInfo_Fields(t *testing.T) {
	info := TypeParamInfo{
		Name:       "T",
		Constraint: nil,
	}

	if info.Name != "T" {
		t.Error("Name should be T")
	}
}

// TestSymbolID_Zero tests basecfg.SymbolID zero value.
func TestSymbolID_Zero(t *testing.T) {
	var sym basecfg.SymbolID
	if sym != 0 {
		t.Error("Zero value should be 0")
	}
}

// TestPoint_Alias tests Point type alias.
func TestPoint_Alias(t *testing.T) {
	var p = basecfg.Point(5)
	if p != 5 {
		t.Error("Point should be aliased correctly")
	}
}

// TestNodeKind_Constants tests node kind constants match base.
func TestNodeKind_Constants(t *testing.T) {
	if NodeEntry != basecfg.NodeEntry {
		t.Error("NodeEntry mismatch")
	}
	if NodeExit != basecfg.NodeExit {
		t.Error("NodeExit mismatch")
	}
	if NodeAssign != basecfg.NodeAssign {
		t.Error("NodeAssign mismatch")
	}
	if NodeCall != basecfg.NodeCall {
		t.Error("NodeCall mismatch")
	}
	if NodeBranch != basecfg.NodeBranch {
		t.Error("NodeBranch mismatch")
	}
	if NodeJoin != basecfg.NodeJoin {
		t.Error("NodeJoin mismatch")
	}
	if NodeReturn != basecfg.NodeReturn {
		t.Error("NodeReturn mismatch")
	}
	if NodeScopeEnter != basecfg.NodeScopeEnter {
		t.Error("NodeScopeEnter mismatch")
	}
	if NodeScopeExit != basecfg.NodeScopeExit {
		t.Error("NodeScopeExit mismatch")
	}
	if NodeTypeDef != basecfg.NodeTypeDef {
		t.Error("NodeTypeDef mismatch")
	}
}
