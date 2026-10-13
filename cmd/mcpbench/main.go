// Command mcpbench is a dependency-free MCP stdio load driver (a Go port of
// scripts/bench-editions.py's stdio mode) so the same workload can run on
// devices without Python. It spawns the server command given after "--".
//
//	mcpbench -sizes 200,2000 -samples 50 -label x -- ./thothmemory-ultralite --data DIR mcp-stdio --token-file F
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type conn struct {
	in    *bufio.Writer
	out   *bufio.Reader
	seq   int
	url   string
	token string
}

func (c *conn) post(b []byte) (map[string]any, error) {
	req, _ := http.NewRequest("POST", c.url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)
	r, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	var m map[string]any
	return m, json.NewDecoder(r.Body).Decode(&m)
}

func (c *conn) rpc(method string, params any) (map[string]any, error) {
	c.seq++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.seq, "method": method, "params": params})
	if c.url != "" {
		return c.post(b)
	}
	c.in.Write(append(b, '\n'))
	if e := c.in.Flush(); e != nil {
		return nil, e
	}
	for {
		line, e := c.out.ReadBytes('\n')
		if e != nil {
			return nil, e
		}
		var m map[string]any
		if json.Unmarshal(line, &m) == nil && m["id"] == float64(c.seq) {
			return m, nil
		}
	}
}

func (c *conn) tool(name string, args map[string]any) (any, error) {
	m, e := c.rpc("tools/call", map[string]any{"name": "memory_" + name, "arguments": args})
	if e != nil {
		return nil, e
	}
	r, _ := m["result"].(map[string]any)
	content, _ := r["content"].([]any)
	if r == nil || len(content) == 0 || r["isError"] == true {
		return nil, fmt.Errorf("%s failed: %v", name, m)
	}
	var out struct{ Data any }
	json.Unmarshal([]byte(content[0].(map[string]any)["text"].(string)), &out)
	return out.Data, nil
}

func op() string { b := make([]byte, 12); rand.Read(b); return "bench-" + hex.EncodeToString(b) }

func stats(v []float64) map[string]any {
	sort.Float64s(v)
	i := int(float64(len(v))*0.95) - 1
	if i < 0 {
		i = 0
	}
	return map[string]any{"n": len(v), "median_ms": round(v[len(v)/2]), "p95_ms": round(v[i]), "max_ms": round(v[len(v)-1])}
}
func round(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func main() {
	sizes := flag.String("sizes", "200,2000", "event totals")
	samples := flag.Int("samples", 50, "samples per size")
	label := flag.String("label", "", "label")
	url := flag.String("http", "", "MCP endpoint (loopback only) instead of a stdio command")
	tokenFile := flag.String("token-file", "", "bearer token file for -http")
	flag.Parse()
	var cmd *exec.Cmd
	var stdin io.WriteCloser
	t0 := time.Now()
	c := &conn{}
	if *url != "" {
		if !strings.HasPrefix(*url, "http://127.0.0.1:") {
			fmt.Fprintln(os.Stderr, "-http must be a loopback URL")
			os.Exit(2)
		}
		tb, e := os.ReadFile(*tokenFile)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(2)
		}
		c.url, c.token = *url, strings.TrimSpace(string(tb))
	} else {
		if flag.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "server command required after --")
			os.Exit(2)
		}
		cmd = exec.Command(flag.Arg(0), flag.Args()[1:]...)
		stdin, _ = cmd.StdinPipe()
		stdout, _ := cmd.StdoutPipe()
		cmd.Stderr = os.Stderr
		if e := cmd.Start(); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		c.in, c.out = bufio.NewWriter(stdin), bufio.NewReaderSize(stdout, 1<<20)
	}
	if _, e := c.rpc("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "mcpbench", "version": "1"}}); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if c.in != nil {
		c.in.WriteString(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
		c.in.Flush()
	}
	result := map[string]any{"label": *label, "connect_and_initialize_ms": round(float64(time.Since(t0).Microseconds()) / 1000)}
	pr, e := c.tool("create_project", map[string]any{"name": "BENCH " + *label, "repository": "local:" + uuid()})
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	project := pr.(map[string]any)["id"].(string)
	words := []string{"migration", "rollback", "cache", "handoff", "تعارض", "المراجعات", "evidence", "revision", "checkpoint", "index"}
	written := 0
	var rows []any
	for _, s := range strings.Split(*sizes, ",") {
		size, _ := strconv.Atoi(s)
		start, filled := time.Now(), 0
		for written < size {
			if _, e := c.tool("append_event", map[string]any{"project_id": project, "operation_id": op(), "body": fmt.Sprintf("note %d %s %s lorem ipsum dolor sit amet", written, words[written%10], words[(written*7)%10])}); e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
			written++
			filled++
		}
		fill := time.Since(start).Seconds()
		smp := map[string][]float64{}
		for i := 0; i < *samples; i++ {
			for _, k := range []string{"context", "search", "write"} {
				st := time.Now()
				var err error
				switch k {
				case "context":
					_, err = c.tool("get_project_context", map[string]any{"project_id": project})
				case "search":
					_, err = c.tool("search", map[string]any{"project_id": project, "query": words[i%10]})
				default:
					_, err = c.tool("append_event", map[string]any{"project_id": project, "operation_id": op(), "body": "sample write"})
				}
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				smp[k] = append(smp[k], float64(time.Since(st).Microseconds())/1000)
			}
		}
		written += *samples
		row := map[string]any{"events": written, "fill_events_per_s": round(float64(filled) / fill)}
		for k, v := range smp {
			row[k] = stats(v)
		}
		rows = append(rows, row)
	}
	result["sizes"] = rows
	if cmd != nil {
		stdin.Close()
		result["stdio_exit_ok_after_disconnect"] = cmd.Wait() == nil
	}
	b, _ := json.MarshalIndent(result, "", " ")
	fmt.Println(string(b))
}

func uuid() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
