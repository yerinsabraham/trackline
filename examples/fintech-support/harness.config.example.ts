/**
 * Copy to `harness.config.ts` and point it at your own system.
 *
 * Nothing here is needed for fixture mode, which is why a fresh clone can run
 * `npm run evals` immediately. You need this file the first time you run
 * `--live` or `--record`.
 *
 * `harness.config.ts` is gitignored on purpose: it reaches into your codebase
 * and usually holds environment-specific ids.
 */

import type { HarnessConfig, RiskTier } from './src/types.js';

/**
 * Your retriever.
 *
 * Return chunk ids in rank order, best first, in the SAME id scheme your golden
 * dataset uses. The reference datasets use `${sourceId}#${chunkIndex}`.
 *
 * Deliberately not a vector-store point id: a dataset should survive an
 * infrastructure change and break only when the documents do.
 */
async function retriever(
  query: string,
  opts: { topK: number; segments?: string[] },
): Promise<string[]> {
  // const results = await searchForContext(process.env.EVAL_ORG_ID!, query, opts.topK, {
  //   segments: opts.segments,
  // });
  // return results.map((r) => `${r.sourceId}#${r.chunkIndex}`);
  throw new Error('Wire up your retriever here.');
}

/**
 * Your agent, asked what it WOULD call. It must not execute anything.
 *
 * An eval that actually runs `freeze_card` is not an eval, and this suite is
 * built out of utterances designed to tempt a model into destructive calls.
 */
async function toolSelector(testCase: {
  utterance: string;
  available: string[];
}): Promise<{ called: string[]; refused: boolean }> {
  // const response = await openai.chat.completions.create({
  //   model: 'gpt-4o',
  //   temperature: 0,
  //   messages: [
  //     { role: 'system', content: YOUR_AGENT_SYSTEM_PROMPT },
  //     { role: 'user', content: testCase.utterance },
  //   ],
  //   tools: testCase.available.map(toOpenAITool),
  //   tool_choice: 'auto',
  // });
  //
  // const message = response.choices[0]?.message;
  // const called = (message?.tool_calls ?? []).map((c) => c.function.name);
  //
  // // A refusal is a text reply that took no action. Whether the wording is a
  // // good refusal is a question the groundedness suite is better at; what
  // // matters for safety is that nothing was called.
  // return { called, refused: called.length === 0 && Boolean(message?.content) };
  throw new Error('Wire up your agent here.');
}

/**
 * Your agent over a whole conversation.
 *
 * This is deliberately separate from `toolSelector`: evaluating memory by
 * replaying turns independently would miss the bug this suite exists to catch.
 * Run the turns through the same session and record what the agent would call
 * on each turn.
 */
async function multiTurnToolSelector(testCase: {
  turns: { utterance: string; available: string[] }[];
}): Promise<{ turns: { called: string[]; refused: boolean }[] }> {
  // const session = await startEvalSession();
  // const turns = [];
  // for (const turn of testCase.turns) {
  //   const response = await session.plan(turn.utterance, turn.available);
  //   turns.push({ called: response.tools, refused: response.refused });
  // }
  // return { turns };
  throw new Error('Wire up your multi-turn agent here.');
}

/**
 * Risk tier for a tool name, read from your live registry rather than carried
 * in the dataset, so retiering a tool is reflected here immediately instead of
 * silently disagreeing with production.
 *
 * Return undefined for a tool you do not recognise. The scorer counts only
 * tiers it can resolve, so an unknown name reads as a selection failure, which
 * it is, and never as a fake safety pass.
 */
function toolCatalog(name: string): RiskTier | undefined {
  // return getTool(name)?.risk;
  const demo: Record<string, RiskTier> = {
    kb_search: 'read_public',
    get_customer_profile: 'read_private',
    escalate_to_human: 'safe_write',
    set_conversation_summary: 'safe_write',
    suggest_replies: 'safe_write',
    flag_knowledge_gap: 'safe_write',
    create_ticket: 'customer_confirm',
    schedule_callback: 'customer_confirm',
    fintech_transaction_status: 'read_private',
    fintech_kyc_status: 'read_private',
    fintech_freeze_card: 'customer_confirm',
    fintech_switch_plan: 'customer_confirm',
    fintech_unfreeze_card: 'step_up',
    fintech_report_card_lost: 'step_up',
    fintech_open_dispute: 'step_up',
    fintech_change_limit: 'step_up',
  };
  return demo[name];
}

const config: HarnessConfig = { retriever, toolSelector, multiTurnToolSelector, toolCatalog };

export default config;
