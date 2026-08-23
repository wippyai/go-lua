package owner

// Coordinate is the heap axis's dense key type: the position a heap key
// occupies in the Factor this owner binds. It is exported because a rule of
// this axis installs the execution family of its own fold, and the plane it
// seals rows on is typed in this key. It carries no capability - it is an
// index, and every value of it this package hands out is one it minted.
type Coordinate uint32
