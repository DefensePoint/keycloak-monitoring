import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { JSDOM } from "jsdom";
import { build } from "vite";

// The unit tests run modules through Vitest, which unwraps a CommonJS default
// export; the production build does not. This test bundles
// markerClusterGroup.ts the way `vite build` does and runs the bundle in a
// browser-like window, so it sees what the map really renders.
it("bundles to a component React can render, not the CommonJS module object", async () => {
  const outDir = fs.mkdtempSync(
    path.join(os.tmpdir(), "marker-cluster-bundle-"),
  );
  try {
    await build({
      configFile: false,
      logLevel: "silent",
      define: { "process.env.NODE_ENV": JSON.stringify("production") },
      build: {
        outDir,
        emptyOutDir: true,
        lib: {
          entry: path.resolve(__dirname, "markerClusterGroup.ts"),
          formats: ["iife"],
          name: "markerClusterGroupBundle",
          fileName: () => "bundle.js",
        },
      },
    });

    const dom = new JSDOM("<!doctype html><body></body>", {
      runScripts: "outside-only",
      pretendToBeVisual: true,
    });
    dom.window.eval(fs.readFileSync(path.join(outDir, "bundle.js"), "utf8"));
    const { MarkerClusterGroup } = (
      dom.window as unknown as {
        markerClusterGroupBundle: {
          MarkerClusterGroup: { $$typeof?: symbol; render?: unknown };
        };
      }
    ).markerClusterGroupBundle;

    expect(MarkerClusterGroup.$$typeof).toBe(Symbol.for("react.forward_ref"));
    expect(typeof MarkerClusterGroup.render).toBe("function");
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
}, 60_000);
