<script lang="ts" module>
  import { api } from "../lib/api";

  // A preview never changes once saved, so each is fetched once a session.
  const cache = new Map<number, Promise<string>>();

  function load(id: number): Promise<string> {
    let url = cache.get(id);
    if (!url) {
      url = api.mealPhoto(id).then((blob) => URL.createObjectURL(blob));
      url.catch(() => cache.delete(id));
      cache.set(id, url);
    }
    return url;
  }
</script>

<script lang="ts">
  import Icon from "./Icon.svelte";

  // The meal's photo as a small rounded tile; a quiet placeholder for meals
  // typed in by hand, so the rows stay aligned.
  let { id, hasPhoto, size = 44 }: { id: number; hasPhoto: boolean; size?: number } = $props();

  let src = $state("");
  $effect(() => {
    src = "";
    if (!hasPhoto) return;
    let stale = false;
    load(id).then(
      (url) => {
        if (!stale) src = url;
      },
      () => {},
    );
    return () => (stale = true);
  });
</script>

<span class="thumb" style="--size: {size}px" aria-hidden="true">
  {#if src}
    <img {src} alt="" />
  {:else if !hasPhoto}
    <Icon name="pencil" size={Math.round(size * 0.42)} />
  {/if}
</span>

<style>
  .thumb {
    flex: none;
    display: grid;
    place-items: center;
    width: var(--size);
    height: var(--size);
    border-radius: 10px;
    overflow: hidden;
    background: var(--field);
    color: var(--hint);
  }

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    animation: appear 0.2s ease-out;
  }

  @keyframes appear {
    from {
      opacity: 0;
    }
  }
</style>
