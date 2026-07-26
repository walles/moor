#!/bin/bash

set -e -o pipefail

if [[ "$1" == "--bench" || "$1" == "--benchmark" ]]; then
  ./benchmark.sh
  exit 0
fi

# Test that we only pass twin colors to these methods, not numbers
grep -En 'Foreground\([1-9]' ./**/*.go && exit 1
grep -En 'Background\([1-9]' ./**/*.go && exit 1

# Compile test first
echo Building sources...
./build.sh

if [[ -z "${SKIP_LINT}" ]]; then
  # Linting
  echo 'Linting, repro any errors locally using "golangci-lint run"...'
  echo '  Linting without tests...'
  golangci-lint run --tests=false
  echo '  Linting with tests...'
  golangci-lint run --tests=true
else
  echo "Skipping golangci-lint (SKIP_LINT is set)..."
fi

# Unit tests
echo "Running unit tests..."
RACE=-race
if [ "$GOARCH" == "386" ]; then
  # -race is not supported on i386
  RACE=""
fi
go test $RACE -timeout 20s ./...

# Ensure we can cross compile
# NOTE: Make sure this list matches the one in release.sh
echo "Testing cross compilation..."
echo "  Linux i386..."
GOOS=linux GOARCH=386 ./build.sh

# Ref:
echo "  Linux amd64..."
GOOS=linux GOARCH=amd64 ./build.sh

# Ref: https://github.com/walles/moor/issues/122
echo "  Linux arm32..."
GOOS=linux GOARCH=arm ./build.sh

echo "  macOS amd64..."
GOOS=darwin GOARCH=amd64 ./build.sh
echo "  Windows amd64..."
GOOS=windows GOARCH=amd64 ./build.sh

# Build locally so we can do our testing
echo "Doing a local build so we can continue testing..."
./build.sh

# Verify sending the output to a file
RESULT="$(mktemp)"
function cleanup {
  rm -rf "${RESULT}"
}
trap cleanup EXIT

echo Test reading from redirected stdin, writing to redirected stdout...
./moor <cmd/moor/moor.go >"${RESULT}"
diff -u cmd/moor/moor.go "${RESULT}"

echo Test reading from redirected stdin, writing to redirected stdout, with a - file name...
./moor - <cmd/moor/moor.go >"${RESULT}"
diff -u cmd/moor/moor.go "${RESULT}"

echo Test redirecting a file by name into file by redirecting stdout...
./moor cmd/moor/moor.go >"${RESULT}"
diff -u cmd/moor/moor.go "${RESULT}"

# Ref: https://github.com/walles/moor/issues/187
echo Test redirecting multiple files by name into redirected stdout...
./moor cmd/moor/moor.go cmd/moor/moor.go >"${RESULT}"
diff -u <(cat cmd/moor/moor.go cmd/moor/moor.go) "${RESULT}"

echo Test redirecting non-existing file by name into redirected stdout...
if ./moor does-not-exist >&/dev/null; then
  echo ERROR: Should have failed on non-existing input file name
  exit 1
fi

echo Testing not crashing with different argument orders...
./moor +123 cmd/moor/moor.go >/dev/null
./moor cmd/moor/moor.go +123 >/dev/null
./moor +123 --trace cmd/moor/moor.go >/dev/null
./moor --trace +123 cmd/moor/moor.go >/dev/null
./moor --trace cmd/moor/moor.go +123 >/dev/null

# We can only do this test if we have a terminal. This means it will be run
# locally but not in CI. Not great, but better than nothing.
#
# Set MOOR_SKIP_INTERACTIVE_TESTS=1 to disable interactive TTY tests.
if [[ "${MOOR_SKIP_INTERACTIVE_TESTS}" != "1" && -t 1 ]]; then
  echo Test auto quitting on single screen...
  echo "  (success)" | ./moor --quit-if-one-screen
fi

echo Test decompressing while piping
# Related to https://github.com/walles/moor/issues/177
./moor sample-files/compressed.txt.gz | grep compressed >/dev/null

echo Test --version...
./moor --version >/dev/null # Should exit with code 0
diff -u <(./moor --version) <(git describe --tags --dirty --always)

echo "Linting man page..."
# -T lint enables the linter
# -W warning ignores "STYLE" hints (like line length) but catches structural issues
mandoc -T lint -W warning moor.1

echo Test that the man page and --help document the same set of options...
MAN_OPTIONS="$(grep -A1 '^\.TP$' moor.1 | grep -E '^\\fB\\-' | cut -d\\ -f4- | sed 's/fR.*//' | sed 's/\\//g')"
MOOR_OPTIONS="$(./moor --help | grep -E '^  -' | cut -d' ' -f3 | grep -v -- -version)"
diff -u <(echo "${MAN_OPTIONS}") <(echo "${MOOR_OPTIONS}")

# FIXME: On unknown command line options, test that help text goes to stderr

./scripts/test-path-help.sh "$(realpath ./moor)"

echo
echo "All tests passed!"
