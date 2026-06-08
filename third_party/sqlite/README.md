# Vendored `sqlite3.h`

`sqlite3.h` here is the official SQLite amalgamation header, vendored so the
sqlite-vec cgo binding can resolve its `#include "sqlite3.h"` on every toolchain.

## Why

`github.com/asg017/sqlite-vec-go-bindings/cgo` compiles `sqlite-vec.c` with
`-DSQLITE_CORE`, whose header does a bare `#include "sqlite3.h"`. SQLite itself is
statically compiled by `github.com/mattn/go-sqlite3`, but that package names its
header `sqlite3-binding.h`, so a plain `sqlite3.h` is only found via a *system*
header. That is fragile:

- **zig cross-compiles (linux targets in the release)** have no system
  `sqlite3.h` → `fatal error: 'sqlite3.h' file not found`.
- **macOS** falls back to Apple's SDK copy, which marks `sqlite3_auto_extension`
  deprecated → build noise.

Putting this header on the include path (`CGO_CFLAGS=-I.../third_party/sqlite`,
wired in the `Makefile` and both workflows) makes every toolchain resolve the
same, correct header.

## Provenance & updating

Copied verbatim from `github.com/mattn/go-sqlite3`'s `sqlite3-binding.h` so the
declarations match the statically linked SQLite exactly.

- SQLite version: **3.53.2** (matches go-sqlite3 v1.14.45)

When bumping `mattn/go-sqlite3`, refresh this file:

```sh
cp "$(go list -m -f '{{.Dir}}' github.com/mattn/go-sqlite3)/sqlite3-binding.h" \
   third_party/sqlite/sqlite3.h
```

SQLite is public domain; see THIRD_PARTY.md.
