// Package healthcheck backs the binaries' -healthcheck flag. The runtime
// images are distroless (no shell, no curl), so container healthchecks can
// only be expressed as the binary probing itself.
package healthcheck

import (
	"fmt"
	"net/http"
	"time"
)

// Probe returns nil when the URL answers 200 within the timeout.
func Probe(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return nil
}
