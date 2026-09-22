import { describe, it, expect } from "vitest";
import {
  isTenantConnectionBroken,
  tenantConnectionError,
} from "./tenantHealth";
import type { Tenant } from "@/shared/types";

const base: Tenant = {
  id: 1,
  tenant_id: "t1",
  name: "T1",
  server_url: "http://kc",
  admin_realm: "master",
  enabled: true,
  last_health_check: "",
  health_status: "healthy",
  is_default: false,
  created_at: "",
  updated_at: "",
};

describe("isTenantConnectionBroken", () => {
  it("returns false for healthy", () => {
    expect(
      isTenantConnectionBroken({ ...base, health_status: "healthy" }),
    ).toBe(false);
  });
  it("returns false for unknown (not yet checked)", () => {
    expect(
      isTenantConnectionBroken({ ...base, health_status: "unknown" }),
    ).toBe(false);
  });
  it("returns true for unhealthy", () => {
    expect(
      isTenantConnectionBroken({ ...base, health_status: "unhealthy" }),
    ).toBe(true);
  });
  it("returns true for degraded", () => {
    expect(
      isTenantConnectionBroken({ ...base, health_status: "degraded" }),
    ).toBe(true);
  });
  it("is case-insensitive", () => {
    expect(
      isTenantConnectionBroken({ ...base, health_status: "UNHEALTHY" }),
    ).toBe(true);
  });
  it("returns false for null/undefined tenant", () => {
    expect(isTenantConnectionBroken(null)).toBe(false);
    expect(isTenantConnectionBroken(undefined)).toBe(false);
  });
});

describe("tenantConnectionError", () => {
  it("prefers last_error", () => {
    expect(
      tenantConnectionError({
        ...base,
        last_error: "connection refused",
        health_message: "x",
      }),
    ).toBe("connection refused");
  });
  it("falls back to health_message", () => {
    expect(
      tenantConnectionError({
        ...base,
        last_error: undefined,
        health_message: "auth failed",
      }),
    ).toBe("auth failed");
  });
  it("returns undefined when neither set", () => {
    expect(
      tenantConnectionError({
        ...base,
        last_error: undefined,
        health_message: undefined,
      }),
    ).toBeUndefined();
  });
});
