module github.com/walles/moor/v2

// Go version policy: No newer than the most recently end-of-lifed Go release,
// so distro packagers building from source aren't forced onto a toolchain their
// repos don't have yet.
//
// Once this reaches 1.26, the whole internal/ptr package can be replaced with
// Go's builtin new(expr) instead.
go 1.25.0

require (
	github.com/adrg/xdg v0.5.3
	github.com/alecthomas/chroma/v2 v2.22.0
	github.com/charlievieth/strcase v0.0.6
	github.com/creack/pty v1.1.24
	github.com/davecgh/go-spew v1.1.1
	github.com/go-enry/go-enry/v2 v2.9.6
	github.com/google/go-cmp v0.7.0
	github.com/klauspost/compress v1.19.1
	github.com/rivo/uniseg v0.4.7
	github.com/sirupsen/logrus v1.9.4
	github.com/ulikunitz/xz v0.5.16
	github.com/walles/twin v0.9.1
	golang.org/x/term v0.45.0
	gotest.tools/v3 v3.5.2
)

require (
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/go-enry/go-oniguruma v1.2.1 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
