import { describe, expect, it } from "vitest";

import { hasEventLocation } from "./eventLocation";

describe("hasEventLocation", () => {
  it("accepts real coordinates", () => {
    expect(hasEventLocation({ lat: 38.72, long: -9.14 })).toBe(true);
    expect(hasEventLocation({ lat: -23.55, long: -46.63 })).toBe(true);
  });

  it("rejects events with no coordinates", () => {
    // Plain Keycloak events carry no AMFA geo fields at all, which is the
    // common case: most rows must not get a pin.
    expect(hasEventLocation({})).toBe(false);
    expect(hasEventLocation({ lat: 38.72 })).toBe(false);
    expect(hasEventLocation({ long: -9.14 })).toBe(false);
    expect(hasEventLocation({ lat: undefined, long: undefined })).toBe(false);
  });

  it("rejects Null Island", () => {
    // A failed GeoIP lookup often returns exactly 0,0, which is a real point
    // in the Gulf of Guinea. Mapping it would claim a location we don't have.
    expect(hasEventLocation({ lat: 0, long: 0 })).toBe(false);
  });

  it("keeps genuine coordinates that have one zero component", () => {
    // Only the 0,0 pair is suspect. The equator and the prime meridian are
    // legitimate on their own.
    expect(hasEventLocation({ lat: 0, long: -9.14 })).toBe(true);
    expect(hasEventLocation({ lat: 51.48, long: 0 })).toBe(true);
  });

  it("rejects out-of-range and non-finite values", () => {
    // Bad upstream data would otherwise reach Leaflet, which throws on an
    // invalid LatLng and takes the surrounding dialog down with it.
    expect(hasEventLocation({ lat: 91, long: 0 })).toBe(false);
    expect(hasEventLocation({ lat: 0, long: 181 })).toBe(false);
    expect(hasEventLocation({ lat: -90.1, long: 10 })).toBe(false);
    expect(hasEventLocation({ lat: NaN, long: 10 })).toBe(false);
    expect(hasEventLocation({ lat: 10, long: Infinity })).toBe(false);
  });

  it("accepts the exact range boundaries", () => {
    expect(hasEventLocation({ lat: 90, long: 180 })).toBe(true);
    expect(hasEventLocation({ lat: -90, long: -180 })).toBe(true);
  });
});
