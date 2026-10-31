import { Children, isValidElement, type ComponentProps, type ReactElement, type ReactNode } from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import CopyButton from "./CopyButton";

// An agent's reply, in the Markdown agents write, rendered as React elements
// and never as HTML. The text comes from a model that has read the project and
// can be steered by what it read, and this page holds the sign-in token: a
// reply must not be able to put markup on it. Raw HTML in a reply is shown as
// the text it is, and an image shows its description, because loading one would
// tell whoever wrote its address anything the model put in it. A link works,
// since opening one takes a person's tap, but only to http or https, in a new
// tab that gets nothing from this page, and with its real site shown whenever
// its words do not already say so.

const ALLOWED = [
  "p", "br", "hr", "strong", "em", "del", "code", "pre", "blockquote", "ul", "ol", "li", "input",
  "h1", "h2", "h3", "h4", "h5", "h6", "table", "thead", "tbody", "tr", "th", "td", "a", "img",
];

const PLUGINS = [remarkGfm];

function textOf(node: ReactNode): string {
  return Children.toArray(node).map((n) => (typeof n === "string" ? n : isValidElement(n) ? textOf((n as ReactElement<{ children?: ReactNode }>).props.children) : "")).join("");
}

/** The site a link goes to, or null for anything that is not a web page. */
function site(href: string | undefined): string | null {
  try {
    const u = new URL(href ?? "");
    // https://paypal.com@evil.com goes to evil.com, and has no honest use here.
    if (u.username || u.password) return null;
    return u.protocol === "https:" || u.protocol === "http:" ? u.hostname : null;
  } catch { return null; }
}

function Link({ href, words, children }: { href?: string; words: string; children: ReactNode }) {
  const host = site(href);
  if (!host) return <>{children}</>;
  const bare = host.replace(/^www\./, "");
  return (
    <>
      <a href={href} target="_blank" rel="noopener noreferrer nofollow">{children}</a>
      {!namesSite(words, bare) && <span className="link-site"> ({bare})</span>}
    </>
  );
}

// Whether a link's words are themselves an address on that site. Finding the
// name anywhere in them is not enough: "paypal.com/evil.com" contains evil.com
// and reads as PayPal.
function namesSite(words: string, bare: string): boolean {
  const w = words.trim();
  // A name written in look-alike letters reads as another site, so it is always
  // spelled out in the form that shows it.
  if (bare.split(".").some((part) => part.startsWith("xn--"))) return false;
  return site(/^https?:\/\//i.test(w) ? w : `https://${w}`)?.replace(/^www\./, "") === bare;
}

const URL_IN_TEXT = /https?:\/\/[^\s<>"]+/g;

/** Something a person typed, with its addresses as links and nothing else changed. */
export function LinkedText({ text }: { text: string }) {
  const out: ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(URL_IN_TEXT)) {
    // A sentence's closing mark is not part of the address.
    const url = m[0].replace(/[.,;:!?)\]'}]+$/, "");
    if (!site(url)) continue;
    out.push(text.slice(last, m.index), <a key={m.index} href={url} target="_blank" rel="noopener noreferrer nofollow">{url}</a>);
    last = m.index + url.length;
  }
  out.push(text.slice(last));
  return <>{out}</>;
}

const ELEMENTS: Components = {
  // The page's own heading is the chat's; a reply's headings sit below it.
  h1: "h3", h2: "h3", h3: "h4", h4: "h5", h5: "h5", h6: "h5",
  a: ({ href, children }) => <Link href={href} words={textOf(children)}>{children}</Link>,
  img: ({ alt }) => <>{alt}</>,
  // Wide things scroll inside their own box, so the chat never scrolls sideways.
  table: ({ children }: ComponentProps<"table">) => <div className="md-table"><table>{children}</table></div>,
  pre: ({ children }: ComponentProps<"pre">) => (
    <div className="md-code"><CopyButton text={textOf(children).replace(/\n$/, "")} /><pre>{children}</pre></div>
  ),
};

export default function ReplyText({ text }: { text: string }) {
  return (
    <div className="reply-text">
      <Markdown remarkPlugins={PLUGINS} allowedElements={ALLOWED} components={ELEMENTS}>{text}</Markdown>
    </div>
  );
}

/** The reply as one line of plain text, for previews. */
export function plain(text: string): string {
  return text
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/!?\[([^\]]+)\]\([^)]*\)/g, "$1")
    // A table's ruled line, then its bars, so a preview reads as words.
    .replace(/^\s*\|?[\s:|-]*-[\s:|-]*\|?\s*$/gm, " ")
    .replace(/\|/g, " ")
    .replace(/^\s*>\s?/gm, "")
    .replace(/[`*#~]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}
