// Copyright IBM Corp. 2018, 2025
// SPDX-License-Identifier: MPL-2.0

package slug

import (
	"fmt"
	"os"

	"github.com/hashicorp/go-slug/internal/ignorefiles"
)

func parseIgnoreFile(rootPath string) *ignorefiles.Ruleset {
	// Use os.Root for secure file access within the root directory
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening root directory %q, default exclusions will apply: %v \n", rootPath, err)
		return ignorefiles.DefaultRuleset
	}
	defer func() { _ = root.Close() }()

	// Look for .terraformignore at our root path/src
	file, err := root.Open(".terraformignore")

	// If there's any kind of file error, punt and use the default ignore patterns
	if err != nil {
		// Only show the error debug if an error *other* than IsNotExist
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error reading .terraformignore, default exclusions will apply: %v \n", err)
		}
		return ignorefiles.DefaultRuleset
	}

	defer func() { _ = file.Close() }()

	ret, err := ignorefiles.ParseIgnoreFileContent(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading .terraformignore, default exclusions will apply: %v \n", err)
		return ignorefiles.DefaultRuleset
	}

	return ret
}

func matchIgnoreRules(path string, ruleset *ignorefiles.Ruleset) ignorefiles.ExcludesResult {
	// Ruleset.Excludes explicitly allows ignoring its error, in which
	// case we are ignoring any individual invalid rules in the set
	// but still taking all of the others into account.
	ret, _ := ruleset.Excludes(path)
	return ret
}
