<script lang="ts">
  import { api, type Supplement } from "../lib/api";
  import { dayKey } from "../lib/day";
  import { slide } from "svelte/transition";
  import { haptic, hapticSuccess } from "../lib/telegram";
  import { easeOut, ms } from "../lib/motion";
  import Icon from "./Icon.svelte";

  // The day's supplements, a tick each. Hidden until the list in the target
  // screen has something. A tap marks at once; a failed save puts back what
  // the server last confirmed. Once everything is ticked the list folds
  // into its title line, to give the space back; a tap there opens it.
  let { day }: { day: Date } = $props();

  let list = $state<Supplement[]>([]);
  let failed = $state(false);
  // What the server last confirmed for each, to fall back to.
  let saved = new Map<number, boolean>();
  let folded = $state(false);
  // A day that opens finished is folded already; only the last tick animates.
  let animate = $state(false);
  let foldTimer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => {
    const key = dayKey(day);
    let stale = false;
    failed = false;
    clearTimeout(foldTimer);
    api.supplements(key).then(
      (l) => {
        if (stale) return;
        list = l;
        saved = new Map(l.map((s) => [s.id, s.taken]));
        animate = false;
        folded = l.length > 0 && l.every((s) => s.taken);
      },
      () => {
        if (!stale) list = [];
      },
    );
    return () => (stale = true);
  });

  const taken = $derived(list.filter((s) => s.taken).length);
  const done = $derived(list.length > 0 && taken === list.length);
  // A failed save can untick one; a folded list with something to do opens.
  const closed = $derived(folded && done);

  function fold(to: boolean) {
    clearTimeout(foldTimer);
    animate = true;
    folded = to;
  }

  let sending: Promise<unknown> = Promise.resolve();
  function toggle(s: Supplement) {
    const next = !s.taken;
    s.taken = next;
    failed = false;
    if (next && done) {
      hapticSuccess();
      // Long enough to see the last tick land.
      clearTimeout(foldTimer);
      foldTimer = setTimeout(() => done && fold(true), 700);
    } else haptic();
    const key = dayKey(day);
    const confirmed = saved;
    // One save at a time, in order, so the last tap wins.
    sending = sending.then(() =>
      api.markSupplement(s.id, key, next).then(
        () => confirmed.set(s.id, next),
        () => {
          if (confirmed !== saved) return; // another day is on screen now
          s.taken = confirmed.get(s.id) ?? false;
          failed = true;
        },
      ),
    );
  }
</script>

{#if list.length}
  <section class="card supplements" class:closed aria-labelledby="supplements-title">
    {#if done}
      <button class="head" onclick={() => fold(!closed)} aria-expanded={!closed}>
        <h2 id="supplements-title">Каждый день</h2>
        <span class="all"><Icon name="check" size={16} />всё выпито</span>
        <span class="chevron" class:open={!closed} aria-hidden="true"><Icon name="chevron-down" size={18} /></span>
      </button>
    {:else}
      <div class="head">
        <h2 id="supplements-title">Каждый день</h2>
        <span class="count num">{taken} из {list.length}</span>
      </div>
    {/if}
    {#if !closed}
      <ul transition:slide={{ duration: animate ? ms(320) : 0, easing: easeOut }}>
        {#each list as s (s.id)}
          <li>
            <button role="checkbox" aria-checked={s.taken} onclick={() => toggle(s)}>
              <span class="tick" class:on={s.taken} aria-hidden="true">
                {#if s.taken}<Icon name="check" size={16} />{/if}
              </span>
              <span class="name">{s.name}</span>
              {#if s.dose}<span class="dose">{s.dose}</span>{/if}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
    {#if failed}<p class="error">Не сохранилось — проверь связь</p>{/if}
  </section>
{/if}

<style>
  .supplements {
    /* Sits under the water card the way that sits under the summary. */
    margin: -12px 0 24px;
    padding: 6px 8px;
  }

  .head {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    min-height: 40px;
    padding: 0 8px;
    border-radius: 12px;
    text-align: left;
  }

  button.head:active {
    background: var(--field);
  }

  h2 {
    flex: 1;
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }

  .count {
    color: var(--hint);
  }

  .all {
    display: flex;
    align-items: center;
    gap: 4px;
    color: var(--ok);
    font-size: 15px;
    animation: appear 0.3s var(--ease-out);
  }

  @keyframes appear {
    from {
      opacity: 0;
      transform: translateX(6px);
    }
  }

  .chevron {
    display: grid;
    color: var(--hint);
    transition: transform 0.3s var(--ease-out);
  }

  .chevron.open {
    transform: rotate(180deg);
  }

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  button {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    min-height: 44px;
    padding: 4px 8px;
    border-radius: 12px;
    text-align: left;
    transition: background 0.12s;
  }

  button:active {
    background: var(--field);
  }

  .tick {
    flex: none;
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    box-shadow: inset 0 0 0 2px var(--field);
    color: #fff;
    transition:
      background 0.2s var(--ease-out),
      box-shadow 0.2s var(--ease-out),
      transform 0.2s var(--ease-out);
  }

  .tick.on {
    background: var(--ok);
    box-shadow: none;
    animation: pop 0.3s var(--ease-out);
  }

  @keyframes pop {
    50% {
      transform: scale(1.18);
    }
  }

  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .dose {
    flex: none;
    max-width: 45%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 14px;
    color: var(--hint);
  }

  .error {
    margin: 4px 8px 6px;
    font-size: 13px;
  }
</style>
