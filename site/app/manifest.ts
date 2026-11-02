import type { MetadataRoute } from "next";
import { DESCRIPTION } from "@/lib/site";

export const dynamic = "force-static";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "trackline",
    short_name: "trackline",
    description: DESCRIPTION,
    start_url: "/app",
    scope: "/",
    display: "standalone",
    background_color: "#f1f1ef",
    theme_color: "#f1f1ef",
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
