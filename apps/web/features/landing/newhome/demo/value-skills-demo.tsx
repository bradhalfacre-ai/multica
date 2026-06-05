"use client";

// Value #4 — "Turn team knowledge into reusable skills". A skills library:
// saved workflows that any agent can run. Auto-plays a highlight cycling across
// the skills, each picked up by a different agent — showing the same skill is
// reusable across the team. Presentational (no providers).

import { useEffect, useState } from "react";
import { cn } from "@multica/ui/lib/utils";
import { AGENTS, SKILLS } from "./mock-data";
import { ValueDemoFrame } from "./value-demo-frame";

export function ValueSkillsDemo() {
  const [active, setActive] = useState(0);
  useEffect(() => {
    const id = window.setInterval(
      () => setActive((n) => (n + 1) % SKILLS.length),
      1600,
    );
    return () => window.clearInterval(id);
  }, []);

  return (
    <ValueDemoFrame height={516}>
      <div className="pointer-events-none flex h-full w-full select-none bg-white">
        <div className="flex h-full w-full flex-col px-9 py-9">
          <div className="flex items-baseline gap-2.5">
            <span className="text-[15px] font-semibold text-[#0a0d12]">Skills</span>
            <span className="text-[12.5px] text-[#0a0d12]/45">
              Reusable workflows · runnable by any agent
            </span>
          </div>

          <div className="mt-5 grid flex-1 grid-cols-2 content-start gap-3">
            {SKILLS.map((s, idx) => {
              const isActive = idx === active;
              const agent = AGENTS[idx % AGENTS.length]!;
              return (
                <div
                  key={s.name}
                  className={cn(
                    "relative rounded-[6px] border bg-white px-4 py-3.5 ring-2 transition-colors duration-300",
                    isActive
                      ? "border-[var(--brand)]/40 ring-[var(--brand)]/20"
                      : "border-[#0a0d12]/8 ring-transparent",
                  )}
                >
                  <div className="flex items-center gap-2">
                    <span className="flex size-6 shrink-0 items-center justify-center rounded-[5px] bg-[var(--brand)]/10 text-[var(--brand)]">
                      <PlaySquare />
                    </span>
                    <span className="truncate text-[13.5px] font-semibold text-[#0a0d12]">
                      {s.name}
                    </span>
                  </div>
                  <p className="mt-2 text-[12.5px] leading-5 text-[#0a0d12]/55">
                    {s.description}
                  </p>

                  {/* Who's running it right now — rotates to show reuse. */}
                  <div
                    className={cn(
                      "mt-2.5 flex items-center gap-1.5 text-[11.5px] font-medium transition-opacity duration-300",
                      isActive ? "text-[#0a0d12]/60 opacity-100" : "opacity-0",
                    )}
                  >
                    <img
                      // eslint-disable-next-line @next/next/no-img-element
                      src={agent.avatar_url ?? ""}
                      alt=""
                      className="size-4 rounded-full"
                    />
                    {agent.name} ran this
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </ValueDemoFrame>
  );
}

function PlaySquare() {
  return (
    <svg viewBox="0 0 16 16" fill="none" className="size-3.5">
      <path d="M6 5.5v5l4-2.5-4-2.5Z" fill="currentColor" />
    </svg>
  );
}
