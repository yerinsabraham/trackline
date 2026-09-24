import type { NextConfig } from "next";

// Static: every page is known at build time, and a site with nothing to run on
// a server has nothing to break or pay for.
const config: NextConfig = {
  output: "export",
  // The trackline package's own lockfile sits one directory up, and without
  // this Next takes that as the project root and looks for dependencies there.
  turbopack: { root: __dirname },
  images: { unoptimized: true },
  trailingSlash: false,
};

export default config;
