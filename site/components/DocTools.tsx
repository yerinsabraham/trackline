"use client";

import { useState } from "react";

// The page as Markdown, for a person pasting it into a chat or an agent reading
// it: the same menu the big developer docs carry, pointed at our own .md copy.
export default function DocTools({ md, title }: { md: string; title: string }) {
  const [copied, setCopied] = useState(false);
  const url = typeof window === "undefined" ? md : new URL(md, window.location.origin).href;
  const ask = encodeURIComponent(`Read ${url} and help me with trackline: ${title}.`);

  const copy = async () => {
    try {
      const text = await (await fetch(md)).text();
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    } catch {
      window.open(md, "_blank");
    }
  };

  return (
    <div className="doc-tools">
      <button type="button" onClick={copy}>{copied ? "Copied" : "Copy page"}</button>
      <a href={md}>View as Markdown</a>
      <a href={`https://claude.ai/new?q=${ask}`} target="_blank" rel="noreferrer">Open in Claude</a>
      <a href={`https://chatgpt.com/?q=${ask}`} target="_blank" rel="noreferrer">Open in ChatGPT</a>
    </div>
  );
}
