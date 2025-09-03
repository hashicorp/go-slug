// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package escapingfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func TargetWithinRoot(root string, target string) (bool, error) {
	if len(target) == 0 || len(root) == 0 {
		return false, nil
	}

	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)

	rel, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return false, fmt.Errorf("couldn't find relative path: %w", err)
	}

	rel = filepath.Clean(rel)

	components := strings.Split(rel, string(os.PathSeparator))

	for _, component := range components {
		if component == ".." {
			return false, nil
		}
	}

	return true, nil
}
