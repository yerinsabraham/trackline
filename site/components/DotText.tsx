"use client";

import { useEffect, useRef, useState } from "react";

type Props = {
  text: string;
  /** Largest font size in px; the text otherwise fits the container's width. */
  max: number;
  /** Grid spacing as a fraction of the font size. Smaller means more, finer dots. */
  pitch: number;
  color: string;
  /** Colour of the unlit cells, for an LED-board look. Omit for no board. */
  board?: string;
  align?: "center" | "left";
  interactive?: boolean;
  /** Every few seconds a cluster drifts off the line, turns red, and is pulled back. */
  drift?: boolean;
  /** Wait until the text is on screen before the dots settle in. */
  introOnView?: boolean;
  fallbackClass?: string;
};

type Dot = { x: number; y: number; ox: number; oy: number; tx: number; ty: number; hot: boolean; delay: number };

const SIGNAL = "#ff4a1c";

// The text is rasterised off-screen, sampled on a square grid, and every filled
// cell becomes a round dot, so each stroke is several rows of dots. No font does
// this, which is why it is drawn rather than set.
export default function DotText({
  text, max, pitch: pitchRatio, color, board, align = "center",
  interactive = false, drift = false, introOnView = false, fallbackClass,
}: Props) {
  const wrapRef = useRef<HTMLSpanElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const fallbackRef = useRef<HTMLSpanElement>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const wrap = wrapRef.current!, canvas = canvasRef.current!, fallback = fallbackRef.current!;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    let dots: Dot[] = [], unlit: number[] = [];
    let W = 0, H = 0, fs = 0, r = 0;
    let running = false, last = 0, visible = true, disposed = false;
    let mouse: { x: number; y: number } | null = null;
    const timers: number[] = [];
    const cleanups: (() => void)[] = [];
    const family = getComputedStyle(fallback).fontFamily;
    const tracking = -0.03;

    function layout() {
      W = wrap.clientWidth;
      if (!W) return false;
      const m = document.createElement("canvas").getContext("2d")!;
      m.font = `800 100px ${family}`;
      if ("letterSpacing" in m) m.letterSpacing = `${tracking * 100}px`;
      fs = Math.min(max, ((W * 0.98) / m.measureText(text).width) * 100);
      const pitch = Math.max(3, fs * pitchRatio);
      r = pitch * 0.4;
      H = Math.ceil(fs * 1.12);
      canvas.style.height = `${H}px`;
      canvas.width = Math.round(W * dpr);
      canvas.height = Math.round(H * dpr);
      ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);

      const off = document.createElement("canvas");
      off.width = Math.ceil(W);
      off.height = H;
      const oc = off.getContext("2d", { willReadFrequently: true })!;
      oc.font = `800 ${fs}px ${family}`;
      if ("letterSpacing" in oc) oc.letterSpacing = `${tracking * fs}px`;
      oc.textAlign = align;
      oc.fillText(text, align === "center" ? W / 2 : 1, fs * 0.86);
      const data = oc.getImageData(0, 0, off.width, off.height).data;
      dots = [];
      unlit = [];
      for (let y = pitch / 2; y < H; y += pitch) {
        for (let x = pitch / 2; x < W; x += pitch) {
          if (data[(Math.round(y) * off.width + Math.round(x)) * 4 + 3] > 110) {
            dots.push({ x, y, ox: 0, oy: 0, tx: 0, ty: 0, hot: false, delay: 0 });
          } else if (board) unlit.push(x, y);
        }
      }
      return true;
    }

    function draw() {
      ctx!.clearRect(0, 0, W, H);
      if (board) {
        ctx!.fillStyle = board;
        ctx!.beginPath();
        for (let i = 0; i < unlit.length; i += 2) {
          ctx!.moveTo(unlit[i] + r, unlit[i + 1]);
          ctx!.arc(unlit[i], unlit[i + 1], r, 0, Math.PI * 2);
        }
        ctx!.fill();
      }
      for (const hot of [false, true]) {
        ctx!.fillStyle = hot ? SIGNAL : color;
        ctx!.beginPath();
        for (const d of dots) {
          if (d.hot !== hot) continue;
          const x = d.x + d.ox, y = d.y + d.oy;
          ctx!.moveTo(x + r, y);
          ctx!.arc(x, y, r, 0, Math.PI * 2);
        }
        ctx!.fill();
      }
    }

    function frame(t: number) {
      if (disposed) return;
      const dt = Math.min(48, t - last);
      last = t;
      const k = 1 - Math.pow(0.88, dt / 16.7);
      let moving = false;
      for (const d of dots) {
        if (d.delay > 0) { d.delay -= dt; moving = true; continue; }
        let tx = d.tx, ty = d.ty;
        if (mouse) {
          const dx = d.x - mouse.x, dy = d.y - mouse.y, dist = Math.hypot(dx, dy), R = fs * 0.7;
          if (dist < R && dist > 0.01) {
            const f = (1 - dist / R) ** 2 * fs * 0.28;
            tx += (dx / dist) * f;
            ty += (dy / dist) * f;
          }
        }
        d.ox += (tx - d.ox) * k;
        d.oy += (ty - d.oy) * k;
        if (Math.abs(d.ox - tx) > 0.05 || Math.abs(d.oy - ty) > 0.05) moving = true;
      }
      draw();
      if (moving || mouse) requestAnimationFrame(frame);
      else running = false;
    }

    function kick() {
      if (reduce) return draw();
      if (!running) { running = true; last = performance.now(); requestAnimationFrame(frame); }
    }

    // Dots arrive from off the line and settle onto it, left to right.
    function intro() {
      if (reduce) return draw();
      for (const d of dots) {
        d.ox = (Math.random() - 0.5) * fs * 1.4;
        d.oy = (Math.random() - 0.5) * fs * 1.1;
        d.delay = (d.x / W) * 520 + Math.random() * 160;
      }
      kick();
    }

    function driftOnce() {
      if (reduce || !visible || document.hidden || !dots.length) return;
      const c = dots[(Math.random() * dots.length) | 0];
      const near = dots.filter((d) => Math.hypot(d.x - c.x, d.y - c.y) < fs * 0.26);
      const dx = fs * (Math.random() < 0.5 ? -0.06 : 0.06), dy = fs * 0.2;
      near.forEach((d) => { d.tx = dx; d.ty = dy; d.hot = true; });
      kick();
      timers.push(window.setTimeout(() => { near.forEach((d) => { d.tx = 0; d.ty = 0; }); kick(); }, 1100));
      timers.push(window.setTimeout(() => { near.forEach((d) => { d.hot = false; }); draw(); }, 1700));
    }

    const start = () => {
      if (disposed || !layout()) return;
      setReady(true);
      draw();
      let lastW = W;

      if (introOnView) {
        const io = new IntersectionObserver((es) => {
          if (es[0].isIntersecting) { io.disconnect(); intro(); }
        }, { threshold: 0.4 });
        io.observe(wrap);
        cleanups.push(() => io.disconnect());
      } else intro();

      const ro = new ResizeObserver(() => {
        if (Math.abs(wrap.clientWidth - lastW) > 1) { layout(); lastW = W; draw(); }
      });
      ro.observe(wrap);
      const vo = new IntersectionObserver((es) => { visible = es[0].isIntersecting; });
      vo.observe(wrap);
      cleanups.push(() => ro.disconnect(), () => vo.disconnect());

      if (drift) {
        const id = window.setInterval(driftOnce, 4200);
        cleanups.push(() => clearInterval(id));
      }
      if (interactive && !reduce) {
        const move = (e: PointerEvent) => {
          const b = canvas.getBoundingClientRect();
          mouse = { x: e.clientX - b.left, y: e.clientY - b.top };
          kick();
        };
        const leave = () => { mouse = null; kick(); };
        wrap.addEventListener("pointermove", move);
        wrap.addEventListener("pointerleave", leave);
        cleanups.push(() => { wrap.removeEventListener("pointermove", move); wrap.removeEventListener("pointerleave", leave); });
      }
    };

    // Sampling before the face has loaded would trace the fallback font.
    document.fonts.load(`800 100px ${family}`).then(start, start);

    return () => {
      disposed = true;
      timers.forEach(clearTimeout);
      cleanups.forEach((f) => f());
    };
  }, [text, max, pitchRatio, color, board, align, interactive, drift, introOnView]);

  return (
    <span ref={wrapRef} className={ready ? "dotwrap ready" : "dotwrap"}>
      <span ref={fallbackRef} className={fallbackClass ? `dot-fallback ${fallbackClass}` : "dot-fallback"}>{text}</span>
      <canvas ref={canvasRef} aria-hidden="true" />
    </span>
  );
}
