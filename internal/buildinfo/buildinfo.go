// Package buildinfo carries version metadata injected at link time:
//
//	go build -ldflags "-X sessiondesk/internal/buildinfo.Version=0.1.0 -X sessiondesk/internal/buildinfo.Commit=abc123 -X sessiondesk/internal/buildinfo.Date=2026-11-10T00:00:00Z -X sessiondesk/internal/buildinfo.Repository=owner/name"
package buildinfo

import "strings"

var (
	// Version is the semantic version without a leading "v". "dev" marks an unversioned local build.
	Version = "dev"
	// Commit is the short git commit the binary was built from.
	Commit = ""
	// Date is the UTC build time in RFC 3339 format.
	Date = ""
	// Repository is the GitHub "owner/name" that publishes releases, read from the REPOSITORY file by the
	// build scripts. Empty disables the update check and the project link.
	Repository = ""
)

// RepositoryURL returns the GitHub page of Repository, or "" when it is not configured or malformed.
func RepositoryURL() string {
	if !validRepository(Repository) {
		return ""
	}
	return "https://github.com/" + Repository
}

// UpdatesEnabled reports whether a release repository is configured.
func UpdatesEnabled() bool { return validRepository(Repository) }

func validRepository(value string) bool {
	owner, name, found := strings.Cut(value, "/")
	if !found || owner == "" || name == "" || strings.Contains(name, "/") || strings.Contains(owner, ".") || name == "." || name == ".." {
		return false
	}
	for _, r := range value {
		if !(r == '/' || r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// Info describes the running build.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Current returns the metadata of the running build.
func Current() Info {
	return Info{Version: strings.TrimPrefix(Version, "v"), Commit: Commit, Date: Date}
}
