import type { Metadata } from "next";

// Every page gets its own share card, so a docs link posted in Slack or on X
// previews as that page, not as the home page.
export const ogKey = (p: string) => (p === "/" ? "home" : p.replace(/^\//, "").replace(/\//g, "-"));

export function pageMeta({ path, title, description, index = true }: {
  path: string;
  title: string;
  description: string;
  index?: boolean;
}): Metadata {
  const image = { url: `/og/${ogKey(path)}.png`, width: 1200, height: 630, alt: title };
  return {
    title,
    description,
    alternates: { canonical: path },
    openGraph: { type: "website", url: path, siteName: "trackline", title, description, images: [image] },
    twitter: { card: "summary_large_image", title, description, images: [image.url] },
    ...(index ? {} : { robots: { index: false } }),
  };
}
