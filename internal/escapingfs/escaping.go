package escapingfs

import (
	"fmt"
	"path/filepath"
	"strings"
)

func TargetWithinRoot(root string, target string) (bool, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false, fmt.Errorf("couldn't find relative path : %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return false, nil
	}
	return true, nil
}
