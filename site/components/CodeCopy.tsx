"use client";

import { useEffect } from "react";

// The guide is rendered from Markdown, so its code blocks are plain <pre>
// elements. This adds a copy button to each one after the page loads.
export default function CodeCopy() {
  useEffect(() => {
    const blocks = document.querySelectorAll<HTMLPreElement>(".prose pre");
    const added: HTMLButtonElement[] = [];
    blocks.forEach((pre) => {
      if (pre.querySelector(".code-copy")) return;
      const b = document.createElement("button");
      b.type = "button";
      b.className = "code-copy";
      b.textContent = "Copy";
      b.onclick = async () => {
        try {
          await navigator.clipboard.writeText(pre.querySelector("code")?.textContent ?? pre.textContent ?? "");
          b.textContent = "Copied";
          setTimeout(() => (b.textContent = "Copy"), 1500);
        } catch {
          b.textContent = "Select and copy";
        }
      };
      pre.appendChild(b);
      added.push(b);
    });
    return () => added.forEach((b) => b.remove());
  }, []);
  return null;
}
