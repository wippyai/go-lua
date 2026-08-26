// Package delta redeems one Later-root work item through the mount-owned
// occurrence derivative.
//
// A Later root is not a full evaluation with a different input version.  The
// session binds the successor Reader only for sealed sibling accesses and
// binds ChangeReaders for Input leaves.  Each occurrence path is then
// ascended from its leaf through the sealed frames in reverse order.  This
// package deliberately does not import eval/step, reopen an algebra
// expression, or maintain a result cache.
package delta
