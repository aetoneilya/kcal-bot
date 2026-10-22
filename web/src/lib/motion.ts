/**
 * Motion here is feedback and continuity, never a show on load: a saved
 * meal counting into the day, a day sliding in from the side it was swiped
 * to. Everything collapses to an instant change under Reduce Motion — the
 * CSS rule in app.css covers keyframes and Svelte transitions, `ms` covers
 * what JavaScript animates.
 */
import { prefersReducedMotion } from "svelte/motion";
import { cubicOut } from "svelte/easing";
import type { TransitionConfig } from "svelte/transition";

export function ms(duration: number): number {
  return prefersReducedMotion.current ? 0 : duration;
}

/** The app's --ease-out, cubic-bezier(0.16, 1, 0.3, 1), as a function. */
export function easeOut(t: number): number {
  return 1 - Math.pow(1 - t, 4);
}

/**
 * What a screen showed last time, so coming back after a change animates
 * from the old figure instead of from zero or not at all.
 */
const shown = new Map<string, unknown>();
export function lastShown<T>(key: string): T | undefined {
  return shown.get(key) as T | undefined;
}
export function remember(key: string, value: unknown): void {
  shown.set(key, value);
}

/** A row that was not there before: rises in. Others appear as they are. */
export function arrive(_node: Element, { fresh }: { fresh: boolean }): TransitionConfig {
  if (!fresh) return { duration: 0 };
  return {
    duration: ms(360),
    easing: cubicOut,
    css: (t, u) => `opacity: ${t}; transform: translateY(${u * 10}px) scale(${0.98 + t * 0.02})`,
  };
}

/** The next day or month slides in from the side it was paged to. */
export function page(_node: Element, { dir }: { dir: number }): TransitionConfig {
  if (!dir) return { duration: 0 };
  return {
    duration: ms(240),
    easing: easeOut,
    css: (t, u) => `opacity: ${0.4 + t * 0.6}; transform: translateX(${u * dir * 28}px)`,
  };
}
