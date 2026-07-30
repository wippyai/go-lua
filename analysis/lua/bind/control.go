package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ControlIssueKind classifies a failed Lua label/goto binding law.
type ControlIssueKind uint8

const (
	ControlIssueDuplicateLabel ControlIssueKind = iota + 1
	ControlIssueUndefinedLabel
	ControlIssueGotoEntersLocal
	ControlIssueInvalidLabel
	ControlIssueInvalidGoto
)

// ControlIssue is binder-owned evidence that authored control cannot be
// resolved soundly. Goto or Label identifies the offending statement; a
// scope-entering Goto also carries its resolved Label. Previous identifies the
// already-visible declaration for a duplicate Label. Local identifies the
// first local scope entered by an invalid Goto.
type ControlIssue struct {
	Kind     ControlIssueKind
	Goto     *ast.GotoStmt
	Label    *ast.LabelStmt
	Previous *ast.LabelStmt
	Local    symbol.ID
}

// GotoTarget returns the exact visible Label bound to a legal Goto. Invalid
// and unresolved gotos have no target and are described by ControlIssues.
func (r *Result) GotoTarget(stmt *ast.GotoStmt) (*ast.LabelStmt, bool) {
	if r == nil || stmt == nil {
		return nil, false
	}
	target, ok := r.gotoTargets[stmt]
	return target, ok && target != nil
}

// ControlIssues returns binder-owned control failures in lexical source order.
// The returned slice never aliases Result storage.
func (r *Result) ControlIssues() []ControlIssue {
	if r == nil || len(r.controlIssues) == 0 {
		return nil
	}
	return append([]ControlIssue(nil), r.controlIssues...)
}

type controlLabel struct {
	stmt       *ast.LabelStmt
	block      int
	active     int
	endOfBlock bool
	visited    bool
}

type controlGoto struct {
	stmt      *ast.GotoStmt
	effective int
	order     int
}

type controlBlock struct {
	restoreLocals int
	labelBase     int
	currentLocal  int
	targetMark    int
	declaredMark  int
	indexed       bool
}

type controlTargetUndo struct {
	name    string
	prior   int
	existed bool
}

type controlDeclaredUndo struct {
	name    string
	prior   int
	existed bool
}

type controlFunction struct {
	locals []symbol.ID
	blocks []int

	targets    map[string]int
	targetUndo []controlTargetUndo

	declared     map[string]int
	declaredUndo []controlDeclaredUndo
}

type controlBinder struct {
	functions []controlFunction
	blocks    []controlBlock
	labels    []controlLabel
	labelAt   map[*ast.LabelStmt]int
	pending   map[int][]controlGoto
	resolved  map[*ast.GotoStmt]*ast.LabelStmt
	issues    []ControlIssue
}

func (c *controlBinder) enterFunction() {
	c.functions = append(c.functions, controlFunction{})
}

func (c *controlBinder) leaveFunction() {
	if len(c.functions) == 0 {
		return
	}
	c.functions = c.functions[:len(c.functions)-1]
}

func (c *controlBinder) function() *controlFunction {
	if len(c.functions) == 0 {
		return nil
	}
	return &c.functions[len(c.functions)-1]
}

func (c *controlBinder) pushBlock() {
	fn := c.function()
	if fn == nil {
		return
	}
	block := len(c.blocks)
	c.blocks = append(c.blocks, controlBlock{
		restoreLocals: len(fn.locals),
		labelBase:     len(fn.locals),
		currentLocal:  len(fn.locals),
		targetMark:    len(fn.targetUndo),
		declaredMark:  len(fn.declaredUndo),
	})
	fn.blocks = append(fn.blocks, block)
}

func (c *controlBinder) popBlock() {
	fn := c.function()
	if fn == nil || len(fn.blocks) == 0 {
		return
	}
	blockIndex := fn.blocks[len(fn.blocks)-1]
	block := c.blocks[blockIndex]
	for i := len(fn.targetUndo) - 1; i >= block.targetMark; i-- {
		undo := fn.targetUndo[i]
		if undo.existed {
			fn.targets[undo.name] = undo.prior
		} else {
			delete(fn.targets, undo.name)
		}
	}
	fn.targetUndo = fn.targetUndo[:block.targetMark]
	for i := len(fn.declaredUndo) - 1; i >= block.declaredMark; i-- {
		undo := fn.declaredUndo[i]
		if undo.existed {
			fn.declared[undo.name] = undo.prior
		} else {
			delete(fn.declared, undo.name)
		}
	}
	fn.declaredUndo = fn.declaredUndo[:block.declaredMark]
	fn.locals = fn.locals[:block.restoreLocals]
	fn.blocks = fn.blocks[:len(fn.blocks)-1]
}

func (c *controlBinder) define(id symbol.ID) {
	fn := c.function()
	if fn == nil || len(fn.blocks) == 0 || id == 0 {
		return
	}
	fn.locals = append(fn.locals, id)
	block := fn.blocks[len(fn.blocks)-1]
	c.blocks[block].currentLocal = len(fn.locals)
}

func (c *controlBinder) indexLabels(stmts []ast.Stmt, resetAtEnd bool) {
	fn := c.function()
	if fn == nil || len(fn.blocks) == 0 {
		return
	}
	blockIndex := fn.blocks[len(fn.blocks)-1]
	block := &c.blocks[blockIndex]
	if block.indexed {
		return
	}
	block.indexed = true
	// Parameters and loop variables are installed after the lexical block is
	// pushed but before its statement list begins. They are therefore part of
	// the label-entry baseline even though popBlock must still restore beneath
	// them when the body ends.
	block.labelBase = len(fn.locals)
	block.currentLocal = len(fn.locals)

	lastNonLabel := len(stmts) - 1
	for lastNonLabel >= 0 {
		label, ok := stmts[lastNonLabel].(*ast.LabelStmt)
		if !ok || label == nil {
			break
		}
		lastNonLabel--
	}
	var direct map[string]int
	for index, item := range stmts {
		label, ok := item.(*ast.LabelStmt)
		if !ok || label == nil {
			continue
		}
		if label.Name == "" {
			continue
		}
		labelIndex := len(c.labels)
		c.labels = append(c.labels, controlLabel{
			stmt:       label,
			block:      blockIndex,
			active:     -1,
			endOfBlock: resetAtEnd && index > lastNonLabel,
		})
		if c.labelAt == nil {
			c.labelAt = make(map[*ast.LabelStmt]int)
		}
		c.labelAt[label] = labelIndex
		if direct == nil {
			direct = make(map[string]int)
		}
		if _, duplicate := direct[label.Name]; !duplicate {
			direct[label.Name] = labelIndex
		}
	}
	if len(direct) == 0 {
		return
	}
	if fn.targets == nil {
		fn.targets = make(map[string]int, len(direct))
	}
	for name, label := range direct {
		prior, existed := fn.targets[name]
		fn.targetUndo = append(fn.targetUndo, controlTargetUndo{
			name: name, prior: prior, existed: existed,
		})
		fn.targets[name] = label
	}
}

func (c *controlBinder) visitLabel(stmt *ast.LabelStmt) {
	fn := c.function()
	if fn == nil || stmt == nil {
		return
	}
	order := c.issueSlot()
	labelIndex, known := c.labelAt[stmt]
	if !known || labelIndex < 0 || labelIndex >= len(c.labels) {
		c.addIssue(order, ControlIssue{
			Kind: ControlIssueInvalidLabel, Label: stmt,
		})
		return
	}
	label := &c.labels[labelIndex]
	label.visited = true
	if label.endOfBlock {
		label.active = c.blocks[label.block].labelBase
	} else {
		label.active = len(fn.locals)
	}

	if prior, duplicate := fn.declared[stmt.Name]; duplicate {
		c.addIssue(order, ControlIssue{
			Kind: ControlIssueDuplicateLabel, Label: stmt,
			Previous: c.labels[prior].stmt,
		})
	} else {
		if fn.declared == nil {
			fn.declared = make(map[string]int)
		}
		fn.declaredUndo = append(fn.declaredUndo, controlDeclaredUndo{name: stmt.Name})
		fn.declared[stmt.Name] = labelIndex
	}

	for _, pending := range c.pending[labelIndex] {
		c.resolveGoto(fn, pending, labelIndex)
	}
	delete(c.pending, labelIndex)
}

func (c *controlBinder) visitGoto(stmt *ast.GotoStmt) {
	fn := c.function()
	if fn == nil || stmt == nil {
		return
	}
	order := c.issueSlot()
	if stmt.Label == "" {
		c.addIssue(order, ControlIssue{
			Kind: ControlIssueInvalidGoto, Goto: stmt,
		})
		return
	}
	labelIndex, found := fn.targets[stmt.Label]
	if !found {
		c.addIssue(order, ControlIssue{
			Kind: ControlIssueUndefinedLabel, Goto: stmt,
		})
		return
	}
	label := c.labels[labelIndex]
	pending := controlGoto{
		stmt:      stmt,
		effective: c.blocks[label.block].currentLocal,
		order:     order,
	}
	if label.visited {
		c.resolveGoto(fn, pending, labelIndex)
		return
	}
	if c.pending == nil {
		c.pending = make(map[int][]controlGoto)
	}
	c.pending[labelIndex] = append(c.pending[labelIndex], pending)
}

func (c *controlBinder) resolveGoto(fn *controlFunction, pending controlGoto, labelIndex int) {
	label := c.labels[labelIndex]
	if pending.effective < label.active {
		var entered symbol.ID
		if pending.effective >= 0 && pending.effective < len(fn.locals) {
			entered = fn.locals[pending.effective]
		}
		c.addIssue(pending.order, ControlIssue{
			Kind: ControlIssueGotoEntersLocal, Goto: pending.stmt,
			Label: label.stmt, Local: entered,
		})
		return
	}
	// A direct target is published only after both visibility and active-local
	// legality have been proved.
	// Result owns this map; controlBinder never exposes its transient indexes.
	if c.resolved == nil {
		c.resolved = make(map[*ast.GotoStmt]*ast.LabelStmt)
	}
	c.resolved[pending.stmt] = label.stmt
}

func (c *controlBinder) addIssue(order int, issue ControlIssue) {
	if order < 0 || order >= len(c.issues) {
		return
	}
	c.issues[order] = issue
}

func (c *controlBinder) issueSlot() int {
	order := len(c.issues)
	c.issues = append(c.issues, ControlIssue{})
	return order
}

func (c *controlBinder) finish(result *Result) {
	if result == nil {
		return
	}
	for _, gotos := range c.pending {
		for _, pending := range gotos {
			c.addIssue(pending.order, ControlIssue{
				Kind: ControlIssueUndefinedLabel, Goto: pending.stmt,
			})
		}
	}
	c.pending = nil
	for _, issue := range c.issues {
		if issue.Kind != 0 {
			result.controlIssues = append(result.controlIssues, issue)
		}
	}
	if len(c.resolved) != 0 {
		result.gotoTargets = make(map[*ast.GotoStmt]*ast.LabelStmt, len(c.resolved))
	}
	for stmt, target := range c.resolved {
		result.gotoTargets[stmt] = target
	}
}
