<script lang="ts">
  import { onMount, untrack } from "svelte";
  import { api, type Reminders } from "../lib/api";
  import { activities, direction, limits, targets, type Activity, type Profile, type Sex } from "../lib/targets";
  import { kcal as fmtKcal } from "../lib/nutrition";
  import { haptic, hapticSuccess } from "../lib/telegram";
  import Icon from "../components/Icon.svelte";
  import WeightLog from "../components/WeightLog.svelte";

  let {
    profile,
    onSaved,
    onWeighed,
  }: { profile: Profile | null; onSaved: (profile: Profile) => void; onWeighed: () => void } = $props();

  // Copied once: the form owns the edits until they are saved.
  const start = untrack(() => profile);
  let sex = $state<Sex | undefined>(start?.sex);
  let birthYear = $state<number | undefined>(start?.birthYear);
  let heightCm = $state<number | undefined>(start?.heightCm);
  let weightKg = $state<number | undefined>(start?.weightKg);
  let goalWeightKg = $state<number | undefined>(start?.goalWeightKg);
  let activity = $state<Activity | undefined>(start?.activity);
  let pace = $state<number>(start?.paceKgWeek || 0.5);
  let busy = $state(false);
  let error = $state("");

  const draft = $derived<Partial<Profile>>({
    sex,
    birthYear: num(birthYear),
    heightCm: num(heightCm),
    weightKg: num(weightKg),
    goalWeightKg: num(goalWeightKg),
    activity,
    paceKgWeek: pace,
  });
  const dir = $derived(
    num(weightKg) !== undefined && num(goalWeightKg) !== undefined
      ? direction({ weightKg: num(weightKg)!, goalWeightKg: num(goalWeightKg)! })
      : null,
  );
  const paces = $derived(dir === "gain" ? [0.25, 0.5] : [0.25, 0.5, 0.75, 1]);
  const result = $derived(targets(draft));
  const goalDate = $derived(
    result?.weeks ? new Date(Date.now() + result.weeks * 7 * 864e5).toLocaleDateString("ru-RU", { day: "numeric", month: "long", year: "numeric" }) : "",
  );

  // Gaining faster than half a kilo a week is mostly fat; snap down when the
  // goal flips from losing to gaining.
  $effect(() => {
    if (dir === "gain" && pace > 0.5) pace = 0.5;
  });

  function num(v: number | undefined | null): number | undefined {
    return typeof v === "number" && Number.isFinite(v) ? v : undefined;
  }

  function pick<T>(set: (v: T) => void, value: T) {
    haptic();
    set(value);
  }

  async function save() {
    if (!result) return;
    busy = true;
    error = "";
    const body = { ...(draft as Profile), paceKgWeek: dir === "hold" ? 0 : pace };
    try {
      const saved = await api.saveProfile(body);
      hapticSuccess();
      onSaved(saved);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  const signed = (v: number) => (v > 0 ? "+" : v < 0 ? "−" : "") + Math.abs(v).toLocaleString("ru-RU", { maximumFractionDigits: 2 });
  const thisYear = new Date().getFullYear();

  // Reminders save as soon as they change: they are not part of the target.
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone || "Europe/Moscow";
  let reminders = $state<Reminders>({ timeZone: zone, summaryAt: "", nudge: false });
  let summaryTime = $state("21:00");
  let reminderError = $state("");

  onMount(() => {
    api
      .reminders()
      .then((r) => {
        reminders = { ...r, timeZone: zone };
        if (r.summaryAt) summaryTime = r.summaryAt;
      })
      .catch(() => {});
  });

  async function saveReminders(next: Partial<Reminders>) {
    haptic();
    const before = reminders;
    reminders = { ...reminders, ...next, timeZone: zone };
    reminderError = "";
    try {
      reminders = await api.saveReminders(reminders);
    } catch (e) {
      reminders = before;
      reminderError = (e as Error).message;
    }
  }
</script>

<div class="screen">
  <h1 class="round">Моя норма</h1>

  <h2 class="section-title"><span>О себе</span></h2>
  <ul class="group">
    <li class="row">
      <span class="label">Пол</span>
      <div class="segmented" role="radiogroup" aria-label="Пол">
        <button role="radio" aria-checked={sex === "male"} class:on={sex === "male"} onclick={() => pick((v) => (sex = v), "male" as Sex)}>Мужской</button>
        <button role="radio" aria-checked={sex === "female"} class:on={sex === "female"} onclick={() => pick((v) => (sex = v), "female" as Sex)}>Женский</button>
      </div>
    </li>
    <li class="row">
      <label for="year" class="label">Год рождения</label>
      <input id="year" class="num" type="number" inputmode="numeric" placeholder="1995" min={thisYear - limits.age[1]} max={thisYear - limits.age[0]} bind:value={birthYear} />
    </li>
    <li class="row">
      <label for="height" class="label">Рост</label>
      <span class="field"><input id="height" class="num" type="number" inputmode="decimal" placeholder="175" bind:value={heightCm} /><span>см</span></span>
    </li>
    <li class="row">
      <label for="weight" class="label">Вес сейчас</label>
      <span class="field"><input id="weight" class="num" type="number" inputmode="decimal" step="0.1" placeholder="80" bind:value={weightKg} /><span>кг</span></span>
    </li>
    <li class="row">
      <label for="goal" class="label">Желаемый вес</label>
      <span class="field"><input id="goal" class="num" type="number" inputmode="decimal" step="0.1" placeholder="75" bind:value={goalWeightKg} /><span>кг</span></span>
    </li>
  </ul>
  <p class="foot">Пол и возраст нужны формуле: обмен веществ у них заметно разный.</p>

  <h2 class="section-title"><span>Активность</span></h2>
  <ul class="group" role="radiogroup" aria-label="Активность">
    {#each activities as a (a.key)}
      <li>
        <button class="choice" role="radio" aria-checked={activity === a.key} onclick={() => pick((v) => (activity = v), a.key)}>
          <span class="text">
            <span>{a.label}</span>
            <span class="sub">{a.hint}</span>
          </span>
          {#if activity === a.key}<span class="tick"><Icon name="check" /></span>{/if}
        </button>
      </li>
    {/each}
  </ul>

  {#if dir === "lose" || dir === "gain"}
    <h2 class="section-title"><span>{dir === "lose" ? "Худеть" : "Набирать"} в неделю</span></h2>
    <div class="paces" role="radiogroup" aria-label="Темп">
      {#each paces as p (p)}
        <button role="radio" aria-checked={pace === p} class:on={pace === p} onclick={() => pick((v) => (pace = v), p)}>
          <b class="num">{p.toLocaleString("ru-RU")}</b> кг
        </button>
      {/each}
    </div>
    <p class="foot">
      {dir === "lose" ? "0,5 кг в неделю — устойчивый темп; быстрее обычно уходит мышечная масса." : "Больше 0,5 кг в неделю набирается в основном жир."}
    </p>
  {/if}

  <h2 class="section-title"><span>Сообщения от бота</span></h2>
  <ul class="group">
    <li class="row">
      <span class="text">
        <span>Итог недели</span>
        <span class="sub">По воскресеньям: среднее за неделю и совет</span>
      </span>
      {#if reminders.summaryAt}
        <input class="num clock" type="time" bind:value={summaryTime} onchange={() => saveReminders({ summaryAt: summaryTime })} aria-label="Время итога недели" />
      {/if}
      <button
        class="switch"
        role="switch"
        aria-checked={Boolean(reminders.summaryAt)}
        aria-label="Итог недели"
        onclick={() => saveReminders({ summaryAt: reminders.summaryAt ? "" : summaryTime })}
      ><i></i></button>
    </li>
    <li class="row">
      <span class="text">
        <span>Напомнить записать</span>
        <span class="sub">В 14:00, если за день ничего нет</span>
      </span>
      <button class="switch" role="switch" aria-checked={reminders.nudge} aria-label="Напомнить записать" onclick={() => saveReminders({ nudge: !reminders.nudge })}><i></i></button>
    </li>
  </ul>
  <p class="foot">
    {reminderError || `По твоему времени — ${zone.replace("_", " ")}.`}
  </p>

  <section class="card result" aria-live="polite">
    {#if result}
      <p class="lead"><b class="round num">{fmtKcal(result.kcal)}</b> ккал в день</p>
      <dl class="macros num">
        <div><dt><i class="dot protein"></i>Белки</dt><dd>{result.protein} г</dd></div>
        <div><dt><i class="dot fat"></i>Жиры</dt><dd>{result.fat} г</dd></div>
        <div><dt><i class="dot carbs"></i>Углеводы</dt><dd>{result.carbs} г</dd></div>
      </dl>
      <ul class="facts num">
        <li><span>Обмен в покое</span><span>{fmtKcal(result.bmr)} ккал</span></li>
        <li><span>С учётом активности</span><span>{fmtKcal(result.tdee)} ккал</span></li>
        {#if result.direction !== "hold"}
          <li><span>{result.direction === "lose" ? "Дефицит" : "Профицит"}</span><span>{fmtKcal(Math.abs(result.kcal - result.tdee))} ккал в день</span></li>
          <li><span>Темп</span><span>{signed(Math.round(result.weeklyKg * 100) / 100)} кг в неделю</span></li>
          {#if goalDate}
            <li><span>До цели</span><span>~{result.weeks} нед., к {goalDate}</span></li>
          {/if}
        {:else}
          <li><span>Цель</span><span>держать вес</span></li>
        {/if}
      </ul>
      {#if result.limitedBy === "deficit"}
        <p class="warn">Выбранный темп требует слишком большого дефицита — ограничил его четвертью от расхода.</p>
      {:else if result.limitedBy === "floor"}
        <p class="warn">Ниже {sex === "male" ? "1 500" : "1 200"} ккал в день без врача лучше не опускаться — поставил этот минимум.</p>
      {:else if result.limitedBy === "surplus"}
        <p class="warn">Профицит ограничен 15% от расхода, чтобы набор шёл без лишнего жира.</p>
      {/if}
    {:else}
      <p class="empty">Заполни всё выше — норма посчитается сразу.</p>
    {/if}
  </section>
  <p class="foot">
    Оценка по формуле Миффлина — Сан-Жеора, точность около ±10%. Через 2–3 недели сравни с весами и поправь вес в профиле.
  </p>

  <div class="dock">
    {#if error}<p class="error">{error}</p>{/if}
    <button class="primary" onclick={save} disabled={busy || !result}>{busy ? "Сохраняю…" : "Сохранить"}</button>
  </div>

  <WeightLog
    {profile}
    onChanged={(kg) => {
      // The form copied the profile once; keep its weight in step, or
      // saving the form would put the old weight back.
      if (kg !== null) weightKg = kg;
      onWeighed();
    }}
  />
</div>

<style>
  .screen {
    display: flex;
    flex-direction: column;
  }

  h1 {
    font-size: 22px;
    font-weight: 700;
    letter-spacing: -0.01em;
    margin: 8px 0 18px;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    min-height: 50px;
    padding: 6px 12px 6px 16px;
  }

  .label {
    flex: none;
  }

  .row input {
    width: 96px;
    text-align: right;
    background: transparent;
    padding: 8px 4px;
    font-weight: 500;
  }

  .row input:focus {
    background: var(--field);
  }

  .field {
    display: flex;
    align-items: baseline;
    gap: 4px;
    color: var(--hint);
  }

  .field input {
    width: 72px;
    color: var(--text);
  }

  .segmented {
    display: flex;
    padding: 2px;
    border-radius: 10px;
    background: var(--field);
  }

  .segmented button {
    min-height: 32px;
    padding: 0 12px;
    border-radius: 8px;
    font-size: 15px;
    color: var(--hint);
  }

  .segmented .on {
    background: var(--card);
    color: var(--text);
    font-weight: 500;
    box-shadow: 0 1px 3px rgb(0 0 0 / 0.12);
  }

  .foot {
    margin: 6px 16px 22px;
    font-size: 13px;
    color: var(--hint);
  }

  .clock {
    width: 84px;
    padding: 6px 8px;
    text-align: center;
  }

  .clock::-webkit-calendar-picker-indicator {
    display: none;
  }

  .switch {
    position: relative;
    flex: none;
    width: 51px;
    height: 31px;
    border-radius: 16px;
    background: var(--line-strong);
    transition: background-color 0.2s;
  }

  .switch i {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 27px;
    height: 27px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 2px 4px rgb(0 0 0 / 0.2);
    transition: transform 0.2s var(--ease-out);
  }

  .switch[aria-checked="true"] {
    background: var(--ok);
  }

  .switch[aria-checked="true"] i {
    transform: translateX(20px);
  }

  .choice {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    min-height: 56px;
    padding: 8px 16px;
    text-align: left;
  }

  .choice:active {
    background: var(--field);
  }

  .text {
    flex: 1;
    display: grid;
    gap: 2px;
  }

  .sub {
    font-size: 13px;
    color: var(--hint);
  }

  .tick {
    color: var(--accent);
  }

  .group + .section-title {
    margin-top: 22px;
  }

  .paces {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(0, 1fr));
    gap: 6px;
  }

  .paces button {
    min-height: 44px;
    border-radius: var(--r-control);
    background: var(--card);
    color: var(--hint);
  }

  .paces b {
    color: var(--text);
    font-weight: 600;
  }

  .paces .on {
    box-shadow: inset 0 0 0 2px var(--accent);
  }

  .result {
    display: grid;
    gap: 14px;
  }

  .result p {
    margin: 0;
  }

  .lead {
    color: var(--hint);
  }

  .lead b {
    font-size: 34px;
    font-weight: 700;
    letter-spacing: -0.02em;
    color: var(--text);
    margin-right: 4px;
  }

  .macros {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    margin: 0;
  }

  .macros > div + div {
    border-left: 1px solid var(--line);
    padding-left: 12px;
  }

  dt {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--hint);
  }

  dd {
    margin: 2px 0 0;
    font-size: 17px;
    font-weight: 600;
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  .protein {
    background: var(--protein);
  }
  .fat {
    background: var(--fat);
  }
  .carbs {
    background: var(--carbs);
  }

  .facts {
    list-style: none;
    margin: 0;
    padding: 12px 0 0;
    border-top: 1px solid var(--line);
    display: grid;
    gap: 6px;
    font-size: 14px;
  }

  .facts li {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }

  .facts li span:first-child {
    color: var(--hint);
  }

  .facts li span:last-child {
    text-align: right;
  }

  .warn {
    font-size: 13px;
    color: var(--warn);
  }

  .empty {
    color: var(--hint);
    text-align: center;
    padding: 8px 0;
  }

  .dock {
    margin-top: auto;
  }

  .dock p {
    margin: 0;
  }
</style>
