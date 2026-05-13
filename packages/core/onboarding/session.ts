// Onboarding session identifier — issued once per funnel entry and attached
// to every onboarding event so PostHog can correlate the full funnel back to
// a single `onboarding_started`. Solves the prior funnel-attribution gap
// where `onboarding_completed` events fired from skip/invite paths (no
// `onboarding_started` in their lineage) were indistinguishable from real
// funnel completions when joining on `distinct_id` alone.
//
// The id is persisted to client storage because the funnel spans page
// reloads (especially on web — desktop bundle install, OAuth redirects).
// It's cleared on completion; entering onboarding again starts a fresh
// session and a fresh id.

import { createSafeId } from "../utils";
import { defaultStorage } from "../platform/storage";

const STORAGE_KEY = "multica_onboarding_session_id";

// In-memory cache so the analytics wrapper doesn't hit storage on every
// event. Storage is read once on first access and on every start/clear.
let cached: string | null | undefined;

function read(): string | null {
  if (cached !== undefined) return cached;
  cached = defaultStorage.getItem(STORAGE_KEY);
  return cached;
}

/**
 * Generate a new session id and persist it. Idempotent — calling twice in
 * the same funnel returns the same id, so a re-mount of the onboarding
 * shell can't accidentally split one funnel across two sessions.
 *
 * The expected fire site is the same place that emits `onboarding_started`.
 */
export function startOnboardingSession(): string {
  const existing = read();
  if (existing) return existing;
  const id = createSafeId();
  cached = id;
  defaultStorage.setItem(STORAGE_KEY, id);
  return id;
}

/**
 * Read the current session id. Returns null when no onboarding session is
 * in progress — the analytics wrapper omits the property in that case
 * rather than emitting an empty string, so HogQL queries can filter
 * `onboarding_session_id IS NOT NULL` to isolate real funnel events from
 * skip/invite paths that legitimately have no session.
 */
export function getOnboardingSessionId(): string | null {
  return read();
}

/**
 * Clear the session. Called at the funnel terminus (after a successful
 * `onboarding_completed`) so a returning user who somehow re-enters
 * onboarding starts a fresh session.
 */
export function clearOnboardingSession(): void {
  cached = null;
  defaultStorage.removeItem(STORAGE_KEY);
}
