package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileWindow_LargeFileTruncatesNotErrors(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, 400*1024)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := ReadFileWindow(dir, "big.txt", 0, 256*1024)
	if err != nil {
		t.Fatalf("large file must not error: %v", err)
	}
	if !res.Truncated {
		t.Fatal("expected truncated")
	}
	if res.TotalSize != 400*1024 {
		t.Fatalf("total=%d", res.TotalSize)
	}
	if res.NextOffset <= 0 {
		t.Fatalf("next offset=%d", res.NextOffset)
	}
	if !strings.Contains(res.Content, "truncated") {
		t.Fatalf("expected continuation hint: %q", res.Content[len(res.Content)-80:])
	}
	// Second chunk continues.
	res2, err := ReadFileWindow(dir, "big.txt", res.NextOffset, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Offset != res.NextOffset {
		t.Fatalf("offset=%d want %d", res2.Offset, res.NextOffset)
	}
}

func TestReadFileWindow_BinaryNotDumped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.bin"), []byte{0, 1, 2, 3, 4, 5}, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := ReadFileWindow(dir, "x.bin", 0, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Binary {
		t.Fatal("expected binary flag")
	}
	if strings.Contains(res.Content, string([]byte{0, 1, 2})) {
		t.Fatal("must not dump binary bytes into content")
	}
}

func TestReadFileText_NoHardRejectOver256k(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, 300*1024)
	for i := range big {
		big[i] = 'a'
	}
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), big, 0o600)
	s, err := ReadFileText(dir, "a.txt", 256*1024)
	if err != nil {
		t.Fatalf("must not hard-reject: %v", err)
	}
	if !strings.Contains(s, "truncated") {
		t.Fatalf("want truncate hint, got len=%d", len(s))
	}
}
