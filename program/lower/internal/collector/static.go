// Package collector's Static construction capability is implemented by the
// relation-complete static_rows and static_freeze files.  Keeping row capture
// and Source-first materialization separate makes the lifecycle boundary
// visible without splitting one semantic relation horizontally.
package collector
