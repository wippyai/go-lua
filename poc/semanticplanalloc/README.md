# Packed symbolic transformer representation POC

This isolated package tests a representation change only; production does not
import it.

The Stage-1 `semanticplan` representation duplicates owned `Path` values in
every lane effect and materializes rebased `Path`/`BoundEffect` slices at every
call. This POC instead uses:

- two immutable, transformer-owned term records and one packed suffix arena;
- compact effect cells containing term IDs and catalog lane ordinals;
- dense root/value binding slots rather than formatted `PathKey` maps;
- a borrowed `PathView` (base plus suffix) consumed through a cursor, avoiding
  construction of concatenated paths and result slices;
- explicit lane coverage: any new catalog lane without an adapter makes the
  transformer contextual, preserving modular fail-closed behavior.

The cursor is useful only if future executable lane adapters resolve `PathView`
directly into their own keys. Calling `Path.Key` or materializing a `Path` in
each adapter would reintroduce most of the avoided allocation. A production
design should resolve each distinct term once per instantiation, then share the
resolved lane-neutral address/key handles across all effect cells.
