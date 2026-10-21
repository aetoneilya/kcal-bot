<script lang="ts">
  import { onMount } from "svelte";
  import Today from "./screens/Today.svelte";
  import Capture from "./screens/Capture.svelte";
  import MealEditor from "./screens/MealEditor.svelte";
  import ProfileScreen from "./screens/Profile.svelte";
  import Calendar from "./screens/Calendar.svelte";
  import Icon from "./components/Icon.svelte";
  import { api, type Draft, type Meal } from "./lib/api";
  import { startOfDay } from "./lib/day";
  import { targets, type Profile } from "./lib/targets";
  import { inTelegram, onBack } from "./lib/telegram";

  type View =
    | { name: "today" }
    | { name: "capture" }
    | { name: "profile" }
    | { name: "calendar" }
    | { name: "editor"; meal?: Meal; draft?: Draft; note?: string; photos?: Blob[] };

  let view = $state<View>({ name: "today" });
  // The day being viewed; a new meal is filed under it.
  let day = $state(startOfDay(new Date()));
  // Undefined while loading, null when the person has not filled it in.
  let profile = $state<Profile | null | undefined>(undefined);
  const target = $derived(profile ? targets(profile) : null);

  const home = () => (view = { name: "today" });

  onMount(() => {
    api
      .me()
      .then((me) => (profile = me.profile))
      .catch(() => (profile = null));
  });

  // Telegram's back button mirrors leaving any sub-screen.
  $effect(() => {
    if (view.name === "today") return;
    return onBack(home);
  });
</script>

<main>
  {#if !inTelegram && view.name !== "today"}
    <!-- A plain browser has no Telegram back button; stand in for it. -->
    <button class="ghost back" onclick={home}><Icon name="chevron-left" />Назад</button>
  {/if}

  {#if view.name === "today"}
    <Today
      bind:day
      {target}
      profileLoaded={profile !== undefined}
      onAdd={() => (view = { name: "capture" })}
      onOpen={(meal) => (view = { name: "editor", meal })}
      onGoal={() => (view = { name: "profile" })}
      onCalendar={() => (view = { name: "calendar" })}
    />
  {:else if view.name === "calendar"}
    <Calendar
      initial={day}
      {target}
      onPick={(picked) => {
        day = picked;
        home();
      }}
      onGoal={() => (view = { name: "profile" })}
    />
  {:else if view.name === "capture"}
    <Capture
      onDraft={(draft, note, photos) => (view = { name: "editor", draft, note, photos })}
      onManual={() => (view = { name: "editor" })}
      onRepeat={(meal) =>
        (view = {
          name: "editor",
          // A copy, not the original: saving it adds a new meal at today's time.
          draft: { title: meal.title, items: meal.items, question: "", options: [] },
          note: meal.note,
        })}
    />
  {:else if view.name === "profile"}
    <ProfileScreen
      profile={profile ?? null}
      onSaved={(saved) => {
        profile = saved;
        home();
      }}
    />
  {:else}
    <MealEditor {day} meal={view.meal} draft={view.draft} note={view.note} photos={view.photos} onDone={home} />
  {/if}
</main>

<style>
  main {
    max-width: 560px;
    min-height: var(--tg-viewport-stable-height, 100dvh);
    margin: 0 auto;
    padding: 8px var(--gutter) 0;
    display: flex;
    flex-direction: column;
  }

  main > :global(:last-child) {
    flex: 1;
  }

  .back {
    align-self: flex-start;
    margin-left: -12px;
    padding-left: 4px;
  }
</style>
