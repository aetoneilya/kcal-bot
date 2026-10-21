import { describe, expect, it } from "vitest";
import type { Item } from "./api";
import { energyShares, grams, lineTotals, sumTotals } from "./nutrition";
import { fitWithin } from "./image";
import { addDays, atTime, dayLabel } from "./day";

const item = (over: Partial<Item>): Item => ({
  name: "x",
  grams: 100,
  kcal100: 0,
  protein100: 0,
  fat100: 0,
  carbs100: 0,
  confidence: "high",
  ...over,
});

describe("nutrition", () => {
  it("scales per-100 g density by weight", () => {
    expect(lineTotals(item({ grams: 150, kcal100: 200, protein100: 20 }))).toEqual({
      kcal: 300,
      protein: 30,
      fat: 0,
      carbs: 0,
    });
  });

  it("sums lines and treats a cleared input as zero", () => {
    const totals = sumTotals([
      item({ grams: 200, kcal100: 110 }),
      item({ grams: NaN, kcal100: 500 }),
      item({ grams: 10, kcal100: 899, fat100: 99.9 }),
    ]);
    expect(Math.round(totals.kcal)).toBe(310);
    expect(totals.fat).toBeCloseTo(9.99);
  });

  it("shows small macros with a decimal and large ones whole", () => {
    expect(grams(4.26)).toBe("4,3");
    expect(grams(42.6)).toBe("43");
  });

  it("splits energy by macro and stays empty on an empty day", () => {
    const shares = energyShares({ kcal: 0, protein: 25, fat: 0, carbs: 75 });
    expect(shares).toEqual({ protein: 0.25, fat: 0, carbs: 0.75 });
    expect(energyShares({ kcal: 0, protein: 0, fat: 0, carbs: 0 })).toEqual({ protein: 0, fat: 0, carbs: 0 });
  });
});

describe("image", () => {
  it("fits the long side and keeps small photos as they are", () => {
    expect(fitWithin(4032, 3024)).toEqual({ width: 1280, height: 960 });
    expect(fitWithin(3024, 4032)).toEqual({ width: 960, height: 1280 });
    expect(fitWithin(800, 600)).toEqual({ width: 800, height: 600 });
  });
});

describe("day", () => {
  it("labels today and yesterday", () => {
    const now = new Date(2026, 9, 19, 9, 0);
    expect(dayLabel(new Date(2026, 9, 19), now)).toBe("Сегодня");
    expect(dayLabel(addDays(now, -1), now)).toBe("Вчера");
  });

  it("puts a time onto a given day", () => {
    expect(atTime(new Date(2026, 9, 18), "13:45")).toEqual(new Date(2026, 9, 18, 13, 45));
  });
});
