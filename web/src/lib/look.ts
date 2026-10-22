/**
 * The owner's look-and-feel lab: which light sits behind the glass, how
 * strong it is, and how much grain lies over it. Settings live in this
 * browser only, one set per colour scheme, until a look is chosen and made
 * the default in app.css. Everyone else sees the defaults.
 */

export type Background = "glow" | "data" | "time" | "aurora" | "beam";
export type Blend = "soft-light" | "overlay" | "multiply" | "screen";
export type Scheme = "light" | "dark";

export interface Look {
  background: Background;
  /** How strong the light is, 0–1 (the default look is 0.62). */
  glow: number;
  /** How visible the grain is, 0–0.3. */
  grain: number;
  blend: Blend;
}

export const backgrounds: { key: Background; label: string; hint: string }[] = [
  { key: "data", label: "От дня", hint: "Как сейчас: зелёный, пока в норме; янтарный, когда перебор" },
  { key: "glow", label: "Свечение", hint: "Цвет темы: два мягких пятна и луч" },
  { key: "time", label: "Время суток", hint: "Утро тёплое, вечер фиолетовый, ночь синяя" },
  { key: "aurora", label: "Сияние", hint: "Пятна медленно плывут, цикл около минуты" },
  { key: "beam", label: "Луч", hint: "Сверху свет и косой луч, как у Calori" },
];

export const blends: { key: Blend; label: string }[] = [
  { key: "soft-light", label: "Мягкий" },
  { key: "overlay", label: "Перекрытие" },
  { key: "multiply", label: "Умножение" },
  { key: "screen", label: "Осветление" },
];

/** The chosen look (2026-10-22): light tinted by how the day goes, and a
 *  little grain, a touch less in the dark where it shows more. */
export const defaults: Look = { background: "data", glow: 0.62, grain: 0.06, blend: "soft-light" };
const darkGrain = 0.05;

export function scheme(): Scheme {
  const set = document.documentElement.dataset.theme;
  if (set === "light" || set === "dark") return set;
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function defaultsFor(s: Scheme): Look {
  return s === "dark" ? { ...defaults, grain: darkGrain } : { ...defaults };
}

export function load(s: Scheme = scheme()): Look {
  try {
    const raw = localStorage.getItem(`look:${s}`);
    if (raw) return { ...defaultsFor(s), ...JSON.parse(raw) };
  } catch {
    // Private mode or blocked storage: the defaults will do.
  }
  return defaultsFor(s);
}

export function save(look: Look, s: Scheme = scheme()): void {
  try {
    localStorage.setItem(`look:${s}`, JSON.stringify(look));
  } catch {
    // Not kept, still applied for this visit.
  }
}

export function clear(s: Scheme = scheme()): void {
  try {
    localStorage.removeItem(`look:${s}`);
  } catch {
    // Nothing to clear.
  }
}

let clock: ReturnType<typeof setInterval> | undefined;

/** Puts a look on the page: CSS variables and the background's name. */
export function apply(look: Look): void {
  const root = document.documentElement;
  root.dataset.bg = look.background;
  root.style.setProperty("--glow-opacity", String(look.glow));
  root.style.setProperty("--grain-opacity", String(look.grain));
  root.style.setProperty("--grain-blend", look.blend);
  if (look.grain > 0) ensureGrain();

  clearInterval(clock);
  for (const name of ["--mood", "--mood-2", "--bloom", "--day-glow"]) root.style.removeProperty(name);
  if (look.background === "time") {
    root.style.setProperty("--mood", timeTint(new Date()));
    clock = setInterval(() => root.style.setProperty("--mood", timeTint(new Date())), 10 * 60 * 1000);
  } else if (look.background === "data" && lastDay) {
    paintDay(lastDay);
  }
}

/** What the diary on screen says about its day. */
export interface DayState {
  meals: number;
  kcal: number;
  /** The day's calorie target; null without a profile. */
  target: number | null;
}

let lastDay: DayState | null = null;

/** The diary tells how its day goes; the "data" background shows it. */
export function setDay(day: DayState): void {
  lastDay = day;
  if (document.documentElement.dataset.bg === "data") paintDay(day);
}

/** The share of the target counted as closing it — the calendar's ±10%. */
const CLOSED = 0.1;

/**
 * The light follows the day: grey and dim with nothing logged, warming from
 * the theme's colour toward green and brightening as the day fills, at its
 * fullest — green and teal — once the target is closed, amber past it.
 * Brightness is --day-glow, multiplying the look's own strength.
 */
function paintDay({ meals, kcal, target }: DayState): void {
  const root = document.documentElement;
  const set = (mood: string, mood2: string, glow: number, bloom = "transparent") => {
    root.style.setProperty("--mood", mood);
    root.style.setProperty("--mood-2", mood2);
    root.style.setProperty("--bloom", bloom);
    root.style.setProperty("--day-glow", glow.toFixed(3));
  };
  const grey = "color-mix(in srgb, var(--hint) 70%, transparent)";
  if (meals === 0) return set(grey, grey, 0.45);
  if (!target) return set("var(--accent)", "var(--carbs)", 1);
  const p = kcal / target;
  if (p > 1 + CLOSED) return set("var(--over)", "var(--fat)", 1.05);
  if (p >= 1 - CLOSED) return set("var(--ok)", "#2bb3c0", 1.6, "color-mix(in srgb, var(--ok) 30%, transparent)");
  const warm = Math.round((p / (1 - CLOSED)) * 100);
  return set(`color-mix(in srgb, var(--ok) ${warm}%, var(--accent))`, "var(--carbs)", 0.5 + 0.7 * p);
}

function timeTint(now: Date): string {
  const h = now.getHours();
  if (h >= 5 && h < 11) return "#f39a4b"; // morning: sunrise orange
  if (h >= 11 && h < 17) return "var(--accent)"; // day: the theme's own
  if (h >= 17 && h < 22) return "#8b6cf0"; // evening: violet
  return "#3d63dd"; // night: deep blue
}

/**
 * Grain is a small tile of grey noise around mid-grey, the neutral point of
 * the soft-light and overlay blends, drawn once and repeated by CSS.
 */
function ensureGrain(): void {
  const root = document.documentElement;
  if (root.style.getPropertyValue("--grain-image")) return;
  const size = 160;
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  const image = ctx.createImageData(size, size);
  for (let i = 0; i < image.data.length; i += 4) {
    // Sum of three uniforms: a cheap bell curve around 128.
    const v = 128 + (Math.random() + Math.random() + Math.random() - 1.5) * 90;
    image.data[i] = image.data[i + 1] = image.data[i + 2] = v;
    image.data[i + 3] = 255;
  }
  ctx.putImageData(image, 0, 0);
  root.style.setProperty("--grain-image", `url("${canvas.toDataURL("image/png")}")`);
}
