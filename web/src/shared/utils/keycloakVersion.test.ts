import { describe, it, expect } from "vitest";
import { enteredVersionNote, shownKeycloakVersion } from "./keycloakVersion";
import type {
  KeycloakHealth,
  KeycloakVersionInfo,
} from "@/shared/types/keycloak.types";

const health = (server_version?: string): KeycloakHealth => ({
  time: "2026-11-20T09:00:00Z",
  status: "UP",
  response_time_ms: 5,
  server_version,
  uptime_millis: 0,
  memory_used_bytes: 0,
  memory_max_bytes: 0,
  memory_free_bytes: 0,
});

const info = (over: Partial<KeycloakVersionInfo>): KeycloakVersionInfo => ({
  current_version: "26.4.2",
  latest_keycloak_version: "26.8.2",
  keycloak_release_url: "",
  is_keycloak_outdated: true,
  has_security_updates: false,
  ...over,
});

describe("shownKeycloakVersion", () => {
  it("shows the version Keycloak reports", () => {
    expect(
      shownKeycloakVersion(
        health("26.8.2"),
        info({ version_source: "keycloak" }),
      ),
    ).toEqual({ version: "26.8.2", entered: false });
  });

  it("prefers the reported version to an entered one", () => {
    const shown = shownKeycloakVersion(
      health("26.8.2"),
      info({ version_source: "tenant_settings" }),
    );
    expect(shown).toEqual({ version: "26.8.2", entered: false });
  });

  it("falls back to the entered version with the day it was set", () => {
    const shown = shownKeycloakVersion(
      health(""),
      info({
        version_source: "tenant_settings",
        version_set_at: "2026-11-20T09:00:00Z",
      }),
    );
    expect(shown).toEqual({
      version: "26.4.2",
      entered: true,
      setOn: "Nov 20, 2026",
    });
    expect(enteredVersionNote(shown)).toBe(
      "Entered in the tenant settings on Nov 20, 2026",
    );
  });

  it("treats a reported 'unknown' as no version", () => {
    const shown = shownKeycloakVersion(
      health("unknown"),
      info({ version_source: "tenant_settings" }),
    );
    expect(shown.version).toBe("26.4.2");
    expect(shown.entered).toBe(true);
    expect(enteredVersionNote(shown)).toBe("Entered in the tenant settings");
  });

  it("knows no version when none was reported or entered", () => {
    expect(shownKeycloakVersion(health(""), null)).toEqual({
      version: "",
      entered: false,
    });
    expect(shownKeycloakVersion(undefined, undefined)).toEqual({
      version: "",
      entered: false,
    });
    expect(enteredVersionNote({ version: "", entered: false })).toBe("");
  });

  it("falls back to the tenant's own entered version when there is no update check result", () => {
    const shown = shownKeycloakVersion(health(""), null, {
      keycloak_version: "26.4.2",
      keycloak_version_set_at: "2026-11-20T09:00:00Z",
    });
    expect(shown).toEqual({
      version: "26.4.2",
      entered: true,
      setOn: "Nov 20, 2026",
    });
  });

  it("prefers the update check's entered version to the tenant's", () => {
    const shown = shownKeycloakVersion(
      health(""),
      info({ current_version: "26.5.0", version_source: "tenant_settings" }),
      { keycloak_version: "26.4.2" },
    );
    expect(shown.version).toBe("26.5.0");
  });

  it("prefers the reported version to the tenant's entered one", () => {
    expect(
      shownKeycloakVersion(health("26.8.2"), null, {
        keycloak_version: "26.4.2",
      }),
    ).toEqual({ version: "26.8.2", entered: false });
  });
});
