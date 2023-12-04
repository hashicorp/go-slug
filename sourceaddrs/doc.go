// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package sourceaddrs deals with the various types of source code address
// that Terraform can gather into a source bundle via the sibling package
// "sourcebundle".
//
// NOTE WELL: Everything in this package is currently experimental and subject
// to breaking changes even in patch releases. We will make stronger commitments
// to backward-compatibility once we have more experience using this
// functionality in real contexts.
package sourceaddrs
