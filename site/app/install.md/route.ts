import { page } from "@/lib/guide";
import { SITE } from "@/lib/site";

export const dynamic = "force-static";

// Written for the agent the user pasted the prompt into, not for a person. The
// steps a person would read are the install guide itself, appended below, so
// the two cannot drift apart.
const FOR_AGENTS = `# Installing trackline: instructions for a coding agent

You are a coding agent, and the user asked you to install trackline in the
project you are working in. trackline watches what a coding agent does and
checks it against what the user asked. Do the steps below, then report back.

1. Check Node: \`node --version\` must be 20 or newer. If it is not, stop and
   tell the user.
2. Install: \`npm install -g trackline\`. If this fails with a permission
   error, do not use sudo. Tell the user and let them choose how to fix it.
3. From the project root, wire trackline into the agent you are:
   - Claude Code: \`trackline init\`
   - Codex: \`trackline init --host codex\`
   - Cursor: \`trackline init --host cursor\`
   It merges into the agent's existing settings and leaves the rest alone.
4. Check it: \`trackline doctor --host <claude|codex|cursor>\`. Show the user
   the output.
5. Tell the user, in plain words:
   - It starts in warn mode: it writes things down and never interrupts.
   - After your next edit, \`trackline status\` should show the hook has run.
     If it says the hook has never run, the setup is not working yet.
   - Codex only: they must run \`/hooks\` in Codex and approve trackline's
     hook, or it will never fire.
   - Cursor only: hooks load once the agent is started inside the project
     folder.

Do not change \`.trackline.json\` or any check's mode unless the user asks.
Everything else is in the docs: ${SITE}/docs

---

# The install guide
`;

export function GET() {
  const guide = page("install")?.markdown ?? "";
  return new Response(FOR_AGENTS + "\n" + guide, {
    headers: { "content-type": "text/markdown; charset=utf-8" },
  });
}
