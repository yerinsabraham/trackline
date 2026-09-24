import type { MetadataRoute } from "next";
import { pages } from "@/lib/guide";
import { SITE } from "@/lib/site";

export const dynamic = "force-static";

export default function sitemap(): MetadataRoute.Sitemap {
  const routes = ["", "/docs", "/evidence", "/privacy", "/terms", ...pages().map((p) => `/docs/${p.slug}`)];
  return routes.map((r) => ({ url: `${SITE}${r}` }));
}
