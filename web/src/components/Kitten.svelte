<script lang="ts">
  // The KalorieKitten mark: a line-drawn cat head. `peek` turns the eyes
  // sideways for the "looking at your plate" moment.
  let { size = 56, peek = false }: { size?: number; peek?: boolean } = $props();
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 64 64"
  fill="none"
  stroke="currentColor"
  stroke-width="2.6"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
  class:peek
>
  <path d="M14 27 12.5 10.5 25 18.5c2.2-.6 4.5-.9 7-.9s4.8.3 7 .9l12.5-8L50 27c2.6 3.4 4 7.2 4 11.2C54 48.6 44.2 55 32 55S10 48.6 10 38.2c0-4 1.4-7.8 4-11.2Z" />
  <g class="eyes">
    <path d="M23.5 35.5v2.5M40.5 35.5v2.5" stroke-width="3.6" />
  </g>
  <path d="M30 43.2h4l-2 2.2-2-2.2ZM32 45.4v1.4c0 1.3-1 2.2-2.4 2.2M32 46.8c0 1.3 1 2.2 2.4 2.2" stroke-width="2" />
  <path d="M8 40.5l9-1.2M8.8 46.5l8.6-3.4M56 40.5l-9-1.2M55.2 46.5l-8.6-3.4" stroke-width="1.8" opacity="0.55" />
</svg>

<style>
  svg {
    flex: none;
    display: block;
  }

  .eyes {
    transition: transform 0.5s var(--ease-out);
  }

  .peek .eyes {
    animation: peek 2.4s var(--ease-out) infinite;
  }

  @keyframes peek {
    0%,
    100% {
      transform: translateX(0);
    }
    30%,
    45% {
      transform: translateX(-2.5px);
    }
    65%,
    80% {
      transform: translateX(2.5px);
    }
  }
</style>
