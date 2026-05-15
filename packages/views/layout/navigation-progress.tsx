"use client";

import { useIsNavigating } from "../navigation";

// 2px top-of-content progress bar shown while a transition-wrapped
// push/replace is mid-flight. Indeterminate by design — we don't know
// when the next route will commit, just that it's coming. Always mounted
// so the container can fade out (200ms) instead of disappearing in one
// frame, which previously felt abrupt.
export function NavigationProgress() {
  const isNavigating = useIsNavigating();
  return (
    <div
      aria-hidden
      data-visible={isNavigating ? "true" : "false"}
      className="pointer-events-none absolute inset-x-0 top-0 z-50 h-0.5 overflow-hidden opacity-0 transition-opacity duration-200 data-[visible=true]:opacity-100"
    >
      <div
        className="h-full w-1/3 animate-nav-progress-sweep bg-brand"
        style={{
          boxShadow:
            "0 0 8px color-mix(in oklab, var(--brand) 60%, transparent), 0 0 2px color-mix(in oklab, var(--brand) 80%, transparent)",
        }}
      />
    </div>
  );
}
