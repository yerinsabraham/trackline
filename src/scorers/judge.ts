/**
 * LLM-as-judge — groundedness only.
 *
 * Everything that can be scored by code is scored by code. A judge is reached
 * for once, for the one question set membership cannot answer: is every claim
 * in this answer supported by the context it was given.
 *
 * Three rules keep a judge honest, and all three are load-bearing:
 *
 *  1. **It never sees the expected label.** The dataset knows whether a row is
 *     grounded; the judge is asked cold and the caller compares. A judge shown
 *     the answer agrees with it.
 *  2. **It judges one claim-set against one context, not "quality".** Asking a
 *     model whether an answer is good returns its taste. Asking whether a
 *     sentence is supported by a passage returns something checkable.
 *  3. **It may abstain.** `uncertain` is a real verdict and is counted
 *     separately rather than rounded to pass or fail — a judge forced to choose
 *     invents a reason, and the abstain rate is itself the signal that a row is
 *     badly written.
 *
 * The judge is a different model from the one under test wherever possible.
 * Self-judging inflates every number it touches.
 */

import OpenAI from 'openai';
import type { GroundednessCase } from '../types.js';

export const DEFAULT_JUDGE_MODEL = 'gpt-4o-mini';

export type Verdict = 'grounded' | 'unsupported' | 'uncertain';

export interface JudgeResult {
  verdict: Verdict;
  /** The specific sentence the judge could not support. Empty when grounded. */
  unsupportedClaim: string;
  reason: string;
}

const SYSTEM = `You check whether an answer is supported by the context it was given.

You are NOT judging whether the answer is helpful, well written, or correct in
the real world. You are judging one thing: does every factual claim in the
answer follow from the context passages.

Rules:
- A claim not present in the context is unsupported, even if it is true.
- Numbers, dates, prices, names and policy specifics must match the context exactly.
- Hedging or an explicit "I don't have that information" is grounded, not unsupported.
- General conversational filler carries no claims and does not make an answer unsupported.
- If the answer is ambiguous, or the context is too thin to tell, say uncertain.

Return the single sentence that is unsupported, verbatim, or "" when grounded.`;

const SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['verdict', 'unsupportedClaim', 'reason'],
  properties: {
    verdict: { type: 'string', enum: ['grounded', 'unsupported', 'uncertain'] },
    unsupportedClaim: { type: 'string' },
    reason: { type: 'string' },
  },
} as const;

let client: OpenAI | null = null;

function getClient(): OpenAI {
  if (!client) {
    const apiKey = process.env.OPENAI_API_KEY;
    if (!apiKey) throw new Error('OPENAI_API_KEY is required to run the groundedness suite');
    client = new OpenAI({ apiKey });
  }
  return client;
}

export async function judgeGroundedness(
  testCase: GroundednessCase,
  model = process.env.EVAL_JUDGE_MODEL ?? DEFAULT_JUDGE_MODEL,
): Promise<JudgeResult> {
  const context = testCase.context.map((c, i) => `[${i + 1}] ${c}`).join('\n\n');

  const response = await getClient().chat.completions.create({
    model,
    // Deterministic as the API allows. A judge that scores differently on a
    // rerun makes every regression unreadable.
    temperature: 0,
    messages: [
      { role: 'system', content: SYSTEM },
      {
        role: 'user',
        content: `CONTEXT\n${context}\n\nQUESTION\n${testCase.question}\n\nANSWER\n${testCase.answer}`,
      },
    ],
    response_format: {
      type: 'json_schema',
      json_schema: { name: 'groundedness', strict: true, schema: SCHEMA },
    },
  });

  const raw = response.choices[0]?.message?.content;
  if (!raw) throw new Error(`judge returned no content for ${testCase.id}`);

  const parsed = JSON.parse(raw) as JudgeResult;
  return parsed;
}

/**
 * Agreement between the judge and the dataset label.
 *
 * `uncertain` is never counted as agreement. It is surfaced as its own rate so
 * a suite drowning in abstentions reads as a broken dataset rather than a
 * failing model.
 */
export function judgeAgrees(verdict: Verdict, expected: GroundednessCase['expect']): boolean {
  return verdict === expected;
}
