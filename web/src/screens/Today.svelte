<script lang="ts">
  import { api, type Meal } from "../lib/api";
  import { add, energyShares, grams, kcal, sumTotals, type Totals } from "../lib/nutrition";
  import { addDays, dayLabel, fullDate, startOfDay, timeLabel } from "../lib/day";
  import { haptic } from "../lib/telegram";
  import type { Targets } from "../lib/targets";
  import Icon from "../components/Icon.svelte";
  import Kitten from "../components/Kitten.svelte";

  let {
    day = $bindable(),
    target,
    profileLoaded,
    onAdd,
    onOpen,
    onGoal,
  }: {
    day: Date;
    target: Targets | null;
    profileLoaded: boolean;
    onAdd: () => void;
    onOpen: (meal: Meal) => void;
    onGoal: () => void;
  } = $props();

  let meals = $state<Meal[]>([]);
  let loading = $state(true);
  let error = $state("");
  let attempt = $state(0);

  const isToday = $derived(day.getTime() === startOfDay(new Date()).getTime());
  const title = $derived(dayLabel(day));
  // Under "Сегодня"/"Вчера" the date is worth spelling out; other days already say it.
  const subtitle = $derived(/\d/.test(title) ? "" : fullDate(day));
  const totals = $derived(
    meals.map((m) => sumTotals(m.items)).reduce(add, { kcal: 0, protein: 0, fat: 0, carbs: 0 } as Totals),
  );
  const shares = $derived(energyShares(totals));
  const macros = $derived([
    { key: "protein", label: "Белки", value: totals.protein, share: shares.protein, goal: target?.protein },
    { key: "fat", label: "Жиры", value: totals.fat, share: shares.fat, goal: target?.fat },
    { key: "carbs", label: "Углеводы", value: totals.carbs, share: shares.carbs, goal: target?.carbs },
  ]);
  // With a target the bar is the day's budget: eaten energy fills it, split by
  // macro. Over budget it rescales so the target becomes a tick inside it.
  const scale = $derived(Math.max(target?.kcal ?? 0, totals.kcal) || 1);
  const left = $derived(target ? target.kcal - Math.round(totals.kcal) : 0);
  const over = $derived(Boolean(target) && left < 0);

  $effect(() => {
    const from = day;
    void attempt;
    let stale = false;
    loading = true;
    error = "";
    api
      .meals(from, addDays(from, 1))
      .then((list) => {
        if (!stale) meals = list;
      })
      .catch((e) => {
        if (!stale) error = e.message;
      })
      .finally(() => {
        if (!stale) loading = false;
      });
    return () => (stale = true);
  });

  function shift(days: number) {
    if (days > 0 && isToday) return;
    haptic();
    day = addDays(day, days);
  }

  // A horizontal swipe moves between days, as in any diary app.
  let touchX = 0;
  let touchY = 0;
  function touchStart(e: TouchEvent) {
    touchX = e.touches[0].clientX;
    touchY = e.touches[0].clientY;
  }
  function touchEnd(e: TouchEvent) {
    const dx = e.changedTouches[0].clientX - touchX;
    const dy = e.changedTouches[0].clientY - touchY;
    if (Math.abs(dx) > 64 && Math.abs(dy) < 40) shift(dx < 0 ? 1 : -1);
  }

  function mealName(meal: Meal): string {
    return meal.title || meal.items.map((i) => i.name).join(", ") || "Без названия";
  }
</script>

<!-- The swipe is a shortcut; the arrow buttons are the accessible way between days. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="screen" ontouchstart={touchStart} ontouchend={touchEnd}>
  <header>
    <button class="nav" onclick={() => shift(-1)} aria-label="Предыдущий день">
      <Icon name="chevron-left" size={24} />
    </button>
    <div class="heading">
      <h1 class="round">{title}</h1>
      {#if subtitle}<p class="hint">{subtitle}</p>{/if}
    </div>
    <button class="nav" onclick={() => shift(1)} disabled={isToday} aria-label="Следующий день">
      <Icon name="chevron-right" size={24} />
    </button>
  </header>

  <section class="card summary" aria-label="Итог дня">
    <div class="top">
      <div class="lead">
        {#if loading && meals.length === 0}
          <span class="skeleton" style="width: 128px; height: 40px"></span>
        {:else}
          <span class="value round num" class:over>{kcal(totals.kcal)}</span>
          <span class="unit num">{target ? `из ${kcal(target.kcal)} ккал` : "ккал"}</span>
        {/if}
      </div>
      {#if target}
        <button class="goal num" class:over onclick={onGoal} aria-label="Моя норма">
          <span class="round">{kcal(Math.abs(left))}</span>
          <span>{over ? "сверх нормы" : "осталось"}</span>
        </button>
      {/if}
    </div>

    <div class="bar" aria-hidden="true">
      {#each macros as m (m.key)}
        <span class="seg {m.key}" style="width: {((m.share * totals.kcal) / scale) * 100}%"></span>
      {/each}
      {#if over && target}
        <span class="tick" style="left: {(target.kcal / scale) * 100}%"></span>
      {/if}
    </div>

    <dl class="macros">
      {#each macros as m (m.key)}
        <div>
          <dt><i class="dot {m.key}"></i>{m.label}</dt>
          <dd class="round num">{grams(m.value)}<span>{m.goal ? ` / ${m.goal}` : ""}&nbsp;г</span></dd>
        </div>
      {/each}
    </dl>

    {#if profileLoaded && !target}
      <button class="invite" onclick={onGoal}>
        Укажи рост, вес и цель — посчитаю, сколько тебе есть в день
        <Icon name="chevron-right" size={18} />
      </button>
    {/if}
  </section>

  <h2 class="section-title">
    <span>Приёмы пищи</span>
    {#if meals.length > 0}<span class="num">{meals.length}</span>{/if}
  </h2>

  {#if error}
    <div class="card state">
      <p>Не получилось загрузить дневник.</p>
      <p class="hint">{error}</p>
      <button class="ghost" onclick={() => attempt++}>Повторить</button>
    </div>
  {:else if loading && meals.length === 0}
    <ul class="group">
      {#each [0, 1] as i (i)}
        <li class="row">
          <span class="skeleton" style="width: 40px; height: 14px"></span>
          <span class="body">
            <span class="skeleton" style="width: 60%; height: 16px"></span>
            <span class="skeleton" style="width: 40%; height: 13px"></span>
          </span>
          <span class="skeleton" style="width: 36px; height: 16px"></span>
        </li>
      {/each}
    </ul>
  {:else if meals.length === 0}
    <div class="card empty">
      <span class="mark"><Kitten size={64} /></span>
      <p class="round">{isToday ? "Сегодня пока пусто" : "В этот день ничего не записано"}</p>
      <p class="hint">Сфотографируй тарелку — разберу её на продукты и посчитаю калории.</p>
    </div>
  {:else}
    <ul class="group">
      {#each meals as meal (meal.id)}
        {@const t = sumTotals(meal.items)}
        <li>
          <button class="row" onclick={() => onOpen(meal)}>
            <span class="time num">{timeLabel(new Date(meal.eatenAt))}</span>
            <span class="body">
              <span class="name">{mealName(meal)}</span>
              <span class="sub num">Б {grams(t.protein)} · Ж {grams(t.fat)} · У {grams(t.carbs)}</span>
            </span>
            <span class="kcal num"><b class="round">{kcal(t.kcal)}</b> ккал</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}

  <div class="dock">
    <button class="primary" onclick={onAdd}>
      <Icon name="camera" size={22} />
      Добавить приём пищи
    </button>
  </div>
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

  .heading {
    text-align: center;
  }

  h1 {
    font-size: 22px;
    font-weight: 700;
    letter-spacing: -0.01em;
    line-height: 1.2;
    text-transform: capitalize;
  }

  .heading .hint {
    margin: 2px 0 0;
    font-size: 13px;
  }

  .nav {
    width: 44px;
    height: 44px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    color: var(--accent);
  }

  .nav:active:not(:disabled) {
    background: var(--field);
  }

  .nav:disabled {
    color: var(--hint);
    opacity: 0.35;
  }

  .summary {
    padding: 18px 16px 14px;
    margin-bottom: 24px;
  }

  .top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
  }

  .lead {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    column-gap: 6px;
    min-height: 44px;
  }

  .value.over {
    color: var(--warn);
  }

  .goal {
    display: grid;
    justify-items: end;
    padding: 2px 0 2px 8px;
    text-align: right;
    font-size: 13px;
    color: var(--hint);
  }

  .goal .round {
    font-size: 20px;
    font-weight: 600;
    color: var(--text);
    line-height: 1.2;
  }

  .goal.over .round {
    color: var(--warn);
  }

  .invite {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    width: 100%;
    margin-top: 14px;
    padding: 12px 0 0;
    border-top: 1px solid var(--line);
    color: var(--accent);
    font-size: 15px;
    text-align: left;
  }

  .value {
    font-size: 40px;
    font-weight: 700;
    letter-spacing: -0.02em;
    line-height: 1.1;
  }

  .unit {
    font-size: 16px;
    color: var(--hint);
  }

  .bar {
    position: relative;
    display: flex;
    gap: 2px;
    height: 6px;
    margin: 14px 0 14px;
    border-radius: 3px;
    overflow: hidden;
    background: var(--field);
  }

  .tick {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 2px;
    margin-left: -1px;
    background: var(--text);
  }

  .seg {
    flex: none;
    transition: width 0.6s var(--ease-out);
  }

  .protein {
    background: var(--protein);
  }
  .fat {
    background: var(--fat);
  }
  .carbs {
    background: var(--carbs);
  }

  .macros {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    margin: 0;
  }

  .macros > div + div {
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

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex: none;
  }

  dd {
    margin: 2px 0 0;
    font-size: 20px;
    font-weight: 600;
  }

  dd span {
    font-size: 14px;
    font-weight: 400;
    color: var(--hint);
  }

  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    min-height: 64px;
    padding: 10px 16px;
    text-align: left;
  }

  button.row:active {
    background: var(--field);
  }

  .time {
    width: 40px;
    flex: none;
    font-size: 14px;
    color: var(--hint);
  }

  .body {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 4px;
  }

  .name {
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sub {
    font-size: 13px;
    color: var(--hint);
    white-space: nowrap;
  }

  .kcal {
    flex: none;
    text-align: right;
    font-size: 13px;
    color: var(--hint);
  }

  .kcal b {
    font-size: 17px;
    font-weight: 600;
    color: var(--text);
  }

  .empty,
  .state {
    text-align: center;
    padding: 28px 24px;
  }

  .empty p,
  .state p {
    margin: 0;
  }

  .empty .round {
    font-size: 17px;
    font-weight: 600;
    margin: 12px 0 6px;
  }

  .mark {
    display: inline-block;
    color: var(--hint);
  }

  .state .ghost {
    margin-top: 8px;
  }

  /* Pinned to the bottom of a short day, sticky over a long one. */
  .dock {
    margin-top: auto;
  }

  .group,
  .empty,
  .state {
    margin-bottom: 16px;
  }
</style>
