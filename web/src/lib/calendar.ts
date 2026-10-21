/**
 * Month view arithmetic: the grid of days and how each day went against the
 * daily target. Days are the phone's local days, like the diary itself.
 */
import { addDays, sameDay, startOfDay } from "./day";

/** Within this share of the target a day counts as on target. */
export const TOLERANCE = 0.1;

export type DayStatus = "empty" | "under" | "ok" | "over" | "open";

export interface DayStat {
  date: Date;
  kcal: number;
  /** kcal minus the target; null without a target or a finished day. */
  delta: number | null;
  status: DayStatus;
}

export function startOfMonth(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), 1);
}

export function addMonths(d: Date, months: number): Date {
  return new Date(d.getFullYear(), d.getMonth() + months, 1);
}

/**
 * The weeks shown for a month, Monday first, as dates or null for the
 * padding cells before the 1st and after the last day.
 */
export function monthGrid(month: Date): (Date | null)[][] {
  const first = startOfMonth(month);
  const lead = (first.getDay() + 6) % 7; // Monday = 0
  const days = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate();
  const cells: (Date | null)[] = Array(lead).fill(null);
  for (let i = 0; i < days; i++) cells.push(addDays(first, i));
  while (cells.length % 7) cells.push(null);
  const weeks: (Date | null)[][] = [];
  for (let i = 0; i < cells.length; i += 7) weeks.push(cells.slice(i, i + 7));
  return weeks;
}

/**
 * How one day went. A day with nothing logged is empty, not "under": the
 * person may simply not have used the app. Today is still open, so it is
 * never judged; only finished days are.
 */
export function dayStat(date: Date, kcal: number, target: number | null, now = new Date()): DayStat {
  const today = startOfDay(now);
  if (kcal <= 0) return { date, kcal: 0, delta: null, status: "empty" };
  if (!target || sameDay(date, today) || date > today) return { date, kcal, delta: null, status: "open" };
  const delta = Math.round(kcal - target);
  const status = Math.abs(delta) <= target * TOLERANCE ? "ok" : delta < 0 ? "under" : "over";
  return { date, kcal, delta, status };
}

export interface MonthSummary {
  logged: number;
  under: number;
  ok: number;
  over: number;
  /** Mean kcal over logged days, today included. */
  average: number;
  /** Sum of deltas over judged days: negative means a net shortfall. */
  balance: number;
}

export function summarize(stats: DayStat[]): MonthSummary {
  const logged = stats.filter((s) => s.status !== "empty");
  const judged = stats.filter((s) => s.delta !== null);
  return {
    logged: logged.length,
    under: stats.filter((s) => s.status === "under").length,
    ok: stats.filter((s) => s.status === "ok").length,
    over: stats.filter((s) => s.status === "over").length,
    average: logged.length ? Math.round(logged.reduce((sum, s) => sum + s.kcal, 0) / logged.length) : 0,
    balance: judged.reduce((sum, s) => sum + (s.delta ?? 0), 0),
  };
}
