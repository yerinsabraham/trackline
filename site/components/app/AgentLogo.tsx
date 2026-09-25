import { ClaudeLogo, CursorLogo, OpenAILogo } from "@/components/HostLogos";

// Each agent by its own mark, everywhere the app names one, so Claude Code,
// Codex and Cursor are told apart at a glance. The marks are the vendors'
// (Simple Icons, CC0), shown to say which agent did the work.

const NAMES: Record<string, string> = {
  "claude-code": "Claude Code", claude: "Claude Code", codex: "Codex", cursor: "Cursor",
};

export function agentName(host: string | null | undefined): string {
  return (host && NAMES[host]) ?? host ?? "Agent";
}

export default function AgentLogo({ host, size = 28 }: { host: string | null | undefined; size?: number }) {
  const h = host === "claude" ? "claude-code" : host;
  const mark =
    h === "claude-code" ? <ClaudeLogo /> :
    h === "codex" ? <OpenAILogo color="#111112" /> :
    h === "cursor" ? <CursorLogo color="#111112" /> : null;
  return (
    <span className={`agent-logo agent-${h ?? "unknown"}`} style={{ width: size, height: size }} title={agentName(host)}>
      {mark}
    </span>
  );
}
