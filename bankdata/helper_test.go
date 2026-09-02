package bankdata

import (
	"bytes"
	"compress/gzip"
	"testing"
)

// newGzipLines builds a gzipped stream from the given lines, used to feed
// ReadSnapshot inputs that WriteSnapshot would never produce.
func newGzipLines(t *testing.T, lines ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	for _, l := range lines {
		if _, err := gz.Write([]byte(l + "\n")); err != nil {
			t.Fatalf("write gzip line: %v", err)
		}
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}
