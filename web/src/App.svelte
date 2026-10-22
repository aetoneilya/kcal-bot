<script lang="ts">
  import { onMount } from "svelte";
  import { fly, fade } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Sheet from "./components/Sheet.svelte";
  import Today from "./screens/Today.svelte";
  import Capture from "./screens/Capture.svelte";
  import MealEditor from "./screens/MealEditor.svelte";
  import ProfileScreen from "./screens/Profile.svelte";
  import Calendar from "./screens/Calendar.svelte";
  import Icon, { type IconName } from "./components/Icon.svelte";
  import CatOverlay from "./components/CatOverlay.svelte";
  import { api, type Draft, type Meal } from "./lib/api";
  import { startOfDay } from "./lib/day";
  import { targets, type Profile } from "./lib/targets";
  import { haptic, inTelegram, onBack } from "./lib/telegram";
  import { ms } from "./lib/motion";
  import { Cats, complimentOfDay, pick, type Treats } from "./lib/treats";

  // Three top-level sections live in the tab bar; adding and editing a meal
  // are tasks on a sheet that grows out of the row or button that opened
  // it and shrinks back into it. The tab underneath stays as it was.
  type Tab = "today" | "calendar" | "profile";
  type Task =
    | { name: "capture" }
    | { name: "editor"; meal?: Meal; draft?: Draft; note?: string; photos?: Blob[] };

  const tabs: { key: Tab; label: string; icon: IconName }[] = [
    { key: "today", label: "Дневник", icon: "list" },
    { key: "calendar", label: "Календарь", icon: "calendar" },
    { key: "profile", label: "Норма", icon: "target" },
  ];

  let tab = $state<Tab>("today");
  let task = $state<Task | null>(null);
  // The day being viewed; a new meal is filed under it.
  let day = $state(startOfDay(new Date()));
  // Undefined while loading, null when the person has not filled it in.
  let profile = $state<Profile | null | undefined>(undefined);
  const target = $derived(profile ? targets(profile) : null);

  // Compliments and cats, for the people the server says get them.
  let treats = $state<Treats | null>(null);
  let cats: Cats | null = null;
  const todaysCompliment = $derived(treats ? complimentOfDay(treats.compliments, new Date()) : "");
  let cat = $state<{ video: Promise<string> | null; compliment: string; title: string } | null>(null);

  function showCat(title = "") {
    if (!treats) return;
    haptic("soft");
    cat = {
      video: cats?.any ? cats.next() : null,
      compliment: pick(treats.compliments, todaysCompliment) ?? "",
      title,
    };
    // Have the next one ready, so it starts at once and with sound.
    cats?.prefetch();
  }

  let sheet = $state<Sheet>();
  let from = $state<Element | null>(null);
  // Bumped after a save or delete, so the diary underneath reloads.
  let changes = $state(0);

  function open(next: Task, origin?: Element) {
    from = origin ?? null;
    task = next;
  }

  /** Where the sheet goes back to: the meal's row if it has one, else Add. */
  function returnTo(): Element | null {
    const id = task?.name === "editor" ? task.meal?.id : undefined;
    const row = id !== undefined ? document.querySelector(`[data-meal="${id}"]`) : null;
    return row ?? document.querySelector(".tabview .dock .primary");
  }

  // A short confirmation after a save or delete, so leaving the editor is
  // not the only sign that it worked.
  let notice = $state("");
  let noticeTimer: ReturnType<typeof setTimeout> | undefined;
  let closing = false;
  async function close(message?: string) {
    if (!task || closing) return;
    closing = true;
    await sheet?.close(returnTo());
    task = null;
    closing = false;
    if (!message) return;
    changes++;
    if (treats && message === "Записано") {
      // A new meal earns a cat; edits and deletes keep the plain notice.
      showCat(message);
      return;
    }
    notice = message;
    clearTimeout(noticeTimer);
    noticeTimer = setTimeout(() => (notice = ""), 1800);
  }
  function go(next: Tab) {
    if (next !== tab) haptic();
    tab = next;
  }

  // A weigh-in changes the profile's weight on the server; read it back.
  function loadProfile() {
    api
      .me()
      .then((me) => {
        profile = me.profile;
        if (me.treats && !treats) {
          api.treats().then((t) => {
            treats = t;
            cats = new Cats(t.cats, api.cat);
            cats.prefetch();
          }, () => {});
        }
      })
      .catch(() => (profile = profile ?? null));
  }

  onMount(loadProfile);

  // Telegram's back button leaves a task or the cat; top-level tabs have none.
  $effect(() => {
    if (cat) return onBack(() => (cat = null));
    if (!task) return;
    return onBack(() => close());
  });


</script>

<main class="with-tabs">
    {#key tab}
    <!-- Tabs switch at once, as on iOS: a fade would also drop the glass's
         blur until it ended, which reads as a blink. -->
    <div class="tabview">
    {#if tab === "today"}
    <Today
      bind:day
      {target}
      profileLoaded={profile !== undefined}
      treat={treats ? { compliment: todaysCompliment, cats: treats.cats.length > 0, onCat: () => showCat() } : null}
      {changes}
      onAdd={(origin) => open({ name: "capture" }, origin)}
      onOpen={(meal, origin) => open({ name: "editor", meal }, origin)}
      onGoal={() => go("profile")}
    />
  {:else if tab === "calendar"}
    <Calendar
      initial={day}
      {target}
      profile={profile ?? null}
      onPick={(picked) => {
        day = picked;
        go("today");
      }}
      onGoal={() => go("profile")}
    />
  {:else}
    <ProfileScreen
      profile={profile ?? null}
      onWeighed={loadProfile}
      onSaved={(saved) => {
        profile = saved;
        go("today");
      }}
    />
    {/if}
    </div>
    {/key}
</main>

{#if task}
  <Sheet bind:this={sheet} {from}>
    <!-- Inside the sheet, going from capture to the editor is a push. -->
    {#key task.name}
    <div class="task" in:fly={{ x: 32, duration: ms(220), easing: cubicOut }}>
      {#if !inTelegram}
        <!-- A plain browser has no Telegram back button; stand in for it. -->
        <button class="ghost back" onclick={() => close()}><Icon name="chevron-left" />Назад</button>
      {/if}
      {#if task.name === "capture"}
        <Capture
          onDraft={(draft, note, photos) => (task = { name: "editor", draft, note, photos })}
          onManual={() => (task = { name: "editor" })}
          onRepeat={(meal) =>
            (task = {
              name: "editor",
              // A copy, not the original: saving it adds a new meal at today's time.
              draft: { title: meal.title, items: meal.items, question: "", options: [] },
              note: meal.note,
            })}
        />
      {:else}
        <MealEditor {day} {target} meal={task.meal} draft={task.draft} note={task.note} photos={task.photos} onDone={close} />
      {/if}
    </div>
    {/key}
  </Sheet>
{/if}

{#if cat}
  <CatOverlay video={cat.video} compliment={cat.compliment} title={cat.title} onClose={() => (cat = null)} />
{/if}

{#if notice}
  <div class="notice" role="status" transition:fade={{ duration: 150 }}><Icon name="check" size={18} />{notice}</div>
{/if}

<nav class="tabbar" aria-label="Разделы">
  {#each tabs as t (t.key)}
    <button class:on={tab === t.key} aria-current={tab === t.key ? "page" : undefined} onclick={() => go(t.key)}>
      <Icon name={t.icon} size={24} />
      <span>{t.label}</span>
    </button>
  {/each}
</nav>

<style>
  main {
    max-width: 560px;
    min-height: var(--tg-viewport-stable-height, 100dvh);
    margin: 0 auto;
    padding: 8px var(--gutter) 0;
    display: flex;
    flex-direction: column;
    --tabbar: 0px;
  }

  /* Screens pin their action above the tab bar instead of under it. */
  main.with-tabs {
    --tabbar: calc(56px + var(--safe-bottom));
    padding-bottom: var(--tabbar);
  }

  main > :global(:last-child) {
    flex: 1;
  }

  .task {
    flex: 1;
    display: flex;
    flex-direction: column;
  }

  .task > :global(:last-child) {
    flex: 1;
  }

  .tabview {
    display: flex;
    flex-direction: column;
  }

  .tabview > :global(:last-child) {
    flex: 1;
  }

  .notice {
    position: fixed;
    top: 12px;
    left: 50%;
    transform: translateX(-50%);
    z-index: 10;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 14px 8px 10px;
    border-radius: 20px;
    background: var(--text);
    color: var(--card);
    font-size: 15px;
    font-weight: 500;
    box-shadow: 0 4px 16px rgb(0 0 0 / 0.18);
  }

  .back {
    align-self: flex-start;
    margin-left: -12px;
    padding-left: 4px;
  }

  .tabbar {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 5;
    display: flex;
    justify-content: center;
    padding: 0 8px var(--safe-bottom);
    background: var(--glass-strong);
    -webkit-backdrop-filter: var(--blur);
    backdrop-filter: var(--blur);
    box-shadow: inset 0 0.5px 0 var(--line);
  }

  .tabbar button {
    flex: 1;
    max-width: 140px;
    height: 56px;
    display: grid;
    justify-items: center;
    align-content: center;
    gap: 2px;
    color: var(--hint);
    font-size: 11px;
    font-weight: 500;
  }

  .tabbar .on {
    color: var(--accent);
  }

  .tabbar .on :global(svg) {
    animation: pop 0.34s var(--ease-out);
  }

  @keyframes pop {
    40% {
      transform: scale(0.84);
    }
    75% {
      transform: scale(1.06);
    }
  }
</style>
