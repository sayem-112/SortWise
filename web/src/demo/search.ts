/* Searching for the demo, done the way the app's database does it: every
   word must match the start of a word in the post (one-letter words match
   whole), best matches first, with excerpts that mark the words found. */

export const MATCH_START = String.fromCharCode(2);
export const MATCH_END = String.fromCharCode(3);

const WORD = /[\p{L}\p{N}\p{Mn}]+/gu;

// Lowercase, without accents, as the database's index compares words.
function fold(value: string) {
  return value.normalize("NFKD").replace(/\p{Mn}/gu, "").toLowerCase();
}

export function terms(query: string) {
  return [...new Set((fold(query).match(WORD) || []).filter(Boolean))];
}

function hit(word: string, term: string) {
  return term.length === 1 ? word === term : word.startsWith(term);
}

export function matches(text: string, query: string) {
  const wanted = terms(query);
  if (!wanted.length) return false;
  const words = fold(text).match(WORD) || [];
  return wanted.every((term) => words.some((word) => hit(word, term)));
}

/* The posts containing every word, best first: rarer words count for more,
   and a word in a short post counts for more than in a long one. */
export function rankWords<T extends { id: number }>(posts: T[], query: string, textOf: (post: T) => string): T[] {
  const wanted = terms(query);
  if (!wanted.length) return [];
  const docs = posts.map((post) => ({ post, words: fold(textOf(post)).match(WORD) || [] }));
  const found = docs.filter((doc) => wanted.every((term) => doc.words.some((word) => hit(word, term))));
  const average = docs.reduce((sum, doc) => sum + doc.words.length, 0) / Math.max(docs.length, 1);
  const rarity = new Map(wanted.map((term) => {
    const holding = docs.filter((doc) => doc.words.some((word) => hit(word, term))).length;
    return [term, Math.log(1 + (docs.length - holding + 0.5) / (holding + 0.5))];
  }));
  const score = (words: string[]) =>
    wanted.reduce((sum, term) => {
      const count = words.filter((word) => hit(word, term)).length;
      return sum + (rarity.get(term)! * count * 2.2) / (count + 1.2 * (0.25 + (0.75 * words.length) / average));
    }, 0);
  return found
    .map((doc) => ({ post: doc.post, score: score(doc.words) }))
    .sort((a, b) => b.score - a.score || b.post.id - a.post.id)
    .map((item) => item.post);
}

/* An excerpt of about `size` words around the first match, with matched
   words marked. Without a match it is the start of the text. */
export function snippet(text: string, query: string, size: number) {
  const wanted = terms(query);
  const words = [...text.matchAll(WORD)].map((match) => ({ start: match.index!, end: match.index! + match[0].length, folded: fold(match[0]) }));
  if (!words.length) return text.slice(0, 200);
  const marked = (index: number) => wanted.some((term) => hit(words[index].folded, term));
  const first = words.findIndex((_, index) => marked(index));
  const from = first < 0 ? 0 : Math.max(0, Math.min(first - Math.floor(size / 3), words.length - size));
  const to = Math.min(words.length, from + size);
  let excerpt = "";
  let cursor = words[from].start;
  for (let index = from; index < to; index++) {
    const word = words[index];
    excerpt += text.slice(cursor, word.start);
    const value = text.slice(word.start, word.end);
    excerpt += marked(index) ? `${MATCH_START}${value}${MATCH_END}` : value;
    cursor = word.end;
  }
  const clean = excerpt.replace(/\s+/g, " ").trim();
  return `${from > 0 ? "…" : ""}${clean}${to < words.length ? "…" : ""}`;
}

export type Vectors = Map<number, Int8Array>;
export type Hit = { id: number; score: number };

// How far above the library's typical closeness a post must be, in units of
// the usual spread, and the fewest posts that is judged from. The app asks
// for 3 spreads; with a library of 80 posts instead of thousands, chance
// alone reaches less far, so the demo asks for 2.5.
const STAND_OUT = 2.5;
const MIN_FOR_SPREAD = 8;

/* The posts about the same thing as the vector, closest first, leaving out
   the posts in `skip`. A post must be within `margin` of the closest one and stand clear
   of how close posts in this library typically are, so a search the library
   has nothing on finds nothing rather than its least bad posts. */
export function nearest(vectors: Vectors, query: Float32Array, limit: number, skip: Set<number>, margin: number, hubs?: Map<number, number>): Hit[] {
  let queryLength = 0;
  for (const value of query) queryLength += value * value;
  queryLength = Math.sqrt(queryLength);
  if (!queryLength) return [];
  const hits: Hit[] = [];
  for (const [id, vector] of vectors) {
    if (skip.has(id) || vector.length !== query.length) continue;
    let dot = 0;
    let length = 0;
    for (let index = 0; index < vector.length; index++) {
      dot += vector[index] * query[index];
      length += vector[index] * vector[index];
    }
    // A post close to everything (often one with little text) is held back
    // by how close it typically is to its neighbours.
    if (length) hits.push({ id, score: dot / (queryLength * Math.sqrt(length)) - (hubs?.get(id) ?? 0) / 2 });
  }
  if (!hits.length) return [];
  hits.sort((a, b) => b.score - a.score || b.id - a.id);
  const floor = hits[0].score - margin;
  let bar = -Infinity;
  if (hits.length >= MIN_FOR_SPREAD) {
    const typical = hits[Math.floor(hits.length / 2)].score;
    const distances = hits.map((item) => Math.abs(item.score - typical)).sort((a, b) => a - b);
    bar = typical + STAND_OUT * 1.4826 * distances[Math.floor(distances.length / 2)];
  }
  const kept: Hit[] = [];
  for (const item of hits) {
    if (item.score < floor || item.score <= bar || kept.length === limit) break;
    kept.push(item);
  }
  return kept;
}

/* How close each post is to its nearest few neighbours, on average. Posts
   close to everything score high; subtracting half of it from a match
   (cross-domain similarity local scaling) keeps them from turning up for
   every search. */
export function hubness(vectors: Vectors, neighbours = 5) {
  const entries = [...vectors];
  const lengths = entries.map(([, vector]) => Math.sqrt(vector.reduce((sum, value) => sum + value * value, 0)) || 1);
  const result = new Map<number, number>();
  entries.forEach(([id, vector], i) => {
    const scores: number[] = [];
    entries.forEach(([, other], j) => {
      if (i === j) return;
      let dot = 0;
      for (let index = 0; index < vector.length; index++) dot += vector[index] * other[index];
      scores.push(dot / (lengths[i] * lengths[j]));
    });
    scores.sort((a, b) => b - a);
    const top = scores.slice(0, neighbours);
    result.set(id, top.reduce((sum, value) => sum + value, 0) / Math.max(top.length, 1));
  });
  return result;
}
