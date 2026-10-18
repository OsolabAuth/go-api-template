package posts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const maxResponseBytes = 1 << 20

var ErrNotFound = errors.New("external post not found")

type Post struct {
	ID     int64  `json:"id"`
	UserID int64  `json:"userId"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil ||
		(u.Scheme != "http" && u.Scheme != "https") ||
		u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("EXTERNAL_API_URL must be an http(s) origin without credentials, path, query or fragment")
	}
	if timeout <= 0 {
		return nil, errors.New("external API timeout must be positive")
	}
	return &Client{
		baseURL: u.Scheme + "://" + u.Host,
		http: &http.Client{
			Timeout:   timeout,
			Transport: http.DefaultTransport.(*http.Transport).Clone(),
			// Keep requests on the configured service, including when it redirects.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *Client) CloseIdleConnections() { c.http.CloseIdleConnections() }

func (c *Client) Get(ctx context.Context, id int64) (Post, error) {
	if id <= 0 {
		return Post{}, errors.New("post id must be positive")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/posts/"+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return Post{}, fmt.Errorf("create external request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Post{}, fmt.Errorf("fetch external post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Post{}, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Post{}, fmt.Errorf("external API returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Post{}, fmt.Errorf("read external post: %w", err)
	}
	if len(body) > maxResponseBytes {
		return Post{}, errors.New("external response exceeds 1 MiB")
	}
	var post Post
	// Unmarshal rejects trailing JSON; unknown fields may be added by the upstream.
	if err := json.Unmarshal(body, &post); err != nil {
		return Post{}, fmt.Errorf("decode external post: %w", err)
	}
	if post.ID != id || post.UserID <= 0 || post.Title == "" || post.Body == "" {
		return Post{}, errors.New("external response has invalid post fields")
	}
	return post, nil
}
