<script lang="ts">
  import { untrack } from "svelte";
  import ItemRow from "../components/ItemRow.svelte";
  import Icon from "../components/Icon.svelte";
  import { api, type Draft, type Item, type Meal } from "../lib/api";
  import { blankItem, grams, kcal, sumTotals } from "../lib/nutrition";
  import { atTime, sameDay, timeValue } from "../lib/day";
  import { confirm, haptic, hapticSuccess } from "../lib/telegram";
  import { base64, largePhoto, thumbnail } from "../lib/image";
  import type { Targets } from "../lib/targets";
  import MacroTiles from "../components/MacroTiles.svelte";

  let {
    day,
    meal,
    draft,
    note = "",
    photos = [],
    target = null,
    onDone,
  }: {
    day: Date;
    meal?: Meal;
    draft?: Draft;
    note?: string;
    photos?: Blob[];
    target?: Targets | null;
    onDone: (notice?: string) => void;
  } = $props();

  // The meal's photo across the top: the one just taken, or the saved big
  // copy of an existing meal. None for meals typed in by hand.
  let hero = $state("");
  $effect(() => {
    let url = "";
    let stale = false;
    untrack(() => {
      if (photos?.length) {
        url = URL.createObjectURL(photos[0]);
        hero = url;
      } else if (meal?.hasPhoto) {
        api.mealPhoto(meal.id, true).then(
          (b) => {
            if (stale) return;
            url = URL.createObjectURL(b);
            hero = url;
          },
          () => {},
        );
      }
    });
    return () => {
      stale = true;
      if (url) URL.revokeObjectURL(url);
    };
  });

  // "Обед · 390 г" over the title, from the time and the weight.
  function slot(hhmm: string): string {
    const h = Number(hhmm.slice(0, 2));
    if (h < 11) return "Завтрак";
    if (h < 16) return "Обед";
    if (h < 18) return "Перекус";
    return "Ужин";
  }

  // The tiles show the portion against the day's target, or the meal per
  // 100 g to compare with a label.
  let per = $state<"portion" | "100g">("portion");

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
  const weight = $derived(items.reduce((sum, i) => sum + (Number(i.grams) || 0), 0));
  const shown = $derived.by(() => {
    if (per === "portion" || weight <= 0) return totals;
    const k = 100 / weight;
    return { kcal: totals.kcal * k, protein: totals.protein * k, fat: totals.fat * k, carbs: totals.carbs * k };
  });
  const overline = $derived(`${slot(time)}${weight > 0 ? ` · ${grams(weight)} г` : ""}`);
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
      else {
        const [photo, photoLarge] = await Promise.all([preview(thumbnail), preview(largePhoto)]);
        await api.createMeal({ ...body, photo, photoLarge });
      }
      hapticSuccess();
      onDone(meal ? "Сохранено" : "Записано");
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  // The first photo, resized, for the diary and the meal's screen. Only a
  // nicety: without it the meal still saves.
  async function preview(resize: (b: Blob) => Promise<Blob>): Promise<string | undefined> {
    if (!photos?.length) return undefined;
    try {
      const small = await resize(photos[0]);
      return small.size <= 400 * 1024 ? await base64(small) : undefined;
    } catch {
      return undefined;
    }
  }

  async function destroy() {
    if (!meal || !(await confirm("Удалить этот приём пищи?"))) return;
    busy = true;
    try {
      await api.deleteMeal(meal.id);
      onDone("Удалено");
    } catch (e) {
      error = (e as Error).message;
      busy = false;
    }
  }
</script>

<div class="screen">
  <!-- "Обед · 420 г" and the time share a line above the title, so a long
       title gets the whole width. -->
  {#snippet meta()}
    <div class="meta">
      <p class="overline num">{overline}</p>
      <label class="time glass">
        <Icon name="clock" size={15} />
        <input class="num" type="time" bind:value={time} aria-label="Время" />
      </label>
    </div>
  {/snippet}
  {#if hero}
    <div class="hero">
      <img src={hero} alt="" />
      <div class="caption">
        {@render meta()}
        <input class="title round" bind:value={title} placeholder="Название" maxlength="80" aria-label="Название" />
      </div>
    </div>
  {:else}
    <header>
      {@render meta()}
      <input class="title round" bind:value={title} placeholder="Название" maxlength="80" aria-label="Название" />
    </header>
  {/if}

  <section class="nutrition" aria-label="Пищевая ценность">
    <div class="per">
      <span class="label">{per === "100g" ? "На 100 г" : "На порцию"}</span>
      <button
        class="toggle"
        role="switch"
        aria-checked={per === "100g"}
        aria-label="Показать на 100 г"
        onclick={() => {
          haptic();
          per = per === "100g" ? "portion" : "100g";
        }}
      >
        <span>100 г</span><i></i>
      </button>
    </div>
    <MacroTiles
      protein={shown.protein}
      fat={shown.fat}
      carbs={shown.carbs}
      energy={shown.kcal}
      goals={per === "portion" && target ? { protein: target.protein, fat: target.fat, carbs: target.carbs, kcal: target.kcal } : null}
    />
  </section>

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
    margin: 4px 0 16px;
  }

  .meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  /* The photo runs edge to edge and fades into the screen, so the title
     below sits on the theme's own colour in light and dark alike. */
  .hero {
    position: relative;
    height: min(44vh, 340px);
    margin: -8px calc(-1 * var(--gutter)) 14px;
  }

  .hero img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    animation: settle 0.6s var(--ease-out);
  }

  @keyframes settle {
    from {
      opacity: 0;
      transform: scale(1.04);
    }
  }

  .hero::after {
    content: "";
    position: absolute;
    inset: 0;
    background: linear-gradient(to bottom, transparent 38%, color-mix(in srgb, var(--bg) 82%, transparent) 76%, var(--bg));
    pointer-events: none;
  }

  .caption {
    position: absolute;
    left: var(--gutter);
    right: var(--gutter);
    bottom: 0;
    z-index: 1;
  }

  .overline {
    margin: 0;
    color: var(--hint);
    font-size: 13px;
    font-weight: 500;
  }

  .caption .title {
    font-size: 26px;
  }

  .nutrition {
    display: grid;
    gap: 10px;
    margin-bottom: 18px;
  }

  /* What the tiles show, and a small switch to flip it to per 100 g. */
  .per {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .per .label {
    color: var(--hint);
    font-size: 13px;
    font-weight: 500;
  }

  .toggle {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 32px;
    color: var(--hint);
    font-size: 13px;
    font-weight: 500;
  }

  .toggle i {
    position: relative;
    width: 36px;
    height: 22px;
    border-radius: 11px;
    background: var(--line-strong);
    transition: background-color 0.2s;
  }

  .toggle i::after {
    content: "";
    position: absolute;
    top: 2px;
    left: 2px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 1px 3px rgb(0 0 0 / 0.25);
    transition: transform 0.2s var(--ease-out);
  }

  .toggle[aria-checked="true"] i {
    background: var(--accent);
  }

  .toggle[aria-checked="true"] i::after {
    transform: translateX(14px);
  }

  .title {
    width: calc(100% + 8px);
    margin-top: 2px;
    font-size: 24px;
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
