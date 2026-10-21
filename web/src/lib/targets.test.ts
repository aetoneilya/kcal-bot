import { describe, expect, it } from "vitest";
import { targets, type Profile } from "./targets";

const now = new Date(2026, 9, 21);
const base: Profile = {
  sex: "male",
  birthYear: 1996, // 30
  heightCm: 180,
  weightKg: 80,
  goalWeightKg: 80,
  activity: "moderate",
  paceKgWeek: 0.5,
};

describe("targets", () => {
  it("uses Mifflin–St Jeor times the activity factor to hold weight", () => {
    const t = targets(base, now)!;
    // 10·80 + 6.25·180 − 5·30 + 5 = 1780; ×1.55 = 2759
    expect(t.bmr).toBe(1780);
    expect(t.tdee).toBe(2759);
    expect(t.kcal).toBe(2760);
    expect(t.direction).toBe("hold");
    expect(t.weeks).toBeNull();
  });

  it("subtracts the planned weekly loss and estimates the time to goal", () => {
    const t = targets({ ...base, goalWeightKg: 74 }, now)!;
    // 0.5 kg/week × 7700 / 7 = 550 kcal a day
    expect(t.kcal).toBe(2210);
    expect(t.direction).toBe("lose");
    expect(t.weeks).toBe(12);
    expect(t.protein).toBe(118); // 1.6 g per kg of the goal weight
    expect(t.limitedBy).toBeNull();
  });

  it("caps an aggressive deficit and never goes below the floor", () => {
    const fast = targets({ ...base, goalWeightKg: 70, paceKgWeek: 1 }, now)!;
    expect(fast.limitedBy).toBe("deficit");
    expect(fast.kcal).toBe(Math.round((2759 * 0.75) / 10) * 10);

    const small = targets(
      { ...base, sex: "female", heightCm: 150, weightKg: 48, goalWeightKg: 44, activity: "sedentary", paceKgWeek: 1 },
      now,
    )!;
    expect(small.kcal).toBe(1200);
    expect(small.limitedBy).toBe("floor");
  });

  it("adds a capped surplus to gain", () => {
    const t = targets({ ...base, goalWeightKg: 86, paceKgWeek: 0.25 }, now)!;
    expect(t.direction).toBe("gain");
    expect(t.kcal).toBe(3030); // 2759 + 275, to the nearest 10
  });

  it("splits the target into macros that add up to it", () => {
    const t = targets({ ...base, goalWeightKg: 74 }, now)!;
    const fromMacros = t.protein * 4 + t.fat * 9 + t.carbs * 4;
    expect(Math.abs(fromMacros - t.kcal)).toBeLessThan(10);
  });

  it("stays empty until every field is valid", () => {
    expect(targets({ ...base, heightCm: 1.8 }, now)).toBeNull();
    expect(targets({ ...base, birthYear: 2015 }, now)).toBeNull();
    const { activity: _, ...noActivity } = base;
    expect(targets(noActivity, now)).toBeNull();
  });
});
