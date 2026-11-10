package buildinfo

import "testing"

func TestRepositoryURL(t *testing.T) {
	saved := Repository
	defer func() { Repository = saved }()
	for value, want := range map[string]string{
		"":                  "",
		"owner/name":        "https://github.com/owner/name",
		"owner":             "",
		"owner/name/extra":  "",
		"owner/na me":       "",
		"../x":              "",
		"Owner-1/repo.name": "https://github.com/Owner-1/repo.name",
	} {
		Repository = value
		if got := RepositoryURL(); got != want {
			t.Errorf("RepositoryURL(%q) = %q, want %q", value, got, want)
		}
	}
}
