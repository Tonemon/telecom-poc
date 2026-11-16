// Package docker is a minimal Docker Engine API client over the Unix socket: list a compose
// project's containers and run commands in them. Standard library only.
package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Container struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Service   string    `json:"service"`
	State     string    `json:"state"`  // running, exited, restarting, ...
	Health    string    `json:"health"` // healthy, unhealthy, starting, or "" without a healthcheck
	Hostname  string    `json:"hostname"`
	StartedAt time.Time `json:"started_at"`
}

// ExecResult is what a command run in a container produced.
type ExecResult struct {
	Stdout string
	Stderr string
	Exit   int
}

// Client is what the agent needs from Docker.
type Client interface {
	List(ctx context.Context, project string) ([]Container, error)
	// Exec runs cmd in the container until it exits.
	Exec(ctx context.Context, container string, cmd []string) (ExecResult, error)
	// ExecStream runs cmd and calls line for every stdout line until it exits or ctx ends.
	ExecStream(ctx context.Context, container string, cmd []string, line func(string)) error
}

type socketClient struct{ http *http.Client }

// NewSocket talks to the Docker daemon at the Unix socket path.
func NewSocket(path string) Client {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	return &socketClient{http: &http.Client{Transport: tr}}
}

func (c *socketClient) do(ctx context.Context, method, path string, body any, out any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docker %s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		defer resp.Body.Close()
		return resp, json.NewDecoder(resp.Body).Decode(out)
	}
	return resp, nil
}

func (c *socketClient) List(ctx context.Context, project string) ([]Container, error) {
	filters, _ := json.Marshal(map[string][]string{"label": {"com.docker.compose.project=" + project}})
	var list []struct {
		ID     string `json:"Id"`
		Names  []string
		Labels map[string]string
	}
	if _, err := c.do(ctx, "GET", "/containers/json?all=1&filters="+url.QueryEscape(string(filters)), nil, &list); err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(list))
	for _, l := range list {
		var in struct {
			Name  string
			State struct {
				Status    string
				StartedAt time.Time
				Health    *struct{ Status string }
			}
			Config struct{ Hostname string }
		}
		if _, err := c.do(ctx, "GET", "/containers/"+l.ID+"/json", nil, &in); err != nil {
			return nil, err
		}
		ct := Container{ID: l.ID, Name: strings.TrimPrefix(in.Name, "/"), Service: l.Labels["com.docker.compose.service"],
			State: in.State.Status, Hostname: in.Config.Hostname, StartedAt: in.State.StartedAt}
		if in.State.Health != nil {
			ct.Health = in.State.Health.Status
		}
		out = append(out, ct)
	}
	return out, nil
}

func (c *socketClient) start(ctx context.Context, container string, cmd []string) (string, io.ReadCloser, error) {
	var created struct {
		ID string `json:"Id"`
	}
	if _, err := c.do(ctx, "POST", "/containers/"+container+"/exec",
		map[string]any{"Cmd": cmd, "AttachStdout": true, "AttachStderr": true}, &created); err != nil {
		return "", nil, err
	}
	resp, err := c.do(ctx, "POST", "/exec/"+created.ID+"/start", map[string]any{"Detach": false, "Tty": false}, nil)
	if err != nil {
		return "", nil, err
	}
	return created.ID, resp.Body, nil
}

func (c *socketClient) Exec(ctx context.Context, container string, cmd []string) (ExecResult, error) {
	id, body, err := c.start(ctx, container, cmd)
	if err != nil {
		return ExecResult{Exit: -1}, err
	}
	defer body.Close()
	var out, errOut bytes.Buffer
	res := ExecResult{Exit: -1}
	err = demux(body, &out, &errOut)
	res.Stdout, res.Stderr = out.String(), errOut.String()
	if err != nil {
		return res, err
	}
	var st struct{ ExitCode int }
	if _, err := c.do(ctx, "GET", "/exec/"+id+"/json", nil, &st); err != nil {
		return res, err
	}
	res.Exit = st.ExitCode
	return res, nil
}

func (c *socketClient) ExecStream(ctx context.Context, container string, cmd []string, line func(string)) error {
	_, body, err := c.start(ctx, container, cmd)
	if err != nil {
		return err
	}
	defer body.Close()
	w := &lineWriter{fn: line}
	err = demux(body, w, io.Discard)
	w.Flush()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// demux splits Docker's multiplexed attach stream (8-byte header: stream, 0, 0, 0, size) into
// stdout and stderr.
func demux(r io.Reader, stdout, stderr io.Writer) error {
	var hdr [8]byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("docker stream header: %w", err)
		}
		dst := stdout
		if hdr[0] == 2 {
			dst = stderr
		}
		if _, err := io.CopyN(dst, r, int64(binary.BigEndian.Uint32(hdr[4:]))); err != nil {
			return fmt.Errorf("docker stream frame: %w", err)
		}
	}
}

// lineWriter calls fn for every complete line written to it; Flush emits the rest.
type lineWriter struct {
	fn  func(string)
	buf []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.fn(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
}

func (w *lineWriter) Flush() {
	if len(w.buf) > 0 {
		w.fn(string(w.buf))
		w.buf = nil
	}
}
