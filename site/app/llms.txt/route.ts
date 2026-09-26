import { agents } from "@/lib/hosts";
import { experiments } from "@/lib/experiments";
import { pages } from "@/lib/guide";
import { SITE } from "@/lib/site";

export const dynamic = "force-static";

// The guide for AI agents, in the llms.txt convention: what trackline is, and a
// link to a Markdown copy of every page worth reading.
export function GET() {
  const body = `# trackline

> trackline checks whether an AI agent is still doing what it was asked. It runs beside coding agents (Claude Code, Codex, Cursor, any MCP client) through hooks, and over production agents' OpenTelemetry traces. It compares each action with the request, the project's rules files and a tool policy, and tells the user, or the agent, when they part ways.

Neutral by construction: an open engine that runs on the user's machine, the same way for every agent. Without an account it sends nothing anywhere.

With an optional account, a dashboard scores how on track each agent is, and the user can send tasks to Claude Code or Codex on their own laptop from their phone, watch them, and allow once or keep blocked what trackline stopped. Every task is signed with the user's passkey and checked on the laptop.

To install it for a user in their project, follow ${SITE}/install.md

## Docs

${pages().map((p) => `- [${p.title}](${SITE}/docs/${p.slug}.md): ${p.description}`).join("\n")}

## What it can do in each agent

${agents().map((a) => `- [${a.title}](${SITE}/agents/${a.slug}): ${a.summary}`).join("\n")}

## Evidence

${experiments().map((e) => `- [${e.title}](${SITE}/evidence/${e.slug}.md): ${e.answer}`).join("\n")}

## Optional

- [Changelog](${SITE}/changelog)
- [Privacy](${SITE}/privacy)
`;
  return new Response(body, { headers: { "content-type": "text/plain; charset=utf-8" } });
}
