<script lang="ts">
  import { onDestroy } from "svelte";

  // A green-screen video drawn with the green made transparent. The video
  // element plays (and sounds) out of sight; WebGL copies each frame to the
  // canvas through a chroma-key shader. Without WebGL the plain video shows.
  let {
    src,
    muted = $bindable(false),
    onPlaying,
  }: { src: string; muted?: boolean; onPlaying?: () => void } = $props();

  let video = $state<HTMLVideoElement>();
  let canvas = $state<HTMLCanvasElement>();
  let keyed = $state(true);
  let stop = () => {};

  // Green screen: how far green rises above the other channels decides the
  // alpha; what is left of green on the fur's edge is pulled down (despill).
  const fragment = `
    precision mediump float;
    varying vec2 uv;
    uniform sampler2D frame;
    void main() {
      vec3 c = texture2D(frame, uv).rgb;
      float green = c.g - max(c.r, c.b);
      float alpha = 1.0 - smoothstep(0.12, 0.32, green);
      c.g = min(c.g, max(c.r, c.b) + 0.03);
      gl_FragColor = vec4(c * alpha, alpha);
    }`;
  const vertex = `
    attribute vec2 corner;
    varying vec2 uv;
    void main() {
      uv = vec2(corner.x, -corner.y) * 0.5 + 0.5;
      gl_Position = vec4(corner, 0.0, 1.0);
    }`;

  function setup(el: HTMLVideoElement, target: HTMLCanvasElement): (() => void) | null {
    const gl = target.getContext("webgl", { premultipliedAlpha: true, alpha: true });
    if (!gl) return null;
    const program = gl.createProgram()!;
    for (const [type, source] of [
      [gl.VERTEX_SHADER, vertex],
      [gl.FRAGMENT_SHADER, fragment],
    ] as const) {
      const shader = gl.createShader(type)!;
      gl.shaderSource(shader, source);
      gl.compileShader(shader);
      gl.attachShader(program, shader);
    }
    gl.linkProgram(program);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return null;
    gl.useProgram(program);

    gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer());
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW);
    const corner = gl.getAttribLocation(program, "corner");
    gl.enableVertexAttribArray(corner);
    gl.vertexAttribPointer(corner, 2, gl.FLOAT, false, 0, 0);

    gl.bindTexture(gl.TEXTURE_2D, gl.createTexture());
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.clearColor(0, 0, 0, 0);

    let alive = true;
    const draw = () => {
      if (!alive) return;
      if (el.readyState >= 2 && el.videoWidth) {
        if (target.width !== el.videoWidth) {
          target.width = el.videoWidth;
          target.height = el.videoHeight;
          gl.viewport(0, 0, target.width, target.height);
        }
        gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGB, gl.RGB, gl.UNSIGNED_BYTE, el);
        gl.clear(gl.COLOR_BUFFER_BIT);
        gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
      }
      next();
    };
    // Draw once per video frame where the browser says when that is.
    const next = () =>
      "requestVideoFrameCallback" in el ? el.requestVideoFrameCallback(draw) : requestAnimationFrame(draw);
    next();
    return () => {
      alive = false;
      gl.getExtension("WEBGL_lose_context")?.loseContext();
    };
  }

  $effect(() => {
    const el = video;
    const target = canvas;
    if (!el || !target) return;
    const teardown = setup(el, target);
    keyed = teardown !== null;
    stop = teardown ?? (() => {});
    return () => stop();
  });

  // Songs need sound, so try with it; a browser that refuses autoplay with
  // sound plays muted and the parent offers a button.
  $effect(() => {
    const el = video;
    if (!el || !src) return;
    el.muted = false;
    el.play().then(
      () => onPlaying?.(),
      () => {
        el.muted = muted = true;
        el.play().then(() => onPlaying?.(), () => {});
      },
    );
  });

  // The parent turns the sound on from its own button.
  $effect(() => {
    const el = video;
    if (el && !muted && el.muted) {
      el.muted = false;
      el.play().catch(() => {});
    }
  });

  onDestroy(() => stop());
</script>

<!-- svelte-ignore a11y_media_has_caption -->
<video bind:this={video} {src} loop playsinline preload="auto" class:hidden={keyed}></video>
<canvas bind:this={canvas} class:hidden={!keyed} aria-hidden="true"></canvas>

<style>
  video,
  canvas {
    display: block;
    width: 100%;
    max-height: 64dvh;
    object-fit: contain;
  }

  video:not(.hidden) {
    border-radius: 22px;
  }

  /* Out of sight but still playing: display:none would stop iOS decoding
     the frames the canvas needs. */
  .hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    opacity: 0;
    pointer-events: none;
  }

  canvas.hidden {
    display: none;
  }
</style>
