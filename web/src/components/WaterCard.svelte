<script lang="ts">
  import { api } from "../lib/api";
  import { dayKey } from "../lib/day";
  import { haptic, hapticTick } from "../lib/telegram";
  import Icon from "./Icon.svelte";
  import CountUp from "./CountUp.svelte";

  // Water for the day on screen: a glass at a time toward the day's goal, or
  // any amount by sliding a finger along the bar. Changes show at once and
  // the newest total is sent; a failed save puts back what the server has.
  let { day }: { day: Date } = $props();

  const GLASS = 250;
  let ml = $state(0);
  let goal = $state(2000);
  let loaded = $state(false);
  let failed = $state(false);
  // Bumped per glass, so the ripple on the button restarts each time.
  let drops = $state(0);

  $effect(() => {
    const key = dayKey(day);
    let stale = false;
    loaded = false;
    api.water(key).then(
      (w) => {
        if (stale) return;
        ml = w.ml;
        goal = w.goalMl;
        loaded = true;
      },
      () => {
        if (!stale) loaded = true;
      },
    );
    return () => (stale = true);
  });

  let sending: Promise<unknown> = Promise.resolve();
  function change(delta: number) {
    const next = Math.max(0, ml + delta);
    if (next === ml) return;
    haptic();
    if (delta > 0) drops++;
    set(next);
  }

  function set(next: number) {
    ml = next;
    failed = false;
    const key = dayKey(day);
    // One save at a time, in order, so the last tap wins.
    sending = sending.then(() =>
      api.saveWater(key, next).then(
        (w) => (goal = w.goalMl),
        () => (failed = true),
      ),
    );
  }

  // Sliding: the finger's way along the bar, as a share of the goal, adds to
  // what was there when it went down — so a touch alone changes nothing, and
  // past the end of the bar it keeps counting. Snaps to 50 ml, ticks at
  // every glass.
  const STEP = 50;
  let slide = $state<{ from: number; x: number; width: number; id: number } | null>(null);
  let preview = $state<number | null>(null);

  function down(e: PointerEvent) {
    if (!loaded || e.button !== 0) return;
    const track = e.currentTarget as HTMLElement;
    slide = { from: ml, x: e.clientX, width: track.getBoundingClientRect().width, id: e.pointerId };
  }

  function move(e: PointerEvent) {
    if (!slide || e.pointerId !== slide.id) return;
    const dx = e.clientX - slide.x;
    if (preview === null) {
      // A little way first, so a tap or the start of a scroll stays one.
      if (Math.abs(dx) < 6) return;
      (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
      haptic();
    }
    const raw = slide.from + (dx / slide.width) * goal;
    const next = Math.min(Math.max(Math.round(raw / STEP) * STEP, 0), MAX);
    if (preview !== null && Math.floor(next / GLASS) !== Math.floor(preview / GLASS)) hapticTick();
    preview = next;
  }

  function up(e: PointerEvent) {
    if (!slide || e.pointerId !== slide.id) return;
    const next = preview;
    slide = null;
    preview = null;
    if (next === null || next === ml) return;
    if (next > ml) drops++;
    set(next);
  }

  function cancel() {
    slide = null;
    preview = null;
  }

  function key(e: KeyboardEvent) {
    const delta = { ArrowRight: STEP, ArrowUp: STEP, ArrowLeft: -STEP, ArrowDown: -STEP }[e.key];
    if (!delta || !loaded) return;
    e.preventDefault();
    change(delta);
  }

  // The server's cap; more than anyone drinks in a day.
  const MAX = 10000;
  const litres = (v: number) => (v / 1000).toLocaleString("ru-RU", { maximumFractionDigits: 2 });
  const shown = $derived(preview ?? ml);
  const share = $derived(Math.min(shown / goal, 1));
  const added = $derived(preview === null || !slide ? 0 : preview - slide.from);
  const signed = (v: number) => (v > 0 ? "+" : v < 0 ? "−" : "±") + Math.abs(v).toLocaleString("ru-RU");
</script>

<section class="card water" aria-label="Вода">
  <div class="text">
    <p class="amount num">
      <b class="round">{#if preview !== null}{litres(preview)}{:else if loaded}<CountUp value={ml} format={litres} />{:else}—{/if}</b>
      <span>из {litres(goal)} л воды</span>
    </p>
    <!-- The swipe between days must not see a slide. -->
    <div
      class="track"
      class:sliding={preview !== null}
      role="slider"
      tabindex="0"
      aria-label="Вода за день"
      aria-valuemin={0}
      aria-valuemax={goal}
      aria-valuenow={shown}
      aria-valuetext="{litres(shown)} л"
      onpointerdown={down}
      onpointermove={move}
      onpointerup={up}
      onpointercancel={cancel}
      onkeydown={key}
      ontouchstart={(e) => e.stopPropagation()}
      ontouchend={(e) => e.stopPropagation()}
    >
      <div class="bar"><span class="fill" class:empty={shown === 0} style="width: {share * 100}%"></span></div>
      {#if preview !== null}
        <span class="tip num" style="--at: {share * 100}%">
          <b>{signed(added)} мл</b>
          <span>{litres(preview)} л</span>
        </span>
      {/if}
    </div>
    {#if failed}<p class="error">Не сохранилось — проверь связь</p>{/if}
  </div>
  <button class="minus" onclick={() => change(-GLASS)} disabled={!loaded || ml === 0} aria-label="Убрать стакан">
    <Icon name="minus" size={20} />
  </button>
  <button class="plus" onclick={() => change(GLASS)} disabled={!loaded} aria-label="Добавить стакан воды">
    <Icon name="plus" size={18} />250 мл
    {#key drops}
      {#if drops}<span class="ripple" aria-hidden="true"></span>{/if}
    {/key}
  </button>
</section>

<style>
  .water {
    display: flex;
    align-items: center;
    gap: 8px;
    /* The summary above keeps its own 24px; match it below. */
    margin: -12px 0 24px;
    padding: 12px 12px 12px 16px;
  }

  .text {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 8px;
  }

  .amount {
    margin: 0;
    display: flex;
    align-items: baseline;
    gap: 6px;
  }

  .amount b {
    font-size: 20px;
    font-weight: 700;
  }

  .amount span {
    color: var(--hint);
    font-size: 14px;
  }

  /* A taller strip than the bar, for the finger; vertical scrolling still
     starts here, a sideways move slides. */
  .track {
    position: relative;
    margin: -9px 0;
    padding: 9px 0;
    touch-action: pan-y;
    cursor: ew-resize;
    -webkit-user-select: none;
    user-select: none;
  }

  .track:focus-visible {
    outline: none;
  }

  .track:focus-visible .bar {
    box-shadow: 0 0 0 2px var(--accent);
  }

  .bar {
    height: 6px;
    border-radius: 6px;
    background: var(--field);
    overflow: hidden;
    transition: height 0.2s var(--ease-out);
  }

  .sliding .bar {
    height: 12px;
    margin: -3px 0;
  }

  /* The water itself: lighter and deeper blues drifting along it, and now
     and then a gleam running across, like light through a glass. */
  .fill {
    position: relative;
    display: block;
    height: 100%;
    min-width: 6px;
    border-radius: inherit;
    background: linear-gradient(
        90deg,
        color-mix(in oklab, var(--water), #0b3d91 22%),
        var(--water) 30%,
        color-mix(in oklab, var(--water), #fff 35%) 50%,
        var(--water) 70%,
        color-mix(in oklab, var(--water), #0b3d91 22%)
      )
      0 0 / 240px 100% repeat-x;
    animation: drift 5s linear infinite;
    overflow: hidden;
    transition: width 0.3s var(--ease-out);
  }

  .fill.empty {
    min-width: 0;
  }

  .sliding .fill {
    transition: none;
  }

  .fill::after {
    content: "";
    position: absolute;
    inset: 0;
    background: linear-gradient(105deg, transparent 35%, rgb(255 255 255 / 0.55) 50%, transparent 65%) no-repeat;
    background-size: 60px 100%;
    background-position: -60px 0;
    animation: gleam 3.2s var(--ease-out) infinite;
  }

  @keyframes drift {
    to {
      background-position: 240px 0;
    }
  }

  @keyframes gleam {
    0%,
    45% {
      background-position: -60px 0;
    }
    100% {
      background-position: calc(100% + 60px) 0;
    }
  }

  .tip {
    position: absolute;
    bottom: calc(100% - 2px);
    left: clamp(44px, var(--at), calc(100% - 44px));
    transform: translateX(-50%);
    display: grid;
    justify-items: center;
    padding: 5px 10px;
    border-radius: 12px;
    background: var(--water);
    color: #fff;
    white-space: nowrap;
    box-shadow: 0 4px 14px color-mix(in srgb, var(--water) 40%, transparent);
    pointer-events: none;
    animation: rise 0.15s var(--ease-out);
  }

  .tip b {
    font-size: 15px;
    font-weight: 700;
  }

  .tip span {
    font-size: 12px;
    opacity: 0.85;
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translate(-50%, 4px);
    }
  }

  .error {
    margin: 0;
    font-size: 13px;
  }

  button {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    height: 44px;
    border-radius: 22px;
    font-weight: 600;
    font-size: 15px;
    transition: transform 0.12s var(--ease-out);
  }

  button:active:not(:disabled) {
    transform: scale(0.95);
  }

  .minus {
    width: 44px;
    background: var(--field);
    color: var(--hint);
  }

  .plus {
    position: relative;
    overflow: hidden;
    padding: 0 14px 0 10px;
    background: color-mix(in srgb, var(--water) 16%, var(--card));
    color: var(--water);
  }
  /* A drop landing: a ring spreading from the button's middle. */
  .ripple {
    position: absolute;
    inset: 0;
    margin: auto;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: var(--water);
    opacity: 0;
    pointer-events: none;
    animation: ripple 0.55s var(--ease-out);
  }

  @keyframes ripple {
    from {
      opacity: 0.35;
      transform: scale(0.3);
    }
    to {
      opacity: 0;
      transform: scale(5);
    }
  }
</style>
