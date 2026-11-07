package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNotFound = errors.New("catalog record not found")
var ErrUnavailable = errors.New("catalog unavailable")
var ErrInvalid = errors.New("invalid catalog query")

type SearchOptions struct {
	Query, AuthorID, PublisherID, Sort string
	Page, PageSize                     int
}

func (o SearchOptions) Values() url.Values {
	v := url.Values{}
	if o.Query != "" {
		v.Set("q", o.Query)
	}
	if o.AuthorID != "" {
		v.Set("authorId", o.AuthorID)
	}
	if o.PublisherID != "" {
		v.Set("publisherId", o.PublisherID)
	}
	if o.Sort != "" {
		v.Set("sort", o.Sort)
	}
	if o.Page > 1 {
		v.Set("page", fmt.Sprint(o.Page))
	}
	if o.PageSize > 0 && o.PageSize != 20 {
		v.Set("pageSize", fmt.Sprint(o.PageSize))
	}
	return v
}

type Source interface {
	Snapshot(context.Context) (Snapshot, error)
	Search(context.Context, SearchOptions) (Page, error)
	Book(context.Context, string) (Book, error)
	Authority(context.Context, string, string) (Authority, error)
	Lookup(context.Context, string) (Location, error)
}
type Client struct {
	BaseURL, Token string
	HTTP           *http.Client
}

func NewClient(base, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(base, "/"), Token: token, HTTP: &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) get(ctx context.Context, path string, dest any) error {
	if c.BaseURL == "" || c.Token == "" {
		return ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/internal/catalog/"+path, nil)
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		switch res.StatusCode {
		case 404:
			return ErrNotFound
		case 400:
			return ErrInvalid
		default:
			return fmt.Errorf("%w: HTTP %d", ErrUnavailable, res.StatusCode)
		}
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 256<<20))
	decoder.UseNumber()
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("%w: invalid response", ErrUnavailable)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("%w: incomplete or oversized response", ErrUnavailable)
	}
	return nil
}
func (c *Client) Snapshot(ctx context.Context) (Snapshot, error) {
	var v Snapshot
	err := c.get(ctx, "snapshot", &v)
	if err == nil && v.SchemaVersion != 1 {
		err = fmt.Errorf("%w: unsupported snapshot version", ErrUnavailable)
	}
	return v, err
}
func (c *Client) Search(ctx context.Context, o SearchOptions) (Page, error) {
	var v Page
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := c.get(ctx, "books?"+o.Values().Encode(), &v)
	return v, err
}
func (c *Client) Book(ctx context.Context, id string) (Book, error) {
	var v Book
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := c.get(ctx, "books/"+url.PathEscape(id), &v)
	return v, err
}
func (c *Client) Authority(ctx context.Context, kind, id string) (Authority, error) {
	var v Authority
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := c.get(ctx, "authorities/"+url.PathEscape(kind)+"/"+url.PathEscape(id), &v)
	return v, err
}
func (c *Client) Lookup(ctx context.Context, code string) (Location, error) {
	var v Location
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := c.get(ctx, "copies/lookup?"+url.Values{"code": {code}}.Encode(), &v)
	return v, err
}
