<script lang="ts">
  import MealThumb from "../components/MealThumb.svelte";
  import { onMount } from "svelte";
  import { api, type Draft, type Meal, type Regular } from "../lib/api";
  import { kcal, sumTotals } from "../lib/nutrition";
  import { plural } from "../lib/plural";
  import { decodeBarcode } from "../lib/barcode";
  import { downscale } from "../lib/image";
  import { haptic } from "../lib/telegram";
  import Icon from "../components/Icon.svelte";
  import Kitten from "../components/Kitten.svelte";

  let {
    onDraft,
    onManual,
    onRepeat,
  }: {
    onDraft: (draft: Draft, note: string, photos: Blob[]) => void;
    onManual: () => void;
    onRepeat: (meal: Meal) => void;
  } = $props();

  let regulars = $state<Regular[]>([]);

  // One meal may carry a few photos: the plate, another angle, the label.
  const MAX_PHOTOS = 3;
  let photos = $state<{ blob: Blob; url: string }[]>([]);
  let active = $state(0);
  let note = $state("");
  let busy = $state(false);
  let error = $state("");
  let left = $state<number | null>(null);

  const ready = $derived(Boolean(photos.length || note.trim()));
  const preview = $derived(photos[active]?.url ?? "");
  const room = $derived(MAX_PHOTOS - photos.length);

  onMount(() => {
    api
      .me()
      .then((me) => (left = me.analysesLeft))
      .catch(() => {});
    api
      .regularMeals()
      .then((list) => (regulars = list))
      .catch(() => {});
    return () => photos.forEach((p) => URL.revokeObjectURL(p.url));
  });

  async function pick(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const files = [...(input.files ?? [])].slice(0, room);
    input.value = ""; // picking the same photo again still fires change
    if (!files.length) return;
    error = "";
    for (const file of files) {
      let blob: Blob;
      try {
        blob = await downscale(file);
      } catch {
        blob = file; // the server sniffs the type; let it decide
      }
      photos.push({ blob, url: URL.createObjectURL(blob) });
    }
    active = photos.length - 1;
    haptic();
  }

  // A barcode skips the model: the label's numbers come from Open Food Facts.
  let scanning = $state(false);
  async function scan(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file) return;
    haptic();
    scanning = true;
    error = "";
    try {
      const code = await decodeBarcode(file);
      if (!code) {
        error = "Не разобрал штрихкод — сними ближе и ровно, чтобы полоски были чёткими.";
        return;
      }
      onDraft(await api.barcode(code), note, []);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      scanning = false;
    }
  }

  function remove(index: number) {
    haptic();
    URL.revokeObjectURL(photos[index].url);
    photos.splice(index, 1);
    active = Math.min(active, Math.max(photos.length - 1, 0));
  }

  async function analyze() {
    haptic("medium");
    busy = true;
    error = "";
    const blobs = photos.map((p) => p.blob);
    try {
      onDraft(await api.analyze(blobs, note), note, blobs);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<div class="screen">
  <h1 class="round">Что на тарелке?</h1>

  <div class="shot" class:filled={preview} class:busy>
    {#if preview}
      <img src={preview} alt="Фото блюда" />
      {#if busy}
        <div class="scan" aria-live="polite">
          <span class="sweep"></span>
          <span class="caption"><Kitten size={22} peek />Разглядываю тарелку…</span>
        </div>
      {:else}
        <button class="drop" onclick={() => remove(active)} aria-label="Убрать это фото"><Icon name="x" size={18} /></button>
      {/if}
    {:else}
      <label class="shoot">
        <span class="lens"><Icon name="camera" size={30} /></span>
        <span class="round">Сфотографировать</span>
        <span class="hint">Сверху, чтобы была видна вся тарелка</span>
        <input type="file" accept="image/*" capture="environment" onchange={pick} hidden disabled={busy} />
      </label>
    {/if}
  </div>

  {#if photos.length}
    <div class="thumbs">
      {#each photos as p, i (p.url)}
        <button class="thumb" class:on={i === active} onclick={() => (active = i)} disabled={busy} aria-label={`Фото ${i + 1}`}>
          <img src={p.url} alt="" />
        </button>
      {/each}
      {#if room > 0}
        <label class="thumb add" class:off={busy}>
          <Icon name="plus" size={22} />
          <input type="file" accept="image/*" capture="environment" onchange={pick} hidden disabled={busy} />
        </label>
      {/if}
    </div>
    {#if photos.length === 1}
      <p class="hint tip">Есть упаковка? Добавь фото этикетки — возьму БЖУ с неё.</p>
    {/if}
  {/if}

  <div class="alt">
    <label class="ghost" class:off={busy || room === 0}>
      <Icon name="image" />Из галереи
      <input type="file" accept="image/*" multiple onchange={pick} hidden disabled={busy || room === 0} />
    </label>
    <label class="ghost" class:off={busy || scanning}>
      <Icon name="barcode" />{scanning ? "Ищу…" : "Штрихкод"}
      <input type="file" accept="image/*" capture="environment" onchange={scan} hidden disabled={busy || scanning} />
    </label>
    <button class="ghost" onclick={onManual} disabled={busy}><Icon name="pencil" />Вручную</button>
  </div>

  {#if regulars.length && !photos.length}
    <h2 class="section-title"><span>Ещё раз — без распознавания</span></h2>
    <ul class="group regulars">
      {#each regulars.slice(0, 5) as r (r.meal.id)}
        <li>
          <button class="regular" onclick={() => onRepeat(r.meal)} disabled={busy}>
            <MealThumb id={r.meal.id} hasPhoto={r.meal.hasPhoto} size={40} />
            <span class="body">
              <span class="name">{r.meal.title || r.meal.items.map((i) => i.name).join(", ")}</span>
              <span class="sub num">{r.times > 1 ? `${r.times} ${plural(r.times, ["раз", "раза", "раз"])} за 2 месяца` : "недавно"}</span>
            </span>
            <span class="kcal num"><b class="round">{kcal(sumTotals(r.meal.items).kcal)}</b> ккал</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}

  <label class="field">
    <span class="label">Чего не видно на фото</span>
    <textarea
      rows="3"
      maxlength="500"
      placeholder="Жарено на масле, рис 200 г, кофе с сахаром"
      bind:value={note}
      disabled={busy}
    ></textarea>
    <span class="hint">Можно и без фото — просто опиши, что съел.</span>
  </label>

  <div class="dock">
    {#if error}
      <p class="error">{error}</p>
    {/if}
    <button class="primary" onclick={analyze} disabled={busy || !ready || left === 0}>
      {#if busy}
        <span class="spinner" aria-hidden="true"></span>Считаю, это 5–20 секунд
      {:else}
        Посчитать калории
      {/if}
    </button>
    {#if left !== null}
      <p class="hint quota num">
        {left === 0 ? "Распознавания на сегодня закончились — можно ввести вручную" : `Осталось распознаваний сегодня: ${left}`}
      </p>
    {/if}
  </div>
</div>

<style>
  .screen {
    display: flex;
    flex-direction: column;
  }

  h1 {
    font-size: 22px;
    font-weight: 700;
    letter-spacing: -0.01em;
    margin: 8px 0 14px;
  }

  .shot {
    position: relative;
    aspect-ratio: 4 / 3;
    border-radius: 20px;
    overflow: hidden;
    background: var(--card);
  }

  .shot:not(.filled) {
    box-shadow: inset 0 0 0 1.5px var(--line-strong);
    background: var(--glass);
    -webkit-backdrop-filter: var(--blur);
    backdrop-filter: var(--blur);
  }

  .shoot {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 4px;
    cursor: pointer;
    padding: 16px;
    text-align: center;
  }

  .shoot:active {
    background: var(--field);
  }

  .shoot .round {
    font-size: 17px;
    font-weight: 600;
    margin-top: 8px;
  }

  .shoot .hint {
    font-size: 13px;
  }

  .lens {
    width: 64px;
    height: 64px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: var(--accent);
    color: var(--accent-text);
  }

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: filter 0.4s var(--ease-out), transform 0.6s var(--ease-out);
  }

  .busy img {
    filter: saturate(0.6) brightness(0.8);
    transform: scale(1.02);
  }

  .drop {
    position: absolute;
    right: 10px;
    top: 10px;
    width: 36px;
    height: 36px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    background: rgb(0 0 0 / 0.55);
    color: #fff;
  }

  .thumbs {
    display: flex;
    gap: 8px;
    margin-top: 10px;
  }

  .thumb {
    width: 64px;
    height: 64px;
    flex: none;
    border-radius: 12px;
    overflow: hidden;
    display: grid;
    place-items: center;
    background: var(--card);
    box-shadow: inset 0 0 0 1px var(--line);
    cursor: pointer;
  }

  .thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    opacity: 0.6;
    transition: opacity 0.15s;
  }

  .thumb.on {
    box-shadow: 0 0 0 2px var(--accent);
  }

  .thumb.on img {
    opacity: 1;
  }

  .thumb.add {
    color: var(--accent);
    box-shadow: inset 0 0 0 1.5px var(--line-strong);
    background: transparent;
  }

  .tip {
    margin: 8px 4px 0;
    font-size: 13px;
  }

  /* The one authored moment: a scan line passing over the plate while the model looks. */
  .scan {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    padding: 14px;
  }

  .sweep {
    position: absolute;
    left: 0;
    right: 0;
    top: 0;
    height: 30%;
    background: linear-gradient(to bottom, transparent, rgb(255 255 255 / 0.28) 85%, rgb(255 255 255 / 0.85));
    animation: sweep 1.8s cubic-bezier(0.65, 0, 0.35, 1) infinite alternate;
  }

  @keyframes sweep {
    from {
      transform: translateY(-100%);
    }
    to {
      transform: translateY(240%);
    }
  }

  .caption {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 8px;
    min-height: 36px;
    padding: 0 14px 0 10px;
    border-radius: 18px;
    background: rgb(0 0 0 / 0.55);
    color: #fff;
    font-size: 14px;
    font-weight: 500;
  }

  .alt {
    display: flex;
    justify-content: center;
    gap: 8px;
    margin: 6px 0 14px;
  }

  .off {
    opacity: 0.45;
    pointer-events: none;
  }

  .regulars {
    margin-bottom: 16px;
  }

  .regular {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    min-height: 56px;
    padding: 8px 16px;
    text-align: left;
  }

  .regular:active:not(:disabled) {
    background: var(--field);
  }

  .regular .body {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 2px;
  }

  .regular .name {
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .regular .sub {
    font-size: 13px;
    color: var(--hint);
  }

  .regular .kcal {
    flex: none;
    font-size: 13px;
    color: var(--hint);
  }

  .regular .kcal b {
    font-size: 17px;
    font-weight: 600;
    color: var(--text);
  }

  .field {
    display: grid;
    gap: 6px;
  }

  .label {
    font-size: 13px;
    font-weight: 500;
    color: var(--hint);
    padding: 0 4px;
  }

  textarea {
    resize: none;
    background: var(--glass-strong);
    -webkit-backdrop-filter: var(--blur);
    backdrop-filter: var(--blur);
    border-radius: var(--r);
    padding: 12px 14px;
  }

  .field .hint {
    font-size: 13px;
    padding: 0 4px;
  }

  .dock {
    margin-top: auto;
  }

  .dock p {
    margin: 0;
  }

  .quota {
    text-align: center;
    font-size: 13px;
  }

  .spinner {
    width: 18px;
    height: 18px;
    border-radius: 50%;
    border: 2px solid currentColor;
    border-right-color: transparent;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(1turn);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .sweep {
      display: none;
    }
  }
</style>
