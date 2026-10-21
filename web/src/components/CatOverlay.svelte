<script lang="ts">
  import { onDestroy } from "svelte";
  import { fade, scale } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import ChromaVideo from "./ChromaVideo.svelte";
  import Icon from "./Icon.svelte";
  import Kitten from "./Kitten.svelte";

  // A singing cat over the diary, its green screen keyed out, with a
  // compliment under it. `video` is an object URL still on its way (null
  // when there are no cats); tapping anywhere closes.
  let {
    video,
    compliment,
    title = "",
    onClose,
  }: { video: Promise<string> | null; compliment: string; title?: string; onClose: () => void } = $props();

  let src = $state("");
  let failed = $state(false);
  let muted = $state(false);

  $effect(() => {
    let stale = false;
    video?.then(
      (url) => {
        if (stale) URL.revokeObjectURL(url);
        else src = url;
      },
      () => (failed = true),
    );
    return () => (stale = true);
  });

  onDestroy(() => {
    if (src) URL.revokeObjectURL(src);
  });

  function unmute(e: MouseEvent) {
    e.stopPropagation();
    muted = false;
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="dialog" aria-modal="true" aria-label="Котик" tabindex="-1" onclick={onClose} transition:fade={{ duration: 180 }}>
  {#if title}
    <p class="title"><Icon name="check" size={18} />{title}</p>
  {/if}

  <div class="stage" in:scale={{ start: 0.9, duration: 260, easing: cubicOut }}>
    {#if video && !failed}
      {#if src}
        <ChromaVideo {src} bind:muted />
        {#if muted}
          <button class="sound" onclick={unmute}><Icon name="volume" size={18} />Со звуком</button>
        {/if}
      {:else}
        <span class="waiting"><Kitten size={72} peek /></span>
      {/if}
    {:else}
      <span class="waiting still"><Kitten size={88} /></span>
    {/if}
  </div>

  {#if compliment}
    <p class="compliment round">{compliment}</p>
  {/if}
  <p class="hint">Нажми, чтобы закрыть</p>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 20;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    padding: 24px var(--gutter) calc(24px + var(--safe-bottom));
    background: color-mix(in srgb, var(--bg) 78%, transparent);
    -webkit-backdrop-filter: blur(10px);
    backdrop-filter: blur(10px);
    color: var(--text);
    text-align: center;
    outline: none;
  }

  .title {
    display: flex;
    align-items: center;
    gap: 6px;
    margin: 0;
    padding: 6px 14px 6px 10px;
    border-radius: 18px;
    background: var(--card);
    font-weight: 600;
    box-shadow: 0 2px 10px rgb(0 0 0 / 0.08);
  }

  .stage {
    position: relative;
    display: grid;
    place-items: center;
    width: min(100%, 380px);
    min-height: 200px;
  }

  .sound {
    position: absolute;
    bottom: 12px;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: 44px;
    padding: 0 16px;
    border-radius: 22px;
    background: var(--treat);
    color: #fff;
    font-size: 15px;
    font-weight: 600;
    box-shadow: 0 4px 14px rgb(0 0 0 / 0.15);
  }

  .waiting {
    display: grid;
    place-items: center;
    width: 160px;
    height: 160px;
    border-radius: 50%;
    background: color-mix(in srgb, var(--treat) 18%, var(--card));
    color: var(--treat);
    animation: bounce 0.9s var(--ease-out) infinite alternate;
  }

  .waiting.still {
    animation: none;
  }

  .compliment {
    max-width: 340px;
    margin: 0;
    font-size: 21px;
    font-weight: 600;
    line-height: 1.3;
    text-wrap: balance;
  }

  .hint {
    margin: 0;
    color: var(--hint);
    font-size: 13px;
  }

  @keyframes bounce {
    from {
      transform: translateY(0) rotate(-4deg);
    }
    to {
      transform: translateY(-10px) rotate(4deg);
    }
  }
</style>
