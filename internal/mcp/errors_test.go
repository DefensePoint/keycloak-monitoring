package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToClientErrorMapsSentinels(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"forbidden", ErrForbidden, ErrForbidden},
		{"wrapped forbidden", fmt.Errorf("checking tenant xyz: %w", ErrForbidden), ErrForbidden},
		{"unauthenticated", ErrUnauthenticated, ErrUnauthenticated},
		{"tenant not available", ErrTenantNotAvailable, ErrTenantNotAvailable},
		{"invalid input sentinel", ErrInvalidInput, ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toClientError(tc.in)
			if got.Error() != tc.want.Error() {
				t.Fatalf("got %q, want %q", got.Error(), tc.want.Error())
			}
		})
	}
}

func TestToClientErrorHidesInternalDetail(t *testing.T) {
	internal := errors.New("pq: connection refused host=10.0.0.5 user=monitoring")
	got := toClientError(fmt.Errorf("query events: %w", internal))
	if got.Error() != "internal error" {
		t.Fatalf("got %q, want generic internal error", got.Error())
	}
}

func TestToClientErrorKeepsInvalidfMessage(t *testing.T) {
	got := toClientError(Invalidf("from must be before to"))
	want := "invalid input: from must be before to"
	if got.Error() != want {
		t.Fatalf("got %q, want %q", got.Error(), want)
	}
	if !errors.Is(got, ErrInvalidInput) {
		t.Fatal("Invalidf error should classify as ErrInvalidInput")
	}
}

func TestWrapToolRecoversPanic(t *testing.T) {
	handler := wrapTool(testGuard(), "panicky", func(context.Context, *mcpsdk.CallToolRequest, whoamiInput) (*mcpsdk.CallToolResult, whoamiOutput, error) {
		panic("secret internal state: db password hunter2")
	})

	res, _, err := handler(context.Background(), &mcpsdk.CallToolRequest{}, whoamiInput{})
	if res != nil {
		t.Fatalf("expected nil result after panic, got %+v", res)
	}
	if err == nil {
		t.Fatal("expected error after panic")
	}
	if err.Error() != "internal error" {
		t.Fatalf("panic leaked to client: %q", err.Error())
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatal("panic payload reached the client error")
	}
}

func TestWrapToolMapsHandlerError(t *testing.T) {
	internal := errors.New("gorm: table keycloak_events does not exist")
	handler := wrapTool(testGuard(), "failing", func(context.Context, *mcpsdk.CallToolRequest, whoamiInput) (*mcpsdk.CallToolResult, whoamiOutput, error) {
		return nil, whoamiOutput{}, internal
	})

	_, _, err := handler(context.Background(), &mcpsdk.CallToolRequest{}, whoamiInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "internal error" {
		t.Fatalf("internal detail leaked: %q", err.Error())
	}
}

func TestWrapToolPassesSuccessThrough(t *testing.T) {
	want := whoamiOutput{Subject: "user-7", ServerVersion: "test"}
	handler := wrapTool(testGuard(), "ok", func(context.Context, *mcpsdk.CallToolRequest, whoamiInput) (*mcpsdk.CallToolResult, whoamiOutput, error) {
		return nil, want, nil
	})

	_, out, err := handler(context.Background(), &mcpsdk.CallToolRequest{}, whoamiInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Subject != want.Subject || out.ServerVersion != want.ServerVersion {
		t.Fatalf("got %+v, want %+v", out, want)
	}
}
