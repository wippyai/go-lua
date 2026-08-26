// Package input is the tuple boundary for relation scans.
//
// Input redeems one sealed arrangement.InputBinding and lowers rows from the
// exact mounted Values reader into ordered shared tuple batches, one per exact
// row cofiber. ExecuteExtent supplies a complete authenticated denominator
// extent, while ExecuteRow supplies one exact denominator member/cofiber. The
// binding's separate Scan layout authenticates the range; Input never reopens
// a relation schema to discover row columns. It does not expose a second row
// representation, issue identities, flatten range partitions, or retain state.
package input
