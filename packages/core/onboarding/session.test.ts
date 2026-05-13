import { afterEach, beforeEach, describe, expect, it } from "vitest";

async function loadModule() {
  // Reset module cache so each test starts with a clean in-memory id.
  const vitest = await import("vitest");
  vitest.vi.resetModules();
  return import("./session");
}

beforeEach(() => {
  // Provide a minimal in-memory localStorage so defaultStorage's
  // typeof-window check passes and we exercise the persistence path.
  const store = new Map<string, string>();
  Object.defineProperty(globalThis, "window", {
    value: {},
    configurable: true,
    writable: true,
  });
  Object.defineProperty(globalThis, "localStorage", {
    value: {
      getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
      setItem: (k: string, v: string) => store.set(k, v),
      removeItem: (k: string) => store.delete(k),
    },
    configurable: true,
    writable: true,
  });
});

afterEach(() => {
  // @ts-expect-error — test-only cleanup
  delete globalThis.window;
  // @ts-expect-error — test-only cleanup
  delete globalThis.localStorage;
});

describe("onboarding session", () => {
  it("startOnboardingSession returns a stable id within the same funnel", async () => {
    const session = await loadModule();
    const id1 = session.startOnboardingSession();
    const id2 = session.startOnboardingSession();
    expect(id1).toBe(id2);
    expect(session.getOnboardingSessionId()).toBe(id1);
  });

  it("clearOnboardingSession resets so the next start gets a fresh id", async () => {
    const session = await loadModule();
    const first = session.startOnboardingSession();
    session.clearOnboardingSession();
    expect(session.getOnboardingSessionId()).toBeNull();
    const second = session.startOnboardingSession();
    expect(second).not.toBe(first);
  });

  it("getOnboardingSessionId returns null when no session has been started", async () => {
    const session = await loadModule();
    expect(session.getOnboardingSessionId()).toBeNull();
  });
});
