<script lang="ts" module>
  // One outline set at one stroke weight, drawn on a 24px grid.
  const paths = {
    "chevron-left": "M15 18l-6-6 6-6",
    "chevron-right": "M9 18l6-6-6-6",
    "chevron-down": "M6 9l6 6 6-6",
    plus: "M12 5v14M5 12h14",
    minus: "M5 12h14",
    x: "M6 6l12 12M18 6L6 18",
    camera:
      "M3 8.5A2.5 2.5 0 0 1 5.5 6h1.8l1.5-2h6.4l1.5 2h1.8A2.5 2.5 0 0 1 21 8.5v9a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5zM12 16.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7z",
    image: "M5.5 4h13A2.5 2.5 0 0 1 21 6.5v11a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5v-11A2.5 2.5 0 0 1 5.5 4zM3 16l5-5 9 9M14 13l2-2 5 5M15.5 8.5h.01",
    pencil: "M4 20h4L19 9a2.1 2.1 0 0 0-4-4L4 16v4zM13.5 6.5l4 4",
    question: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM9.6 9.2a2.5 2.5 0 0 1 4.8.9c0 1.7-2.4 2.2-2.4 3.6M12 17h.01",
    clock: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 7.5V12l3 2",
    target: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 16.5a4.5 4.5 0 1 0 0-9 4.5 4.5 0 0 0 0 9zM12 12h.01",
    check: "M5 12.5l4.5 4.5L19 7.5",
    barcode: "M4 6v12M7 6v12M10.5 6v12M13 6v12M16.5 6v12M20 6v12",
    list: "M5 4.5h14A1.5 1.5 0 0 1 20.5 6v12a1.5 1.5 0 0 1-1.5 1.5H5A1.5 1.5 0 0 1 3.5 18V6A1.5 1.5 0 0 1 5 4.5zM7.5 9h9M7.5 12h9M7.5 15h5",
    calendar: "M5.5 5h13A2 2 0 0 1 20.5 7v11.5a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2zM3.5 10h17M8 3v4M16 3v4M8 14h.01M12 14h.01M16 14h.01",
    trend: "M3.5 20.5h17M5 16l4.5-4.5 3.5 3 6-7M15 7.5h4v4",
    volume: "M4 9.5h3.5L12 5.5v13l-4.5-4H4zM15.5 9a4 4 0 0 1 0 6M18 6.5a7.5 7.5 0 0 1 0 11",
  } as const;

  export type IconName = keyof typeof paths;
</script>

<script lang="ts">
  let { name, size = 20 }: { name: IconName; size?: number } = $props();
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.8"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  <path d={paths[name]} />
</svg>

<style>
  svg {
    flex: none;
    display: block;
  }
</style>
