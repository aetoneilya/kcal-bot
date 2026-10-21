/**
 * Daily calorie and macro targets from a profile.
 *
 * Resting energy is Mifflin–St Jeor, the equation the Academy of Nutrition and
 * Dietetics found closest to measured values for most adults (±10%). It is
 * scaled by a standard activity factor, then shifted by the energy in the
 * planned weekly weight change (~7700 kcal per kg of body fat). Guard rails
 * keep the result sane: a deficit never exceeds a quarter of maintenance and
 * intake never drops below the common 1200 / 1500 kcal floors.
 */

export type Sex = "male" | "female";
export type Activity = "sedentary" | "light" | "moderate" | "active" | "very_active";

export interface Profile {
  sex: Sex;
  birthYear: number;
  heightCm: number;
  weightKg: number;
  goalWeightKg: number;
  activity: Activity;
  paceKgWeek: number;
}

export const activities: { key: Activity; label: string; hint: string; factor: number }[] = [
  { key: "sedentary", label: "Почти без движения", hint: "Сидячая работа, спорта нет", factor: 1.2 },
  { key: "light", label: "Лёгкая", hint: "Тренировки 1–3 раза в неделю или много ходьбы", factor: 1.375 },
  { key: "moderate", label: "Средняя", hint: "Тренировки 3–5 раз в неделю", factor: 1.55 },
  { key: "active", label: "Высокая", hint: "Тренировки 6–7 раз в неделю", factor: 1.725 },
  { key: "very_active", label: "Очень высокая", hint: "Физическая работа и тренировки", factor: 1.9 },
];

/** Same bounds the server enforces. */
export const limits = {
  age: [16, 100],
  heightCm: [120, 230],
  weightKg: [30, 300],
  paceKgWeek: [0, 1],
} as const;

export const KCAL_PER_KG = 7700;
const MAX_DEFICIT_SHARE = 0.25;
const MAX_SURPLUS_SHARE = 0.15;
const FLOOR: Record<Sex, number> = { male: 1500, female: 1200 };
/** Within this many kg of the goal the plan is to hold weight. */
const HOLD_BAND_KG = 0.5;

export type Direction = "lose" | "hold" | "gain";

export interface Targets {
  bmr: number;
  tdee: number;
  kcal: number;
  protein: number;
  fat: number;
  carbs: number;
  direction: Direction;
  /** Weight change per week the target actually delivers, kg (signed). */
  weeklyKg: number;
  /** Weeks to the goal at that rate; null when holding. */
  weeks: number | null;
  /** Set when a guard rail moved the target away from the chosen pace. */
  limitedBy: "deficit" | "floor" | "surplus" | null;
}

export function direction(p: Pick<Profile, "weightKg" | "goalWeightKg">): Direction {
  const diff = p.goalWeightKg - p.weightKg;
  if (Math.abs(diff) < HOLD_BAND_KG) return "hold";
  return diff < 0 ? "lose" : "gain";
}

/** Null until every field is present and within bounds. */
export function targets(p: Partial<Profile>, now = new Date()): Targets | null {
  if (!isComplete(p, now)) return null;
  const age = now.getFullYear() - p.birthYear;
  const bmr = 10 * p.weightKg + 6.25 * p.heightCm - 5 * age + (p.sex === "male" ? 5 : -161);
  const factor = activities.find((a) => a.key === p.activity)!.factor;
  const tdee = bmr * factor;

  const dir = direction(p);
  const planned = dir === "hold" ? 0 : (p.paceKgWeek * KCAL_PER_KG) / 7;
  let kcal = tdee;
  let limitedBy: Targets["limitedBy"] = null;
  if (dir === "lose") {
    const deficit = Math.min(planned, tdee * MAX_DEFICIT_SHARE);
    if (deficit < planned) limitedBy = "deficit";
    kcal = tdee - deficit;
    if (kcal < FLOOR[p.sex]) {
      kcal = Math.min(FLOOR[p.sex], tdee);
      limitedBy = "floor";
    }
  } else if (dir === "gain") {
    const surplus = Math.min(planned, tdee * MAX_SURPLUS_SHARE);
    if (surplus < planned) limitedBy = "surplus";
    kcal = tdee + surplus;
  }
  // The pace comes from the exact figure; the shown target is rounded to 10.
  const weeklyKg = ((kcal - tdee) * 7) / KCAL_PER_KG;
  kcal = Math.round(kcal / 10) * 10;

  // Protein scales with body mass; when losing, the goal weight stands in for
  // lean mass so a heavy start does not inflate it. Fat takes 30% of energy,
  // carbs the rest.
  const proteinBase = dir === "lose" ? p.goalWeightKg : p.weightKg;
  const protein = Math.round(1.6 * proteinBase);
  const fat = Math.round((kcal * 0.3) / 9);
  const carbs = Math.max(0, Math.round((kcal - protein * 4 - fat * 9) / 4));

  const weeks =
    dir === "hold" || Math.abs(weeklyKg) < 0.01
      ? null
      : Math.round(Math.abs(p.goalWeightKg - p.weightKg) / Math.abs(weeklyKg));

  return {
    bmr: Math.round(bmr),
    tdee: Math.round(tdee),
    kcal,
    protein,
    fat,
    carbs,
    direction: dir,
    weeklyKg,
    weeks,
    limitedBy,
  };
}

function isComplete(p: Partial<Profile>, now: Date): p is Profile {
  const within = (v: number | undefined, [lo, hi]: readonly [number, number]) =>
    typeof v === "number" && Number.isFinite(v) && v >= lo && v <= hi;
  return (
    (p.sex === "male" || p.sex === "female") &&
    typeof p.birthYear === "number" &&
    within(now.getFullYear() - p.birthYear, limits.age) &&
    within(p.heightCm, limits.heightCm) &&
    within(p.weightKg, limits.weightKg) &&
    within(p.goalWeightKg, limits.weightKg) &&
    within(p.paceKgWeek, limits.paceKgWeek) &&
    activities.some((a) => a.key === p.activity)
  );
}
