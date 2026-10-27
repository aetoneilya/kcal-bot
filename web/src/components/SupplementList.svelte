<script lang="ts">
  import { api, type Supplement } from "../lib/api";
  import { dayKey } from "../lib/day";
  import { haptic, hapticSuccess } from "../lib/telegram";
  import Icon from "./Icon.svelte";

  // The supplements to tick off each day in the diary, at the bottom of the
  // target screen. Tapping one puts it into the form to change.
  let list = $state<Supplement[]>([]);
  let loaded = $state(false);
  let error = $state("");
  let busy = $state(false);

  let editing = $state<number | null>(null);
  let name = $state("");
  let dose = $state("");
  let nameInput = $state<HTMLInputElement>();

  $effect(() => {
    api.supplements(dayKey(new Date())).then(
      (l) => {
        list = l;
        loaded = true;
      },
      (e) => {
        error = e.message;
        loaded = true;
      },
    );
  });

  function edit(s: Supplement) {
    haptic();
    editing = s.id;
    name = s.name;
    dose = s.dose;
    error = "";
    nameInput?.focus();
  }

  function reset() {
    editing = null;
    name = "";
    dose = "";
  }

  async function save() {
    if (!name.trim()) return;
    busy = true;
    error = "";
    try {
      if (editing === null) {
        list = [...list, await api.addSupplement(name, dose)];
      } else {
        const saved = await api.updateSupplement(editing, name, dose);
        list = list.map((s) => (s.id === saved.id ? { ...saved, taken: s.taken } : s));
      }
      hapticSuccess();
      reset();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  async function remove(s: Supplement) {
    haptic();
    error = "";
    try {
      await api.deleteSupplement(s.id);
      list = list.filter((x) => x.id !== s.id);
      if (editing === s.id) reset();
    } catch (e) {
      error = (e as Error).message;
    }
  }
</script>

<section class="supplements" aria-labelledby="supplements-title">
  <h2 id="supplements-title" class="round">Бады</h2>

  {#if list.length}
    <ul class="group">
      {#each list as s (s.id)}
        <li class="row" class:editing={editing === s.id}>
          <button class="text" onclick={() => edit(s)}>
            <span>{s.name}</span>
            {#if s.dose}<span class="sub">{s.dose}</span>{/if}
          </button>
          <button class="del" onclick={() => remove(s)} aria-label="Удалить {s.name}"><Icon name="x" size={16} /></button>
        </li>
      {/each}
    </ul>
  {/if}

  <form
    class="card entry"
    onsubmit={(e) => {
      e.preventDefault();
      save();
    }}
  >
    <input bind:this={nameInput} bind:value={name} maxlength="60" placeholder="Название, например Витамин D3" aria-label="Название" />
    <div class="line">
      <input bind:value={dose} maxlength="40" placeholder="Доза — по желанию" aria-label="Доза" />
      <button class="primary" type="submit" disabled={busy || !name.trim()}>
        {busy ? "…" : editing === null ? "Добавить" : "Сохранить"}
      </button>
    </div>
    {#if editing !== null}
      <button class="ghost cancel" type="button" onclick={reset}>Отмена</button>
    {/if}
  </form>
  {#if error}<p class="error">{error}</p>{/if}
  {#if loaded}
    <p class="foot">
      {list.length ? "Отмечай в дневнике под водой — по галочке в день." : "Добавь то, что пьёшь каждый день, — в дневнике появятся галочки."}
    </p>
  {/if}
</section>

<style>
  .supplements {
    display: flex;
    flex-direction: column;
    padding-bottom: 16px;
  }

  h2 {
    font-size: 20px;
    font-weight: 700;
    letter-spacing: -0.01em;
    margin: 12px 0 12px;
  }

  .group {
    margin-bottom: 12px;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 50px;
    padding: 0 6px 0 0;
    transition: background 0.15s;
  }

  .row.editing {
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }

  .text {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 2px;
    padding: 8px 0 8px 16px;
    text-align: left;
  }

  .text span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sub {
    font-size: 13px;
    color: var(--hint);
  }

  .del {
    flex: none;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    color: var(--hint);
  }

  .del:active {
    background: var(--field);
  }

  .entry {
    display: grid;
    gap: 8px;
    padding: 10px;
  }

  .line {
    display: flex;
    gap: 8px;
  }

  .line input {
    flex: 1;
    min-width: 0;
  }

  .line .primary {
    width: auto;
    min-height: 44px;
    font-size: 15px;
  }

  .cancel {
    justify-self: center;
    min-height: 36px;
  }

  .foot {
    margin: 6px 16px 0;
    font-size: 13px;
    color: var(--hint);
  }

  .error {
    margin: 8px 4px 0;
  }
</style>
