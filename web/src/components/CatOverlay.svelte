<script lang="ts">
  import { onDestroy } from "svelte";
  import { fade, scale } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Icon from "./Icon.svelte";
  import Kitten from "./Kitten.svelte";

  // A dancing cat over everything, with a compliment under it. `video` is
  // an object URL still on its way (null when there are no cats); tapping
  // anywhere closes.
  let {
    video,
    compliment,
    title = "",
    onClose,
  }: { video: Promise<string> | null; compliment: string; title?: string; onClose: () => void } = $props();

  let src = $state("");
  let failed = $state(false);
  let muted = $state(false);
  let player = $state<HTMLVideoElement>();

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

  // Reels dance to music, so try with sound; a browser that refuses
  // autoplay with sound gets it muted and a button to turn it on.
  $effect(() => {
    const el = player;
    if (!el || !src) return;
    el.muted = false;
    el.play().catch(() => {
      el.muted = muted = true;
      el.play().catch(() => {});
    });
  });

  function unmute(e: MouseEvent) {
    e.stopPropagation();
    if (!player) return;
    player.muted = muted = false;
    player.play().catch(() => {});
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
        <!-- svelte-ignore a11y_media_has_caption -->
        <video bind:this={player} {src} loop playsinline preload="auto"></video>
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
    gap: 16px;
    padding: 24px var(--gutter) calc(24px + var(--safe-bottom));
    background: rgb(12 8 14 / 0.88);
    color: #fff;
    text-align: center;
    outline: none;
  }

  .title {
    display: flex;
    align-items: center;
    gap: 6px;
    margin: 0;
    font-weight: 600;
    opacity: 0.9;
  }

  .stage {
    position: relative;
    display: grid;
    place-items: center;
    width: min(100%, 360px);
    max-height: 62dvh;
  }

  video {
    width: 100%;
    max-height: 62dvh;
    border-radius: 22px;
    background: #000;
    object-fit: contain;
  }

  .sound {
    position: absolute;
    bottom: 12px;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: 40px;
    padding: 0 14px;
    border-radius: 20px;
    background: rgb(0 0 0 / 0.55);
    color: #fff;
    font-size: 15px;
    font-weight: 500;
  }

  .waiting {
    display: grid;
    place-items: center;
    width: 160px;
    height: 160px;
    border-radius: 50%;
    background: color-mix(in srgb, var(--treat) 30%, transparent);
    color: #fff;
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
    color: rgb(255 255 255 / 0.55);
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
