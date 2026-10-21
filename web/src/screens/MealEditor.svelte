<script lang="ts">
  import { untrack } from "svelte";
  import ItemRow from "../components/ItemRow.svelte";
  import Icon from "../components/Icon.svelte";
  import { api, type Draft, type Item, type Meal } from "../lib/api";
  import { blankItem, grams, kcal, sumTotals } from "../lib/nutrition";
  import { atTime, sameDay, timeValue } from "../lib/day";
  import { confirm, haptic, hapticSuccess } from "../lib/telegram";

  let {
    day,
    meal,
    draft,
    note = "",
    photos = [],
    onDone,
  }: { day: Date; meal?: Meal; draft?: Draft; note?: string; photos?: Blob[]; onDone: () => void } = $props();

  // A new meal on today defaults to now; on a past day, to noon.
  function initialTime(): string {
    if (meal) return timeValue(new Date(meal.eatenAt));
    return sameDay(day, new Date()) ? timeValue(new Date()) : "12:00";
  }

  // The editor is a fresh instance per meal, so it copies its input once and
  // owns the edits from then on.
  const source = untrack(() => meal ?? draft);
  let title = $state(source?.title ?? "");
  let items = $state<Item[]>(source?.items.length ? source.items.map((i) => ({ ...i })) : [blankItem()]);
  let time = $state(initialTime());
  let busy = $state(false);
  let error = $state("");

  // The model's clarifying question, answered in place: the answer re-runs the
  // estimate on the same photo and note, and the new draft replaces this one.
  let question = $state(untrack(() => draft?.question ?? ""));
  let options = $state<string[]>(untrack(() => draft?.options ?? []));
  let typed = $state("");
  let rethinking = $state(false);
  const canAnswer = $derived(Boolean(draft) && Boolean(question));

  const totals = $derived(sumTotals(items));
  const valid = $derived(items.length > 0 && items.every((i) => i.name.trim() && Number(i.grams) >= 0));

  async function answer(reply: string) {
    if (!reply.trim() || rethinking) return;
    haptic("medium");
    rethinking = true;
    error = "";
    try {
      const next = await api.analyze(photos, note, { question, answer: reply });
      title = next.title || title;
      if (next.items.length) items = next.items.map((i) => ({ ...i }));
      question = next.question;
      options = next.options ?? [];
      typed = "";
    } catch (e) {
      error = (e as Error).message;
    } finally {
      rethinking = false;
    }
  }

  function remove(index: number) {
    haptic();
    items.splice(index, 1);
  }

  function addLine() {
    haptic();
    items.push(blankItem());
  }

  async function save() {
    busy = true;
    error = "";
    const mealDay = meal ? new Date(meal.eatenAt) : day;
    const body = {
      eatenAt: atTime(mealDay, time).toISOString(),
      title: title.trim(),
      note: meal?.note ?? note,
      items: items.map((i) => ({
        ...i,
        name: i.name.trim(),
        grams: Number(i.grams) || 0,
        kcal100: Number(i.kcal100) || 0,
        protein100: Number(i.protein100) || 0,
        fat100: Number(i.fat100) || 0,
        carbs100: Number(i.carbs100) || 0,
      })),
    };
    try {
      if (meal) await api.updateMeal(meal.id, body);
      else await api.createMeal(body);
      hapticSuccess();
      onDone();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  async function destroy() {
    if (!meal || !(await confirm("Удалить этот приём пищи?"))) return;
    busy = true;
    try {
      await api.deleteMeal(meal.id);
      onDone();
    } catch (e) {
      error = (e as Error).message;
      busy = false;
    }
  }
</script>

<div class="screen">
  <header>
    <input class="title round" bind:value={title} placeholder="Название" maxlength="80" aria-label="Название" />
    <label class="time">
      <Icon name="clock" size={16} />
      <input class="num" type="time" bind:value={time} aria-label="Время" />
    </label>
  </header>

  {#if question}
    <div class="question" class:rethinking aria-busy={rethinking}>
      <span class="icon"><Icon name="question" /></span>
      <div class="ask">
        <p>{question}</p>
        {#if canAnswer}
          {#if options.length}
            <div class="options">
              {#each options as option (option)}
                <button class="option" onclick={() => answer(option)} disabled={rethinking}>{option}</button>
              {/each}
            </div>
          {/if}
          <form
            class="reply"
            onsubmit={(e) => {
              e.preventDefault();
              answer(typed);
            }}
          >
            <input bind:value={typed} placeholder="Свой ответ" maxlength="200" disabled={rethinking} aria-label="Свой ответ" />
            <button class="send" type="submit" disabled={rethinking || !typed.trim()} aria-label="Ответить">
              <Icon name="chevron-right" />
            </button>
          </form>
          <p class="hint">
            {rethinking ? "Пересчитываю с учётом ответа…" : "Ответ пересчитает приём пищи — это одно распознавание из дневного лимита."}
          </p>
        {:else}
          <p class="hint">Поправь строки ниже — например, убери масло, если его не было.</p>
        {/if}
      </div>
    </div>
  {/if}

  {#if draft && draft.items.length === 0}
    <p class="hint lone">Еды на фото не нашлось. Добавь продукты вручную.</p>
  {/if}

  <h2 class="section-title">
    <span>Продукты</span>
    <span class="legend">
      <span><i class="dot protein"></i>Б</span>
      <span><i class="dot fat"></i>Ж</span>
      <span><i class="dot carbs"></i>У</span>
    </span>
  </h2>
  <ul class="group">
    {#each items as _, index}
      <li><ItemRow bind:item={items[index]} onRemove={() => remove(index)} /></li>
    {/each}
    <li>
      <button class="add" onclick={addLine}><Icon name="plus" />Добавить продукт</button>
    </li>
  </ul>

  {#if meal}
    <button class="delete" onclick={destroy} disabled={busy}>Удалить приём пищи</button>
  {/if}

  <div class="dock">
    <div class="totals num">
      <span class="sum"><b class="round">{kcal(totals.kcal)}</b> ккал</span>
      <span class="hint">Б {grams(totals.protein)} · Ж {grams(totals.fat)} · У {grams(totals.carbs)}</span>
    </div>
    {#if error}
      <p class="error">{error}</p>
    {/if}
    <button class="primary" onclick={save} disabled={busy || !valid}>
      {busy ? "Сохраняю…" : meal ? "Сохранить" : "Записать"}
    </button>
  </div>
</div>

<style>
  .screen {
    display: flex;
    flex-direction: column;
  }

  header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 4px 0 16px;
  }

  .title {
    font-size: 22px;
    font-weight: 700;
    letter-spacing: -0.01em;
    background: transparent;
    padding: 6px 8px;
    margin-left: -8px;
  }

  .title:focus {
    background: var(--field);
  }

  .time {
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
    height: 36px;
    padding: 0 4px 0 10px;
    border-radius: 18px;
    background: var(--card);
    color: var(--hint);
  }

  .time input {
    width: 58px;
    padding: 0;
    background: transparent;
    border: 0;
    color: var(--text);
    font-weight: 500;
  }

  /* The chip's own clock icon replaces the browser's picker glyph. */
  .time input::-webkit-calendar-picker-indicator {
    display: none;
  }

  .question {
    display: flex;
    gap: 10px;
    margin-bottom: 16px;
    padding: 12px 14px;
    border-radius: var(--r);
    background: color-mix(in srgb, var(--accent) 10%, var(--card));
  }

  .question .icon {
    color: var(--accent);
    padding-top: 1px;
  }

  .question p {
    margin: 0;
  }

  .question .hint {
    margin-top: 4px;
    font-size: 13px;
  }

  .ask {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 8px;
  }

  .ask .hint {
    margin-top: 0;
  }

  .options {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .option {
    min-height: 36px;
    padding: 0 14px;
    border-radius: 18px;
    background: var(--card);
    color: var(--accent);
    font-weight: 500;
    font-size: 15px;
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 35%, transparent);
  }

  .option:active:not(:disabled) {
    background: color-mix(in srgb, var(--accent) 14%, var(--card));
  }

  .reply {
    display: flex;
    gap: 6px;
  }

  .reply input {
    background: var(--card);
    padding: 8px 12px;
    font-size: 15px;
  }

  .send {
    width: 40px;
    flex: none;
    display: grid;
    place-items: center;
    border-radius: var(--r-control);
    background: var(--accent);
    color: var(--accent-text);
  }

  .rethinking .ask > :not(.hint) {
    opacity: 0.5;
    transition: opacity 0.2s;
  }

  .lone {
    margin: 0 4px 12px;
  }

  .legend {
    display: flex;
    gap: 10px;
  }

  .legend span {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
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

  .add {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    min-height: 48px;
    padding: 0 16px;
    color: var(--accent);
    font-weight: 500;
  }

  .add:active {
    background: var(--field);
  }

  .delete {
    align-self: center;
    margin-top: 16px;
    min-height: 44px;
    padding: 0 16px;
    color: var(--danger);
  }

  .dock {
    margin-top: auto;
  }

  .group {
    margin-bottom: 8px;
  }

  .totals {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 12px;
    padding: 0 2px;
  }

  .sum {
    color: var(--hint);
    font-size: 14px;
  }

  .sum b {
    font-size: 22px;
    font-weight: 700;
    color: var(--text);
  }

  .totals .hint {
    font-size: 13px;
    white-space: nowrap;
  }

  .dock p {
    margin: 0;
  }
</style>
