/** @type {import('next').NextConfig} */
const isProd = process.env.NODE_ENV === "production";

// Static export is required for Electron file:// loading.
// - output: "export" produces pure HTML/CSS/JS in dist/
// - distDir: "dist" keeps build output predictable for electron/main.js
// - assetPrefix: "./" makes all _next/ assets relative so file:// works
// - trailingSlash: true ensures directories resolve predictably on file://
// - images.unoptimized: true disables Next Image optimization server
// Development uses its own cache so production builds cannot remove live dev assets.
const nextConfig = {
  output: isProd ? "export" : undefined,
  distDir: isProd ? "dist" : ".next",
  assetPrefix: isProd ? "./" : undefined,
  trailingSlash: true,
  images: { unoptimized: true },
  experimental: { optimizePackageImports: ["@phosphor-icons/react"] },
  // Disable React strict double-invoke noise for streaming deltas in dev
  reactStrictMode: false,
};

module.exports = nextConfig;
