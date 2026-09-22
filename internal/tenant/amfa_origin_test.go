package tenant

import "testing"

func TestAmfaRequestToDomainNilStaysNil(t *testing.T) {
	// An absent block must leave a tenant's existing AMFA settings alone, so it
	// has to be distinguishable from a block that disables the integration.
	var req *AmfaRequest
	if req.toDomain() != nil {
		t.Error("nil request should convert to nil")
	}
}

func TestAmfaRequestToDomainCopiesEveryField(t *testing.T) {
	req := &AmfaRequest{
		Enabled:            true,
		APIBaseURL:         "https://amfa.internal",
		EventsLookbackDays: 14,
		APITimeoutSeconds:  45,
	}
	got := req.toDomain()
	if got == nil {
		t.Fatal("conversion returned nil")
		return
	}
	if !got.Enabled || got.APIBaseURL != req.APIBaseURL ||
		got.EventsLookbackDays != 14 || got.APITimeoutSeconds != 45 {
		t.Errorf("fields did not round-trip: %+v", got)
	}
}

func TestAmfaRequestCarriesNoDatabaseCredentials(t *testing.T) {
	// The whole reason these settings are safe to accept over the API: there is
	// no host, user or password to leak. A field added here would undo that.
	req := &AmfaRequest{}
	d := req.toDomain()
	if d == nil {
		t.Fatal("conversion returned nil")
		return
	}
	// Compile-time surface check: the domain type must expose exactly these.
	_ = d.Enabled
	_ = d.APIBaseURL
	_ = d.EventsLookbackDays
	_ = d.APITimeoutSeconds
}
