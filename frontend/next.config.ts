import type { NextConfig } from "next";

// Quarry ships as a static bundle embedded in the Go binary, so the app is
// exported to ./out and served by Wails — no Node server at runtime.
const nextConfig: NextConfig = {
  output: "export",
  images: { unoptimized: true },
  devIndicators: false,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
