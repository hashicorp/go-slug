// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

import (
	"fmt"

	"github.com/hashicorp/go-slug/sourceaddrs"
)

// Diagnostics is a collection of problems (errors and warnings) that occurred
// during an operation.
type Diagnostics []Diagnostic

// Diagnostics represents a single problem (error or warning) that has occurred
// during an operation.
//
// This interface has no concrete implementations in this package.
// Implementors of [DependencyFinder] will need to implement this interface
// to report any problems they find while analyzing the designated source
// artifact. For example, a [DependencyFinder] that uses the HCL library
// to analyze an HCL-based language would probably implement this interface
// in terms of HCL's Diagnostic type.
type Diagnostic interface {
	Severity() DiagSeverity
	Description() DiagDescription
	Source() DiagSource

	// ExtraInfo returns the raw extra information value. This is a low-level
	// API which requires some work on the part of the caller to properly
	// access associated information. This convention comes from HCL and
	// Terraform and this is here primarily for their benefit; sourcebundle
	// passes through these values verbatim without trying to interpret them.
	ExtraInfo() interface{}
}

func (diags Diagnostics) HasErrors() bool {
	for _, diag := range diags {
		if diag.Severity() == DiagError {
			return true
		}
	}
	return false
}

func (diags Diagnostics) Append(more ...interface{}) Diagnostics {
	for _, item := range more {
		if item == nil {
			continue
		}

		switch item := item.(type) {
		case Diagnostic:
			diags = append(diags, item)
		case Diagnostics:
			diags = append(diags, item...)
		default:
			panic(fmt.Errorf("can't construct diagnostic(s) from %T", item))
		}
	}
	return diags
}

type DiagSeverity rune

const (
	DiagError   DiagSeverity = 'E'
	DiagWarning DiagSeverity = 'W'
)

type DiagDescription struct {
	Summary string
	Detail  string
}

type DiagSource struct {
	Subject *SourceRange
	Context *SourceRange
}

type SourceRange struct {
	// Filename is a human-oriented label for the file that the range belongs
	// to. This is often the string representation of a source address, but
	// isn't guaranteed to be.
	Filename   string
	Start, End SourcePos
}

type SourcePos struct {
	Line, Column, Byte int
}

// diagnosticInSourcePackage is a thin wrapper around diagnostic that
// reinterprets the filenames in any source ranges to be relative to a
// particular remote source package, so it's unambiguous which remote
// source package the diagnostic originated in.
type diagnosticInSourcePackage struct {
	wrapped Diagnostic
	pkg     sourceaddrs.RemotePackage
}

// inRemoteSourcePackage modifies the reciever in-place so that all of the
// diagnostics will have their source filenames (if any) interpreted as
// sub-paths within the given source package.
//
// For convenience, returns the same diags slice whose backing array has now
// been modified with different diagnostics.
func (diags Diagnostics) inRemoteSourcePackage(pkg sourceaddrs.RemotePackage) Diagnostics {
	for i, diag := range diags {
		diags[i] = diagnosticInSourcePackage{
			wrapped: diag,
			pkg:     pkg,
		}
	}
	return diags
}

var _ Diagnostic = diagnosticInSourcePackage{}

func (diag diagnosticInSourcePackage) Description() DiagDescription {
	return diag.wrapped.Description()
}

func (diag diagnosticInSourcePackage) ExtraInfo() interface{} {
	return diag.wrapped.ExtraInfo()
}

func (diag diagnosticInSourcePackage) Severity() DiagSeverity {
	return diag.wrapped.Severity()
}

func (diag diagnosticInSourcePackage) Source() DiagSource {
	ret := diag.wrapped.Source()
	if ret.Subject != nil && sourceaddrs.ValidSubPath(ret.Subject.Filename) {
		newRng := *ret.Subject // shallow copy
		newRng.Filename = diag.pkg.SourceAddr(newRng.Filename).String()
		ret.Subject = &newRng
	}
	if ret.Context != nil && sourceaddrs.ValidSubPath(ret.Context.Filename) {
		newRng := *ret.Context // shallow copy
		newRng.Filename = diag.pkg.SourceAddr(newRng.Filename).String()
		ret.Context = &newRng
	}
	return ret
}

// internalDiagnostic is a diagnostic type used to report this package's own
// errors as diagnostics.
//
// This package doesn't ever work directly with individual source file contents,
// so an internal diagnostic never has source location information.
type internalDiagnostic struct {
	severity DiagSeverity
	summary  string
	detail   string
}

var _ Diagnostic = (*internalDiagnostic)(nil)

// Description implements Diagnostic
func (d *internalDiagnostic) Description() DiagDescription {
	return DiagDescription{
		Summary: d.summary,
		Detail:  d.detail,
	}
}

// ExtraInfo implements Diagnostic
func (d *internalDiagnostic) ExtraInfo() interface{} {
	return nil
}

// Severity implements Diagnostic
func (d *internalDiagnostic) Severity() DiagSeverity {
	return d.severity
}

// Source implements Diagnostic
func (d *internalDiagnostic) Source() DiagSource {
	return DiagSource{
		// Never any source location information for internal diagnostics.
	}
}
