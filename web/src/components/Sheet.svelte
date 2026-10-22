<script lang="ts">
  import { onMount, type Snippet } from "svelte";
  import { prefersReducedMotion } from "svelte/motion";

  // A screen that grows out of the thing that opened it — a meal's row, the
  // Add button — and shrinks back into it on close, so it is clear where it
  // came from and where it went. What is under it stays mounted and keeps
  // its scroll. Without a place to grow from it rises from below.
  let { from, children }: { from: Element | null; children: Snippet } = $props();

  let sheet = $state<HTMLDivElement>();
  let content = $state<HTMLDivElement>();

  // iOS's own curve for sheets and zooms: quick out of the gate, long settle.
  const curve = "cubic-bezier(0.32, 0.72, 0, 1)";
  const full = "inset(0px 0px 0px 0px round 0px)";

  /** The clip that shows only `rect` of the viewport, rounded like a card. */
  function clipTo(rect: DOMRect): string {
    const right = innerWidth - rect.right;
    const bottom = innerHeight - rect.bottom;
    return `inset(${rect.top}px ${right}px ${bottom}px ${rect.left}px round 14px)`;
  }

  /** A theme colour as the browser resolved it, for animating to it. */
  function resolved(token: string): string {
    const probe = document.createElement("i");
    probe.style.cssText = `position:absolute;visibility:hidden;background:var(${token})`;
    document.body.append(probe);
    const color = getComputedStyle(probe).backgroundColor;
    probe.remove();
    return color;
  }

  /** Where an element is and what it looks like, if it is worth growing
   *  from: on screen and not degenerate. A see-through row takes the card's
   *  colour it sits on. */
  function shape(el: Element | null): { rect: DOMRect; color: string } | null {
    if (!el?.isConnected) return null;
    const rect = el.getBoundingClientRect();
    if (rect.width < 8 || rect.height < 8 || rect.bottom < 0 || rect.top > innerHeight) return null;
    let color = getComputedStyle(el).backgroundColor;
    if (color === "transparent" || color === "rgba(0, 0, 0, 0)") color = resolved("--card");
    return { rect, color };
  }

  onMount(() => {
    if (!sheet || !content) return;
    if (prefersReducedMotion.current) {
      sheet.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 120 });
      return;
    }
    const origin = shape(from);
    if (!origin) {
      sheet.animate([{ transform: "translateY(24px)", opacity: 0 }, { transform: "none", opacity: 1 }], {
        duration: 300,
        easing: curve,
      });
      return;
    }
    // The card opens to full screen, turning from the row's white into the
    // screen's grey, while what is inside slides from the row's height to the
    // top — the row seems to become the screen.
    sheet.animate(
      [
        { clipPath: clipTo(origin.rect), backgroundColor: origin.color },
        { clipPath: full, backgroundColor: resolved("--bg") },
      ],
      { duration: 440, easing: curve },
    );
    content.animate(
      [
        { transform: `translateY(${origin.rect.top}px) scale(0.96)`, opacity: 0 },
        { opacity: 1, offset: 0.45 },
        { transform: "none", opacity: 1 },
      ],
      { duration: 440, easing: curve },
    );
  });

  /** Shrinks into `to` (or sinks away without one); resolves when done. */
  export async function close(to: Element | null): Promise<void> {
    if (!sheet || !content) return;
    const options = { duration: 320, easing: curve, fill: "forwards" as const };
    if (prefersReducedMotion.current) {
      await sheet.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 100, fill: "forwards" }).finished;
      return;
    }
    const target = shape(to);
    if (!target) {
      await sheet.animate([{ transform: "none", opacity: 1 }, { transform: "translateY(32px)", opacity: 0 }], {
        ...options,
        duration: 240,
      }).finished;
      return;
    }
    // The content is scrolled up by scrollTop; bring its top down to the row.
    const top = target.rect.top + sheet.scrollTop;
    content.animate(
      [
        { transform: "none", opacity: 1 },
        { opacity: 0, offset: 0.5 },
        { transform: `translateY(${top}px) scale(0.96)`, opacity: 0 },
      ],
      options,
    );
    await sheet.animate(
      [
        { clipPath: full, backgroundColor: resolved("--bg") },
        { clipPath: clipTo(target.rect), backgroundColor: target.color },
      ],
      options,
    ).finished;
  }
</script>

<div class="sheet" bind:this={sheet}>
  <div class="content" bind:this={content}>
    {@render children()}
  </div>
</div>

<style>
  .sheet {
    position: fixed;
    inset: 0;
    z-index: 8;
    overflow-y: auto;
    overscroll-behavior: contain;
    background: var(--bg);
    -webkit-overflow-scrolling: touch;
  }

  /* The same light and grain as the screen under it, fixed to the view. */
  .sheet::before,
  .sheet::after {
    content: "";
    position: fixed;
    inset: 0;
    pointer-events: none;
  }

  .sheet::before {
    background: var(--glow);
    opacity: var(--glow-opacity, 0.62);
  }

  :global(:root[data-bg="data"]) .sheet::before {
    opacity: calc(var(--glow-opacity, 0.62) * var(--day-glow, 1));
  }

  .sheet::after {
    background-image: var(--grain-image, none);
    background-size: 160px 160px;
    opacity: var(--grain-opacity, 0.06);
    mix-blend-mode: var(--grain-blend, soft-light);
  }

  .content {
    position: relative;
    z-index: 1;
    max-width: 560px;
    min-height: 100%;
    margin: 0 auto;
    padding: 8px var(--gutter) 0;
    display: flex;
    flex-direction: column;
    transform-origin: top center;
  }

  .content > :global(:last-child) {
    flex: 1;
  }
</style>
