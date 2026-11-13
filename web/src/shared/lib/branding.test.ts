import { describe, it, expect, beforeEach } from "vitest";
import {
  DEFAULT_BRANDING,
  brandingSubtitle,
  filePrefix,
  rememberBranding,
  rememberedBranding,
} from "./branding";

const acme = {
  name: "Acme Monitoring Platform",
  short_name: "ACME",
  logo_text: "ACME",
};

describe("branding", () => {
  beforeEach(() => localStorage.clear());

  it("puts the rest of the name under the logo text it starts with", () => {
    expect(brandingSubtitle(DEFAULT_BRANDING)).toBe("Monitoring Tool");
    expect(brandingSubtitle(acme)).toBe("Monitoring Platform");
  });

  it("shows the whole name when it does not start with the logo text", () => {
    expect(brandingSubtitle({ ...acme, logo_text: "Ops" })).toBe(
      "Acme Monitoring Platform",
    );
  });

  it("names files like the server does", () => {
    expect(filePrefix(DEFAULT_BRANDING)).toBe("kmt");
    expect(filePrefix(acme)).toBe("acme");
    expect(filePrefix({ ...acme, short_name: "Acme EU" })).toBe("acme-eu");
    expect(filePrefix({ ...acme, short_name: "Ü" })).toBe("u");
    expect(filePrefix({ ...acme, short_name: "Café Straße Øst" })).toBe(
      "cafe-strasse-ost",
    );
    expect(filePrefix({ ...acme, short_name: "ÆØ" })).toBe("aeo");
    // A name with nothing to write in ASCII never falls back to KMT's.
    expect(filePrefix({ ...acme, short_name: "МН" })).toBe("monitoring");
    expect(filePrefix({ ...acme, short_name: "监控" })).toBe("monitoring");
    expect(filePrefix({ ...acme, short_name: "***" })).toBe("monitoring");
  });

  it("remembers the last branding, and nothing when none was stored", () => {
    expect(rememberedBranding()).toBeNull();
    rememberBranding(acme);
    expect(rememberedBranding()).toEqual(acme);
    localStorage.setItem("kmt:branding", "{not json");
    expect(rememberedBranding()).toBeNull();
    localStorage.setItem("kmt:branding", JSON.stringify({ name: 1 }));
    expect(rememberedBranding()).toBeNull();
  });
});
