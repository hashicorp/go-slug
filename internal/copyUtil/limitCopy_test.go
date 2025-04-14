package copyUtil

import (
	"bytes"
	"strings"
	"testing"
)

func TestCopyWithLimit_UnderLimit(t *testing.T) {
	srcData := strings.Repeat("A", 2*chunkSize) // 2 chunks worth of data
	src := strings.NewReader(srcData)
	var dst bytes.Buffer

	err := CopyWithLimit(&dst, src)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if dst.String() != srcData {
		t.Fatalf("data mismatch: expected %d bytes, got %d bytes", len(srcData), dst.Len())
	}
}

func TestCopyWithLimit_OverLimit(t *testing.T) {
	srcData := strings.Repeat("B", chunkSize*maxChunks+1) // Just over the limit
	src := strings.NewReader(srcData)
	var dst bytes.Buffer

	err := CopyWithLimit(&dst, src)
	if err == nil {
		t.Fatal("expected error due to copy limit exceeded, got nil")
	}

	if !strings.Contains(err.Error(), "copy limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCopyWithLimit_EOFBeforeChunk(t *testing.T) {
	srcData := "short data"
	src := strings.NewReader(srcData)
	var dst bytes.Buffer

	err := CopyWithLimit(&dst, src)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if dst.String() != srcData {
		t.Fatalf("data mismatch: expected %q, got %q", srcData, dst.String())
	}
}

func TestCopyWithLimit_EmptySource(t *testing.T) {
	src := strings.NewReader("")
	var dst bytes.Buffer

	err := CopyWithLimit(&dst, src)
	if err != nil {
		t.Fatalf("expected no error on empty source, got: %v", err)
	}

	if dst.Len() != 0 {
		t.Fatalf("expected empty output, got %d bytes", dst.Len())
	}
}
