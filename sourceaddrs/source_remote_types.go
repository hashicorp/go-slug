// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"net/url"
	"strings"
)

type remoteSourceType interface {
	PrepareURL(u *url.URL) error
}

var remoteSourceTypes = map[string]remoteSourceType{
	"git":   gitSourceType{},
	"http":  httpSourceType{},
	"https": httpSourceType{},
}

type gitSourceType struct{}

func (gitSourceType) PrepareURL(u *url.URL) error {
	// The Git source type requires one of the URL schemes that Git itself
	// supports. We're also currently being more rigid than Git to ease
	// initial implementation. We will extend this over time as the source
	// bundle mechanism graduates from experimental to real use.

	if u.Scheme != "ssh" && u.Scheme != "https" {
		// NOTE: We don't support "git" or "http" here because we require
		// source code to originate from sources that can support
		// authentication and encryption, to reduce the risk of mitm attacks
		// introducing malicious code.
		return fmt.Errorf("a Git repository URL must use either the https or ssh scheme")
	}

	qs := u.Query()
	for k, vs := range qs {
		if k != "ref" {
			return fmt.Errorf("a Git repository URL's query string may include only the argument 'ref'")
		}
		if len(vs) > 1 {
			return fmt.Errorf("a Git repository URL's query string may include only one 'ref' argument")
		}
	}

	return nil
}

type httpSourceType struct{}

func (httpSourceType) PrepareURL(u *url.URL) error {
	if u.Scheme == "http" {
		return fmt.Errorf("source package addresses may not use unencrypted HTTP")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("invalid scheme %q for https source type", u.Scheme)
	}

	// For our initial implementation the address must be something that
	// go-getter would've recognized as referring to a gzipped tar archive,
	// to reduce the scope of the initial source bundler fetcher
	// implementations. We may extend this later, but if we do then we should
	// use go-getter's syntax for anything go-getter also supports.
	//
	// Go-getter's treatment of HTTP is quite odd, because by default it does
	// an extra module-registry-like indirection where it expects the
	// given URL to return a header pointing to another source address type.
	// We don't intend to support that here, but we do want to support the
	// behavior of go-getter's special case for URLs whose paths end with
	// suffixes that match those typically used for archives, and its magical
	// treatment of the "archive" query string argument as a way to force
	// treatment of archives. This does mean that we can't fetch from any
	// URL that _really_ needs an "archive" query string parameter, but that's
	// been true for Terraform for many years and hasn't been a problem, so
	// we'll accept that for now and wait to see if any need for it arises.
	//
	// Ideally we'd just make an HTTP request and then decide what to do based
	// on the Content-Type of the response, like a sensible HTTP client would,
	// but for now compatibility with go-getter is more important than being
	// sensible.

	qs := u.Query()
	if vs := qs["archive"]; len(vs) > 0 {
		if len(vs) > 1 {
			return fmt.Errorf("a HTTPS URL's query string may include only one 'archive' argument")
		}
		if vs[0] != "tar.gz" && vs[0] != "tgz" {
			return fmt.Errorf("the special 'archive' query string argument must be set to 'tgz' if present")
		}
		if vs[0] == "tar.gz" {
			qs.Set("archive", "tgz") // normalize on the shorter form
		}
		// NOTE: We don't remove the "archive" argument here because the code
		// which eventually fetches this will need it to understand what kind
		// of archive it's supposed to be fetching, but that final client ought
		// to remove this argument itself to avoid potentially confusing the
		// remote server, since this is an argument reserved for go-getter and
		// for the subset of go-getter's syntax we're implementing here.
		u.RawQuery = qs.Encode()
	} else {
		p := u.EscapedPath()
		if !(strings.HasSuffix(p, ".tar.gz") || strings.HasSuffix(p, ".tgz")) {
			return fmt.Errorf("a HTTPS URL's path must end with either .tar.gz or .tgz")
		}
	}

	if len(qs["checksum"]) != 0 {
		// This is another go-getter oddity. go-getter would treat this as
		// a request to verify that the result matches the given checksum
		// and not send this argument to the server. However, go-getter actually
		// doesn't support this (it returns an error) when it's dealing with
		// an archive. We'll explicitly reject it to avoid folks being
		// misled into thinking that it _is_ working, and thus believing
		// they've achieved a verification that isn't present, though we
		// might relax this later since go-getter wouldn't have allowed this
		// anyway.
		return fmt.Errorf("a HTTPS URL's query string must not include 'checksum' argument")
	}

	return nil
}
