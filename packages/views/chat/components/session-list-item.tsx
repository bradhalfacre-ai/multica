"use client";

import { useState } from "react";
import { Check, Pencil, Trash2 } from "lucide-react";
import { cn } from "@multica/ui/lib/utils";
import { UnicodeSpinner } from "@multica/ui/components/common/unicode-spinner";
import { ActorAvatar } from "../../common/actor-avatar";
import { useT } from "../../i18n";
import type { Agent, ChatSession } from "@multica/core/types";

interface SessionListItemProps {
  session: ChatSession;
  agent: Agent | null;
  isCurrent: boolean;
  isRunning: boolean;
  isRenaming: boolean;
  formatTimeAgo: (dateStr: string) => string;
  onSelect: () => void;
  onStartRename: () => void;
  onSubmitRename: (value: string) => void;
  onCancelRename: () => void;
  onDelete: () => void;
  className?: string;
}

export function SessionListItem({
  session,
  agent,
  isCurrent,
  isRunning,
  isRenaming,
  formatTimeAgo,
  onSelect,
  onStartRename,
  onSubmitRename,
  onCancelRename,
  onDelete,
  className,
}: SessionListItemProps) {
  const { t } = useT("chat");

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => {
        if (isRenaming) return;
        onSelect();
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter" && !isRenaming) onSelect();
      }}
      className={cn(
        "group flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-sm cursor-default transition-colors",
        isCurrent ? "bg-accent" : "hover:bg-accent/50",
        className,
      )}
    >
      {agent ? (
        <ActorAvatar
          actorType="agent"
          actorId={agent.id}
          size={24}
          enableHoverCard
          showStatusDot
        />
      ) : (
        <span className="size-6 shrink-0" />
      )}
      <div className="min-w-0 flex-1">
        {isRenaming ? (
          <SessionRenameInlineInput
            initialValue={session.title ?? ""}
            onSubmit={onSubmitRename}
            onCancel={onCancelRename}
          />
        ) : (
          <>
            <div className="truncate text-sm">
              {session.title?.trim() || t(($) => $.window.untitled)}
            </div>
            <div className="truncate text-xs text-muted-foreground/70">
              {formatTimeAgo(session.updated_at)}
            </div>
          </>
        )}
      </div>
      {!isRenaming && isRunning ? (
        <span
          aria-label={t(($) => $.window.running)}
          title={t(($) => $.window.running)}
          className="shrink-0"
        >
          <UnicodeSpinner name="breathe" className="text-muted-foreground text-sm" />
        </span>
      ) : !isRenaming && session.has_unread ? (
        <span
          aria-label={t(($) => $.window.unread)}
          title={t(($) => $.window.unread)}
          className="shrink-0"
        >
          <Check className="size-3.5 text-brand" />
        </span>
      ) : null}
      {!isRenaming && isCurrent && !session.has_unread && !isRunning && (
        <Check className="size-3.5 text-muted-foreground shrink-0" />
      )}
      {!isRenaming && (
        <div className="flex items-center gap-0.5 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onStartRename();
            }}
            className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            aria-label={t(($) => $.session_history.row_rename_aria)}
            title={t(($) => $.session_history.row_rename_aria)}
          >
            <Pencil className="size-3.5" />
          </button>
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onDelete();
            }}
            className="rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            aria-label={t(($) => $.session_history.row_delete_aria)}
          >
            <Trash2 className="size-3.5" />
          </button>
        </div>
      )}
    </div>
  );
}

function SessionRenameInlineInput({
  initialValue,
  onSubmit,
  onCancel,
}: {
  initialValue: string;
  onSubmit: (value: string) => void;
  onCancel: () => void;
}) {
  const { t } = useT("chat");
  const [value, setValue] = useState(initialValue);

  return (
    <input
      ref={(el) => {
        el?.focus();
        el?.select();
      }}
      type="text"
      value={value}
      maxLength={200}
      aria-label={t(($) => $.session_history.row_rename_aria)}
      onChange={(e) => setValue(e.target.value)}
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === "Enter") {
          e.preventDefault();
          onSubmit(value);
        } else if (e.key === "Escape") {
          e.preventDefault();
          onCancel();
        }
      }}
      onBlur={() => onSubmit(value)}
      className="w-full rounded-sm bg-background px-1 py-0.5 text-sm outline-none ring-1 ring-border focus-visible:ring-brand"
    />
  );
}
