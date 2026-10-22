<script lang="ts">
  import { api, type Meal } from "../lib/api";
  import { add, energyShares, grams, kcal, sumTotals, type Totals } from "../lib/nutrition";
  import { addDays, dayKey, dayLabel, fullDate, startOfDay, timeLabel } from "../lib/day";
  import { arrive, lastShown, page, remember } from "../lib/motion";
  import { haptic } from "../lib/telegram";
  import type { Targets } from "../lib/targets";
  import Icon from "../components/Icon.svelte";
  import Kitten from "../components/Kitten.svelte";
  import MealThumb from "../components/MealThumb.svelte";
  import WaterCard from "../components/WaterCard.svelte";
  import CountUp from "../components/CountUp.svelte";
  import MacroTiles from "../components/MacroTiles.svelte";

  let {
    day = $bindable(),
    target,
    profileLoaded,
    onAdd,
    onOpen,
    onGoal,
    treat = null,
    changes = 0,
  }: {
    day: Date;
    target: Targets | null;
    profileLoaded: boolean;
    /** Compliment of the day and a cat button, for the people who get them. */
    treat?: { compliment: string; cats: boolean; onCat: () => void } | null;
    // The element is where the opened screen grows from.
    onAdd: (origin: Element) => void;
    onOpen: (meal: Meal, origin: Element) => void;
    /** Bumped when a meal was saved or deleted elsewhere: reload. */
    changes?: number;
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
    void changes;
    let stale = false;
    loading = true;
    error = "";
    api
      .meals(from, addDays(from, 1))
      .then((list) => {
        if (stale) return;
        // Meals that were not there the last time this day was on screen
        // are the ones just saved; they get to arrive.
        const key = dayKey(from);
        const before = lastShown<Set<number>>(`${key}:ids`);
        fresh = new Set(before ? list.filter((m) => !before.has(m.id)).map((m) => m.id) : []);
        remember(`${key}:ids`, new Set(list.map((m) => m.id)));
        barFrom = lastShown<number[]>(`${key}:bar`) ?? null;
        grown = false;
        meals = list;
        loadedKey = key;
        requestAnimationFrame(() => requestAnimationFrame(() => (grown = true)));
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
    dir = days;
    day = addDays(day, days);
  }

  // The day whose meals are on screen; its content slides in from the side
  // it was paged to once they have loaded, and counts up from what was shown
  // there before.
  let loadedKey = $state("");
  let dir = $state(0);
  let fresh = $state(new Set<number>());
  let barFrom = $state<number[] | null>(null);
  let grown = $state(false);
  const widths = $derived(macros.map((m) => ((m.share * totals.kcal) / scale) * 100));
  $effect(() => {
    if (!loadedKey || loading) return;
    remember(`${loadedKey}:kcal`, totals.kcal);
    remember(`${loadedKey}:left`, Math.abs(left));
    remember(`${loadedKey}:bar`, widths);
  });

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

  {#if treat && isToday && (treat.compliment || treat.cats)}
    <section class="treat" aria-label="Комплимент дня">
      {#if treat.compliment}<p class="round">{treat.compliment}</p>{/if}
      {#if treat.cats}
        <button class="cat" onclick={treat.onCat}><Kitten size={26} />Котик</button>
      {/if}
    </section>
  {/if}

  {#key loadedKey}
  <div class="day" in:page={{ dir }}>
  <section class="card summary" aria-label="Итог дня">
    <div class="top">
      <div class="lead">
        {#if loading && meals.length === 0}
          <span class="skeleton" style="width: 128px; height: 40px"></span>
        {:else}
          <span class="value round num" class:over><CountUp value={totals.kcal} from={lastShown(`${loadedKey}:kcal`)} format={kcal} /></span>
          <span class="unit num">{target ? `из ${kcal(target.kcal)} ккал` : "ккал"}</span>
        {/if}
      </div>
      {#if target}
        <button class="goal num" class:over onclick={onGoal} aria-label="Моя норма">
          <span class="round">
            {#if loading && meals.length === 0}…{:else}<CountUp value={Math.abs(left)} from={lastShown(`${loadedKey}:left`)} format={kcal} />{/if}
          </span>
          <span>{over ? "сверх нормы" : "осталось"}</span>
        </button>
      {/if}
    </div>

    <div class="bar" aria-hidden="true">
      {#each macros as m, i (m.key)}
        <span class="seg {m.key}" style="width: {grown || !barFrom ? widths[i] : (barFrom[i] ?? 0)}%"></span>
      {/each}
      {#if over && target}
        <span class="tick" style="left: {(target.kcal / scale) * 100}%"></span>
      {/if}
    </div>

    <MacroTiles
      protein={totals.protein}
      fat={totals.fat}
      carbs={totals.carbs}
      goals={target ? { protein: target.protein, fat: target.fat, carbs: target.carbs } : null}
    />

    {#if profileLoaded && !target}
      <button class="invite" onclick={onGoal}>
        Укажи рост, вес и цель — посчитаю, сколько тебе есть в день
        <Icon name="chevron-right" size={18} />
      </button>
    {/if}
  </section>

  <WaterCard {day} />

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
    <ul class="meals">
      {#each [0, 1] as i (i)}
        <li class="meal glass">
          <span class="skeleton" style="width: 56px; height: 56px; border-radius: 50%"></span>
          <span class="body">
            <span class="skeleton" style="width: 30%; height: 12px"></span>
            <span class="skeleton" style="width: 60%; height: 16px"></span>
            <span class="skeleton" style="width: 45%; height: 13px"></span>
          </span>
        </li>
      {/each}
    </ul>
  {:else if meals.length === 0}
    <div class="card empty">
      <span class="mark"><Kitten size={64} blink /></span>
      <p class="round">{isToday ? "Сегодня пока пусто" : "В этот день ничего не записано"}</p>
      <p class="hint">Сфотографируй тарелку — разберу её на продукты и посчитаю калории.</p>
    </div>
  {:else}
    <ul class="meals">
      {#each meals as meal (meal.id)}
        {@const t = sumTotals(meal.items)}
        <li class:fresh={fresh.has(meal.id)} in:arrive|global={{ fresh: fresh.has(meal.id) }}>
          <button class="meal glass" data-meal={meal.id} onclick={(e) => onOpen(meal, e.currentTarget)}>
            <MealThumb id={meal.id} hasPhoto={meal.hasPhoto} size={56} round />
            <span class="body">
              <span class="energy num"><b>{kcal(t.kcal)}</b> ккал · {timeLabel(new Date(meal.eatenAt))}</span>
              <span class="name">{mealName(meal)}</span>
              <span class="mac num" aria-label={`Белки ${grams(t.protein)} г, жиры ${grams(t.fat)} г, углеводы ${grams(t.carbs)} г`}>
                <span class="m-protein"><Icon name="drumstick" size={14} />{grams(t.protein)} г</span>
                <span class="m-fat"><Icon name="drop" size={14} />{grams(t.fat)} г</span>
                <span class="m-carbs"><Icon name="wheat" size={14} />{grams(t.carbs)} г</span>
              </span>
            </span>
            <span class="go"><Icon name="chevron-right" size={18} /></span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  </div>
  {/key}

  <div class="dock">
    <button class="primary" onclick={(e) => onAdd(e.currentTarget)}>
      <Icon name="camera" size={22} />
      Добавить приём пищи
    </button>
  </div>
</div>

<style>
  .treat {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
    padding: 12px 12px 12px 16px;
    border-radius: var(--r);
    background: color-mix(in srgb, var(--treat) 14%, var(--card));
  }

  .treat p {
    flex: 1;
    margin: 0;
    font-size: 16px;
    font-weight: 600;
    line-height: 1.3;
  }

  .treat .cat {
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: 44px;
    margin-left: auto;
    padding: 0 14px 0 10px;
    border-radius: 22px;
    background: var(--treat);
    color: #fff;
    font-weight: 600;
    transition: transform 0.12s var(--ease-out);
  }

  .treat .cat:active {
    transform: scale(0.96);
  }

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

  .day {
    display: flex;
    flex-direction: column;
  }

  /* A meal just saved glows once, so the eye finds where it went. */
  .fresh .meal {
    animation: glow 1.4s 0.2s ease-out;
  }

  @keyframes glow {
    from {
      background: color-mix(in srgb, var(--accent) 14%, transparent);
    }
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

  .summary :global(.tiles) {
    margin-top: 2px;
  }

  .meals {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 8px;
  }

  /* One frosted card per meal: the plate, then what and how much. */
  .meal {
    display: flex;
    align-items: center;
    gap: 14px;
    width: 100%;
    min-height: 80px;
    padding: 12px 10px 12px 12px;
    border-radius: 20px;
    text-align: left;
    transition: transform 0.15s var(--ease-out);
  }

  button.meal:active {
    transform: scale(0.98);
  }

  .body {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 3px;
  }

  .energy {
    font-size: 12px;
    color: var(--hint);
    white-space: nowrap;
  }

  .energy b {
    color: var(--text);
    font-weight: 600;
  }

  .name {
    font-size: 16px;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mac {
    display: flex;
    gap: 12px;
    margin-top: 2px;
    font-size: 13px;
    color: var(--hint);
    white-space: nowrap;
  }

  .mac > span {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .mac :global(svg) {
    color: var(--tint);
  }

  .m-protein {
    --tint: var(--protein);
  }
  .m-fat {
    --tint: var(--fat);
  }
  .m-carbs {
    --tint: var(--carbs);
  }

  .go {
    flex: none;
    color: var(--line-strong);
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
