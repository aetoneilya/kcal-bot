<script lang="ts">
  import { onMount } from "svelte";
  import { fly, fade } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Today from "./screens/Today.svelte";
  import Capture from "./screens/Capture.svelte";
  import MealEditor from "./screens/MealEditor.svelte";
  import ProfileScreen from "./screens/Profile.svelte";
  import Calendar from "./screens/Calendar.svelte";
  import WeightScreen from "./screens/Weight.svelte";
  import Icon, { type IconName } from "./components/Icon.svelte";
  import { api, type Draft, type Meal } from "./lib/api";
  import { startOfDay } from "./lib/day";
  import { targets, type Profile } from "./lib/targets";
  import { haptic, inTelegram, onBack } from "./lib/telegram";

  // Four top-level sections live in the tab bar; adding and editing a meal
  // are tasks pushed over them, with the tab bar hidden and Back to return.
  type Tab = "today" | "calendar" | "weight" | "profile";
  type Task =
    | { name: "capture" }
    | { name: "editor"; meal?: Meal; draft?: Draft; note?: string; photos?: Blob[] };

  const tabs: { key: Tab; label: string; icon: IconName }[] = [
    { key: "today", label: "Дневник", icon: "list" },
    { key: "calendar", label: "Календарь", icon: "calendar" },
    { key: "weight", label: "Вес", icon: "trend" },
    { key: "profile", label: "Норма", icon: "target" },
  ];

  let tab = $state<Tab>("today");
  let task = $state<Task | null>(null);
  // The day being viewed; a new meal is filed under it.
  let day = $state(startOfDay(new Date()));
  // Undefined while loading, null when the person has not filled it in.
  let profile = $state<Profile | null | undefined>(undefined);
  const target = $derived(profile ? targets(profile) : null);

  // A short confirmation after a save or delete, so leaving the editor is
  // not the only sign that it worked.
  let notice = $state("");
  let noticeTimer: ReturnType<typeof setTimeout> | undefined;
  function close(message?: string) {
    task = null;
    if (!message) return;
    notice = message;
    clearTimeout(noticeTimer);
    noticeTimer = setTimeout(() => (notice = ""), 1800);
  }
  function go(next: Tab) {
    if (next !== tab) haptic();
    task = null;
    tab = next;
  }

  // A weigh-in changes the profile's weight on the server; read it back.
  function loadProfile() {
    api
      .me()
      .then((me) => (profile = me.profile))
      .catch(() => (profile = profile ?? null));
  }

  onMount(loadProfile);

  // Telegram's back button leaves a task; top-level tabs have none.
  $effect(() => {
    if (!task) return;
    return onBack(() => close());
  });
</script>

<main class:with-tabs={!task}>
  {#if task}
    <!-- Tasks slide in from the right like a pushed screen. -->
    <div class="task" in:fly={{ x: 32, duration: 220, easing: cubicOut }}>
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
        <MealEditor {day} meal={task.meal} draft={task.draft} note={task.note} photos={task.photos} onDone={close} />
      {/if}
    </div>
  {:else if tab === "today"}
    <Today
      bind:day
      {target}
      profileLoaded={profile !== undefined}
      onAdd={() => (task = { name: "capture" })}
      onOpen={(meal) => (task = { name: "editor", meal })}
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
  {:else if tab === "weight"}
    <WeightScreen profile={profile ?? null} onChanged={loadProfile} />
  {:else}
    <ProfileScreen
      profile={profile ?? null}
      onSaved={(saved) => {
        profile = saved;
        go("today");
      }}
    />
  {/if}
</main>

{#if notice}
  <div class="notice" role="status" transition:fade={{ duration: 150 }}><Icon name="check" size={18} />{notice}</div>
{/if}

{#if !task}
  <nav class="tabbar" aria-label="Разделы">
    {#each tabs as t (t.key)}
      <button class:on={tab === t.key} aria-current={tab === t.key ? "page" : undefined} onclick={() => go(t.key)}>
        <Icon name={t.icon} size={24} />
        <span>{t.label}</span>
      </button>
    {/each}
  </nav>
{/if}

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
    background: var(--card);
    box-shadow: 0 -1px 0 var(--line);
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
</style>
