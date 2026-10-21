import { describe, expect, it } from "vitest";
import { dayStat, monthGrid, summarize } from "./calendar";

const now = new Date(2026, 9, 21, 15, 0); // Wed 21 Oct 2026

describe("calendar", () => {
  it("lays a month out in Monday-first weeks", () => {
    const weeks = monthGrid(new Date(2026, 9, 5)); // October 2026 starts on a Thursday
    expect(weeks).toHaveLength(5);
    expect(weeks[0].slice(0, 3)).toEqual([null, null, null]);
    expect(weeks[0][3]).toEqual(new Date(2026, 9, 1));
    expect(weeks[4][5]).toEqual(new Date(2026, 9, 31));
    expect(weeks.every((w) => w.length === 7)).toBe(true);
  });

  it("judges finished days against the target with a 10% band", () => {
    const day = new Date(2026, 9, 20);
    expect(dayStat(day, 1500, 2000, now)).toMatchObject({ status: "under", delta: -500 });
    expect(dayStat(day, 2150, 2000, now)).toMatchObject({ status: "ok", delta: 150 });
    expect(dayStat(day, 2400, 2000, now)).toMatchObject({ status: "over", delta: 400 });
  });

  it("never judges an empty day, today, or a day without a target", () => {
    expect(dayStat(new Date(2026, 9, 19), 0, 2000, now).status).toBe("empty");
    expect(dayStat(new Date(2026, 9, 21), 900, 2000, now).status).toBe("open");
    expect(dayStat(new Date(2026, 9, 19), 1800, null, now)).toMatchObject({ status: "open", delta: null });
  });

  it("sums a month", () => {
    const stats = [
      dayStat(new Date(2026, 9, 18), 1500, 2000, now),
      dayStat(new Date(2026, 9, 19), 2400, 2000, now),
      dayStat(new Date(2026, 9, 20), 0, 2000, now),
      dayStat(new Date(2026, 9, 21), 900, 2000, now),
    ];
    expect(summarize(stats)).toEqual({ logged: 3, under: 1, ok: 0, over: 1, average: 1600, balance: -100 });
  });
});
