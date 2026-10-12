package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ClientConfig struct {
	Endpoint  string `json:"endpoint"`
	TokenFile string `json:"token_file"`
}
type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}
func LoadConfig(path string) (ClientConfig, string, error) {
	var c ClientConfig
	b, e := os.ReadFile(path)
	if e != nil {
		return c, "", e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, "", e
	}
	u, e := url.Parse(c.Endpoint)
	if e != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") || u.User != nil {
		return c, "", errors.New("client endpoint must be local HTTP")
	}
	if !filepath.IsAbs(c.TokenFile) {
		c.TokenFile = filepath.Join(filepath.Dir(path), c.TokenFile)
	}
	b, e = os.ReadFile(c.TokenFile)
	return c, strings.TrimSpace(string(b)), e
}
func Client(ctx context.Context, path string) (*mcp.ClientSession, error) {
	c, token, e := LoadConfig(path)
	if e != nil {
		return nil, e
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "thothmemory-cli", Version: "0.1.1"}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: c.Endpoint, HTTPClient: &http.Client{Transport: bearer{token}, Timeout: 20 * time.Second}}, nil)
}
func Doctor(ctx context.Context, path string) error {
	cs, e := Client(ctx, path)
	if e != nil {
		return errors.New("MCP handshake failed; check server, endpoint and token file")
	}
	defer cs.Close()
	ls, e := cs.ListTools(ctx, nil)
	if e != nil {
		return e
	}
	res, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "memory_get_health", Arguments: map[string]any{}})
	if e != nil {
		return e
	}
	if res.IsError {
		return errors.New("MCP health tool failed")
	}
	fmt.Printf("MCP handshake: OK\nAuthentication: OK\nTool discovery: %d tools\nDatabase: ready\n", len(ls.Tools))
	return nil
}
func Bridge(ctx context.Context, path string) error {
	cs, e := Client(ctx, path)
	if e != nil {
		return errors.New("ThothMemory unavailable; run thothmemory doctor")
	}
	defer cs.Close()
	ls, e := cs.ListTools(ctx, nil)
	if e != nil {
		return e
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "ThothMemory-bridge", Version: "0.1.1"}, nil)
	for _, tool := range ls.Tools {
		srv.AddTool(tool, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return cs.CallTool(ctx, &mcp.CallToolParams{Name: r.Params.Name, Arguments: r.Params.Arguments})
		})
	}
	return srv.Run(ctx, &mcp.StdioTransport{})
}

func Preflight(ctx context.Context, path string) error {
	c, _, e := LoadConfig(path)
	if e != nil {
		return e
	}
	u, _ := url.Parse(c.Endpoint)
	listener, e := net.Listen("tcp", u.Host)
	if e == nil {
		listener.Close()
		fmt.Println("Loopback port available")
		return nil
	}
	if e = Doctor(ctx, path); e == nil {
		fmt.Println("Existing ThothMemory installation detected")
		return nil
	}
	return errors.New("configured port is occupied and did not authenticate as this installation; select a free THOTH_PORT and matching client endpoint")
}
