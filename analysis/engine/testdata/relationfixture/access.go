package testfixture

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Base is the exact empty mounted database root.
func (fixture Fixture) Base() database.Version { return fixture.base }

// LeftRoot contains only the two left committed rows. It is the canonical
// NoSelection input when paired with RightReader(LeftRoot()).
func (fixture Fixture) LeftRoot() database.Version { return fixture.left }

// BothRoot contains both relations and is the positive correspondence root.
func (fixture Fixture) BothRoot() database.Version { return fixture.both }

// DependencyLeft is the owner-issued schedule identity for the fixture's
// input expression. The dependency is sealed with the expression and is
// exposed only as an identity for solver laws.
func (fixture Fixture) DependencyLeft() model.DependencyID { return fixture.leftDependency }

// DependencyRight is the independent schedule identity for the right Input
// expression. Keeping both identities in the same fixture lets solver laws
// prove that a left publication does not wake the right query.
func (fixture Fixture) DependencyRight() model.DependencyID { return fixture.rightDependency }

// DependencyComplete is the owner-issued schedule identity for the fixture's
// complete-input expression.
func (fixture Fixture) DependencyComplete() model.DependencyID {
	return fixture.completeDependency
}

// DependencyTwoScalarApply is the owner-issued schedule identity for the
// existing two-child scalar Apply expression.
func (fixture Fixture) DependencyTwoScalarApply() model.DependencyID {
	return fixture.twoScalarApplyDependency
}

// TwoScalarApplyObservation is the schema-sealed observation family whose
// parent population is left and whose output extent is apply. It is used by
// observation laws to redeem the existing NoSelection worker without
// manufacturing an application or a second descriptor.
func (fixture Fixture) TwoScalarApplyObservation() identity.ContentID {
	return fixture.twoScalarApplyObservation
}

// BaseToLeftDelta is the exact committed transition that publishes the two
// left rows. It is a read-only solver-law seam; callers cannot forge or
// submit a delta through this accessor.
func (fixture Fixture) BaseToLeftDelta() (database.Delta, bool) {
	if !fixture.leftDelta.Available() || !fixture.leftDelta.Base().Same(fixture.base) || !fixture.leftDelta.Next().Same(fixture.left) {
		return database.Delta{}, false
	}
	return fixture.leftDelta, true
}

// LeftToBothDelta is the exact committed transition that publishes the two
// right rows after the left root. It is intentionally only an immutable
// observation of the fixture's existing publication, not a second writer.
func (fixture Fixture) LeftToBothDelta() (database.Delta, bool) {
	if !fixture.rightDelta.Available() || !fixture.rightDelta.Base().Same(fixture.left) || !fixture.rightDelta.Next().Same(fixture.both) {
		return database.Delta{}, false
	}
	return fixture.rightDelta, true
}

func (fixture Fixture) Mounted() witness.Mounted               { return fixture.mounted }
func (fixture Fixture) Geometry() geometry.Geometry            { return fixture.view }
func (fixture Fixture) RelationLeft() model.RelationID         { return fixture.leftRelation }
func (fixture Fixture) RelationRight() model.RelationID        { return fixture.rightRelation }
func (fixture Fixture) KeyLeft() model.KeyID                   { return fixture.leftKey }
func (fixture Fixture) KeyLeftCoordinate() model.KeyID         { return fixture.leftCoordinateKey }
func (fixture Fixture) KeyRight() model.KeyID                  { return fixture.rightKey }
func (fixture Fixture) RelationApply() model.RelationID        { return fixture.applyRelation }
func (fixture Fixture) KeyApply() model.KeyID                  { return fixture.applyKey }
func (fixture Fixture) ApplyValueColumn() model.ColumnID       { return fixture.applyValue }
func (fixture Fixture) ApplyFactColumn() model.ColumnID        { return fixture.applyFact }
func (fixture Fixture) KeyColumnsLeft() [2]model.ColumnID      { return fixture.leftKeys }
func (fixture Fixture) KeyColumnsRight() [2]model.ColumnID     { return fixture.rightKeys }
func (fixture Fixture) PayloadColumnsLeft() [2]model.ColumnID  { return fixture.leftPayload }
func (fixture Fixture) PayloadColumnsRight() [2]model.ColumnID { return fixture.rightPayload }
func (fixture Fixture) RowsLeft() [2]model.RowID               { return fixture.leftRows }
func (fixture Fixture) RowsRight() [2]model.RowID              { return fixture.rightRows }
func (fixture Fixture) RowApply() model.RowID                  { return fixture.applyRow }
func (fixture Fixture) LayoutLeftKey() arrangement.Layout      { return fixture.leftKeyLayout }
func (fixture Fixture) LayoutRightKey() arrangement.Layout     { return fixture.rightKeyLayout }
func (fixture Fixture) LayoutLeftPayload() arrangement.Layout  { return fixture.leftValueLayout }
func (fixture Fixture) LayoutRightPayload() arrangement.Layout {
	return fixture.rightValueLayout
}

// LayoutInput is the exact relation-directory layout bound to the mounted
// Input expression. It remains distinct from every delivered vector layout
// over the same relation.
func (fixture Fixture) LayoutInput() arrangement.Layout      { return fixture.inputLayout }
func (fixture Fixture) LayoutRightInput() arrangement.Layout { return fixture.rightInputLayout }
func (fixture Fixture) LayoutApplyFact() arrangement.Layout  { return fixture.applyFactLayout }
func (fixture Fixture) Scratch() *store.ReadScratch          { return fixture.scratch }

// LeftInputNode redeems the compiler-issued left Input producer. Operators
// use this node's sealed range authority to construct upstream tuple batches;
// callers cannot choose a replacement layout or range identity.
func (fixture Fixture) LeftInputNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.leftExpression)
}

// RightInputNode redeems the compiler-issued right Input producer. Keeping
// this beside LeftInputNode ensures tuple laws obtain both range authorities
// from the sealed expression plan rather than manufacturing a range in the
// test package.
func (fixture Fixture) RightInputNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.rightExpression)
}

// JoinNode redeems the compiler-issued correspondence operator. Join laws
// must consume this exact mounted binding; reconstructing one from a broad
// layout would erase the declared contract coordinates.
func (fixture Fixture) JoinNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.payloadExpression)
}

// ProjectNode redeems the compiler-issued projection whose source is the
// left relation and whose destination/key layout is the right relation. The
// operator law consumes this sealed binding; it must not reconstruct the
// mapping or target key from fixture fields.
func (fixture Fixture) ProjectNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.projectExpression)
}

// SelectNode redeems the compiler-issued scope selection. Differential laws
// must consume this exact mounted binding; reconstructing a SelectBinding would
// bypass the fixture's sealed expression and scope authority.
func (fixture Fixture) SelectNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.selectExpression)
}

// GroupNode redeems the compiler-issued key grouping expression. The returned
// node owns both the key layout and the producer range authority.
func (fixture Fixture) GroupNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.groupExpression)
}

// MergeNode redeems the compiler-issued keyed merge expression. Differential
// laws use its sealed key and range instead of constructing a private binding.
func (fixture Fixture) MergeNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.mergeExpression)
}

// CompleteBinding redeems the compiler-issued Complete binding for the
// fixture's left denominator. The binding owns the output range authority.
func (fixture Fixture) CompleteBinding() (arrangement.CompleteBinding, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.CompleteBinding{}, false
	}
	node, ok := execution.Entry(fixture.completeExpression)
	if !ok {
		return arrangement.CompleteBinding{}, false
	}
	return node.Complete()
}

// Readers are redeemed from exact committed roots and the fixture's mounted
// layout/geometry pair. This package does not implement or fake Reader.
func (fixture Fixture) ReaderLeftKey(root database.Version) (read.Reader, bool) {
	return read.Bind(root, fixture.leftKeyLayout, fixture.view, fixture.scratch)
}

func (fixture Fixture) ReaderRightKey(root database.Version) (read.Reader, bool) {
	return read.Bind(root, fixture.rightKeyLayout, fixture.view, fixture.scratch)
}

// ReaderLeftInput and ReaderRightInput are the exact relation-wide readers
// bound to the sealed Input producer layouts. Tuple input laws use these
// readers so their batches carry the producer-issued range proof.
func (fixture Fixture) ReaderLeftInput(root database.Version) (read.Reader, bool) {
	return read.Bind(root, fixture.inputLayout, fixture.view, fixture.scratch)
}

func (fixture Fixture) ReaderRightInput(root database.Version) (read.Reader, bool) {
	return read.Bind(root, fixture.rightInputLayout, fixture.view, fixture.scratch)
}

func (fixture Fixture) ReaderLeftPayload(root database.Version) (read.Reader, bool) {
	return read.Bind(root, fixture.leftValueLayout, fixture.view, fixture.scratch)
}

func (fixture Fixture) ReaderRightPayload(root database.Version) (read.Reader, bool) {
	return read.Bind(root, fixture.rightValueLayout, fixture.view, fixture.scratch)
}

// OverlapScopes are individually non-empty normalized scopes whose
// conjunction is non-empty. DisjointScopes are valid scopes whose conjunction
// is contradictory and is therefore refused by Geometry.
func (fixture Fixture) OverlapScopes() (witness.Scope, witness.Scope) {
	return fixture.scopes.overlapLeft, fixture.scopes.overlapRight
}

func (fixture Fixture) DisjointScopes() (witness.Scope, witness.Scope) {
	return fixture.scopes.disjointLeft, fixture.scopes.disjointRight
}
