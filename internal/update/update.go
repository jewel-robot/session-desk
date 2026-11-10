// Package update checks GitHub Releases for a newer Session Desk build.
// It only reports what is available; downloading and installing stay with the user.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Status values reported to the interface.
const (
	StatusAvailable = "available" // a newer release exists
	StatusLatest    = "latest"    // running the newest release
	StatusNone      = "none"      // the repository has no published releases or is not reachable anonymously
	StatusError     = "error"     // network or parse failure
	StatusDisabled  = "disabled"  // automatic checks are turned off
)

// Result is the outcome of a release check.
type Result struct {
	Status      string    `json:"status"`
	Current     string    `json:"current"`
	Latest      string    `json:"latest,omitempty"`
	URL         string    `json:"url,omitempty"`
	DownloadURL string    `json:"download_url,omitempty"`
	AssetName   string    `json:"asset_name,omitempty"`
	PublishedAt string    `json:"published_at,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	Error       string    `json:"error,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

// Checker queries the GitHub "latest release" endpoint and caches the answer.
type Checker struct {
	Repository string
	Current    string
	BaseURL    string // defaults to https://api.github.com
	Client     *http.Client
	TTL        time.Duration

	mu     sync.Mutex
	cached *Result
}

// NewChecker returns a checker for the given repository ("owner/name") and running version.
func NewChecker(repository, current string) *Checker {
	return &Checker{Repository: repository, Current: current, BaseURL: "https://api.github.com", Client: &http.Client{Timeout: 10 * time.Second}, TTL: 6 * time.Hour}
}

type release struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Check returns the cached result when it is fresh, unless force is set.
func (c *Checker) Check(ctx context.Context, force bool) Result {
	c.mu.Lock()
	if !force && c.cached != nil && time.Since(c.cached.CheckedAt) < c.TTL {
		result := *c.cached
		c.mu.Unlock()
		return result
	}
	c.mu.Unlock()
	result := c.fetch(ctx)
	if result.Status != StatusError {
		c.mu.Lock()
		c.cached = &result
		c.mu.Unlock()
	}
	return result
}

func (c *Checker) fetch(ctx context.Context) Result {
	result := Result{Current: c.Current, CheckedAt: time.Now().UTC()}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/repos/"+c.Repository+"/releases/latest", nil)
	if err != nil {
		return failed(result, err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "SessionDesk/"+c.Current)
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return failed(result, errors.New("无法连接 GitHub，请检查网络后重试"))
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotFound:
		// GitHub answers 404 both when no release exists and when the repository is private.
		result.Status = StatusNone
		return result
	case response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests:
		return failed(result, errors.New("GitHub 请求次数已达上限，请稍后再试"))
	case response.StatusCode != http.StatusOK:
		return failed(result, fmt.Errorf("GitHub 返回状态 %d", response.StatusCode))
	}
	var latest release
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&latest); err != nil {
		return failed(result, errors.New("无法解析发布信息"))
	}
	if latest.Draft || latest.TagName == "" {
		result.Status = StatusNone
		return result
	}
	result.Latest = strings.TrimPrefix(strings.TrimSpace(latest.TagName), "v")
	result.URL = latest.HTMLURL
	result.PublishedAt = latest.PublishedAt
	result.Notes = truncate(latest.Body, 4000)
	for _, preferred := range []string{"-setup.exe", ".exe", "windows-amd64", ".zip"} {
		for _, asset := range latest.Assets {
			if strings.Contains(strings.ToLower(asset.Name), preferred) {
				result.DownloadURL, result.AssetName = asset.BrowserDownloadURL, asset.Name
				break
			}
		}
		if result.DownloadURL != "" {
			break
		}
	}
	if Newer(result.Latest, c.Current) {
		result.Status = StatusAvailable
	} else {
		result.Status = StatusLatest
	}
	return result
}

func failed(result Result, err error) Result {
	result.Status = StatusError
	result.Error = err.Error()
	return result
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// Newer reports whether candidate is a higher semantic version than current.
// A "dev" or unparsable current version never reports an update, so local builds stay quiet.
func Newer(candidate, current string) bool {
	a, okA := parse(candidate)
	b, okB := parse(current)
	if !okA || !okB {
		return false
	}
	for i := 0; i < 3; i++ {
		if a.parts[i] != b.parts[i] {
			return a.parts[i] > b.parts[i]
		}
	}
	// 1.0.0 is newer than 1.0.0-beta.1; compare prereleases lexically otherwise.
	switch {
	case a.pre == b.pre:
		return false
	case a.pre == "":
		return true
	case b.pre == "":
		return false
	default:
		return a.pre > b.pre
	}
}

type version struct {
	parts [3]int
	pre   string
}

func parse(text string) (version, bool) {
	text = strings.TrimPrefix(strings.TrimSpace(text), "v")
	text, _, _ = strings.Cut(text, "+")
	core, pre, _ := strings.Cut(text, "-")
	fields := strings.Split(core, ".")
	if len(fields) == 0 || len(fields) > 3 {
		return version{}, false
	}
	var result version
	for i, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil || number < 0 {
			return version{}, false
		}
		result.parts[i] = number
	}
	result.pre = pre
	return result, true
}
