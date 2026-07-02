// Package mcp exposes TypstPad to AI agents via the Model Context Protocol.
// Tools proxy to the REST API, so authorization and CRDT semantics are
// identical for humans, scripts and agents.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RESTClient struct {
	URL   string
	Token string
	HTTP  *http.Client
}

func NewRESTClient(url, token string) *RESTClient {
	return &RESTClient{URL: url, Token: token, HTTP: &http.Client{Timeout: 120 * time.Second}}
}

func (c *RESTClient) Do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.URL, "/")+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil {
		if raw, ok := out.(*string); ok {
			*raw = string(data)
			return nil
		}
		return json.Unmarshal(data, out)
	}
	return nil
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return textResult(string(data)), nil, nil
}

// NewServer builds the MCP server with all TypstPad tools bound to c.
func NewServer(c *RESTClient) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "typstpad", Version: "0.1.0"}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all Typst projects you can access, with their ids, names and your role.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		var out json.RawMessage
		if err := c.Do(ctx, "GET", "/api/projects", nil, &out); err != nil {
			return nil, nil, err
		}
		return jsonResult(out)
	})

	type projectArgs struct {
		ProjectID string `json:"project_id" jsonschema:"the project id"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_files",
		Description: "List the files in a project (text files and assets) with their ids and paths.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args projectArgs) (*mcp.CallToolResult, any, error) {
		var out json.RawMessage
		if err := c.Do(ctx, "GET", "/api/projects/"+args.ProjectID+"/files", nil, &out); err != nil {
			return nil, nil, err
		}
		return jsonResult(out)
	})

	type fileArgs struct {
		FileID string `json:"file_id" jsonschema:"the file id (from list_files)"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "read_file",
		Description: "Read the current text of a Typst file, including unsaved live edits from collaborators.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args fileArgs) (*mcp.CallToolResult, any, error) {
		var raw string
		if err := c.Do(ctx, "GET", "/api/files/"+args.FileID+"/content", nil, &raw); err != nil {
			return nil, nil, err
		}
		return textResult(raw), nil, nil
	})

	type editArgs struct {
		FileID  string  `json:"file_id" jsonschema:"the file id"`
		From    *int    `json:"from,omitempty" jsonschema:"start character offset of the range to replace"`
		To      *int    `json:"to,omitempty" jsonschema:"end character offset of the range to replace"`
		Insert  string  `json:"insert,omitempty" jsonschema:"text to insert at the range"`
		Content *string `json:"content,omitempty" jsonschema:"full new file content (alternative to from/to; merged as a minimal diff)"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "apply_edit",
		Description: "Edit a Typst file through the collaborative CRDT so concurrent human editors merge cleanly. " +
			"Provide either from/to/insert for a range edit, or content to replace the whole file. Requires editor access.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editArgs) (*mcp.CallToolResult, any, error) {
		body := map[string]any{}
		if args.Content != nil {
			body["content"] = *args.Content
		} else {
			body["from"], body["to"], body["insert"] = args.From, args.To, args.Insert
		}
		if err := c.Do(ctx, "POST", "/api/files/"+args.FileID+"/edit", body, nil); err != nil {
			return nil, nil, err
		}
		return textResult("edit applied"), nil, nil
	})

	type suggestArgs struct {
		FileID string `json:"file_id" jsonschema:"the file id"`
		Type   string `json:"type" jsonschema:"insert, delete or replace"`
		From   int    `json:"from" jsonschema:"start character offset"`
		To     int    `json:"to,omitempty" jsonschema:"end character offset (for delete/replace)"`
		Text   string `json:"text,omitempty" jsonschema:"proposed text (for insert/replace)"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "propose_suggestion",
		Description: "Propose a tracked change (suggestion) instead of editing directly. A human editor can accept " +
			"or reject it in the UI. Use this when you want review before your change lands.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args suggestArgs) (*mcp.CallToolResult, any, error) {
		var out json.RawMessage
		if err := c.Do(ctx, "POST", "/api/files/"+args.FileID+"/suggestions",
			map[string]any{"type": args.Type, "from": args.From, "to": args.To, "text": args.Text}, &out); err != nil {
			return nil, nil, err
		}
		return jsonResult(out)
	})

	type commentArgs struct {
		ProjectID string  `json:"project_id" jsonschema:"the project id"`
		FileID    *string `json:"file_id,omitempty" jsonschema:"file to anchor the comment to (optional)"`
		Body      string  `json:"body" jsonschema:"the comment text"`
		From      *int    `json:"from,omitempty" jsonschema:"anchor range start offset (optional)"`
		To        *int    `json:"to,omitempty" jsonschema:"anchor range end offset (optional)"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_comment",
		Description: "Add a comment to a project, optionally anchored to a text range in a file.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args commentArgs) (*mcp.CallToolResult, any, error) {
		var out json.RawMessage
		if err := c.Do(ctx, "POST", "/api/projects/"+args.ProjectID+"/comments",
			map[string]any{"fileId": args.FileID, "body": args.Body, "from": args.From, "to": args.To}, &out); err != nil {
			return nil, nil, err
		}
		return jsonResult(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_compile_diagnostics",
		Description: "Compile the project with the native Typst compiler and return errors/warnings with file:line:col positions.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args projectArgs) (*mcp.CallToolResult, any, error) {
		var out json.RawMessage
		if err := c.Do(ctx, "POST", "/api/projects/"+args.ProjectID+"/compile", map[string]any{}, &out); err != nil {
			return nil, nil, err
		}
		return jsonResult(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_version_history",
		Description: "List the project's version history (auto-snapshots and named versions).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args projectArgs) (*mcp.CallToolResult, any, error) {
		var out json.RawMessage
		if err := c.Do(ctx, "GET", "/api/projects/"+args.ProjectID+"/versions", nil, &out); err != nil {
			return nil, nil, err
		}
		return jsonResult(out)
	})

	type versionArgs struct {
		ProjectID string `json:"project_id" jsonschema:"the project id"`
		Name      string `json:"name" jsonschema:"a label for this version"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_version",
		Description: "Create a named version (snapshot) of the project's current state, e.g. before large edits.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args versionArgs) (*mcp.CallToolResult, any, error) {
		var out json.RawMessage
		if err := c.Do(ctx, "POST", "/api/projects/"+args.ProjectID+"/versions",
			map[string]string{"name": args.Name}, &out); err != nil {
			return nil, nil, err
		}
		return jsonResult(out)
	})

	return s
}

// HTTPHandler returns a streamable-HTTP MCP handler. Each request builds a
// server bound to the caller's bearer token, proxying to selfURL.
func HTTPHandler(selfURL string) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		return NewServer(NewRESTClient(selfURL, token))
	}, nil)
}
