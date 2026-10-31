import { Children, isValidElement, type ComponentProps, type ReactElement, type ReactNode } from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import CopyButton from "./CopyButton";

// An agent's reply, in the Markdown agents write, rendered as React elements
// and never as HTML. The text comes from a model that has read the project and
// can be steered by what it read, and this page holds the sign-in token: a
// reply must not be able to put markup on it. Raw HTML in a reply is shown as
// the text it is. Links show their text only, so a reply cannot send anyone
// anywhere, and an image shows its description, because loading one would tell
// whoever wrote its address anything the model put in it.

const ALLOWED = [
  "p", "br", "hr", "strong", "em", "del", "code", "pre", "blockquote", "ul", "ol", "li", "input",
  "h1", "h2", "h3", "h4", "h5", "h6", "table", "thead", "tbody", "tr", "th", "td", "a", "img",
];

const PLUGINS = [remarkGfm];

function textOf(node: ReactNode): string {
  return Children.toArray(node).map((n) => (typeof n === "string" ? n : isValidElement(n) ? textOf((n as ReactElement<{ children?: ReactNode }>).props.children) : "")).join("");
}

const ELEMENTS: Components = {
  // The page's own heading is the chat's; a reply's headings sit below it.
  h1: "h3", h2: "h3", h3: "h4", h4: "h5", h5: "h5", h6: "h5",
  a: ({ children }) => <>{children}</>,
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
