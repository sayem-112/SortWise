import { describe, expect, it } from "vitest";
import { parseTime } from "./format";

describe("parseTime", () => {
  it("reads the server's zone-less times as UTC", () => {
    expect(parseTime("2026-10-07 17:23:28").toISOString()).toBe("2026-10-07T17:23:28.000Z");
  });

  it("leaves full ISO times alone", () => {
    expect(parseTime("2026-10-07T17:23:28Z").toISOString()).toBe("2026-10-07T17:23:28.000Z");
    expect(parseTime("2026-10-07T23:23:28+06:00").toISOString()).toBe("2026-10-07T17:23:28.000Z");
  });
});
