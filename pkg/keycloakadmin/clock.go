package keycloakadmin

import (
	"net/http"
	"time"
)

const (
	// clockSkewWarnThreshold is how far Keycloak's clock may differ from
	// this server's before it is reported. The monitor tells late events
	// from new ones by their Keycloak time.
	clockSkewWarnThreshold = 10 * time.Second
	// clockSkewWarnEvery limits the report to one per this period.
	clockSkewWarnEvery = 10 * time.Minute
)

// checkClock compares the Date header of a Keycloak response with this
// server's clock. Behind a load balancer the responses come from different
// nodes, so over time every node is compared.
func (c *Client) checkClock(resp *http.Response) {
	date, err := http.ParseTime(resp.Header.Get("Date"))
	if err != nil {
		return
	}
	now := c.timeNow()
	// Date holds whole seconds, so it is compared with the second this
	// server is in.
	difference := date.Sub(now.Truncate(time.Second))
	if difference <= clockSkewWarnThreshold && difference >= -clockSkewWarnThreshold {
		return
	}

	c.clockMu.Lock()
	if !c.lastClockWarn.IsZero() && now.Sub(c.lastClockWarn) < clockSkewWarnEvery {
		c.clockMu.Unlock()
		return
	}
	c.lastClockWarn = now
	c.clockMu.Unlock()

	c.logger.Warn("Keycloak's clock differs from this server's; events may be collected late, or missed if it exceeds the late commit window's margin",
		"keycloak", resp.Request.URL.Scheme+"://"+resp.Request.URL.Host,
		"keycloak_time", date.UTC().Format(time.RFC3339),
		"local_time", now.UTC().Format(time.RFC3339),
		"difference", difference.String())
}

func (c *Client) timeNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
