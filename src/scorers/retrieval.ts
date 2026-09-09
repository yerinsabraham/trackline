/**
 * Retrieval scorers.
 *
 * These are deterministic and need no model, which is the point: they run on
 * every commit for free, and they are the metrics that move when somebody
 * changes chunk size, the embedding model, `minScore`, the authority
 * precedence order, or a Qdrant filter.
 *
 * Recall@k is the primary gate. A generation failure caused by missing context
 * is indistinguishable from a hallucination once it reaches the customer, and
 * recall is the only place it is visible as its own number.
 */

/** Fraction of the relevant chunks that appear anywhere in the top k. */
export function recallAtK(retrieved: string[], relevant: string[], k: number): number {
  if (relevant.length === 0) return 1;
  const top = new Set(retrieved.slice(0, k));
  const hits = relevant.filter((id) => top.has(id)).length;
  return hits / relevant.length;
}

/** Fraction of the top k that is relevant. Falls as retrieval pads the context window. */
export function precisionAtK(retrieved: string[], relevant: string[], k: number): number {
  const top = retrieved.slice(0, k);
  if (top.length === 0) return 0;
  const rel = new Set(relevant);
  return top.filter((id) => rel.has(id)).length / top.length;
}

/**
 * Reciprocal rank of the first relevant chunk.
 *
 * Position matters beyond presence: a model reads the top of its context far
 * more reliably than the bottom, so a correct chunk sitting at rank 8 is worth
 * less than the same chunk at rank 1 even though recall cannot tell them apart.
 */
export function reciprocalRank(retrieved: string[], relevant: string[]): number {
  const rel = new Set(relevant);
  const idx = retrieved.findIndex((id) => rel.has(id));
  return idx === -1 ? 0 : 1 / (idx + 1);
}

/**
 * Normalised discounted cumulative gain over graded relevance.
 *
 * Used where "relevant" is not binary — the canonical policy page and a passing
 * mention of the same topic are both relevant, and a ranking that puts the
 * mention first is worse. `grades` carries that; anything ungraded but listed
 * as relevant counts as 1.
 */
export function ndcgAtK(
  retrieved: string[],
  relevant: string[],
  k: number,
  grades: Record<string, number> = {},
): number {
  const gradeOf = (id: string) => grades[id] ?? (relevant.includes(id) ? 1 : 0);
  const discount = (i: number) => Math.log2(i + 2);

  const dcg = retrieved
    .slice(0, k)
    .reduce((sum, id, i) => sum + gradeOf(id) / discount(i), 0);

  const ideal = [...new Set([...relevant, ...Object.keys(grades)])]
    .map(gradeOf)
    .sort((a, b) => b - a)
    .slice(0, k)
    .reduce((sum, g, i) => sum + g / discount(i), 0);

  return ideal === 0 ? 1 : dcg / ideal;
}

/**
 * A retrieval that returned nothing at all.
 *
 * Tracked separately because it is a different bug from a bad ranking, and it
 * is the one that has actually happened here: a `segments match any [...]`
 * clause matched neither an empty array nor a missing field and silently hid
 * every document indexed before tagging existed. Recall averaged over a suite
 * degrades gently in that situation. This number goes straight to 1.
 */
export function emptyRate(results: { retrieved: string[] }[]): number {
  if (results.length === 0) return 0;
  return results.filter((r) => r.retrieved.length === 0).length / results.length;
}
