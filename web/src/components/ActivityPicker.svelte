<script lang="ts">
  import { untrack } from "svelte";
  import { Spring } from "svelte/motion";
  import { activities, type Activity } from "../lib/targets";
  import { ms } from "../lib/motion";
  import { haptic } from "../lib/telegram";

  // Activity as a slider: drag, and the name and hint above change with
  // each step while the arm's biceps fills out. Undefined until the person
  // touches it, since the target needs a real answer.
  let { value = $bindable() }: { value: Activity | undefined } = $props();

  const last = activities.length - 1;
  const index = $derived(value ? activities.findIndex((a) => a.key === value) : -1);
  // Where the thumb sits before anything is chosen: the middle.
  const position = $derived(index < 0 ? Math.floor(last / 2) : index);
  const level = $derived(index < 0 ? null : activities[index]);

  function set(i: number) {
    const next = activities[i].key;
    if (next !== value) haptic("soft");
    value = next;
  }

  // The bulge springs to each level and overshoots a little, like a flex.
  const bulge = new Spring(untrack(() => bulgeFor(position)), { stiffness: 0.12, damping: 0.32 });
  $effect(() => {
    const target = bulgeFor(position);
    if (ms(1) === 0) bulge.set(target, { instant: true });
    else bulge.target = target;
  });

  function bulgeFor(i: number): number {
    return 2 + (i / last) * 13;
  }

  // The arm on a 64-unit grid: upper arm from the shoulder at the left to
  // the elbow, forearm up to the fist. b lifts the biceps' crest.
  const top = $derived(`M6 44 Q 24 ${40 - 2 * bulge.current} 40 40`);
  const outline = $derived(
    `${top} L 40 24 L 53 22 L 54 48 Q 54 56 44 57 Q 26 60 6 54`,
  );
  const strength = $derived(Math.max(0, (bulge.current - 8) / 7));
</script>

<div class="picker glass">
  <div class="head">
    <svg class="arm" viewBox="0 0 64 64" width="72" height="72" aria-hidden="true">
      <path class="fill" d="{outline} Z" style="opacity: {0.12 + 0.2 * (bulge.current / 15)}" />
      <path class="line" d={outline} />
      <path class="line fold" d="M40 40 Q 44 44 44 50" />
      <rect class="fist" x="36" y="7" width="20" height="17" rx="7" />
      <path class="line" d="M41 13.5h10M41 18h10" opacity="0.5" />
      <g class="line power" style="opacity: {strength}">
        <path d="M14 {29 - bulge.current} l-3 -4" />
        <path d="M23 {26 - bulge.current} v-5" />
        <path d="M32 {29 - bulge.current} l3 -4" />
      </g>
    </svg>
    <div class="text" aria-live="polite">
      {#if level}
        <b class="round">{level.label}</b>
        <span>{level.hint}</span>
      {:else}
        <b class="round">Сколько ты двигаешься?</b>
        <span>Потяни ползунок или коснись его</span>
      {/if}
    </div>
  </div>

  <div class="track" style="--p: {(position / last) * 100}%" class:unset={!level}>
    <span class="fillbar"></span>
    {#each activities as a, i (a.key)}
      <span class="tick" class:passed={level && i <= position} style="left: {(i / last) * 100}%"></span>
    {/each}
    <input
      type="range"
      min="0"
      max={last}
      step="1"
      value={position}
      aria-label="Активность"
      aria-valuetext={level ? `${level.label}: ${level.hint}` : "не выбрано"}
      oninput={(e) => set(Number(e.currentTarget.value))}
      onpointerup={(e) => set(Number(e.currentTarget.value))}
      onkeyup={(e) => set(Number(e.currentTarget.value))}
    />
  </div>
  <div class="ends" aria-hidden="true"><span>меньше</span><span>больше</span></div>
</div>

<style>
  .picker {
    display: grid;
    gap: 14px;
    padding: 14px 16px 12px;
    border-radius: var(--r);
  }

  .head {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 72px;
  }

  .arm {
    flex: none;
    color: var(--accent);
    overflow: visible;
  }

  .fill {
    fill: currentColor;
  }

  .line,
  .line path {
    fill: none;
    stroke: currentColor;
    stroke-width: 2.6;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .fold {
    stroke-width: 2;
    opacity: 0.5;
  }

  .fist {
    fill: var(--card);
    stroke: currentColor;
    stroke-width: 2.6;
  }

  .text {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .text b {
    font-size: 18px;
    font-weight: 700;
    line-height: 1.25;
  }

  .text span {
    color: var(--hint);
    font-size: 14px;
    line-height: 1.3;
  }

  /* The track draws itself; the range input on top takes the touches. */
  .track {
    position: relative;
    height: 32px;
    margin: 0 4px;
  }

  .track::before,
  .fillbar {
    content: "";
    position: absolute;
    left: 0;
    top: 50%;
    height: 6px;
    margin-top: -3px;
    border-radius: 3px;
  }

  .track::before {
    right: 0;
    background: var(--field);
  }

  .fillbar {
    width: var(--p);
    background: var(--accent);
    transition: width 0.18s var(--ease-out);
  }

  .unset .fillbar {
    opacity: 0;
  }

  .tick {
    position: absolute;
    top: 50%;
    width: 6px;
    height: 6px;
    margin: -3px 0 0 -3px;
    border-radius: 50%;
    background: var(--line-strong);
    pointer-events: none;
  }

  .tick.passed {
    background: var(--accent-text);
    opacity: 0.8;
  }

  input {
    position: absolute;
    inset: 0;
    width: calc(100% + 28px);
    margin: 0 -14px;
    height: 32px;
    padding: 0;
    background: transparent;
    border: 0;
    -webkit-appearance: none;
    appearance: none;
    cursor: pointer;
  }

  input:focus {
    border: 0;
  }

  input::-webkit-slider-runnable-track {
    height: 32px;
    background: transparent;
  }

  input::-moz-range-track {
    height: 32px;
    background: transparent;
  }

  input::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 28px;
    height: 28px;
    margin-top: 2px;
    border-radius: 50%;
    background: #fff;
    box-shadow:
      0 2px 6px rgb(0 0 0 / 0.22),
      0 0 0 0.5px rgb(0 0 0 / 0.08);
    transition: transform 0.15s var(--ease-out);
  }

  input:active::-webkit-slider-thumb {
    transform: scale(1.12);
  }

  input::-moz-range-thumb {
    width: 28px;
    height: 28px;
    border: 0;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 2px 6px rgb(0 0 0 / 0.22);
  }

  .unset input::-webkit-slider-thumb {
    box-shadow:
      0 0 0 3px color-mix(in srgb, var(--accent) 35%, transparent),
      0 2px 6px rgb(0 0 0 / 0.22);
  }

  .ends {
    display: flex;
    justify-content: space-between;
    margin-top: -8px;
    color: var(--hint);
    font-size: 12px;
  }
</style>
