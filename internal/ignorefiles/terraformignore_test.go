// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ignorefiles

import (
	"testing"
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
		match, err := rs.Excludes(p.path)
		if err != nil {
			t.Errorf("invalid rule syntax when checking %s at index %d", p.path, i)
			continue
		}
		if match != p.match {
			t.Fatalf("%s at index %d should be %t", p.path, i, p.match)
		}
	}
}
