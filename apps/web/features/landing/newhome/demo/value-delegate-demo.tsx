"use client";

// Value #2 — "Delegate to agents like teammates". A focused conversation on an
// issue: a person hands work to an agent in a comment, the agent picks it up
// ("working…") and replies like a teammate with the result + a PR. Auto-plays:
// working → reply → hold → reset. Presentational (no providers), styled to
// match the product's comment thread.

import { useEffect, useState } from "react";
import { AGENTS } from "./mock-data";
import { ValueDemoFrame } from "./value-demo-frame";

const CLAUDE = AGENTS.find((a) => a.id === "a-claude")!;

function Avatar({ kind, initials }: { kind: "agent" | "member"; initials?: string }) {
  if (kind === "agent") {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={CLAUDE.avatar_url ?? ""}
        alt=""
        className="size-7 shrink-0 rounded-full ring-1 ring-[#0a0d12]/8"
      />
    );
  }
  return (
    <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-[#0a0d12]/[0.07] text-[11px] font-semibold text-[#0a0d12]/60">
      {initials}
    </span>
  );
}

function Comment({
  name,
  badge,
  time,
  kind,
  initials,
  children,
}: {
  name: string;
  badge?: string;
  time: string;
  kind: "agent" | "member";
  initials?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex gap-3">
      <Avatar kind={kind} initials={initials} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-[13.5px] font-semibold text-[#0a0d12]">{name}</span>
          {badge && (
            <span className="rounded-full bg-[var(--brand)]/12 px-1.5 py-0.5 text-[10.5px] font-semibold text-[var(--brand)]">
              {badge}
            </span>
          )}
          <span className="text-[12px] text-[#0a0d12]/40">{time}</span>
        </div>
        <div className="mt-1.5 rounded-[6px] rounded-tl-[2px] border border-[#0a0d12]/8 bg-[#0a0d12]/[0.02] px-3.5 py-2.5 text-[13.5px] leading-6 text-[#0a0d12]/80">
          {children}
        </div>
      </div>
    </div>
  );
}

export function ValueDelegateDemo() {
  // phase 0 = agent working, phase 1 = agent replied (held), then loop.
  const [phase, setPhase] = useState(0);
  useEffect(() => {
    const seq = [
      [0, 2200],
      [1, 3600],
    ] as const;
    let i = 0;
    let timer: number;
    const tick = () => {
      const [p, hold] = seq[i % seq.length]!;
      setPhase(p);
      i += 1;
      timer = window.setTimeout(tick, hold);
    };
    tick();
    return () => window.clearTimeout(timer);
  }, []);

  return (
    <ValueDemoFrame height={476}>
      <div className="pointer-events-none flex h-full w-full select-none bg-white">
        <div className="flex h-full w-full flex-col px-9 py-9">
          {/* Issue header */}
          <div className="flex items-center gap-2 text-[12.5px]">
            <span className="font-medium text-[#0a0d12]/45">MUL-137</span>
            <span className="text-[#0a0d12]/25">·</span>
            <span className="font-semibold text-[#0a0d12]">
              Add rate limiting to the public API
            </span>
          </div>
          <div className="mt-1 flex items-center gap-1.5 text-[12px] text-[#0a0d12]/45">
            <span className="inline-block size-1.5 rounded-full bg-amber-500" />
            In Progress
            <span className="text-[#0a0d12]/25">·</span>
            Assigned to
            <img
              // eslint-disable-next-line @next/next/no-img-element
              src={CLAUDE.avatar_url ?? ""}
              alt=""
              className="size-3.5 rounded-full"
            />
            <span className="font-medium text-[#0a0d12]/65">Claude Code</span>
          </div>

          <div className="mt-6 flex flex-1 flex-col gap-5">
            <Comment name="Alex Rivera" time="2m ago" kind="member" initials="AR">
              <span className="font-medium text-[var(--brand)]">@Claude Code</span>{" "}
              can you add token-bucket rate limiting to the public API gateway? 100
              req/min per key, return 429 + Retry-After when it&rsquo;s exceeded.
            </Comment>

            {phase === 0 ? (
              <div className="flex items-center gap-3">
                <Avatar kind="agent" />
                <div className="flex items-center gap-2 rounded-full bg-[#0a0d12]/[0.04] px-3 py-1.5">
                  <span className="text-[12.5px] font-medium text-[#0a0d12]/55">
                    Claude Code is working
                  </span>
                  <span className="newhome-typing flex gap-0.5">
                    <span className="size-1.5 rounded-full bg-[#0a0d12]/30" />
                    <span className="size-1.5 rounded-full bg-[#0a0d12]/30" />
                    <span className="size-1.5 rounded-full bg-[#0a0d12]/30" />
                  </span>
                </div>
              </div>
            ) : (
              <div className="newhome-card-land">
                <Comment
                  name="Claude Code"
                  badge="Agent"
                  time="just now"
                  kind="agent"
                >
                  Done. Added a token-bucket limiter on the gateway — 100 req/min
                  per key, <code className="text-[12.5px]">X-RateLimit-*</code>{" "}
                  headers, and a 429 + Retry-After path. Opened a PR with tests.
                  <span className="mt-2 flex w-fit items-center gap-1.5 rounded-[6px] border border-[#0a0d12]/8 bg-white px-2 py-1 text-[12px] font-medium text-[#0a0d12]/70">
                    <span className="text-[#0a0d12]/40">⎇</span> PR #3721 ·
                    gateway: token-bucket rate limiting
                  </span>
                </Comment>
              </div>
            )}
          </div>
        </div>
      </div>
    </ValueDemoFrame>
  );
}
