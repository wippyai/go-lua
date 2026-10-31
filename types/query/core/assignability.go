package core

import (
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/subtype"
)

// AssignabilityKey attaches the assignability mode of a check session to its
// query context.
var AssignabilityKey = db.NewAttachmentKey[subtype.Assignability]("core.Assignability")

// AssignabilityOf returns the assignability mode attached to ctx, or
// subtype.Gradual when none is attached.
func AssignabilityOf(ctx *db.QueryContext) subtype.Assignability {
	mode, _ := db.Attached(ctx, AssignabilityKey)
	return mode
}

// WithAssignability attaches mode to ctx.
func WithAssignability(ctx *db.QueryContext, mode subtype.Assignability) {
	db.Attach(ctx, AssignabilityKey, mode)
}
