// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package escapingfs

import (
	"path/filepath"
	"testing"
)

func TestTargetWithinRoot(t *testing.T) {
	tempDir := "/tmp/root" // Example root directory
	tests := []struct {
		name     string
		root     string
		target   string
		expected bool
	}{
		{
			name:     "Target inside root",
			root:     tempDir,
			target:   filepath.Join(tempDir, "subdir", "file.txt"),
			expected: true,
		},
		{
			name:     "Target is root itself",
			root:     tempDir,
			target:   tempDir,
			expected: true,
		},
		{
			name:     "Target outside root",
			root:     tempDir,
			target:   "/tmp/otherdir/file.txt",
			expected: false,
		},
		{
			name:     "Target is parent of root",
			root:     filepath.Join(tempDir, "subdir"),
			target:   tempDir,
			expected: false,
		},
		{
			name:     "Target within root but target filename starts with ..",
			root:     tempDir,
			target:   filepath.Join(tempDir, "..file.txt"),
			expected: true,
		},
		{
			name:     "Target within root but target directory starts with ..",
			root:     tempDir,
			target:   filepath.Join(tempDir, "..subdir", "file.txt"),
			expected: true,
		},
		{
			name:     "Path with space",
			root:     tempDir,
			target:   filepath.Join(tempDir, "sub dir", "file.txt"),
			expected: true,
		},
		{
			name:     "Path with space and traversal",
			root:     tempDir,
			target:   filepath.Join(tempDir, "  ../../../", "file.txt"),
			expected: false,
		},
		{
			name:     "Path traversal start with and followed by ../",
			root:     tempDir,
			target:   filepath.Join(tempDir, "./..", "sensitive", "file.txt"),
			expected: false,
		},
		{
			name:     "Path traversal with ../ at beginning",
			root:     tempDir,
			target:   filepath.Join(tempDir, "..", "sensitive", "file.txt"),
			expected: false,
		},
		{
			name:     "Path traversal with multiple ../",
			root:     tempDir,
			target:   filepath.Join(tempDir, "..", "..", "etc", "passwd"),
			expected: false,
		},
		{
			name:     "Empty target path",
			root:     tempDir,
			target:   "",
			expected: false,
		},
		{
			name:     "Relative root path",
			root:     ".",
			target:   "./file.txt",
			expected: true,
		},
		{
			name:     "Relative target escaping relative root",
			root:     "./subdir",
			target:   "../file.txt",
			expected: false,
		},
		{
			name:     "Root with trailing slash",
			root:     tempDir + "/",
			target:   filepath.Join(tempDir, "file.txt"),
			expected: true,
		},
		{
			name:     "Target with redundant path elements",
			root:     tempDir,
			target:   filepath.Join(tempDir, "subdir", "..", "file.txt"),
			expected: true,
		},
		{
			name:     "Target with redundant path elements escaping",
			root:     tempDir,
			target:   filepath.Join(tempDir, "..", tempDir, "file.txt"),
			expected: false, // Contains .. so should be rejected
		},
		{
			name:     "Same path with different representations",
			root:     "/tmp/root",
			target:   "/tmp/root/./file.txt",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TargetWithinRoot(tt.root, tt.target)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
