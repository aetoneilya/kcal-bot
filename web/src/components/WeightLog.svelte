<script lang="ts">
  import { untrack } from "svelte";
  import { api, type Weight } from "../lib/api";
  import { addDays, dayKey, fromDayKey, startOfDay } from "../lib/day";
  import type { Profile } from "../lib/targets";
  import { haptic, hapticSuccess } from "../lib/telegram";
  import WeightChart from "./WeightChart.svelte";
  import Icon from "./Icon.svelte";

  // The weigh-ins, at the bottom of the target screen. onChanged gets the
  // newest weight after a save or delete; the server has already made it the
  // profile's.
  let { profile, onChanged }: { profile: Profile | null; onChanged: (latestKg: number | null) => void } = $props();

  const ranges = [
    { days: 30, label: "Месяц" },
    { days: 90, label: "3 месяца" },
    { days: 365, label: "Год" },
  ];
  let span = $state(90);
  let weights = $state<Weight[]>([]);
  let loading = $state(true);
  let error = $state("");
  let attempt = $state(0);
  let busy = $state(false);

  const today = startOfDay(new Date());
  const todayKey = dayKey(today);
  // Prefill with the latest known weight: most days it moves by tenths.
  let value = $state<number | undefined>(untrack(() => profile?.weightKg));

  $effect(() => {
    const days = span;
    void attempt;
    let stale = false;
    loading = true;
    error = "";
    api
      .weights(dayKey(addDays(today, -days)), todayKey)
      .then((list) => {
        if (stale) return;
        weights = list;
        const last = list[list.length - 1];
        if (last) value = last.weightKg;
      })
      .catch((e) => {
        if (!stale) error = e.message;
      })
      .finally(() => {
        if (!stale) loading = false;
      });
    return () => (stale = true);
  });

  const latest = $derived(weights[weights.length - 1] ?? null);
  const monthAgo = $derived.by(() => {
    const cut = dayKey(addDays(today, -30));
    return [...weights].reverse().find((w) => w.day <= cut) ?? weights[0] ?? null;
  });
  const change = $derived(latest && monthAgo && monthAgo !== latest ? latest.weightKg - monthAgo.weightKg : null);
  const toGoal = $derived(latest && profile ? latest.weightKg - profile.goalWeightKg : null);
  const todaySaved = $derived(latest?.day === todayKey);

  const kg = (v: number) => v.toLocaleString("ru-RU", { maximumFractionDigits: 1 });
  const signed = (v: number) => (v > 0 ? "+" : v < 0 ? "−" : "±") + kg(Math.abs(v));

  async function save() {
    if (typeof value !== "number" || !Number.isFinite(value)) return;
    busy = true;
    error = "";
    try {
      const saved = await api.saveWeight(todayKey, value);
      weights = [...weights.filter((w) => w.day !== saved.day), saved].sort((a, b) => a.day.localeCompare(b.day));
      hapticSuccess();
      onChanged(weights[weights.length - 1]?.weightKg ?? null);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  async function remove(day: string) {
    haptic();
    try {
      await api.deleteWeight(day);
      weights = weights.filter((w) => w.day !== day);
      onChanged(weights[weights.length - 1]?.weightKg ?? null);
    } catch (e) {
      error = (e as Error).message;
    }
  }
</script>

<section class="weight" aria-labelledby="weight-title">
  <h2 id="weight-title" class="round">Вес</h2>

  <section class="card head">
    <div class="top">
      <div>
        <p class="value round num">{latest ? kg(latest.weightKg) : "—"}<span>&nbsp;кг</span></p>
        <p class="hint num">
          {#if change !== null}{signed(change)} кг за месяц{:else if latest}первая запись{:else}записей пока нет{/if}
        </p>
      </div>
      {#if toGoal !== null && profile}
        <div class="goal num">
          <b class="round">{Math.abs(toGoal) < 0.5 ? "в цели" : kg(Math.abs(toGoal)) + " кг"}</b>
          <span>{Math.abs(toGoal) < 0.5 ? `цель ${kg(profile.goalWeightKg)} кг` : `до цели ${kg(profile.goalWeightKg)}`}</span>
        </div>
      {/if}
    </div>

    {#if weights.length > 1}
      <div class="ranges" role="radiogroup" aria-label="Период">
        {#each ranges as r (r.days)}
          <button role="radio" aria-checked={span === r.days} class:on={span === r.days} onclick={() => (span = r.days)}>{r.label}</button>
        {/each}
      </div>
      <WeightChart {weights} goal={profile?.goalWeightKg ?? null} />
    {:else if !loading}
      <p class="hint empty">Взвешивайся утром натощак раз в несколько дней — здесь появится график, а норма калорий будет пересчитываться по свежему весу.</p>
    {/if}
  </section>

  <form
    class="card entry"
    onsubmit={(e) => {
      e.preventDefault();
      save();
    }}
  >
    <label for="w" class="label">{todaySaved ? "Сегодня, исправить" : "Вес сегодня"}</label>
    <span class="field">
      <input id="w" class="num" type="number" inputmode="decimal" step="0.1" min="30" max="300" placeholder="80,0" bind:value />
      <span>кг</span>
    </span>
    <button class="primary save" type="submit" disabled={busy || typeof value !== "number"}>{busy ? "…" : "Записать"}</button>
  </form>
  {#if error}<p class="error">{error}</p>{/if}
  {#if profile}
    <p class="foot">Последний вес становится текущим в профиле — норма калорий пересчитывается сама.</p>
  {/if}

  {#if weights.length}
    <h2 class="section-title"><span>Записи</span><span class="num">{weights.length}</span></h2>
    <ul class="group">
      {#each [...weights].reverse().slice(0, 30) as w, i (w.day)}
        {@const prev = [...weights].reverse()[i + 1]}
        <li class="row">
          <span class="day">{fromDayKey(w.day).toLocaleDateString("ru-RU", { day: "numeric", month: "long", weekday: "short" })}</span>
          <span class="num delta">{prev ? signed(w.weightKg - prev.weightKg) : ""}</span>
          <b class="num">{kg(w.weightKg)}</b>
          <button class="del" onclick={() => remove(w.day)} aria-label="Удалить запись"><Icon name="x" size={16} /></button>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .weight {
    display: flex;
    flex-direction: column;
    padding-bottom: 16px;
  }

  h2 {
    font-size: 20px;
    font-weight: 700;
    letter-spacing: -0.01em;
    margin: 28px 0 12px;
  }

  .head {
    display: grid;
    gap: 14px;
    margin-bottom: 12px;
  }

  .head p {
    margin: 0;
  }

  .top {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
  }

  .value {
    font-size: 36px;
    font-weight: 700;
    letter-spacing: -0.02em;
    line-height: 1.1;
  }

  .value span {
    font-size: 16px;
    font-weight: 400;
    color: var(--hint);
  }

  .goal {
    display: grid;
    justify-items: end;
    font-size: 13px;
    color: var(--hint);
  }

  .goal b {
    font-size: 20px;
    font-weight: 600;
    color: var(--text);
  }

  .ranges {
    display: flex;
    padding: 2px;
    border-radius: 10px;
    background: var(--field);
  }

  .ranges button {
    flex: 1;
    min-height: 32px;
    border-radius: 8px;
    font-size: 14px;
    color: var(--hint);
  }

  .ranges .on {
    background: var(--card);
    color: var(--text);
    font-weight: 500;
    box-shadow: 0 1px 3px rgb(0 0 0 / 0.12);
  }

  .empty {
    font-size: 14px;
  }

  .entry {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 10px 10px 16px;
  }

  .label {
    flex: 1;
  }

  .field {
    display: flex;
    align-items: baseline;
    gap: 4px;
    color: var(--hint);
  }

  .field input {
    width: 76px;
    text-align: right;
    font-weight: 600;
    font-size: 17px;
    color: var(--text);
  }

  .save {
    width: auto;
    min-height: 42px;
    font-size: 15px;
  }

  .foot {
    margin: 6px 16px 22px;
    font-size: 13px;
    color: var(--hint);
  }

  .error {
    margin: 8px 4px 0;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 48px;
    padding: 4px 6px 4px 16px;
  }

  .day {
    flex: 1;
  }

  .delta {
    font-size: 13px;
    color: var(--hint);
  }

  .del {
    width: 36px;
    height: 36px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    color: var(--hint);
  }
</style>
