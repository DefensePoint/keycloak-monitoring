import { describe, expect, it } from "vitest";
import { PERMISSIONS, AMFA_PERMISSIONS } from "./permissions";

describe("AMFA permissions", () => {
  it("exports AMFA_PERMISSIONS.READ as 'amfa:read'", () => {
    expect(AMFA_PERMISSIONS.READ).toBe("amfa:read");
  });

  it("includes AMFA in the PERMISSIONS root object", () => {
    expect(PERMISSIONS.AMFA).toBe(AMFA_PERMISSIONS);
  });
});
