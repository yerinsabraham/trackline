import data from "@/data/hosts.json";

// A copy of engine/internal/hosts, kept current by a Go test
// (go test ./internal/hosts -update-site). The engine is the source of truth
// for what trackline can do in each agent; these pages only show it.
export type Host = {
  key: string;
  name: string;
  blocks: boolean;
  reasonReachesAgent: boolean;
  intent: string;
  evidence: string;
  limits: string[];
};

type Extra = { slug: string; title: string; setup: string[]; guide: string; summary: string };

// What the engine's table does not hold: the address, and how to set it up.
const EXTRA: Record<string, Extra> = {
  claude: {
    slug: "claude-code", title: "Claude Code", guide: "install",
    summary: "trackline runs as a Claude Code hook: it sees every action before it runs, and can stop one and tell the agent why.",
    setup: ["npm install -g trackline", "trackline init", "trackline doctor --host claude"],
  },
  codex: {
    slug: "codex", title: "Codex", guide: "install",
    summary: "trackline runs as a Codex hook: it sees every action before it runs, and can stop one and tell the agent why.",
    setup: ["npm install -g trackline", "trackline init --host codex", "trackline doctor --host codex"],
  },
  cursor: {
    slug: "cursor", title: "Cursor", guide: "install",
    summary: "trackline runs as a Cursor hook: it sees every action before it runs, and can stop one and tell the agent why.",
    setup: ["npm install -g trackline", "trackline init --host cursor", "trackline doctor --host cursor"],
  },
  mcp: {
    slug: "mcp", title: "Any MCP client", guide: "mcp",
    summary: "trackline runs as an MCP server the agent can ask before it acts. It advises: it cannot stop an agent that does not ask.",
    setup: ["npm install -g trackline", "trackline mcp --root /path/to/your/project"],
  },
  production: {
    slug: "production", title: "Production traces", guide: "production",
    summary: "trackline reads the OpenTelemetry traces a deployed agent already sends. It alerts after the fact: a trace cannot be stopped.",
    setup: ["npm install -g trackline", "trackline serve"],
  },
};

export type Agent = Host & Extra;

export function agents(): Agent[] {
  return (data as Host[]).map((h) => ({ ...h, ...EXTRA[h.key] }));
}

export function agent(slug: string): Agent | undefined {
  return agents().find((a) => a.slug === slug);
}
