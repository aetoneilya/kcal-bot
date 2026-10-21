<script lang="ts">
  // Weight over time: one 2px line, a dashed goal line, a recessive grid with
  // three or four ticks, and a crosshair that snaps to the nearest weigh-in.
  import type { Weight } from "../lib/api";
  import { fromDayKey } from "../lib/day";

  let { weights, goal = null }: { weights: Weight[]; goal?: number | null } = $props();

  const W = 340;
  const H = 200;
  // The top band is the tooltip's room, so it never covers the controls above.
  const pad = { top: 40, right: 12, bottom: 22, left: 34 };
  const plotW = W - pad.left - pad.right;
  const plotH = H - pad.top - pad.bottom;

  const points = $derived(weights.map((w) => ({ t: fromDayKey(w.day).getTime(), v: w.weightKg, day: w.day })));
  const xMin = $derived(points.length ? points[0].t : 0);
  const xMax = $derived(points.length ? points[points.length - 1].t : 1);

  // Round the value range outward to whole kilos, include the goal when it is
  // near enough to be worth seeing, and keep at least 2 kg of span.
  const range = $derived.by(() => {
    const values = points.map((p) => p.v);
    let lo = Math.min(...values);
    let hi = Math.max(...values);
    if (goal != null && goal > lo - 6 && goal < hi + 6) {
      lo = Math.min(lo, goal);
      hi = Math.max(hi, goal);
    }
    lo = Math.floor(lo - 0.5);
    hi = Math.ceil(hi + 0.5);
    if (hi - lo < 2) hi = lo + 2;
    return { lo, hi };
  });
  const ticks = $derived.by(() => {
    const span = range.hi - range.lo;
    const step = span <= 4 ? 1 : span <= 8 ? 2 : span <= 20 ? 5 : 10;
    const out: number[] = [];
    for (let v = Math.ceil(range.lo / step) * step; v <= range.hi; v += step) out.push(v);
    return out;
  });

  const x = (t: number) => pad.left + (xMax === xMin ? plotW / 2 : ((t - xMin) / (xMax - xMin)) * plotW);
  const y = (v: number) => pad.top + (1 - (v - range.lo) / (range.hi - range.lo)) * plotH;

  const last = $derived(points[points.length - 1]);
  const path = $derived(points.map((p, i) => `${i ? "L" : "M"}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join(""));
  const showGoal = $derived(goal != null && goal >= range.lo && goal <= range.hi);
  const xLabels = $derived.by(() => {
    if (points.length < 2) return points.map((p) => p.t);
    const n = Math.min(4, points.length);
    return Array.from({ length: n }, (_, i) => xMin + ((xMax - xMin) * i) / (n - 1));
  });

  const fmtDay = (t: number) => new Date(t).toLocaleDateString("ru-RU", { day: "numeric", month: "short" }).replace(".", "");
  const fmtKg = (v: number) => v.toLocaleString("ru-RU", { maximumFractionDigits: 1 });

  let hover = $state<number | null>(null);
  let svg = $state<SVGSVGElement>();

  function track(e: PointerEvent) {
    if (!svg) return;
    const box = svg.getBoundingClientRect();
    const px = ((e.clientX - box.left) / box.width) * W;
    let best = 0;
    for (let i = 1; i < points.length; i++) {
      if (Math.abs(x(points[i].t) - px) < Math.abs(x(points[best].t) - px)) best = i;
    }
    hover = points.length ? best : null;
  }
</script>

{#if points.length}
  <figure>
    <svg
      bind:this={svg}
      viewBox="0 0 {W} {H}"
      role="img"
      aria-label="График веса"
      onpointermove={track}
      onpointerdown={track}
      onpointerleave={() => (hover = null)}
    >
      {#each ticks as v (v)}
        <line class="grid" x1={pad.left} x2={W - pad.right} y1={y(v)} y2={y(v)} />
        <text class="tick" x={pad.left - 6} y={y(v) + 4} text-anchor="end">{v}</text>
      {/each}
      {#each xLabels as t, i (i)}
        <text class="tick" x={x(t)} y={H - 6} text-anchor={i === 0 ? "start" : i === xLabels.length - 1 ? "end" : "middle"}>{fmtDay(t)}</text>
      {/each}

      {#if showGoal}
        <line class="goal" x1={pad.left} x2={W - pad.right} y1={y(goal!)} y2={y(goal!)} />
        <text class="goal-label" x={W - pad.right} y={y(goal!) - 5} text-anchor="end">цель {fmtKg(goal!)}</text>
      {/if}

      <path class="line" d={path} />
      <circle class="dot" cx={x(last.t)} cy={y(last.v)} r="4" />

      {#if hover !== null}
        {@const p = points[hover]}
        <line class="cross" x1={x(p.t)} x2={x(p.t)} y1={pad.top} y2={pad.top + plotH} />
        <circle class="dot" cx={x(p.t)} cy={y(p.v)} r="4.5" />
      {/if}
    </svg>
    {#if hover !== null}
      {@const p = points[hover]}
      <div class="tip" style="left: clamp(40px, {(x(p.t) / W) * 100}%, calc(100% - 40px))">
        <b class="num">{fmtKg(p.v)} кг</b>
        <span>{fmtDay(p.t)}</span>
      </div>
    {/if}
  </figure>
{/if}

<style>
  figure {
    position: relative;
    margin: 0;
  }

  svg {
    display: block;
    width: 100%;
    height: auto;
    touch-action: pan-y;
    overflow: visible;
  }

  .grid {
    stroke: var(--line);
    stroke-width: 1;
  }

  .tick {
    fill: var(--hint);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }

  .goal {
    stroke: var(--hint);
    stroke-width: 1.5;
    stroke-dasharray: 4 4;
  }

  .goal-label {
    fill: var(--hint);
    font-size: 11px;
  }

  .line {
    fill: none;
    stroke: var(--accent);
    stroke-width: 2;
    stroke-linejoin: round;
    stroke-linecap: round;
  }

  .dot {
    fill: var(--accent);
    stroke: var(--card);
    stroke-width: 2;
  }

  .cross {
    stroke: var(--hint);
    stroke-width: 1;
  }

  .tip {
    position: absolute;
    top: 0;
    transform: translateX(-50%);
    display: grid;
    justify-items: center;
    padding: 4px 8px;
    border-radius: 8px;
    background: var(--text);
    color: var(--card);
    font-size: 12px;
    white-space: nowrap;
    pointer-events: none;
    box-shadow: 0 2px 8px rgb(0 0 0 / 0.15);
  }

  .tip b {
    font-size: 14px;
  }

  .tip span {
    opacity: 0.75;
  }
</style>
