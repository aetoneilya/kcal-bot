<script lang="ts">
  import { api, type Supplement } from "../lib/api";
  import { dayKey } from "../lib/day";
  import { haptic, hapticSuccess } from "../lib/telegram";
  import Icon from "./Icon.svelte";

  // The day's supplements, a tick each. Hidden until the list in the target
  // screen has something. A tap marks at once; a failed save puts back what
  // the server last confirmed.
  let { day }: { day: Date } = $props();

  let list = $state<Supplement[]>([]);
  let failed = $state(false);
  // What the server last confirmed for each, to fall back to.
  let saved = new Map<number, boolean>();

  $effect(() => {
    const key = dayKey(day);
    let stale = false;
    failed = false;
    api.supplements(key).then(
      (l) => {
        if (stale) return;
        list = l;
        saved = new Map(l.map((s) => [s.id, s.taken]));
      },
      () => {
        if (!stale) list = [];
      },
    );
    return () => (stale = true);
  });

  const taken = $derived(list.filter((s) => s.taken).length);

  let sending: Promise<unknown> = Promise.resolve();
  function toggle(s: Supplement) {
    const next = !s.taken;
    s.taken = next;
    failed = false;
    if (next && taken === list.length) hapticSuccess();
    else haptic();
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
  <section class="card supplements" aria-labelledby="supplements-title">
    <h2 id="supplements-title">
      <span>Каждый день</span>
      <span class="num" class:done={taken === list.length}>{taken} из {list.length}</span>
    </h2>
    <ul>
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
    {#if failed}<p class="error">Не сохранилось — проверь связь</p>{/if}
  </section>
{/if}

<style>
  .supplements {
    /* Sits under the water card the way that sits under the summary. */
    margin: -12px 0 24px;
    padding: 12px 8px 6px;
  }

  h2 {
    display: flex;
    justify-content: space-between;
    margin: 0 8px 4px;
    font-size: 15px;
    font-weight: 600;
  }

  h2 .num {
    font-weight: 400;
    color: var(--hint);
    transition: color 0.2s;
  }

  h2 .done {
    color: var(--ok);
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
