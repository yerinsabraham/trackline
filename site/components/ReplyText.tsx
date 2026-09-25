import { Fragment, type ReactNode } from "react";

// An agent's reply, in the Markdown agents write, rendered as React elements
// and never as HTML. The text comes from a model that has read the project and
// can be steered by what it read, and this page holds the sign-in token: a
// reply must not be able to put markup on it. Links show their text only, so
// a reply cannot send anyone anywhere either.
export default function ReplyText({ text }: { text: string }) {
  const blocks: ReactNode[] = [];
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.startsWith("```")) {
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i].startsWith("```")) body.push(lines[i++]);
      i++;
      blocks.push(<pre key={blocks.length}><code>{body.join("\n")}</code></pre>);
      continue;
    }
    const bullet = /^\s*(?:[-*]|\d+[.)])\s+/;
    if (bullet.test(line)) {
      const ordered = /^\s*\d/.test(line);
      const items: string[] = [];
      while (i < lines.length && bullet.test(lines[i])) items.push(lines[i++].replace(bullet, ""));
      const List = ordered ? "ol" : "ul";
      blocks.push(<List key={blocks.length}>{items.map((t, k) => <li key={k}>{inline(t)}</li>)}</List>);
      continue;
    }
    if (!line.trim()) { i++; continue; }
    const para: string[] = [];
    while (i < lines.length && lines[i].trim() && !lines[i].startsWith("```") && !bullet.test(lines[i])) {
      para.push(lines[i++].replace(/^#{1,6}\s+/, ""));
    }
    blocks.push(<p key={blocks.length}>{inline(para.join(" "))}</p>);
  }
  return <div className="reply-text">{blocks}</div>;
}

// `code`, **bold**, and [text](target) shown as its text.
function inline(s: string): ReactNode[] {
  const out: ReactNode[] = [];
  const re = /`([^`]+)`|\*\*([^*]+)\*\*|\[([^\]]+)\]\([^)]*\)/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(s))) {
    if (m.index > last) out.push(<Fragment key={out.length}>{s.slice(last, m.index)}</Fragment>);
    if (m[1] !== undefined) out.push(<code key={out.length}>{m[1]}</code>);
    else if (m[2] !== undefined) out.push(<strong key={out.length}>{m[2]}</strong>);
    else out.push(<Fragment key={out.length}>{m[3]}</Fragment>);
    last = re.lastIndex;
  }
  if (last < s.length) out.push(<Fragment key={out.length}>{s.slice(last)}</Fragment>);
  return out;
}

/** The reply as one line of plain text, for previews. */
export function plain(text: string): string {
  return text.replace(/```[\s\S]*?```/g, " ").replace(/\[([^\]]+)\]\([^)]*\)/g, "$1").replace(/[`*#]/g, "").replace(/\s+/g, " ").trim();
}
