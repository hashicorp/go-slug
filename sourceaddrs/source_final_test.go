package sourceaddrs

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/apparentlymart/go-versions/versions"
)

func TestResolveRelativeFinalSource(t *testing.T) {
	onePointOh := versions.MustParseVersion("1.0.0")

	tests := []struct {
		Base    FinalSource
		Rel     FinalSource
		Want    FinalSource
		WantErr string
	}{
		{
			Base: MustParseSource("./a/b").(FinalSource),
			Rel:  MustParseSource("../c").(FinalSource),
			Want: MustParseSource("./a/c").(FinalSource),
		},
		{
			Base: MustParseSource("./a").(FinalSource),
			Rel:  MustParseSource("../c").(FinalSource),
			Want: MustParseSource("./c").(FinalSource),
		},
		{
			Base: MustParseSource("./a").(FinalSource),
			Rel:  MustParseSource("../../c").(FinalSource),
			Want: MustParseSource("../c").(FinalSource),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop").(FinalSource),
			Rel:  MustParseSource("git::https://github.com/hashicorp/go-slug.git//blah/blah").(FinalSource),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git//blah/blah").(FinalSource),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop").(FinalSource),
			Rel:  MustParseSource("git::https://example.com/foo.git").(FinalSource),
			Want: MustParseSource("git::https://example.com/foo.git").(FinalSource),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop").(FinalSource),
			Rel:  MustParseSource("../bloop").(FinalSource),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/bloop").(FinalSource),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop").(FinalSource),
			Rel:  MustParseSource("../").(FinalSource),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep").(FinalSource),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop").(FinalSource),
			Rel:  MustParseSource("../..").(FinalSource),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git").(FinalSource),
		},
		{
			Base:    MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop").(FinalSource),
			Rel:     MustParseSource("../../../baz").(FinalSource),
			WantErr: `invalid traversal from git::https://github.com/hashicorp/go-slug.git//beep/boop: relative path ../../../baz traverses up too many levels from source path beep/boop`,
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git").(FinalSource),
			Rel:  MustParseSource("./boop").(FinalSource),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git//boop").(FinalSource),
		},
		{
			Base: MustParseSource("example.com/foo/bar/baz//beep/boop").(RegistrySource).Versioned(onePointOh),
			Rel:  MustParseSource("../").(FinalSource),
			Want: MustParseSource("example.com/foo/bar/baz//beep").(RegistrySource).Versioned(onePointOh),
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s + %s", test.Base, test.Rel), func(t *testing.T) {
			got, gotErr := ResolveRelativeFinalSource(test.Base, test.Rel)

			if test.WantErr != "" {
				if gotErr == nil {
					t.Fatalf("unexpected success\ngot result: %s (%T)\nwant error: %s", got, got, test.WantErr)
				}
				if got, want := gotErr.Error(), test.WantErr; got != want {
					t.Fatalf("wrong error\ngot error:  %s\nwant error: %s", got, want)
				}
				return
			}

			if gotErr != nil {
				t.Fatalf("unexpected error: %s", gotErr)
			}

			// Two addresses are equal if they have the same string representation
			// and the same dynamic type.
			gotStr := got.String()
			wantStr := test.Want.String()
			if gotStr != wantStr {
				t.Errorf("wrong result\ngot:  %s\nwant: %s", gotStr, wantStr)
			}

			if gotType, wantType := reflect.TypeOf(got), reflect.TypeOf(test.Want); gotType != wantType {
				t.Errorf("wrong result type\ngot:  %s\nwant: %s", gotType, wantType)
			}
		})
	}
}
