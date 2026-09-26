/**
 * Example harness config, in plain JavaScript.
 *
 * Copy to `harness.config.mjs` and fill in the three functions.
 *
 * Prefer this over the TypeScript example if you are on Node 20, or on Node 22
 * before 22.18: those cannot import a `.ts` config at all. This one loads
 * everywhere, which is also why CI uses it to exercise the live path.
 *
 * The same three functions as `harness.config.example.ts`, without the types.
 */

/** Chunk ids in rank order, best first, in the id scheme your dataset uses. */
async function retriever(query, { topK, segments }) {
  // return myVectorStore.search(query, { topK, segments });
  return [];
}

/** Ask the agent what it WOULD call. It must not execute anything. */
async function toolSelector(testCase) {
  // const plan = await myAgent.plan(testCase.utterance, testCase.available);
  // return { called: plan.tools, refused: plan.refused };
  return { called: [], refused: false };
}

/** Run all turns through one eval session and return calls for each turn. */
async function multiTurnToolSelector(testCase) {
  // const session = await startEvalSession();
  // const turns = [];
  // for (const turn of testCase.turns) {
  //   const plan = await session.plan(turn.utterance, turn.available);
  //   turns.push({ called: plan.tools, refused: plan.refused });
  // }
  // return { turns };
  return { turns: testCase.turns.map(() => ({ called: [], refused: false })) };
}

/**
 * Risk tier for a tool name, read from your live registry.
 *
 * Return undefined for a name you do not recognise: the scorer counts only
 * tiers it can resolve, so an unknown name reads as a selection failure, which
 * it is, and never as a fake safety pass.
 */
const demo = {
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

function toolCatalog(name) {
  // return getTool(name)?.risk;
  return demo[name];
}

export default { retriever, toolSelector, multiTurnToolSelector, toolCatalog };
