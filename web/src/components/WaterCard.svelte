<script lang="ts">
  import { api } from "../lib/api";
  import { dayKey } from "../lib/day";
  import { haptic } from "../lib/telegram";
  import Icon from "./Icon.svelte";

  // Water for the day on screen: a glass at a time toward the day's goal.
  // Taps update at once and the newest total is sent; a failed save puts
  // back what the server has.
  let { day }: { day: Date } = $props();

  const GLASS = 250;
  let ml = $state(0);
  let goal = $state(2000);
  let loaded = $state(false);
  let failed = $state(false);

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

  const litres = (v: number) => (v / 1000).toLocaleString("ru-RU", { maximumFractionDigits: 2 });
  const share = $derived(Math.min(ml / goal, 1));
</script>

<section class="card water" aria-label="Вода">
  <div class="text">
    <p class="amount num">
      <b class="round">{loaded ? litres(ml) : "—"}</b>
      <span>из {litres(goal)} л воды</span>
    </p>
    <div class="bar" aria-hidden="true"><span style="width: {share * 100}%"></span></div>
    {#if failed}<p class="error">Не сохранилось — проверь связь</p>{/if}
  </div>
  <button class="minus" onclick={() => change(-GLASS)} disabled={!loaded || ml === 0} aria-label="Убрать стакан">
    <Icon name="minus" size={20} />
  </button>
  <button class="plus" onclick={() => change(GLASS)} disabled={!loaded} aria-label="Добавить стакан воды">
    <Icon name="plus" size={18} />250 мл
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

  .bar {
    height: 6px;
    border-radius: 3px;
    background: var(--field);
    overflow: hidden;
  }

  .bar span {
    display: block;
    height: 100%;
    border-radius: 3px;
    background: var(--water);
    transition: width 0.3s var(--ease-out);
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
    padding: 0 14px 0 10px;
    background: color-mix(in srgb, var(--water) 16%, var(--card));
    color: var(--water);
  }
</style>
