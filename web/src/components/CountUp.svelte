<script lang="ts">
  import { untrack } from "svelte";
  import { Tween } from "svelte/motion";
  import { easeOut, ms } from "../lib/motion";

  // A number that counts from where it was to where it is: from `from` when
  // it first shows (the figure the screen showed last time), then from each
  // value to the next.
  let {
    value,
    from,
    format = (v: number) => String(Math.round(v)),
  }: { value: number; from?: number; format?: (v: number) => string } = $props();

  const shown = new Tween(untrack(() => from ?? value), { easing: easeOut });
  $effect(() => {
    const target = value;
    untrack(() => shown.set(target, { duration: ms(Math.min(900, 400 + Math.abs(target - shown.current) / 4)) }));
  });
</script>

{format(shown.current)}
