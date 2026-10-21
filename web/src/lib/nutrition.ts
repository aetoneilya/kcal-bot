import type { Item } from "./api";

export interface Totals {
  kcal: number;
  protein: number;
  fat: number;
  carbs: number;
}

const zero: Totals = { kcal: 0, protein: 0, fat: 0, carbs: 0 };

/** What one line contributes: per-100 g density times the weight. */
export function lineTotals(item: Item): Totals {
  const k = (Number(item.grams) || 0) / 100;
  return {
    kcal: k * (Number(item.kcal100) || 0),
    protein: k * (Number(item.protein100) || 0),
    fat: k * (Number(item.fat100) || 0),
    carbs: k * (Number(item.carbs100) || 0),
  };
}

export function sumTotals(items: Item[]): Totals {
  return items.map(lineTotals).reduce(add, zero);
}

export function add(a: Totals, b: Totals): Totals {
  return {
    kcal: a.kcal + b.kcal,
    protein: a.protein + b.protein,
    fat: a.fat + b.fat,
    carbs: a.carbs + b.carbs,
  };
}

/**
 * Each macro's share of the energy it supplies (4 / 9 / 4 kcal per gram),
 * for drawing the day's composition. All zero when there is nothing yet.
 */
export function energyShares(t: Totals): { protein: number; fat: number; carbs: number } {
  const p = t.protein * 4;
  const f = t.fat * 9;
  const c = t.carbs * 4;
  const sum = p + f + c;
  if (sum <= 0) return { protein: 0, fat: 0, carbs: 0 };
  return { protein: p / sum, fat: f / sum, carbs: c / sum };
}

/** Calories are shown whole; nobody tracks 0.4 kcal. */
export function kcal(value: number): string {
  return Math.round(value).toLocaleString("ru-RU");
}

/** Macros to one decimal below 10 g, whole above — precision follows size. */
export function grams(value: number): string {
  const rounded = value < 10 ? Math.round(value * 10) / 10 : Math.round(value);
  return rounded.toLocaleString("ru-RU");
}

export function blankItem(): Item {
  return { name: "", grams: 100, kcal100: 0, protein100: 0, fat100: 0, carbs100: 0, confidence: "high" };
}
