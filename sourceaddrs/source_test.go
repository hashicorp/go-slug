// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"

	regaddr "github.com/hashicorp/terraform-registry-address"
	svchost "github.com/hashicorp/terraform-svchost"
)

func TestParseSource(t *testing.T) {
	tests := []struct {
		Given   string
		Want    Source
		WantErr string
	}{
		{
			Given:   "",
			WantErr: `a valid source address is required`,
		},
		{
			Given:   " hello",
			WantErr: `source address must not have leading or trailing spaces`,
		},
		{
			Given:   "hello ",
			WantErr: `source address must not have leading or trailing spaces`,
		},
		{
			Given: "./boop",
			Want: LocalSource{
				relPath: "./boop",
			},
		},
		{
			Given:   "./boop/../beep",
			WantErr: `invalid local source address "./boop/../beep": relative path must be written in canonical form "./beep"`,
		},
		{
			Given: "../boop",
			Want: LocalSource{
				relPath: "../boop",
			},
		},
		{
			Given: "../",
			Want: LocalSource{
				relPath: "../",
			},
		},
		{
			Given:   "..",
			WantErr: `invalid local source address "..": relative path must be written in canonical form "../"`,
		},
		{
			Given: "./",
			Want: LocalSource{
				relPath: "./",
			},
		},
		{
			Given:   ".",
			WantErr: `invalid local source address ".": relative path must be written in canonical form "./"`,
		},
		{
			Given:   "./.",
			WantErr: `invalid local source address "./.": relative path must be written in canonical form "./"`,
		},
		{
			Given:   "../boop/../beep",
			WantErr: `invalid local source address "../boop/../beep": relative path must be written in canonical form "../beep"`,
		},
		{
			Given: "hashicorp/subnets/cidr",
			Want: RegistrySource{
				pkg: regaddr.ModulePackage{
					Host:         regaddr.DefaultModuleRegistryHost,
					Namespace:    "hashicorp",
					Name:         "subnets",
					TargetSystem: "cidr",
				},
			},
		},
		{
			Given: "hashicorp/subnets/cidr//blah/blah",
			Want: RegistrySource{
				pkg: regaddr.ModulePackage{
					Host:         regaddr.DefaultModuleRegistryHost,
					Namespace:    "hashicorp",
					Name:         "subnets",
					TargetSystem: "cidr",
				},
				subPath: "blah/blah",
			},
		},
		{
			Given:   "hashicorp/subnets/cidr//blah/blah/../bloop",
			WantErr: `invalid module registry source address "hashicorp/subnets/cidr//blah/blah/../bloop": invalid sub-path: must be slash-separated relative path without any .. or . segments`,
		},
		{
			Given: "terraform.example.com/bleep/bloop/blorp",
			Want: RegistrySource{
				pkg: regaddr.ModulePackage{
					Host:         svchost.Hostname("terraform.example.com"),
					Namespace:    "bleep",
					Name:         "bloop",
					TargetSystem: "blorp",
				},
			},
		},
		{
			Given: "テラフォーム.example.com/bleep/bloop/blorp",
			Want: RegistrySource{
				pkg: regaddr.ModulePackage{
					Host:         svchost.Hostname("xn--jckxc1b4b2b6g.example.com"),
					Namespace:    "bleep",
					Name:         "bloop",
					TargetSystem: "blorp",
				},
			},
		},
		{
			Given: "git::https://github.com/hashicorp/go-slug.git",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://github.com/hashicorp/go-slug.git"),
				},
			},
		},
		{
			Given: "git::https://github.com/hashicorp/go-slug.git//blah/blah",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://github.com/hashicorp/go-slug.git"),
				},
				subPath: "blah/blah",
			},
		},
		{
			Given: "git::https://github.com/hashicorp/go-slug.git?ref=main",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://github.com/hashicorp/go-slug.git?ref=main"),
				},
			},
		},
		{
			Given:   "git::https://github.com/hashicorp/go-slug.git?ref=main&ref=main",
			WantErr: `invalid remote source address "git::https://github.com/hashicorp/go-slug.git?ref=main&ref=main": a Git repository URL's query string may include only one 'ref' argument`,
		},
		{
			Given: "git::https://github.com/hashicorp/go-slug.git//blah/blah?ref=main",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://github.com/hashicorp/go-slug.git?ref=main"),
				},
				subPath: "blah/blah",
			},
		},
		{
			Given:   "git::https://github.com/hashicorp/go-slug.git?sshkey=blahblah",
			WantErr: `invalid remote source address "git::https://github.com/hashicorp/go-slug.git?sshkey=blahblah": a Git repository URL's query string may include only the argument 'ref'`,
		},
		{
			Given:   "git::https://github.com/hashicorp/go-slug.git?depth=1",
			WantErr: `invalid remote source address "git::https://github.com/hashicorp/go-slug.git?depth=1": a Git repository URL's query string may include only the argument 'ref'`,
		},
		{
			Given:   "git::https://git@github.com/hashicorp/go-slug.git",
			WantErr: `invalid remote source address "git::https://git@github.com/hashicorp/go-slug.git": must not use username or password in URL portion`,
		},
		{
			Given:   "git::https://git:blit@github.com/hashicorp/go-slug.git",
			WantErr: `invalid remote source address "git::https://git:blit@github.com/hashicorp/go-slug.git": must not use username or password in URL portion`,
		},
		{
			Given:   "git::https://:blit@github.com/hashicorp/go-slug.git",
			WantErr: `invalid remote source address "git::https://:blit@github.com/hashicorp/go-slug.git": must not use username or password in URL portion`,
		},
		{
			Given: "git::ssh://github.com/hashicorp/go-slug.git",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("ssh://github.com/hashicorp/go-slug.git"),
				},
			},
		},
		{
			Given: "git::ssh://github.com/hashicorp/go-slug.git//blah/blah?ref=main",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("ssh://github.com/hashicorp/go-slug.git?ref=main"),
				},
				subPath: "blah/blah",
			},
		},
		{
			Given:   "git://github.com/hashicorp/go-slug.git",
			WantErr: `invalid remote source address "git://github.com/hashicorp/go-slug.git": a Git repository URL must use either the https or ssh scheme`,
		},
		{
			Given:   "git::git://github.com/hashicorp/go-slug.git",
			WantErr: `invalid remote source address "git::git://github.com/hashicorp/go-slug.git": don't specify redundant "git" source type for "git" URL`,
		},
		{
			Given: "github.com/hashicorp/go-slug.git",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://github.com/hashicorp/go-slug.git"),
				},
			},
		},
		{
			Given: "github.com/hashicorp/go-slug",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://github.com/hashicorp/go-slug.git"),
				},
			},
		},
		{
			Given: "github.com/hashicorp/go-slug/bleep",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://github.com/hashicorp/go-slug.git"),
				},
				subPath: "bleep",
			},
		},
		{
			Given: "gitlab.com/hashicorp/go-slug.git",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://gitlab.com/hashicorp/go-slug.git"),
				},
			},
		},
		{
			Given: "gitlab.com/hashicorp/go-slug",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://gitlab.com/hashicorp/go-slug.git"),
				},
			},
		},
		{
			Given: "gitlab.com/hashicorp/go-slug/bleep",
			// NOTE: gitlab.com _also_ hosts a Terraform Module registry, and so
			// the registry address interpretation takes precedence if it
			// matches. Users must write an explicit git:: source address if
			// they want this to be interpreted as a Git source address.
			Want: RegistrySource{
				pkg: regaddr.ModulePackage{
					Host:         svchost.Hostname("gitlab.com"),
					Namespace:    "hashicorp",
					Name:         "go-slug",
					TargetSystem: "bleep",
				},
			},
		},
		{
			// This is the explicit Git source address version of the previous
			// case, overriding the default interpretation as module registry.
			Given: "git::https://gitlab.com/hashicorp/go-slug//bleep",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://gitlab.com/hashicorp/go-slug"),
				},
				subPath: "bleep",
			},
		},
		{
			Given: "gitlab.com/hashicorp/go-slug/bleep/bloop",
			// Two or more subpath portions is fine for Git interpretation,
			// because that's not ambigious with module registry. This is
			// an annoying inconsistency but necessary for backward
			// compatibility with go-getter's interpretations.
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "git",
					url:        *mustParseURL("https://gitlab.com/hashicorp/go-slug.git"),
				},
				subPath: "bleep/bloop",
			},
		},
		{
			Given: "https://example.com/foo.tar.gz",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "https",
					url:        *mustParseURL("https://example.com/foo.tar.gz"),
				},
			},
		},
		{
			Given: "https://example.com/foo.tar.gz//bleep/bloop",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "https",
					url:        *mustParseURL("https://example.com/foo.tar.gz"),
				},
				subPath: "bleep/bloop",
			},
		},
		{
			Given: "https://example.com/foo.tar.gz?something=anything",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "https",
					url:        *mustParseURL("https://example.com/foo.tar.gz?something=anything"),
				},
			},
		},
		{
			Given: "https://example.com/foo.tar.gz//bleep/bloop?something=anything",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "https",
					url:        *mustParseURL("https://example.com/foo.tar.gz?something=anything"),
				},
				subPath: "bleep/bloop",
			},
		},
		{
			Given: "https://example.com/foo.tgz",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "https",
					url:        *mustParseURL("https://example.com/foo.tgz"),
				},
			},
		},
		{
			Given: "https://example.com/foo?archive=tar.gz",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "https",
					url:        *mustParseURL("https://example.com/foo?archive=tgz"),
				},
			},
		},
		{
			Given: "https://example.com/foo?archive=tgz",
			Want: RemoteSource{
				pkg: RemotePackage{
					sourceType: "https",
					url:        *mustParseURL("https://example.com/foo?archive=tgz"),
				},
			},
		},
		{
			Given:   "https://example.com/foo.zip",
			WantErr: `invalid remote source address "https://example.com/foo.zip": a HTTPS URL's path must end with either .tar.gz or .tgz`,
		},
		{
			Given:   "https://example.com/foo?archive=zip",
			WantErr: `invalid remote source address "https://example.com/foo?archive=zip": the special 'archive' query string argument must be set to 'tgz' if present`,
		},
		{
			Given:   "http://example.com/foo.tar.gz",
			WantErr: `invalid remote source address "http://example.com/foo.tar.gz": source package addresses may not use unencrypted HTTP`,
		},
		{
			Given:   "http::http://example.com/foo.tar.gz",
			WantErr: `invalid remote source address "http::http://example.com/foo.tar.gz": don't specify redundant "http" source type for "http" URL`,
		},
		{
			Given:   "https::https://example.com/foo.tar.gz",
			WantErr: `invalid remote source address "https::https://example.com/foo.tar.gz": don't specify redundant "https" source type for "https" URL`,
		},
		{
			Given:   "https://foo@example.com/foo.tgz",
			WantErr: `invalid remote source address "https://foo@example.com/foo.tgz": must not use username or password in URL portion`,
		},
		{
			Given:   "https://foo:bar@example.com/foo.tgz",
			WantErr: `invalid remote source address "https://foo:bar@example.com/foo.tgz": must not use username or password in URL portion`,
		},
		{
			Given:   "https://:bar@example.com/foo.tgz",
			WantErr: `invalid remote source address "https://:bar@example.com/foo.tgz": must not use username or password in URL portion`,
		},
	}

	for _, test := range tests {
		t.Run(test.Given, func(t *testing.T) {
			got, gotErr := ParseSource(test.Given)

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

func TestResolveRelativeSource(t *testing.T) {
	tests := []struct {
		Base    Source
		Rel     Source
		Want    Source
		WantErr string
	}{
		{
			Base: MustParseSource("./a/b"),
			Rel:  MustParseSource("../c"),
			Want: MustParseSource("./a/c"),
		},
		{
			Base: MustParseSource("./a"),
			Rel:  MustParseSource("../c"),
			Want: MustParseSource("./c"),
		},
		{
			Base: MustParseSource("./a"),
			Rel:  MustParseSource("../../c"),
			Want: MustParseSource("../c"),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop"),
			Rel:  MustParseSource("git::https://github.com/hashicorp/go-slug.git//blah/blah"),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git//blah/blah"),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop"),
			Rel:  MustParseSource("git::https://example.com/foo.git"),
			Want: MustParseSource("git::https://example.com/foo.git"),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop"),
			Rel:  MustParseSource("../bloop"),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/bloop"),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop"),
			Rel:  MustParseSource("../"),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep"),
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop"),
			Rel:  MustParseSource("../.."),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git"),
		},
		{
			Base:    MustParseSource("git::https://github.com/hashicorp/go-slug.git//beep/boop"),
			Rel:     MustParseSource("../../../baz"),
			WantErr: `invalid traversal from git::https://github.com/hashicorp/go-slug.git//beep/boop: relative path ../../../baz traverses up too many levels from source path beep/boop`,
		},
		{
			Base: MustParseSource("git::https://github.com/hashicorp/go-slug.git"),
			Rel:  MustParseSource("./boop"),
			Want: MustParseSource("git::https://github.com/hashicorp/go-slug.git//boop"),
		},
		{
			Base: MustParseSource("example.com/foo/bar/baz//beep/boop"),
			Rel:  MustParseSource("../"),
			Want: MustParseSource("example.com/foo/bar/baz//beep"),
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s + %s", test.Base, test.Rel), func(t *testing.T) {
			got, gotErr := ResolveRelativeSource(test.Base, test.Rel)

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

func TestSourceFilename(t *testing.T) {
	tests := []struct {
		Addr Source
		Want string
	}{
		{
			MustParseSource("./foo.tf"),
			"foo.tf",
		},
		{
			MustParseSource("./boop/foo.tf"),
			"foo.tf",
		},
		{
			MustParseSource("git::https://example.com/foo.git//foo.tf"),
			"foo.tf",
		},
		{
			MustParseSource("git::https://example.com/foo.git//boop/foo.tf"),
			"foo.tf",
		},
		{
			MustParseSource("git::https://example.com/foo.git//boop/foo.tf?ref=main"),
			"foo.tf",
		},
		{
			MustParseSource("hashicorp/subnets/cidr//main.tf"),
			"main.tf",
		},
		{
			MustParseSource("hashicorp/subnets/cidr//test/simple.tf"),
			"simple.tf",
		},
	}

	for _, test := range tests {
		t.Run(test.Addr.String(), func(t *testing.T) {
			got := SourceFilename(test.Addr)
			if got != test.Want {
				t.Errorf(
					"wrong result\naddr: %s\ngot:  %s\nwant: %s",
					test.Addr, got, test.Want,
				)
			}
		})
	}
}

func mustParseURL(s string) *url.URL {
	ret, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return ret
}
