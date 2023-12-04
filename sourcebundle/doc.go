// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package sourcebundle deals with the construction of and later consumption of
// "source bundles", which are in some sense "meta-slugs" that capture a
// variety of different source packages together into a single working
// directory, which can optionally be bundled up into an archive for insertion
// into a blob storage system.
//
// Whereas single slugs (as implemented in the parent package) have very little
// predefined structure aside from the possibility of a .terraformignore file,
// source bundles have a more prescriptive structure that allows callers to
// use a source bundle as a direct substitute for fetching the individual
// source packages it was built from.
//
// NOTE WELL: Everything in this package is currently experimental and subject
// to breaking changes even in patch releases. We will make stronger commitments
// to backward-compatibility once we have more experience using this
// functionality in real contexts.
package sourcebundle
