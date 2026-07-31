package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/telecom-poc/provisioner/internal/provisioning"
)

type Client struct {
	base  string
	token string
	http  *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{base: baseURL, token: token, http: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) do(method, path string, body, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, c.base+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e ErrorResponse
		b, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(b, &e)
		if e.Error == "" {
			e.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("server: %s", e.Error)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) Issue(req IssueRequest) ([]IssuedSIM, error) {
	var out []IssuedSIM
	return out, c.do(http.MethodPost, "/subscribers/issue", req, &out)
}
func (c *Client) Add(req AddRequest) (IssuedSIM, error) {
	var out IssuedSIM
	return out, c.do(http.MethodPost, "/subscribers", req, &out)
}
func (c *Client) Remove(imsi string) error {
	return c.do(http.MethodDelete, "/subscribers/"+imsi, nil, nil)
}
func (c *Client) Suspend(imsi string, req ReasonRequest) error {
	return c.do(http.MethodPost, "/subscribers/"+imsi+"/suspend", req, nil)
}
func (c *Client) Resume(imsi string, req ReasonRequest) error {
	return c.do(http.MethodPost, "/subscribers/"+imsi+"/resume", req, nil)
}
func (c *Client) SetPlan(imsi string, req PlanRequest) error {
	return c.do(http.MethodPatch, "/subscribers/"+imsi+"/plan", req, nil)
}
func (c *Client) SetIP(imsi string, req IPRequest) error {
	return c.do(http.MethodPatch, "/subscribers/"+imsi+"/ip", req, nil)
}
func (c *Client) List() ([]SubscriberView, error) {
	var out []SubscriberView
	return out, c.do(http.MethodGet, "/subscribers", nil, &out)
}
func (c *Client) Get(imsi string) (SubscriberView, error) {
	var out SubscriberView
	return out, c.do(http.MethodGet, "/subscribers/"+imsi, nil, &out)
}
func (c *Client) History(imsi string) ([]provisioning.AuditRecord, error) {
	var out []provisioning.AuditRecord
	return out, c.do(http.MethodGet, "/subscribers/"+imsi+"/history", nil, &out)
}
