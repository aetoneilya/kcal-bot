<script lang="ts">
  import { fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import { apply, backgrounds, blends, clear, defaultsFor, load, save, scheme, type Look } from "../lib/look";
  import { haptic } from "../lib/telegram";
  import { ms } from "../lib/motion";

  // The owner's lab: pick the background, turn the light and the grain up
  // or down, and see it live on the diary above. Each colour scheme keeps
  // its own settings; "Скопировать" hands them over to be made the default.
  let { onClose }: { onClose: () => void } = $props();

  const current = scheme();
  let look = $state<Look>(load(current));
  let copied = $state(false);

  $effect(() => {
    const next = { ...look };
    apply(next);
    save(next, current);
  });

  const percent = (v: number, of: number) => `${Math.round((v / of) * 100)}%`;
  const summary = $derived(JSON.stringify({ scheme: current, ...look, glow: +look.glow.toFixed(2), grain: +look.grain.toFixed(3) }));

  async function copy() {
    haptic();
    try {
      await navigator.clipboard.writeText(summary);
      copied = true;
      setTimeout(() => (copied = false), 1600);
    } catch {
      copied = false;
    }
  }

  function reset() {
    haptic("medium");
    clear(current);
    look = defaultsFor(current);
  }
</script>

<section class="lab glass" aria-label="Лаборатория вида" transition:fly={{ y: 40, duration: ms(260), easing: cubicOut }}>
  <header>
    <b class="round">Лаборатория</b>
    <span class="hint">{current === "dark" ? "тёмная тема" : "светлая тема"}</span>
    <button class="ghost close" onclick={onClose}>Готово</button>
  </header>

  <p class="label">Фон</p>
  <div class="chips">
    {#each backgrounds as b (b.key)}
      <button class:on={look.background === b.key} onclick={() => { haptic(); look.background = b.key; }}>{b.label}</button>
    {/each}
  </div>
  <p class="hint small">{backgrounds.find((b) => b.key === look.background)?.hint}</p>

  <label class="slider">
    <span>Свечение <b class="num">{percent(look.glow, 0.62)}</b></span>
    <input type="range" min="0" max="1" step="0.02" bind:value={look.glow} />
  </label>

  <label class="slider">
    <span>Зерно <b class="num">{percent(look.grain, 0.3)}</b></span>
    <input type="range" min="0" max="0.3" step="0.005" bind:value={look.grain} />
  </label>

  {#if look.grain > 0}
    <p class="label">Наложение зерна</p>
    <div class="chips">
      {#each blends as b (b.key)}
        <button class:on={look.blend === b.key} onclick={() => { haptic(); look.blend = b.key; }}>{b.label}</button>
      {/each}
    </div>
  {/if}

  <div class="actions">
    <button class="ghost" onclick={reset}>Сбросить</button>
    <button class="primary" onclick={copy}>{copied ? "Скопировано" : "Скопировать настройки"}</button>
  </div>
  <code class="num">{summary}</code>
</section>

<style>
  .lab {
    position: fixed;
    left: 8px;
    right: 8px;
    bottom: calc(8px + var(--safe-bottom));
    z-index: 15;
    max-width: 544px;
    max-height: 52dvh;
    margin: 0 auto;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: 12px 16px 14px;
    border-radius: 22px;
    background: var(--glass-strong);
  }

  header {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }

  header b {
    font-size: 17px;
  }

  .close {
    margin-left: auto;
    margin-right: -10px;
    min-height: 36px;
  }

  .label {
    margin: 10px 0 6px;
    font-size: 13px;
    font-weight: 500;
    color: var(--hint);
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .chips button {
    min-height: 34px;
    padding: 0 12px;
    border-radius: 17px;
    background: var(--field);
    font-size: 14px;
    font-weight: 500;
  }

  .chips .on {
    background: var(--accent);
    color: var(--accent-text);
  }

  .small {
    margin: 6px 0 0;
    font-size: 13px;
  }

  .slider {
    display: grid;
    gap: 4px;
    margin-top: 12px;
    font-size: 14px;
  }

  .slider span {
    display: flex;
    justify-content: space-between;
  }

  .slider input {
    padding: 0;
    background: none;
    border: 0;
    accent-color: var(--accent);
  }

  .actions {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 8px;
    margin-top: 14px;
  }

  .actions .primary {
    min-height: 44px;
    font-size: 15px;
  }

  code {
    display: block;
    margin-top: 10px;
    font-size: 11px;
    color: var(--hint);
    word-break: break-all;
    user-select: all;
  }
</style>
