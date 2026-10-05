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

1. Already installed? Run \`command -v trackline\` and, if it prints a path,
   \`trackline help\`. If both work, skip to step 4: do not install over it.
   trackline may have been installed another way, and npm refuses to
   overwrite that install.
2. Check Node: \`node --version\` must be 20 or newer. If it is not, stop and
   tell the user.
3. Install: \`npm install -g trackline\`. If this fails with a permission
   error, do not use sudo. Tell the user and let them choose how to fix it.
4. From the project root, wire trackline into the agent you are:
   - Claude Code: \`trackline init\`
   - Codex: \`trackline init --host codex\`
   - Cursor: \`trackline init --host cursor\`
   It merges into the agent's existing settings and leaves the rest alone.
5. Check it: \`trackline doctor --host <claude|codex|cursor>\`. Show the user
   the output.
6. Put the project on the user's dashboard at ${SITE}/app. Without this step
   the app stays empty. First ask the user one question, and wait for the
   answer; never answer it for them:
   "Should your trackline dashboard (at ${SITE}) include our conversation,
   meaning your messages to me and my replies, or only trackline's findings
   about what I did?"
   Then run \`trackline connect --share\` if they want the conversation
   included, or \`trackline connect --no-requests\` if not. If this computer
   is not linked to their account yet, connect prints a link and a code and
   exits: show both to the user, and once they say they have approved it in
   their browser, run the same command again to finish. If the user does not
   want a dashboard, skip this step; trackline works fully without one.
7. Tell the user, in plain words:
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
