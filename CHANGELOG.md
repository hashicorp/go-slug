## Unreleased

### Improvements

### Changes

### Fixed

### Security

## v0.16.8

### Improvements
- Update sourceaddrs package to parse and understand Registry Components ([#101](https://github.com/hashicorp/go-slug/pull/101))

### Fixed
- Subpath handling for short-hand url expansion for GitHub and GitLab addresses ([#104](https://github.com/hashicorp/go-slug/pull/104))

### Security
- SECVULN-7809: Fix HasPrefix usage ([#100](https://github.com/hashicorp/go-slug/pull/100))

## v0.16.7

### Fixed

- Support query paths in Github and Gitlab sources addresses. ([#95](https://github.com/hashicorp/go-slug/pull/95))

## v0.16.6

### Improvements
IND-2704 Coverage test by @KaushikiAnand in #85
Remove Mac OS meta-data file and prevent others being added in the future by @jsnfwlr in #87
Add Changelog file by @mohanmanikanta2299 in #92

### Changes
[COMPLIANCE] Add Copyright and License Headers by @hashicorp-copywrite in #84
Pin action refs to latest trusted by TSCCR by @hashicorp-tsccr in #89

### Fixed
irregular mode file checks for Windows symlinks by @notchairmk in #79
