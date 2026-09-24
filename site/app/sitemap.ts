import type { MetadataRoute } from "next";
import { experiments } from "@/lib/experiments";
import { pages } from "@/lib/guide";
import { agents } from "@/lib/hosts";
import { SITE } from "@/lib/site";

export const dynamic = "force-static";

export default function sitemap(): MetadataRoute.Sitemap {
  const routes = [
    "", "/docs", "/evidence", "/agents", "/changelog", "/about", "/privacy", "/terms",
    ...pages().map((p) => `/docs/${p.slug}`),
    ...experiments().map((e) => `/evidence/${e.slug}`),
    ...agents().map((a) => `/agents/${a.slug}`),
  ];
  return routes.map((r) => ({ url: `${SITE}${r}` }));
}
