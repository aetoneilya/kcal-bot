/**
 * The light behind the glass follows the day on screen, and a fine grain
 * lies over it. The colours and the strength are CSS variables (app.css);
 * this sets them from the diary and draws the grain tile.
 */

/** What the diary on screen says about its day. */
export interface DayState {
  meals: number;
  kcal: number;
  /** The day's calorie target; null without a profile. */
  target: number | null;
}

/** The share of the target counted as closing it — the calendar's ±10%. */
const CLOSED = 0.1;

/**
 * Grey and dim with nothing logged, warming from the theme's colour toward
 * green and brightening as the day fills, at its fullest — green, teal and
 * a bloom from below — once the target is closed, amber past it. Brightness
 * is --day-glow, multiplying the light's own strength.
 */
export function setDay({ meals, kcal, target }: DayState): void {
  const root = document.documentElement;
  const set = (mood: string, mood2: string, glow: number, bloom = "transparent") => {
    root.style.setProperty("--mood", mood);
    root.style.setProperty("--mood-2", mood2);
    root.style.setProperty("--bloom", bloom);
    root.style.setProperty("--day-glow", glow.toFixed(3));
  };
  const grey = "color-mix(in oklab, var(--hint) 70%, transparent)";
  if (meals === 0) return set(grey, grey, 0.45);
  if (!target) return set("var(--accent)", "var(--carbs)", 1);
  const p = kcal / target;
  if (p > 1 + CLOSED) return set("var(--over)", "var(--fat)", 1.05);
  if (p >= 1 - CLOSED) return set("var(--ok)", "#2bb3c0", 1.6, "color-mix(in oklab, var(--ok) 30%, transparent)");
  // Mixed in OKLCH, so the way from blue to green stays vivid instead of
  // passing through grey.
  const warm = Math.round((p / (1 - CLOSED)) * 100);
  return set(`color-mix(in oklch, var(--ok) ${warm}%, var(--accent))`, "var(--carbs)", 0.5 + 0.7 * p);
}

/**
 * Grain is a small tile of grey noise around mid-grey, the neutral point of
 * the soft-light blend, drawn once and repeated by CSS. It also hides the
 * banding a soft gradient gets on a phone screen.
 */
export function drawGrain(): void {
  const size = 160;
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = size;
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  const image = ctx.createImageData(size, size);
  for (let i = 0; i < image.data.length; i += 4) {
    // Sum of three uniforms: a cheap bell curve around 128.
    const v = 128 + (Math.random() + Math.random() + Math.random() - 1.5) * 90;
    image.data[i] = image.data[i + 1] = image.data[i + 2] = v;
    image.data[i + 3] = 255;
  }
  ctx.putImageData(image, 0, 0);
  document.documentElement.style.setProperty("--grain-image", `url("${canvas.toDataURL("image/png")}")`);
}
