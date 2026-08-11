// Package transaction provides the small, fail-closed mutation boundary for
// flashrefactor.  It owns no refactoring semantics: callers provide an exact
// rendered file set, and this package either installs precisely that set and
// verifies it, or restores the repository byte-for-byte.
//
// Each regular-file installation is staged, synced, and renamed in its own
// directory. A successful Run is atomic per file and rollback is attempted
// after every observed failure. Before the first rename, an fsynced journal
// under .flashrefactor/transaction records every input preimage, expected
// output, and transaction state. Filesystems do not offer a portable atomic
// commit across several directories, so a crash can leave a mixed revision.
// That state is never guessed or replayed automatically: callers explicitly
// Inspect then Rollback, or Complete a fully applied state with an in-lease
// postflight.
//
// Run also owns an exclusive, root-local lease from snapshot through output
// verification. A second cooperative workbench transaction fails closed. A
// crash can leave that lease behind, which intentionally requires explicit
// recovery rather than guessing whether another workbench still owns it.
// Read-only inputs are observed under the same lease and supplied as immutable
// Preimage values to in-lease guards.
// Existing hard-linked inputs are rejected: replacing one path would otherwise
// silently break an inode alias outside the declared transaction.
package transaction
