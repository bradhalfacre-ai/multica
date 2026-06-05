"use client";

// Shared scale frame for the value-section micro-demos (#2–#4). Each demo lays
// out at a fixed natural size, then this frame scales it down by the shared
// DEMO_ZOOM so every value card's demo matches the hero board's on-screen scale
// and the cards line up at one height. Value #1 (the board) carries its own
// frame because it also needs providers; the natural size is kept identical
// here so all four cards are the same height.

import { DEMO_ZOOM } from "./zoom";

// Default natural width for the content-light demos (#2–#4). Sized so that,
// scaled by DEMO_ZOOM, the panel fits the demo half of the card at the design
// widths (≥1200px container) without bleeding/clipping. Height is per-demo
// (sized to its content) so panels stay snug.
export const VALUE_DEMO_W = 720;

export function ValueDemoFrame({
  width = VALUE_DEMO_W,
  height,
  children,
}: {
  width?: number;
  height: number;
  children: React.ReactNode;
}) {
  return (
    <div
      className="overflow-hidden"
      style={{ width: width * DEMO_ZOOM, height: height * DEMO_ZOOM }}
    >
      <div
        className="origin-top-left"
        style={{ width, height, transform: `scale(${DEMO_ZOOM})` }}
      >
        {children}
      </div>
    </div>
  );
}
