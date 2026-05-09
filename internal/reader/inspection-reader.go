package reader

import "io"

// Track the first N bytes read from the file, to be able to tell apart file
// replacement from appending when the file grows.
const headerBytesCapacity = 1024

// Pass-through reader that counts the number of bytes read.
type inspectionReader struct {
	base       io.Reader
	bytesCount int64

	headerBytes []byte

	endedWithNewline bool
}

func (r *inspectionReader) Read(p []byte) (n int, err error) {
	n, err = r.base.Read(p)
	r.bytesCount += int64(n)

	if err != nil {
		return
	}

	if n > 0 {
		r.endedWithNewline = p[n-1] == '\n'

		if len(r.headerBytes) < headerBytesCapacity {
			wanted := n
			if len(r.headerBytes)+wanted > headerBytesCapacity {
				wanted = headerBytesCapacity - len(r.headerBytes)
			}
			r.headerBytes = append(r.headerBytes, p[:wanted]...)
		}
	} else {
		r.endedWithNewline = false
	}

	return
}
