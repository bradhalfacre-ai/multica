"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import type { CommentTriggerPreviewAgent } from "@multica/core/types";

const COMMENT_TRIGGER_PREVIEW_DEBOUNCE_MS = 300;
const MENTION_RE = /\[@?(.+?)\]\(mention:\/\/(member|agent|squad|issue|all)\/([0-9a-fA-F-]+|all)\)/g;
const NOTE_COMMAND_RE = /^\/note(?:$|\s)/i;

export type CommentTriggerPreviewStatus = "idle" | "loading" | "error";

export interface UseCommentTriggerPreviewResult {
  agents: CommentTriggerPreviewAgent[];
  status: CommentTriggerPreviewStatus;
}

export function isNoteCommentDraft(content: string): boolean {
  return NOTE_COMMAND_RE.test(content.replace(/^[ \t\r\n]+/, ""));
}

export function commentTriggerPreviewSignature(content: string): string {
  if (!content.trim() || isNoteCommentDraft(content)) return "empty";

  const seen = new Set<string>();
  const tokens: string[] = [];
  for (const match of content.matchAll(MENTION_RE)) {
    const type = match[2];
    const id = match[3];
    if (!type || !id || type === "issue") continue;
    const token = `${type}:${id}`;
    if (seen.has(token)) continue;
    seen.add(token);
    tokens.push(token);
  }

  return `nonempty|${tokens.join(",")}`;
}

function useDebouncedSignature(signature: string) {
  const [debouncedSignature, setDebouncedSignature] = useState("empty");

  useEffect(() => {
    if (signature === "empty") {
      setDebouncedSignature("empty");
      return;
    }

    const timer = window.setTimeout(() => {
      setDebouncedSignature(signature);
    }, COMMENT_TRIGGER_PREVIEW_DEBOUNCE_MS);

    return () => window.clearTimeout(timer);
  }, [signature]);

  return debouncedSignature;
}

export function useCommentTriggerPreview({
  issueId,
  parentId,
  content,
}: {
  issueId: string;
  parentId?: string;
  content: string;
}): UseCommentTriggerPreviewResult {
  const signature = useMemo(() => commentTriggerPreviewSignature(content), [content]);
  const debouncedSignature = useDebouncedSignature(signature);
  const contentRef = useRef(content);

  useEffect(() => {
    contentRef.current = content;
  }, [content]);

  const previewQuery = useQuery({
    queryKey: ["issues", "comment-trigger-preview", issueId, parentId ?? "", debouncedSignature],
    queryFn: () => api.previewCommentTriggers(issueId, contentRef.current, parentId),
    enabled: signature !== "empty" && debouncedSignature !== "empty",
    retry: false,
    staleTime: Infinity,
  });

  if (signature === "empty" || debouncedSignature === "empty") {
    return { agents: [], status: "idle" };
  }

  const status: CommentTriggerPreviewStatus = previewQuery.isError
    ? "error"
    : previewQuery.isFetching
      ? "loading"
      : "idle";

  return {
    agents: previewQuery.data?.agents ?? [],
    status,
  };
}
