// INPUT: 文件与页号。
// OUTPUT: ReadPage、Rows：定长字节行。
// POS: 十六进制视图的数据。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package hexview

import (
	"fmt"
	"io"
	"os"
)

const RowBytes = 16
const PageBytes = 64 << 10

// ReadPage reads one bounded page at offset without retaining an open file.
func ReadPage(path string, offset, size int64) ([]byte, error) {
	if offset < 0 || offset%PageBytes != 0 || size < 0 || offset >= size {
		return nil, fmt.Errorf("invalid page offset")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	n := min(int64(PageBytes), size-offset)
	data := make([]byte, int(n))
	got, err := f.ReadAt(data, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if got != len(data) {
		return nil, io.ErrUnexpectedEOF
	}
	return data, nil
}

// Rows returns the number of rows needed to show size bytes.
func Rows(size int64) int64 {
	if size <= 0 {
		return 0
	}
	return 1 + (size-1)/RowBytes
}
