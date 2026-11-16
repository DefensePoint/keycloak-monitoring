import { describe, it, expect } from "vitest";
import {
  HEALTH_METRICS_HINT,
  getHealthAvailability,
  getHealthMetricsHint,
} from "./healthAvailability";
import type { KeycloakHealth } from "@/shared/types";

const base: KeycloakHealth = {
  time: "",
  status: "UP",
  response_time_ms: 15,
  server_version: "26.7.5",
  uptime_millis: 3_600_000,
  memory_used_bytes: 512,
  memory_max_bytes: 1024,
  memory_free_bytes: 512,
};

const allAvailable = {
  memoryFree: true,
  memory: true,
  uptime: true,
  version: true,
};

describe("getHealthAvailability", () => {
  it("reports every detail available for full server info", () => {
    expect(getHealthAvailability(base)).toEqual(allAvailable);
  });

  it.each([null, undefined])("reports nothing available for %s", (health) => {
    expect(getHealthAvailability(health)).toEqual({
      memoryFree: false,
      memory: false,
      uptime: false,
      version: false,
    });
  });

  it("keeps a real zero used and free memory when the maximum is known", () => {
    expect(
      getHealthAvailability({
        ...base,
        memory_used_bytes: 0,
        memory_free_bytes: 0,
      }),
    ).toEqual(allAvailable);
  });

  it("treats a zero memory maximum as unavailable, also with zero used", () => {
    expect(
      getHealthAvailability({
        ...base,
        memory_used_bytes: 0,
        memory_max_bytes: 0,
      }).memory,
    ).toBe(false);
  });

  it("treats used memory above the maximum as unavailable", () => {
    expect(
      getHealthAvailability({ ...base, memory_used_bytes: 2048 }).memory,
    ).toBe(false);
  });

  it("keeps used memory equal to the maximum", () => {
    expect(
      getHealthAvailability({ ...base, memory_used_bytes: 1024 }).memory,
    ).toBe(true);
  });

  it.each([NaN, Infinity, -1])(
    "treats %s memory, free memory and uptime as unavailable",
    (value) => {
      expect(
        getHealthAvailability({
          ...base,
          memory_used_bytes: value,
          memory_free_bytes: value,
          uptime_millis: value,
        }),
      ).toEqual({
        ...allAvailable,
        memoryFree: false,
        memory: false,
        uptime: false,
      });
      expect(
        getHealthAvailability({ ...base, memory_max_bytes: value }).memory,
      ).toBe(false);
    },
  );

  it("treats zero uptime as unavailable", () => {
    expect(getHealthAvailability({ ...base, uptime_millis: 0 }).uptime).toBe(
      false,
    );
  });

  it.each(["", "  ", "unknown", "UNKNOWN", " Unknown ", undefined])(
    "treats the version %j as unavailable",
    (server_version) => {
      expect(getHealthAvailability({ ...base, server_version }).version).toBe(
        false,
      );
    },
  );
});

describe("getHealthMetricsHint", () => {
  it("gives the hint while the server is up and details are missing", () => {
    expect(getHealthMetricsHint(base, true)).toBe(HEALTH_METRICS_HINT);
  });

  it("gives no hint when nothing is missing", () => {
    expect(getHealthMetricsHint(base, false)).toBeNull();
  });

  it.each(["DOWN", "DEGRADED", "UNKNOWN"])(
    "gives no hint when the server is %s",
    (status) => {
      expect(getHealthMetricsHint({ ...base, status }, true)).toBeNull();
    },
  );

  it.each([null, undefined])("gives no hint for %s health", (health) => {
    expect(getHealthMetricsHint(health, true)).toBeNull();
  });
});
