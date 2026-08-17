// package: client / transport
// type:    adapter (the wire under every client call)
// job:     one GET, one POST and one result reader, so a hub error arrives as a Go error with
// the hub's own words in it. Every method in this package is a line over these three.
// limits:  no endpoints and no types of its own; what to call is the caller's.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (c *HTTP) get(path string, out any) error {
	resp, err := c.hc.Get(c.base + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readResult(resp, out)
}

func (c *HTTP) post(path string, body any) error {
	return c.postResult(path, body, nil)
}

// postResult posts body and decodes a successful JSON response into out.
func (c *HTTP) postResult(path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.hc.Post(c.base+path, "application/json", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readResult(resp, out)
}

// readResult decodes a successful JSON body into out (if non-nil), or turns a
// non-2xx into the hub's reported error.
func readResult(resp *http.Response, out any) error {
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("hub error: %s", resp.Status)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
