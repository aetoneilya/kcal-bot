<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Draft } from "../lib/api";
  import { downscale } from "../lib/image";
  import { haptic } from "../lib/telegram";
  import Icon from "../components/Icon.svelte";
  import Kitten from "../components/Kitten.svelte";

  let {
    onDraft,
    onManual,
  }: { onDraft: (draft: Draft, note: string, photo: Blob | null) => void; onManual: () => void } = $props();

  let photo = $state<Blob | null>(null);
  let preview = $state("");
  let note = $state("");
  let busy = $state(false);
  let error = $state("");
  let left = $state<number | null>(null);

  const ready = $derived(Boolean(photo || note.trim()));

  onMount(() => {
    api
      .me()
      .then((me) => (left = me.analysesLeft))
      .catch(() => {});
    return () => preview && URL.revokeObjectURL(preview);
  });

  async function pick(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ""; // picking the same photo again still fires change
    if (!file) return;
    error = "";
    try {
      photo = await downscale(file);
    } catch {
      photo = file; // the server sniffs the type; let it decide
    }
    if (preview) URL.revokeObjectURL(preview);
    preview = URL.createObjectURL(photo);
    haptic();
  }

  async function analyze() {
    haptic("medium");
    busy = true;
    error = "";
    try {
      onDraft(await api.analyze(photo, note), note, photo);
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
        <label class="retake">
          <Icon name="camera" size={18} />Переснять
          <input type="file" accept="image/*" capture="environment" onchange={pick} hidden />
        </label>
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

  <div class="alt">
    <label class="ghost" class:off={busy}>
      <Icon name="image" />Из галереи
      <input type="file" accept="image/*" onchange={pick} hidden disabled={busy} />
    </label>
    <button class="ghost" onclick={onManual} disabled={busy}><Icon name="pencil" />Вручную</button>
  </div>

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
    background: transparent;
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

  .retake {
    position: absolute;
    right: 10px;
    bottom: 10px;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-height: 36px;
    padding: 0 12px;
    border-radius: 18px;
    background: rgb(0 0 0 / 0.55);
    color: #fff;
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
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
    background: var(--card);
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
