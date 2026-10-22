<script lang="ts">
  import Icon, { type IconName } from "./Icon.svelte";
  import { grams, kcal as fmtKcal } from "../lib/nutrition";

  // Macros as tiles: the macro's icon in its colour, the amount and, when
  // there is a goal, a thin bar of how much of it this is. With `energy`
  // a fourth tile shows calories.
  let {
    protein,
    fat,
    carbs,
    energy,
    goals,
    unit = "г",
  }: {
    protein: number;
    fat: number;
    carbs: number;
    energy?: number;
    goals?: { protein: number; fat: number; carbs: number; kcal?: number } | null;
    unit?: string;
  } = $props();

  type Tile = { key: string; label: string; icon: IconName; value: string; goal?: string; share: number | null };
  const share = (v: number, goal?: number) => (goal ? Math.min(v / goal, 1) : null);

  const tiles = $derived.by<Tile[]>(() => {
    const list: Tile[] = [
      { key: "protein", label: "Белки", icon: "drumstick", value: grams(protein), goal: goals ? String(goals.protein) : undefined, share: share(protein, goals?.protein) },
      { key: "fat", label: "Жиры", icon: "drop", value: grams(fat), goal: goals ? String(goals.fat) : undefined, share: share(fat, goals?.fat) },
      { key: "carbs", label: "Углеводы", icon: "wheat", value: grams(carbs), goal: goals ? String(goals.carbs) : undefined, share: share(carbs, goals?.carbs) },
    ];
    if (energy !== undefined) {
      list.push({ key: "kcal", label: "Калории", icon: "flame", value: fmtKcal(energy), share: share(energy, goals?.kcal) });
    }
    return list;
  });
</script>

<dl class="tiles" class:four={tiles.length > 3} style="--n: {tiles.length}">
  {#each tiles as t (t.key)}
    <div class="tile {t.key}">
      <dt>
        <span class="icon"><Icon name={t.icon} size={15} /></span>
        {t.label}
      </dt>
      <dd class="num">
        <b class="round">{t.value}</b><span>{t.goal ? ` / ${t.goal}` : ""} {t.key === "kcal" ? "ккал" : unit}</span>
      </dd>
      {#if t.share !== null}
        <span class="meter" aria-hidden="true"><i style="width: {t.share * 100}%"></i></span>
      {/if}
    </div>
  {/each}
</dl>

<style>
  .tiles {
    display: grid;
    grid-template-columns: repeat(var(--n), minmax(0, 1fr));
    gap: 8px;
    margin: 0;
  }

  .tile {
    display: grid;
    align-content: start;
    gap: 6px;
    min-width: 0;
    padding: 10px 10px 11px;
    border-radius: 14px;
    background: color-mix(in srgb, var(--tint) 9%, var(--field));
    --tint: var(--hint);
  }

  .protein {
    --tint: var(--protein);
  }
  .fat {
    --tint: var(--fat);
  }
  .carbs {
    --tint: var(--carbs);
  }
  .kcal {
    --tint: var(--accent);
  }

  dt {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--hint);
    font-size: 12px;
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .icon {
    flex: none;
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    color: var(--tint);
    background: color-mix(in srgb, var(--tint) 16%, transparent);
  }

  dd {
    margin: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  dd b {
    font-size: 18px;
    font-weight: 700;
  }

  dd span {
    color: var(--hint);
    font-size: 12px;
  }

  /* Four across is narrow: the icon sits over the label, the unit under
     the figure. */
  .four dt {
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
  }

  .four dd b {
    font-size: 16px;
  }

  .four dd span {
    display: block;
  }

  .meter {
    display: block;
    height: 4px;
    border-radius: 2px;
    background: color-mix(in srgb, var(--tint) 14%, transparent);
    overflow: hidden;
  }

  .meter i {
    display: block;
    height: 100%;
    border-radius: 2px;
    background: var(--tint);
    transition: width 0.6s var(--ease-out);
  }
</style>
