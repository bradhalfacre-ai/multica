"use client";

// Value #3 — "Every run is on the record". A focused transcript that streams
// the agent's run, event by event (thinking → reads → edits → tests → result),
// the same record you can reopen on any run. Auto-plays the stream, then holds
// and replays. Presentational (no providers); colors mirror the real transcript
// (thinking = violet, tool = blue, result = green).

import { useEffect, useState } from "react";
import { cn } from "@multica/ui/lib/utils";
import { AGENTS, TRANSCRIPT_BY_ISSUE } from "./mock-data";
import { ValueDemoFrame } from "./value-demo-frame";

const CLAUDE = AGENTS.find((a) => a.id === "a-claude")!;

interface TItem {
  type: "thinking" | "text" | "tool_use" | "tool_result";
  content?: string;
  tool?: string;
  input?: Record<string, unknown>;
  output?: string;
}

const ITEMS = (TRANSCRIPT_BY_ISSUE["issue-129"] ?? []) as unknown as TItem[];

function meta(m: TItem): { label: string; cls: string } {
  switch (m.type) {
    case "thinking":
      return { label: "Thinking", cls: "bg-violet-500/12 text-violet-700" };
    case "text":
      return { label: "Claude", cls: "bg-[#0a0d12]/[0.06] text-[#0a0d12]/65" };
    case "tool_use":
      return { label: m.tool ?? "Tool", cls: "bg-blue-500/12 text-blue-700" };
    case "tool_result":
      return { label: m.tool ?? "Result", cls: "bg-emerald-500/12 text-emerald-700" };
  }
}

function summarize(m: TItem): string {
  if (m.type === "thinking" || m.type === "text") return m.content ?? "";
  if (m.type === "tool_use") {
    const i = m.input ?? {};
    return (
      (i.file_path as string) ??
      (i.command as string) ??
      (i.summary as string) ??
      ""
    );
  }
  return m.output ?? "";
}

export function ValueTranscriptDemo() {
  // Stream events in; once all are shown, hold, then restart.
  const [shown, setShown] = useState(1);
  useEffect(() => {
    let timer: number;
    const step = () => {
      setShown((n) => {
        if (n >= ITEMS.length) {
          timer = window.setTimeout(() => setShown(1), 2400);
          return n;
        }
        timer = window.setTimeout(step, 620);
        return n + 1;
      });
    };
    timer = window.setTimeout(step, 620);
    return () => window.clearTimeout(timer);
  }, []);

  const items = ITEMS.slice(0, shown);

  return (
    <ValueDemoFrame height={624}>
      <div className="pointer-events-none flex h-full w-full select-none bg-white">
        <div className="flex h-full w-full flex-col px-9 py-9">
          {/* Run header */}
          <div className="flex items-center gap-2.5 border-b border-[#0a0d12]/8 pb-4">
            <img
              // eslint-disable-next-line @next/next/no-img-element
              src={CLAUDE.avatar_url ?? ""}
              alt=""
              className="size-7 rounded-full ring-1 ring-[#0a0d12]/8"
            />
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="text-[13.5px] font-semibold text-[#0a0d12]">
                  Claude Code
                </span>
                <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/12 px-1.5 py-0.5 text-[10.5px] font-semibold text-emerald-700">
                  <span className="size-1.5 rounded-full bg-emerald-500" />
                  Running
                </span>
              </div>
              <div className="truncate text-[12px] text-[#0a0d12]/45">
                Implement OAuth login flow · MUL-129
              </div>
            </div>
            <span className="ml-auto text-[12px] tabular-nums text-[#0a0d12]/40">
              4m 12s
            </span>
          </div>

          {/* Streaming event list */}
          <div className="relative mt-4 flex-1 overflow-hidden">
            {/* timeline rail */}
            <span className="absolute bottom-1 left-[7px] top-1 w-px bg-[#0a0d12]/8" />
            <div className="flex flex-col gap-3">
              {items.map((m, idx) => {
                const { label, cls } = meta(m);
                const last = idx === items.length - 1;
                return (
                  <div
                    key={idx}
                    className={cn("flex gap-3", last && "newhome-card-land")}
                  >
                    <span
                      className={cn(
                        "relative z-10 mt-1 size-3.5 shrink-0 rounded-full ring-4 ring-white",
                        m.type === "thinking"
                          ? "bg-violet-400"
                          : m.type === "tool_result"
                            ? "bg-emerald-400"
                            : m.type === "tool_use"
                              ? "bg-blue-400"
                              : "bg-[#0a0d12]/25",
                      )}
                    />
                    <div className="min-w-0 flex-1 pb-0.5">
                      <span
                        className={cn(
                          "mr-2 inline-block rounded-[5px] px-1.5 py-0.5 text-[11px] font-semibold",
                          cls,
                        )}
                      >
                        {label}
                      </span>
                      <span className="text-[13px] leading-6 text-[#0a0d12]/70">
                        {summarize(m)}
                      </span>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>
    </ValueDemoFrame>
  );
}
