// Package collab integrates the Node Hocuspocus sidecar: issuing websocket
// JWTs, reverse-proxying the websocket, and the internal "doc-ops" HTTP API
// that lets Go (and through it the REST/MCP surface) read and edit live
// CRDT documents.
package collab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Client struct {
	BaseURL string // e.g. http://collab:8090
	Secret  string // shared HS256 + internal-API secret
	HTTP    *http.Client
}

func New(baseURL, secret string) *Client {
	return &Client{
		BaseURL: baseURL,
		Secret:  secret,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Token mints a short-lived websocket JWT for one document.
type TokenClaims struct {
	Doc   string `json:"doc"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Mode  string `json:"mode"` // rw | ro
	jwt.RegisteredClaims
}

func (c *Client) MintToken(docID, userID, name, color, mode string) (string, error) {
	claims := TokenClaims{
		Doc:   docID,
		Name:  name,
		Color: color,
		Mode:  mode,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(c.Secret))
}

// WSProxy reverse-proxies /collab websocket upgrades to the sidecar.
func (c *Client) WSProxy() (http.Handler, error) {
	target, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	return httputil.NewSingleHostReverseProxy(target), nil
}

// doc-ops API

type EditRequest struct {
	From   int    `json:"from"`
	To     int    `json:"to"`
	Insert string `json:"insert"`
	Origin string `json:"origin,omitempty"`
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Secret", c.Secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return errNotLoaded
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("doc-ops %s %s: %s: %s", method, path, resp.Status, data)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

var errNotLoaded = fmt.Errorf("document not loaded in collab server")

func IsNotLoaded(err error) bool { return err == errNotLoaded }

// Text returns the live text of a doc. With ifLoaded, returns errNotLoaded
// instead of forcing the sidecar to load the doc from persistence.
func (c *Client) Text(ctx context.Context, docID string, ifLoaded bool) (string, error) {
	path := "/docs/" + docID + "/text"
	if ifLoaded {
		path += "?ifLoaded=1"
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return "", err
	}
	return out.Text, nil
}

// Edit applies a range edit through the CRDT (merges with concurrent editors).
func (c *Client) Edit(ctx context.Context, docID string, req EditRequest) error {
	return c.do(ctx, http.MethodPost, "/docs/"+docID+"/edit", req, nil)
}

// SetContent replaces the doc content via minimal diff.
func (c *Client) SetContent(ctx context.Context, docID, text string) error {
	return c.do(ctx, http.MethodPut, "/docs/"+docID+"/content", map[string]string{"text": text}, nil)
}

// RelPos encodes absolute offsets into Yjs relative-position anchors (base64).
func (c *Client) RelPos(ctx context.Context, docID string, from, to int) (anchorStart, anchorEnd string, err error) {
	var out struct {
		AnchorStart string `json:"anchorStart"`
		AnchorEnd   string `json:"anchorEnd"`
	}
	err = c.do(ctx, http.MethodPost, "/docs/"+docID+"/relpos",
		map[string]int{"from": from, "to": to}, &out)
	return out.AnchorStart, out.AnchorEnd, err
}

// AbsPos decodes relative-position anchors to current absolute offsets
// (null → -1 when an anchor's context was deleted).
func (c *Client) AbsPos(ctx context.Context, docID string, anchors []string) ([]int, error) {
	var out struct {
		Positions []int `json:"positions"`
	}
	err := c.do(ctx, http.MethodPost, "/docs/"+docID+"/abspos",
		map[string][]string{"anchors": anchors}, &out)
	return out.Positions, err
}
