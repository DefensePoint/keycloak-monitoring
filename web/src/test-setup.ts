import "@testing-library/jest-dom";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// Cleanup after each test
afterEach(() => {
  cleanup();
});

/**
 * Web Storage for tests, installed unconditionally.
 *
 * Node 22 and later expose their own experimental `localStorage` and
 * `sessionStorage` globals, and they take precedence over the ones jsdom sets
 * up. Node's `localStorage` is a hollow object: `getItem`, `clear` and
 * `length` are all undefined, so `localStorage.clear()` throws
 * "localStorage.clear is not a function" and every test in the file fails
 * before its first assertion. That is why TenantContext.test.tsx showed nine
 * failures locally while passing in CI, which runs node:20-alpine.
 *
 * Node's `sessionStorage` does work, but it is process-global rather than
 * per-jsdom-environment, so values written by one test file can be read by
 * another sharing the worker. Both are replaced here for the same reason:
 * whichever Node the developer has, the suite behaves the same.
 *
 * This file is evaluated once per test file, so each file starts with empty
 * storage. Within a file, values persist until cleared, matching a browser.
 */
function createMemoryStorage(): Storage {
  const data = new Map<string, string>();
  const storage: Storage = {
    get length() {
      return data.size;
    },
    clear() {
      data.clear();
    },
    getItem(key: string) {
      const value = data.get(String(key));
      return value === undefined ? null : value;
    },
    key(index: number) {
      return Array.from(data.keys())[index] ?? null;
    },
    removeItem(key: string) {
      data.delete(String(key));
    },
    setItem(key: string, value: string) {
      data.set(String(key), String(value));
    },
  };
  return storage;
}

for (const name of ["localStorage", "sessionStorage"] as const) {
  Object.defineProperty(globalThis, name, {
    value: createMemoryStorage(),
    // Writable and configurable so a test can still stub or spy on it.
    writable: true,
    configurable: true,
  });
}

// Mock window.matchMedia
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => {},
  }),
});
