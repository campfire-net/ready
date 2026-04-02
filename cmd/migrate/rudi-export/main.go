// rudi-export exports work items from a rudi server (rd.3dl.dev) as JSONL.
//
// Usage:
//
//	rudi-export --project ceo [--base-url https://rd.3dl.dev] [--token ...] [--output file.jsonl]
//
// Authentication: pass the OAuth2 bearer token via --token or RD_TOKEN env var.
// The token is stored at ~/.rudi/tokens/<host>.json by the rd login command.
//
// Output: JSONL, one item per line. Each line is a JSON object matching RudiItem.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// RudiItem is a work item exported from the rudi server.
// Fields match the rudi API response for GET /api/v1/items.
type RudiItem struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Type        string        `json:"type"`
	Level       string        `json:"level,omitempty"`
	Priority    string        `json:"priority"`
	Status      string        `json:"status"`
	Context     string        `json:"context,omitempty"`
	Project     string        `json:"project,omitempty"`
	CreatedBy   string        `json:"created_by,omitempty"`
	Responsible string        `json:"responsible,omitempty"`
	CreatedAt   string        `json:"created_at,omitempty"`
	UpdatedAt   string        `json:"updated_at,omitempty"`
	ETA         string        `json:"eta,omitempty"`
	ParentID    string        `json:"parent_id,omitempty"`
	Blocks      []string      `json:"blocks,omitempty"`
	BlockedBy   []string      `json:"blocked_by,omitempty"`
	History     []HistoryEntry `json:"history,omitempty"`
}

// HistoryEntry is a single status-change event in an item's audit trail.
type HistoryEntry struct {
	Timestamp  string `json:"timestamp"`
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
	ChangedBy  string `json:"changed_by"`
	Note       string `json:"note,omitempty"`
}

func main() {
	var (
		project = flag.String("project", "", "project prefix to export (required, e.g. \"ceo\")")
		baseURL = flag.String("base-url", "https://rd.3dl.dev", "rudi server base URL")
		token   = flag.String("token", "", "OAuth2 bearer token (or set RD_TOKEN env)")
		output  = flag.String("output", "", "output file path (default: stdout)")
	)
	flag.Parse()

	if *project == "" {
		fmt.Fprintln(os.Stderr, "error: --project is required")
		flag.Usage()
		os.Exit(1)
	}

	// Resolve token.
	tok := *token
	if tok == "" {
		tok = os.Getenv("RD_TOKEN")
	}
	if tok == "" {
		// Try reading from the token store used by `rd login`.
		tok = loadStoredToken(*baseURL)
	}
	if tok == "" {
		fmt.Fprintln(os.Stderr, "error: no token — pass --token, set RD_TOKEN, or run `rd login`")
		os.Exit(1)
	}

	// Open output writer.
	var w io.Writer = os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: opening output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	bw := bufio.NewWriter(w)
	defer bw.Flush()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	exported, err := export(ctx, *baseURL, tok, *project, bw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "exported %d items for project %q\n", exported, *project)
}

// export fetches all items for the given project and writes them as JSONL to w.
// Returns the number of items exported.
func export(ctx context.Context, baseURL, token, project string, w io.Writer) (int, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	enc := json.NewEncoder(w)

	const pageSize = 100
	offset := 0
	total := 0

	for {
		items, err := listItems(ctx, client, baseURL, token, project, pageSize, offset)
		if err != nil {
			return total, fmt.Errorf("listing items at offset %d: %w", offset, err)
		}

		for _, item := range items {
			if err := enc.Encode(item); err != nil {
				return total, fmt.Errorf("encoding item %s: %w", item.ID, err)
			}
			total++
		}

		if len(items) < pageSize {
			break // last page
		}
		offset += pageSize
	}

	return total, nil
}

// listItems calls GET /api/v1/items with the given filters.
func listItems(ctx context.Context, client *http.Client, baseURL, token, project string, limit, offset int) ([]RudiItem, error) {
	params := url.Values{
		"project": []string{project},
		"limit":   []string{strconv.Itoa(limit)},
	}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}

	reqURL := baseURL + "/api/v1/items?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized — check token (run `rd login` to refresh)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET %s returned %d: %s", reqURL, resp.StatusCode, body)
	}

	var items []RudiItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return items, nil
}

// loadStoredToken attempts to read a stored OAuth token from the rudi token
// store (~/.rudi/tokens/<host>.json). Returns an empty string on any error.
func loadStoredToken(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	host := u.Host
	if host == "" {
		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	tokenPath := fmt.Sprintf("%s/.rudi/tokens/%s.json", home, host)
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return ""
	}

	var stored struct {
		AccessToken string    `json:"access_token"`
		ExpiresAt   time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return ""
	}

	// Warn if expired but still return — server will reject if truly expired.
	if stored.ExpiresAt.Before(time.Now()) {
		fmt.Fprintf(os.Stderr, "warning: stored token expired at %s — run `rd login` to refresh\n",
			stored.ExpiresAt.Format(time.RFC3339))
	}

	return stored.AccessToken
}
