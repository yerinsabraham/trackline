// What trackline cannot do, named plainly: a tool that looks complete stops
// getting better. The evidence page shows these.

export const LIMITS: [string, string][] = [
  ["In production it alerts, it cannot stop.", "A trace is a record of what already happened."],
  ["Production needs content capture on.", "Without it, a trace does not say which tool was called, and trackline reports that rather than guessing."],
  ["Scope stays quiet when your request names no file.", "It will not invent a scope you did not state."],
  ["Through MCP, the agent chooses whether to ask.", "An agent that does not ask is not watched."],
  ["Production has not yet met real traffic.", "It was tested on a stream sent by the real OpenTelemetry libraries, with scripted conversations."],
  ["From your phone, your laptop has to be awake.", "The task runs on your own machine. If it is offline, a message waits up to 12 hours, and the app says so."],
  ["Codex takes tasks from the phone only once its hook is trusted.", "Codex runs a project's hook only after you trust it, and trackline will not start an agent it cannot watch."],
  ["Cursor cannot take tasks from the phone yet.", "trackline watches Cursor on your laptop as usual."],
];
