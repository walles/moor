package reader

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
	log "github.com/sirupsen/logrus"
	"github.com/ulikunitz/xz"
)

var gzipMagic = []byte{0x1f, 0x8b}
var bzip2Magic = []byte{0x42, 0x5a, 0x68}
var zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}
var xzMagic = []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}

// Closes both a zstd decompressor and whatever it was decompressing.
//
// The zstd decompressor works in goroutines that only its own Close() stops,
// and that Close() returns no error, so it can't be an io.Closer by itself.
//
// The decompressor isn't safe for concurrent use with Close(), so Close()
// must only be called once nothing is concurrently reading from it. To
// unblock a concurrent Read() instead, call interrupt().
type zstdCloser struct {
	decompressor *zstd.Decoder

	// What the decompressor is reading from, nil if we don't own it
	compressed io.Closer

	// Close() and interrupt() can both end up closing compressed; this makes
	// sure that only happens once.
	closeCompressedOnce *sync.Once
}

func newZstdCloser(decompressor *zstd.Decoder, compressed io.Closer) zstdCloser {
	return zstdCloser{
		decompressor:        decompressor,
		compressed:          compressed,
		closeCompressedOnce: &sync.Once{},
	}
}

func (closer zstdCloser) closeCompressed() error {
	if closer.compressed == nil {
		return nil
	}

	var err error
	closer.closeCompressedOnce.Do(func() {
		err = closer.compressed.Close()
	})
	return err
}

func (closer zstdCloser) Close() error {
	closer.decompressor.Close()

	return closer.closeCompressed()
}

// interrupt unblocks a concurrent in-flight Read() by closing what the
// decompressor reads from, without touching the decompressor itself. Unlike
// Close(), this is safe to call while a Read() is in flight.
func (closer zstdCloser) interrupt() error {
	return closer.closeCompressed()
}

// The second return value is the file name with any compression extension removed.
func ZOpen(filename string) (io.ReadCloser, string, error) {
	stream, highlightingFilename, _, err := zOpenWithStat(filename)
	return stream, highlightingFilename, err
}

// zOpenWithStat is ZOpen() plus a stat of the opened file, nil if stat failed.
//
// Stat-ing the opened file rather than its name makes sure the stat is of the
// file we read, even if another file is moved into place in between. On
// Windows, it also makes os.SameFile() compare the ID the file had when we
// opened it, rather than the ID of whatever has the name when os.SameFile() is
// first called.
func zOpenWithStat(filename string) (io.ReadCloser, string, os.FileInfo, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, "", nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		log.Debugf("Failed to stat opened file %s: %s", filename, err)
	}

	stream, highlightingFilename, err := zOpenFile(file, filename)
	if err != nil {
		return nil, "", nil, err
	}

	return stream, highlightingFilename, stat, nil
}

// Decompress file based on its contents, with filename only used for naming the
// result.
//
// Takes ownership of file: closing the returned reader closes it.
func zOpenFile(file *os.File, filename string) (io.ReadCloser, string, error) {
	_, err := file.Seek(0, 0)
	if err != nil {
		// File is not seekable, so we can't probe its contents.
		// https://github.com/walles/moor/issues/385
		return file, filename, nil
	}

	// Read the first 6 bytes to determine the compression type
	firstBytes := make([]byte, 6)
	_, err = file.Read(firstBytes)
	if err != nil {
		if err == io.EOF {
			// File was empty
			return file, filename, nil
		}
		return nil, "", fmt.Errorf("failed to read file: %w", err)
	}

	// Reset file reader to start of file
	_, err = file.Seek(0, 0)
	if err != nil {
		return nil, "", fmt.Errorf("failed to seek to start of file: %w", err)
	}

	switch {
	case bytes.HasPrefix(firstBytes, gzipMagic):
		log.Debugf("File is gzip compressed: %v", filename)
		reader, err := gzip.NewReader(file)
		if err != nil {
			return nil, "", err
		}

		newName := strings.TrimSuffix(filename, ".gz")

		// Ref: https://github.com/walles/moor/issues/194
		if before, ok := strings.CutSuffix(newName, ".tgz"); ok {
			newName = before + ".tar"
		}

		return struct {
			io.Reader
			io.Closer
		}{reader, file}, newName, err

	case bytes.HasPrefix(firstBytes, bzip2Magic):
		log.Debugf("File is bzip2 compressed: %v", filename)
		return struct {
			io.Reader
			io.Closer
		}{bzip2.NewReader(file), file}, strings.TrimSuffix(filename, ".bz2"), nil

	case bytes.HasPrefix(firstBytes, zstdMagic):
		log.Debugf("File is zstd compressed: %v", filename)
		decoder, err := zstd.NewReader(file)
		if err != nil {
			return nil, "", err
		}

		newName := strings.TrimSuffix(filename, ".zst")
		newName = strings.TrimSuffix(newName, ".zstd")

		// zstdCloser is embedded concretely rather than behind io.Closer, so
		// that its interrupt() method (see below) is promoted to this struct
		// too, not just Close().
		return struct {
			io.Reader
			zstdCloser
		}{decoder, newZstdCloser(decoder, file)}, newName, nil

	case bytes.HasPrefix(firstBytes, xzMagic):
		log.Debugf("File is xz compressed: %v", filename)
		xzReader, err := xz.NewReader(file)
		if err != nil {
			return nil, "", err
		}

		return struct {
			io.Reader
			io.Closer
		}{xzReader, file}, strings.TrimSuffix(filename, ".xz"), nil
	}

	log.Debugf("File is assumed to be uncompressed: %v", filename)
	return file, filename, nil
}

// Make closing reader close closer as well.
//
// Decompressors don't close what they read from, so this is what keeps the
// compressed stream closable.
//
// A nil closer gives back the reader untouched, so that closing it stays
// impossible rather than panicking.
func withCloser(reader io.Reader, closer io.Closer) io.Reader {
	if closer == nil {
		return reader
	}

	return struct {
		io.Reader
		io.Closer
	}{reader, closer}
}

// ZReader returns a reader that decompresses the input stream. Any input stream
// compression will be automatically detected. Uncompressed streams will be
// returned as-is.
//
// If the input is an io.Closer then so is the returned reader, and closing that
// closes the input.
//
// Ref: https://github.com/walles/moor/issues/261
func ZReader(input io.Reader) (io.Reader, error) {
	// Kept across the decompressor wrapping below, which would otherwise lose
	// it
	closer, _ := input.(io.Closer)

	// Read the first 6 bytes to determine the compression type
	firstBytes := make([]byte, 6)
	count, err := input.Read(firstBytes)
	if err != nil {
		if err == io.EOF {
			// Stream was empty
			return input, nil
		}
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}
	firstBytes = firstBytes[:count]

	// Reset input reader to start of stream
	input = io.MultiReader(bytes.NewReader(firstBytes), input)

	switch {
	case bytes.HasPrefix(firstBytes, gzipMagic):
		log.Info("Input stream is gzip compressed")
		gzipReader, err := gzip.NewReader(input)
		if err != nil {
			return nil, err
		}

		return withCloser(gzipReader, closer), nil

	case bytes.HasPrefix(firstBytes, zstdMagic):
		log.Info("Input stream is zstd compressed")
		zstdReader, err := zstd.NewReader(input)
		if err != nil {
			return nil, err
		}

		// Not using withCloser(): it stores its closer behind the io.Closer
		// interface, which would only promote Close() and hide interrupt().
		return struct {
			io.Reader
			zstdCloser
		}{zstdReader, newZstdCloser(zstdReader, closer)}, nil

	case bytes.HasPrefix(firstBytes, bzip2Magic):
		log.Info("Input stream is bzip2 compressed")
		return withCloser(bzip2.NewReader(input), closer), nil

	case bytes.HasPrefix(firstBytes, xzMagic):
		log.Info("Input stream is xz compressed")
		xzReader, err := xz.NewReader(input)
		if err != nil {
			return nil, err
		}

		return withCloser(xzReader, closer), nil

	default:
		// No magic numbers matched
		log.Info("Input stream is assumed to be uncompressed")
		return withCloser(input, closer), nil
	}
}
