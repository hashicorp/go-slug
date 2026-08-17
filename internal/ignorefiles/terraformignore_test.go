// Copyright IBM Corp. 2018, 2025
// SPDX-License-Identifier: MPL-2.0

package ignorefiles

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestTerraformIgnore(t *testing.T) {
	// path to directory without .terraformignore
	rs, err := LoadPackageIgnoreRules("testdata/external-dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.rules) != 3 {
		t.Fatal("A directory without .terraformignore should get the default patterns")
	}

	// load the .terraformignore file's patterns
	rs, err = LoadPackageIgnoreRules("testdata/archive-dir")
	if err != nil {
		t.Fatal(err)
	}

	type file struct {
		// the actual path, should be file path format /dir/subdir/file.extension
		path string
		// should match
		match bool
	}
	paths := []file{
		0: {
			path:  ".terraform/",
			match: true,
		},
		1: {
			path:  "included.txt",
			match: false,
		},
		2: {
			path:  ".terraform/foo/bar",
			match: true,
		},
		3: {
			path:  ".terraform/foo/bar/more/directories/so/many",
			match: true,
		},
		4: {
			path:  ".terraform/foo/ignored-subdirectory/",
			match: true,
		},
		5: {
			path:  "baz.txt",
			match: true,
		},
		6: {
			path:  "parent/foo/baz.txt",
			match: true,
		},
		7: {
			path:  "parent/foo/bar.tf",
			match: true,
		},
		8: {
			path:  "parent/bar/bar.tf",
			match: false,
		},
		// baz.txt is ignored, but a file name including it should not be
		9: {
			path:  "something/with-baz.txt",
			match: false,
		},
		10: {
			path:  "something/baz.x",
			match: false,
		},
		// Getting into * patterns
		11: {
			path:  "foo/ignored-doc.md",
			match: true,
		},
		// Should match [a-z] group
		12: {
			path:  "bar/something-a.txt",
			match: true,
		},
		// ignore sub- terraform.d paths...
		13: {
			path:  "some-module/terraform.d/x",
			match: true,
		},
		// ...but not the root one
		14: {
			path:  "terraform.d/",
			match: false,
		},
		15: {
			path:  "terraform.d/foo",
			match: false,
		},
		// We ignore the directory, but a file of the same name could exist
		16: {
			path:  "terraform.d",
			match: false,
		},
		// boop.txt is ignored everywhere...
		17: {
			path:  "baz/boop.txt",
			match: true,
		},
		// ...except in root directory
		18: {
			path:  "boop.txt",
			match: false,
		},
	}
	for i, p := range paths {
		result, err := rs.Excludes(p.path)
		if err != nil {
			t.Errorf("invalid rule syntax when checking %s at index %d", p.path, i)
			continue
		}
		if result.Excluded != p.match {
			t.Fatalf("%s at index %d should be %t", p.path, i, p.match)
		}
	}
}

func TestTerraformIgnoreNoExclusionOptimization(t *testing.T) {
	rs, err := LoadPackageIgnoreRules("testdata/with-exclusion")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.rules) != 7 {
		t.Fatalf("Expected 7 rules, got %d", len(rs.rules))
	}

	// reflects that no negations follow the last rule
	afterValue := false
	for i := len(rs.rules) - 1; i >= 0; i-- {
		r := rs.rules[i]
		if r.negationsAfter != afterValue {
			t.Errorf("Expected exclusionsAfter to be %v at index %d", afterValue, i)
		}
		if r.negated {
			afterValue = true
		}
	}

	// last two will be dominating
	for _, r := range []string{"logs/", "tmp/"} {
		result, err := rs.Excludes(r)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Dominating {
			t.Errorf("Expected %q to be a dominating rule", r)
		}
	}

	if actual, _ := rs.Excludes("src/baz/ignored"); !actual.Excluded {
		t.Errorf("Expected %q to be excluded, but it was included", "src/baz/ignored")
	}

}

// TestUnicodeNormalizationMatching verifies that NFC patterns match NFD paths and vice versa.
func TestUnicodeNormalizationMatching(t *testing.T) {
	testCases := []struct {
		name        string
		pattern     string
		path        string
		shouldMatch bool
	}{
		{
			name:        "NFC pattern matches NFD path - acute accent",
			pattern:     "données.txt",                    // NFC: c3 a9
			path:        "donne\u0301es.txt",              // NFD: 65 cc 81
			shouldMatch: true,
		},
		{
			name:        "NFD pattern matches NFC path - acute accent",
			pattern:     "donne\u0301es.txt",              // NFD: 65 cc 81
			path:        "données.txt",                    // NFC: c3 a9
			shouldMatch: true,
		},
		{
			name:        "NFC pattern matches NFD path - umlaut",
			pattern:     "config_zürich.json",             // NFC: c3 bc
			path:        "config_zu\u0308rich.json",       // NFD: 75 cc 88
			shouldMatch: true,
		},
		{
			name:        "NFD pattern matches NFC path - umlaut",
			pattern:     "config_zu\u0308rich.json",       // NFD: 75 cc 88
			path:        "config_zürich.json",             // NFC: c3 bc
			shouldMatch: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify test setup - patterns should be byte-different but normalize to same
			if tc.pattern == tc.path {
				t.Fatal("Test setup error: pattern and path should be byte-different")
			}
			if norm.NFC.String(tc.pattern) != norm.NFC.String(tc.path) {
				t.Fatal("Test setup error: pattern and path should normalize to same value")
			}

			// Create rule with the pattern
			rule := rule{val: "**/" + tc.pattern}
			match, err := rule.match(tc.path)
			if err != nil {
				t.Fatalf("Match error: %v", err)
			}

			if match != tc.shouldMatch {
				t.Errorf("Expected match=%v, got match=%v\nPattern: %q (bytes: %x)\nPath: %q (bytes: %x)",
					tc.shouldMatch, match,
					tc.pattern, []byte(tc.pattern),
					tc.path, []byte(tc.path))
			}
		})
	}
}

// TestUnicodeNormalizationEdgeCases tests various Unicode normalization scenarios
func TestUnicodeNormalizationEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		pattern     string
		path        string
		shouldMatch bool
	}{
		{
			name:        "ASCII-only should still work",
			pattern:     "normal.txt",
			path:        "normal.txt",
			shouldMatch: true,
		},
		{
			name:        "Different characters should not match",
			pattern:     "café.txt",
			path:        "cafe.txt", // Missing accent entirely
			shouldMatch: false,
		},
		{
			name:        "Spanish ñ - NFC to NFD",
			pattern:     "español.txt",
			path:        "espan\u0303ol.txt", // n + combining tilde
			shouldMatch: true,
		},
		{
			name:        "Spanish ñ - NFD to NFC",
			pattern:     "espan\u0303ol.txt",
			path:        "español.txt",
			shouldMatch: true,
		},
		{
			name:        "Multiple accents - NFC",
			pattern:     "résumé.pdf",
			path:        "re\u0301sume\u0301.pdf", // Both e's with combining acute
			shouldMatch: true,
		},
		{
			name:        "Greek characters",
			pattern:     "αβγ.txt",
			path:        "αβγ.txt",
			shouldMatch: true,
		},
		{
			name:        "Emoji should work",
			pattern:     "test_😀.txt",
			path:        "test_😀.txt",
			shouldMatch: true,
		},
		{
			name:        "Mixed ASCII and Unicode",
			pattern:     "config_café_2024.json",
			path:        "config_cafe\u0301_2024.json",
			shouldMatch: true,
		},
		{
			name:        "Wildcard with Unicode",
			pattern:     "*.café",
			path:        "test.cafe\u0301",
			shouldMatch: true,
		},
		{
			name:        "Directory with Unicode - NFC pattern NFD path",
			pattern:     "café/**",
			path:        "cafe\u0301/test.txt", // NFD: e + combining acute
			shouldMatch: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rule := rule{val: "**/" + tc.pattern}
			match, err := rule.match(tc.path)
			if err != nil {
				t.Fatalf("Match error: %v", err)
			}
			if match != tc.shouldMatch {
				t.Errorf("Expected match=%v, got match=%v for pattern=%q path=%q",
					tc.shouldMatch, match, tc.pattern, tc.path)
			}
		})
	}
}

// TestUnicodeNormalizationWithRuleset tests normalization with full ruleset
func TestUnicodeNormalizationWithRuleset(t *testing.T) {
	testCases := []struct {
		name          string
		ignoreContent string
		testPath      string
		shouldExclude bool
	}{
		{
			name:          "NFC rule excludes NFD path",
			ignoreContent: "données.txt\n",
			testPath:      "donne\u0301es.txt",
			shouldExclude: true,
		},
		{
			name:          "NFD rule excludes NFC path",
			ignoreContent: "donne\u0301es.txt\n",
			testPath:      "données.txt",
			shouldExclude: true,
		},
		{
			name:          "Wildcard with Unicode",
			ignoreContent: "*.café\n",
			testPath:      "test.cafe\u0301",
			shouldExclude: true,
		},
		{
			name:          "Directory pattern with Unicode",
			ignoreContent: "münchen/\n",
			testPath:      "mu\u0308nchen/test.txt",
			shouldExclude: true,
		},
		{
			name:          "Negation with Unicode",
			ignoreContent: "*.txt\n!important_données.txt\n",
			testPath:      "important_donne\u0301es.txt",
			shouldExclude: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary directory with .terraformignore
			tmpDir, err := os.MkdirTemp("", "unicode-test-")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			// Write .terraformignore
			ignorePath := filepath.Join(tmpDir, ".terraformignore")
			err = os.WriteFile(ignorePath, []byte(tc.ignoreContent), 0644)
			if err != nil {
				t.Fatal(err)
			}

			// Load rules
			rs, err := LoadPackageIgnoreRules(tmpDir)
			if err != nil {
				t.Fatal(err)
			}

			// Test exclusion
			result, err := rs.Excludes(tc.testPath)
			if err != nil {
				t.Fatal(err)
			}

			if result.Excluded != tc.shouldExclude {
				t.Errorf("Expected excluded=%v, got excluded=%v for path=%q with rules=%q",
					tc.shouldExclude, result.Excluded, tc.testPath, tc.ignoreContent)
			}
		})
	}
}

// TestUnicodeNormalizationPreservesASCII ensures the NFC normalization change does not
// alter matching behaviour for plain ASCII patterns and paths.
func TestUnicodeNormalizationPreservesASCII(t *testing.T) {
	testCases := []struct {
		pattern  string
		path     string
		expected bool
	}{
		// *.txt matches only the single filename component ending in .txt
		{"*.txt", "file.txt", true},
		{"*.txt", "test.go", false},
		{"*.txt", "src/app/main.js", false},
		{"*.txt", ".git/config", false},
		{"*.txt", "node_modules/package/index.js", false},
		{"*.txt", "build/output.bin", false},
		// test.go matches exactly that filename
		{"test.go", "file.txt", false},
		{"test.go", "test.go", true},
		{"test.go", "src/app/main.js", false},
		{"test.go", ".git/config", false},
		{"test.go", "node_modules/package/index.js", false},
		{"test.go", "build/output.bin", false},
		// src/**/*.js matches any .js file under src/
		{"src/**/*.js", "file.txt", false},
		{"src/**/*.js", "test.go", false},
		{"src/**/*.js", "src/app/main.js", true},
		{"src/**/*.js", ".git/config", false},
		{"src/**/*.js", "node_modules/package/index.js", false},
		{"src/**/*.js", "build/output.bin", false},
		// directory patterns (trailing /) do not match file paths when used as raw rule.val
		{".git/", "file.txt", false},
		{".git/", "test.go", false},
		{".git/", "src/app/main.js", false},
		{".git/", ".git/config", false},
		{".git/", "node_modules/package/index.js", false},
		{".git/", "build/output.bin", false},
		{"node_modules/", "file.txt", false},
		{"node_modules/", "test.go", false},
		{"node_modules/", "src/app/main.js", false},
		{"node_modules/", ".git/config", false},
		{"node_modules/", "node_modules/package/index.js", false},
		{"node_modules/", "build/output.bin", false},
		{"build/", "file.txt", false},
		{"build/", "test.go", false},
		{"build/", "src/app/main.js", false},
		{"build/", ".git/config", false},
		{"build/", "node_modules/package/index.js", false},
		{"build/", "build/output.bin", false},
	}

	for _, tc := range testCases {
		// Verify NFC normalization is a no-op for pure ASCII strings.
		if norm.NFC.String(tc.pattern) != tc.pattern {
			t.Errorf("NFC normalization changed ASCII pattern: %q", tc.pattern)
		}
		if norm.NFC.String(tc.path) != tc.path {
			t.Errorf("NFC normalization changed ASCII path: %q", tc.path)
		}

		r := rule{val: tc.pattern}
		match, err := r.match(tc.path)
		if err != nil {
			t.Fatalf("ASCII pattern %q failed to match path %q: %v", tc.pattern, tc.path, err)
		}
		if match != tc.expected {
			t.Errorf("pattern=%q path=%q: expected match=%v, got match=%v",
				tc.pattern, tc.path, tc.expected, match)
		}
	}
}
