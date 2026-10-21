<script lang="ts">
  import { api } from "../lib/api";
  import { sameDay, startOfDay } from "../lib/day";
  import { add, kcal as fmtKcal, sumTotals, type Totals } from "../lib/nutrition";
  import { addMonths, dayStat, monthGrid, startOfMonth, summarize, TOLERANCE, type DayStat } from "../lib/calendar";
  import { KCAL_PER_KG, targets, type Profile, type Targets } from "../lib/targets";
  import type { Weight } from "../lib/api";
  import { addDays, dayKey } from "../lib/day";
  import { haptic } from "../lib/telegram";
  import { untrack } from "svelte";
  import Icon from "../components/Icon.svelte";

  let {
    initial,
    target,
    profile,
    onPick,
    onGoal,
  }: {
    initial: Date;
    target: Targets | null;
    profile: Profile | null;
    onPick: (day: Date) => void;
    onGoal: () => void;
  } = $props();

  let weights = $state<Weight[]>([]);

  // A past day is judged against the target its weight called for then: the
  // newest weigh-in on or before it, or the profile's weight without one.
  function targetOn(date: Date): number | null {
    if (!profile || !target) return null;
    const key = dayKey(date);
    // Before the first weigh-in the earliest one is the best guess.
    let kg = weights[0]?.weightKg ?? profile.weightKg;
    for (const w of weights) {
      if (w.day > key) break;
      kg = w.weightKg;
    }
    return targets({ ...profile, weightKg: kg })?.kcal ?? target.kcal;
  }

  // Opens on the month of the day being viewed, then navigates on its own.
  let month = $state(startOfMonth(untrack(() => initial)));
  let totals = $state<Map<number, number>>(new Map());
  let loading = $state(true);
  let error = $state("");
  let attempt = $state(0);

  const today = startOfDay(new Date());
  const thisMonth = startOfMonth(today);
  const weeks = $derived(monthGrid(month));
  const goal = $derived(target?.kcal ?? null);
  const stats = $derived(
    weeks.flat().map((date) => (date ? dayStat(date, totals.get(date.getTime()) ?? 0, goal === null ? null : targetOn(date)) : null)),
  );
  const summary = $derived(summarize(stats.filter((s): s is DayStat => s !== null)));
  const title = $derived(month.toLocaleDateString("ru-RU", { month: "long", year: "numeric" }).replace(" г.", ""));

  $effect(() => {
    const from = month;
    void attempt;
    let stale = false;
    loading = true;
    error = "";
    api
      .meals(from, addMonths(from, 1))
      .then((meals) => {
        if (stale) return;
        // Meals come as instants; file each under the phone's local day.
        const byDay = new Map<number, Totals>();
        for (const meal of meals) {
          const key = startOfDay(new Date(meal.eatenAt)).getTime();
          byDay.set(key, add(byDay.get(key) ?? { kcal: 0, protein: 0, fat: 0, carbs: 0 }, sumTotals(meal.items)));
        }
        totals = new Map([...byDay].map(([k, t]) => [k, t.kcal]));
      })
      .then(() => (profile ? api.weights(dayKey(addDays(from, -120)), dayKey(addMonths(from, 1))) : []))
      .then((list) => {
        if (!stale) weights = list;
      })
      .catch((e) => {
        if (!stale) error = e.message;
      })
      .finally(() => {
        if (!stale) loading = false;
      });
    return () => (stale = true);
  });

  function shift(months: number) {
    haptic();
    month = addMonths(month, months);
  }

  function short(delta: number): string {
    const abs = Math.abs(delta);
    const text = abs >= 1000 ? `${(abs / 1000).toLocaleString("ru-RU", { maximumFractionDigits: 1 })}к` : String(abs);
    return (delta > 0 ? "+" : "−") + text;
  }

  const weekdays = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];
  const kg = $derived(Math.abs(summary.balance) / KCAL_PER_KG);
</script>

<div class="screen">
  <header>
    <button class="nav" onclick={() => shift(-1)} aria-label="Предыдущий месяц"><Icon name="chevron-left" size={24} /></button>
    <h1 class="round">{title}</h1>
    <button class="nav" onclick={() => shift(1)} disabled={month >= thisMonth} aria-label="Следующий месяц">
      <Icon name="chevron-right" size={24} />
    </button>
  </header>

  <section class="card summary" aria-label="Итог месяца">
    {#if target}
      <dl class="counts num">
        <div><dt><i class="dot under"></i>Недоел</dt><dd class="round">{summary.under}</dd></div>
        <div><dt><i class="dot ok"></i>В норме</dt><dd class="round">{summary.ok}</dd></div>
        <div><dt><i class="dot over"></i>Переел</dt><dd class="round">{summary.over}</dd></div>
      </dl>
      <ul class="facts num">
        <li><span>Дней с записями</span><span>{summary.logged}</span></li>
        <li><span>В среднем за день</span><span>{summary.logged ? `${fmtKcal(summary.average)} из ${fmtKcal(target.kcal)} ккал` : "—"}</span></li>
        <li>
          <span>Итог к норме</span>
          <span class:under={summary.balance < 0} class:over={summary.balance > 0}>
            {#if summary.balance === 0}
              —
            {:else}
              {summary.balance > 0 ? "+" : "−"}{fmtKcal(Math.abs(summary.balance))} ккал ≈ {summary.balance > 0 ? "+" : "−"}{kg.toLocaleString("ru-RU", { maximumFractionDigits: 1 })} кг
            {/if}
          </span>
        </li>
      </ul>
    {:else}
      <p class="hint">Без нормы календарь показывает только, сколько съедено.</p>
      <button class="invite" onclick={onGoal}>Посчитать мою норму <Icon name="chevron-right" size={18} /></button>
    {/if}
  </section>

  <section class="card month" aria-busy={loading}>
    <div class="row weekdays">
      {#each weekdays as d (d)}<span>{d}</span>{/each}
    </div>
    {#if error}
      <div class="state">
        <p>Не получилось загрузить месяц.</p>
        <button class="ghost" onclick={() => attempt++}>Повторить</button>
      </div>
    {:else}
      {#each weeks as week, w (w)}
        <div class="row">
          {#each week as date, d (d)}
            {@const s = stats[w * 7 + d]}
            {#if date && s}
              <button
                class="day {s.status}"
                class:today={sameDay(date, today)}
                class:future={date > today}
                disabled={date > today}
                onclick={() => onPick(date)}
                aria-label={`${date.toLocaleDateString("ru-RU", { day: "numeric", month: "long" })}: ${s.kcal ? fmtKcal(s.kcal) + " ккал" : "нет записей"}`}
              >
                <span class="n num">{date.getDate()}</span>
                <span class="d num">
                  {#if loading && !totals.size}
                    <i class="skeleton"></i>
                  {:else if s.delta !== null}
                    {short(s.delta)}
                  {:else if s.kcal}
                    {fmtKcal(s.kcal)}
                  {/if}
                </span>
              </button>
            {:else}
              <span class="day pad"></span>
            {/if}
          {/each}
        </div>
      {/each}
    {/if}
  </section>

  {#if target}
    <p class="foot">
      В норме — в пределах ±{TOLERANCE * 100}% от нормы того дня: она считается по весу, записанному на тот момент. Сегодня ещё идёт и не оценивается; дни без записей не считаются.
    </p>
  {/if}

  <button class="ghost back-today" onclick={() => onPick(today)}>К сегодняшнему дню</button>
</div>

<style>
  .screen {
    display: flex;
    flex-direction: column;
  }

  header {
    display: grid;
    grid-template-columns: 44px 1fr 44px;
    align-items: center;
    min-height: 56px;
    margin-bottom: 8px;
  }

  h1 {
    text-align: center;
    font-size: 22px;
    font-weight: 700;
    letter-spacing: -0.01em;
    text-transform: capitalize;
  }

  .nav {
    width: 44px;
    height: 44px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    color: var(--accent);
  }

  .nav:disabled {
    color: var(--hint);
    opacity: 0.35;
  }

  .summary {
    margin-bottom: 12px;
  }

  .summary p {
    margin: 0;
  }

  .counts {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    margin: 0 0 14px;
  }

  .counts > div + div {
    border-left: 1px solid var(--line);
    padding-left: 12px;
  }

  dt {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--hint);
  }

  dd {
    margin: 2px 0 0;
    font-size: 24px;
    font-weight: 700;
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  .dot.under {
    background: var(--under);
  }
  .dot.ok {
    background: var(--ok);
  }
  .dot.over {
    background: var(--over);
  }

  .facts {
    list-style: none;
    margin: 0;
    padding: 12px 0 0;
    border-top: 1px solid var(--line);
    display: grid;
    gap: 6px;
    font-size: 14px;
  }

  .facts li {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }

  .facts li span:first-child {
    color: var(--hint);
  }

  .facts .under {
    color: var(--under);
  }

  .facts .over {
    color: var(--over);
  }

  .invite {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    margin-top: 10px;
    color: var(--accent);
    font-size: 15px;
  }

  .month {
    padding: 10px 8px 12px;
    display: grid;
    gap: 4px;
  }

  .row {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 4px;
  }

  .weekdays span {
    text-align: center;
    font-size: 12px;
    color: var(--hint);
    padding: 2px 0 4px;
  }

  .day {
    display: grid;
    align-content: center;
    justify-items: center;
    gap: 1px;
    aspect-ratio: 1 / 1.05;
    border-radius: 10px;
    min-width: 0;
  }

  .n {
    font-size: 15px;
    font-weight: 500;
  }

  .d {
    font-size: 10.5px;
    line-height: 1.1;
    min-height: 12px;
    color: var(--hint);
    white-space: nowrap;
  }

  .skeleton {
    display: block;
    width: 22px;
    height: 8px;
  }

  .day.under {
    background: color-mix(in srgb, var(--under) 16%, transparent);
  }
  .day.under .d {
    color: var(--under);
  }

  .day.ok {
    background: color-mix(in srgb, var(--ok) 16%, transparent);
  }
  .day.ok .d {
    color: var(--ok);
  }

  .day.over {
    background: color-mix(in srgb, var(--over) 18%, transparent);
  }
  .day.over .d {
    color: var(--over);
  }

  .day.open {
    background: var(--field);
  }

  .today {
    box-shadow: inset 0 0 0 1.5px var(--accent);
  }

  .today .n {
    color: var(--accent);
    font-weight: 700;
  }

  .future {
    opacity: 0.35;
  }

  button.day:active:not(:disabled) {
    transform: scale(0.94);
    transition: transform 0.1s;
  }

  .state {
    text-align: center;
    padding: 24px 0;
  }

  .state p {
    margin: 0 0 6px;
  }

  .foot {
    margin: 8px 16px 0;
    font-size: 13px;
    color: var(--hint);
  }

  .back-today {
    align-self: center;
    margin: 12px 0;
  }
</style>
