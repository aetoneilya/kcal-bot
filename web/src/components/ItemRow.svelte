<script lang="ts">
  import type { Item } from "../lib/api";
  import { grams, kcal, lineTotals } from "../lib/nutrition";
  import Icon from "./Icon.svelte";

  let { item = $bindable(), onRemove }: { item: Item; onRemove: () => void } = $props();

  // Density is usually right and rarely touched, so it hides behind a tap.
  let open = $state(false);
  const t = $derived(lineTotals(item));
  const unsure = $derived(item.confidence === "low");

  function step(delta: number) {
    item.grams = Math.max(0, Math.round((Number(item.grams) || 0) + delta));
  }
</script>

<div class="item">
  <div class="top">
    <input class="name" bind:value={item.name} placeholder="Продукт" maxlength="80" aria-label="Продукт" />
    <button class="remove" onclick={onRemove} aria-label="Убрать продукт"><Icon name="x" size={18} /></button>
  </div>

  <div class="weight">
    <div class="stepper" class:unsure>
      <button onclick={() => step(-10)} aria-label="Минус 10 грамм"><Icon name="minus" size={18} /></button>
      <label>
        <input class="num" type="number" inputmode="decimal" min="0" max="5000" bind:value={item.grams} aria-label="Вес, г" />
        <span>г</span>
      </label>
      <button onclick={() => step(10)} aria-label="Плюс 10 грамм"><Icon name="plus" size={18} /></button>
    </div>
    <div class="line num"><b class="round">{kcal(t.kcal)}</b> ккал</div>
  </div>

  {#if unsure}
    <p class="flag">Вес на глаз — проверь</p>
  {/if}

  <button class="details num" onclick={() => (open = !open)} aria-expanded={open}>
    <span><i class="dot protein"></i>{grams(t.protein)}</span>
    <span><i class="dot fat"></i>{grams(t.fat)}</span>
    <span><i class="dot carbs"></i>{grams(t.carbs)}</span>
    <span class="more">на 100 г <span class="chev" class:open><Icon name="chevron-down" size={16} /></span></span>
  </button>

  {#if open}
    <div class="per100">
      <label>ккал<input class="num" type="number" inputmode="decimal" min="0" max="900" bind:value={item.kcal100} /></label>
      <label>белки<input class="num" type="number" inputmode="decimal" min="0" max="100" bind:value={item.protein100} /></label>
      <label>жиры<input class="num" type="number" inputmode="decimal" min="0" max="100" bind:value={item.fat100} /></label>
      <label>углев.<input class="num" type="number" inputmode="decimal" min="0" max="100" bind:value={item.carbs100} /></label>
    </div>
  {/if}
</div>

<style>
  .item {
    display: grid;
    gap: 8px;
    padding: 10px 12px 10px 16px;
  }

  .top {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  /* Reads as text until touched. */
  .name {
    font-weight: 600;
    font-size: 17px;
    text-overflow: ellipsis;
    background: transparent;
    padding: 6px 8px;
    margin-left: -8px;
  }

  .name:focus {
    background: var(--field);
  }

  .remove {
    width: 36px;
    height: 36px;
    display: grid;
    place-items: center;
    flex: none;
    border-radius: 50%;
    color: var(--hint);
  }

  .remove:active {
    background: var(--field);
  }

  .weight {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .stepper {
    display: flex;
    align-items: center;
    height: 40px;
    border-radius: var(--r-control);
    background: var(--field);
    box-shadow: inset 0 0 0 1px transparent;
  }

  .stepper.unsure {
    box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--warn) 70%, transparent);
  }

  .stepper button {
    width: 44px;
    height: 40px;
    display: grid;
    place-items: center;
    color: var(--accent);
    border-radius: var(--r-control);
  }

  .stepper button:active {
    background: var(--field);
  }

  .stepper label {
    display: flex;
    align-items: baseline;
    gap: 2px;
    width: 76px;
    justify-content: center;
  }

  .stepper input {
    width: 52px;
    padding: 0;
    background: transparent;
    border: 0;
    text-align: right;
    font-size: 17px;
    font-weight: 600;
  }

  .stepper span {
    color: var(--hint);
    font-size: 15px;
  }

  .line {
    margin-left: auto;
    font-size: 13px;
    color: var(--hint);
  }

  .line b {
    font-size: 17px;
    font-weight: 600;
    color: var(--text);
  }

  .flag {
    margin: -2px 0 0;
    font-size: 13px;
    color: var(--warn);
  }

  .details {
    display: flex;
    align-items: center;
    gap: 14px;
    min-height: 32px;
    font-size: 13px;
    color: var(--hint);
    text-align: left;
  }

  .details > span {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }

  .dot {
    width: 7px;
    height: 7px;
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

  .more {
    margin-left: auto;
    gap: 2px !important;
  }

  .chev {
    display: inline-flex;
    transition: transform 0.2s var(--ease-out);
  }

  .chev.open {
    transform: rotate(180deg);
  }

  .per100 {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 6px;
    padding-bottom: 4px;
  }

  .per100 label {
    display: grid;
    gap: 3px;
    font-size: 12px;
    color: var(--hint);
    text-align: center;
  }

  .per100 input {
    color: var(--text);
    padding: 8px 4px;
    text-align: center;
  }
</style>
