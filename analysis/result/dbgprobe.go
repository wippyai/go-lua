package result

import "sync/atomic"

// dbgprobe.go carries the structural counter for the native summary join.
//
// Program publishes no inverse index over its occurrence family, so a join
// that resolves an occurrence by identity per joining row costs the product
// of the two family widths. The counted quantity is the Program row read:
// deriving the row's catalog address and authenticating the row against its
// own identity. It is the join's unit of work, and it is a count rather than
// a duration, so the law that bounds it reads the same on a loaded machine as
// on an idle one.
//
// Result projections are built per artifact and artifacts project
// concurrently, so the counter is atomic.
var dbgNativeJoinRowReads atomic.Uint64

// dbgNativeJoinRowRead records one Program row the native summary join read.
func dbgNativeJoinRowRead() { dbgNativeJoinRowReads.Add(1) }

// dbgNativeJoinRowReadCount reports the rows the native summary join has read
// since the last reset.
func dbgNativeJoinRowReadCount() uint64 { return dbgNativeJoinRowReads.Load() }

// dbgNativeJoinRowReadReset clears the accumulated native summary join reads.
func dbgNativeJoinRowReadReset() { dbgNativeJoinRowReads.Store(0) }
