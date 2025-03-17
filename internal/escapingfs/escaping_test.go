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
