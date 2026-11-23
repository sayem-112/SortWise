import type { Vectors } from "./search";

/* The library map, as the app's semantic package draws it
   (internal/semantic/layout.go and neighbours.go): posts start on an island
   per category, then each is pulled toward its closest posts and pushed from
   a few at random, as UMAP lays out a data map. A post whose closest posts
   are mostly in other categories is marked fringe: mixed ground. */

export type MapPoint = { id: number; x: number; y: number; fringe?: boolean };
export type MapIsland = { category: number; x: number; y: number; radius: number };

const ROUNDS = 60;
const SETTLED = 1e-7;
const WIDTH = 1.7;

function dot(a: Float64Array, b: Float64Array) {
  let sum = 0;
  for (let index = 0; index < a.length; index++) sum += a[index] * b[index];
  return sum;
}

function toUnit(vector: Float64Array, after?: Float64Array) {
  if (after) {
    const along = dot(vector, after);
    for (let index = 0; index < vector.length; index++) vector[index] -= along * after[index];
  }
  const length = Math.sqrt(dot(vector, vector));
  if (!length) return false;
  for (let index = 0; index < vector.length; index++) vector[index] /= length;
  return true;
}

/* The direction of greatest variance, at right angles to after, by power
   iteration from a fixed start; its largest entry is made positive so the
   map never flips. */
function component(rows: Float64Array[], size: number, after?: Float64Array) {
  let direction = new Float64Array(size).map((_, index) => 1 + (index % 7));
  toUnit(direction, after);
  for (let round = 0; round < ROUNDS; round++) {
    const next = new Float64Array(size);
    for (const row of rows) {
      const weight = dot(row, direction);
      for (let index = 0; index < size; index++) next[index] += weight * row[index];
    }
    if (!toUnit(next, after)) break;
    const settled = Math.abs(dot(next, direction)) > 1 - SETTLED;
    direction = next;
    if (settled) break;
  }
  let largest = 0;
  direction.forEach((value, index) => {
    if (Math.abs(value) > Math.abs(direction[largest])) largest = index;
  });
  if (direction[largest] < 0) direction = direction.map((value) => -value);
  return direction;
}

type Island = { group: number; members: { id: number; row: Float64Array }[]; centre: Float64Array; x: number; y: number; radius: number };

export function archipelago(vectors: Vectors, groups: Map<number, number>) {
  const byGroup = new Map<number, Island>();
  let size = 0;
  for (const [id, vector] of vectors) {
    const group = groups.get(id);
    const length = Math.sqrt(vector.reduce((sum, value) => sum + value * value, 0));
    if (group === undefined || !length) continue;
    size = vector.length;
    const row = Float64Array.from(vector, (value) => value / length);
    let island = byGroup.get(group);
    if (!island) {
      island = { group, members: [], centre: new Float64Array(size), x: 0, y: 0, radius: 0 };
      byGroup.set(group, island);
    }
    island.members.push({ id, row });
    row.forEach((value, index) => (island!.centre[index] += value));
  }
  const order = [...byGroup.values()].sort((a, b) => b.members.length - a.members.length || a.group - b.group);
  if (!order.length) return { points: [] as MapPoint[], islands: [] as MapIsland[], aspect: 1 / WIDTH };
  for (const island of order) {
    island.centre = island.centre.map((value) => value / island.members.length);
    island.radius = Math.sqrt(island.members.length);
  }

  // Islands start where their average meanings fall, weighted by size.
  if (order.length > 1) {
    const total = order.reduce((sum, island) => sum + island.members.length, 0);
    const mean = new Float64Array(size);
    for (const island of order) island.centre.forEach((value, index) => (mean[index] += island.members.length * value));
    const centred = order.map((island) => island.centre.map((value, index) => value - mean[index] / total));
    const weighted = centred.map((row, index) => row.map((value) => value * Math.sqrt(order[index].members.length)));
    const first = component(weighted, size);
    const second = component(weighted, size, first);
    const xs = centred.map((row) => dot(row, first));
    const ys = centred.map((row) => dot(row, second));
    const spreadX = Math.max(1e-9, ...xs.map(Math.abs));
    const spreadY = Math.max(1e-9, ...ys.map(Math.abs));
    const reach = Math.sqrt(order.reduce((sum, island) => sum + island.radius ** 2, 0)) * 1.2;
    order.forEach((island, index) => {
      island.x = (WIDTH * reach * xs[index]) / spreadX;
      island.y = (reach * ys[index]) / spreadY;
    });
    // Pushed apart until none overlap, with a gentle pull to the middle.
    for (let round = 0; round < 400; round++) {
      let moved = false;
      for (let i = 0; i < order.length; i++) {
        for (let j = i + 1; j < order.length; j++) {
          const a = order[i];
          const b = order[j];
          const need = a.radius + b.radius + 0.18 * (a.radius + b.radius) + 1;
          let dx = b.x - a.x;
          let dy = b.y - a.y;
          let distance = Math.hypot(dx, dy);
          if (distance >= need) continue;
          if (distance < 1e-9) {
            const angle = (i * 7 + j * 13) * 0.618;
            dx = Math.cos(angle);
            dy = Math.sin(angle);
            distance = 1;
          }
          const push = (need - distance) / distance;
          const share = a.radius ** 2 / (a.radius ** 2 + b.radius ** 2);
          a.x -= dx * push * (1 - share);
          a.y -= dy * push * (1 - share);
          b.x += dx * push * share;
          b.y += dy * push * share;
          moved = true;
        }
      }
      for (const island of order) {
        island.x *= 0.995;
        island.y *= 0.995;
      }
      if (!moved && round > 20) break;
    }
  }

  // On each island: by how posts differ from its average, in their natural
  // shape, scaled so most sit well inside and eased so none fall off.
  const placed: { id: number; x: number; y: number; row: Float64Array; group: number }[] = [];
  for (const island of order) {
    const count = island.members.length;
    if (count === 1) {
      placed.push({ id: island.members[0].id, x: island.x, y: island.y, row: island.members[0].row, group: island.group });
      continue;
    }
    const rows = island.members.map((member) => member.row.map((value, index) => value - island.centre[index]));
    const first = component(rows, size);
    const second = component(rows, size, first);
    const spots = rows.map((row) => {
      const x = dot(row, first);
      const y = dot(row, second);
      return { x, y, distance: Math.hypot(x, y) };
    });
    const sorted = spots.map((spot) => spot.distance).sort((a, b) => a - b);
    const typical = sorted[Math.floor(((sorted.length - 1) * 85) / 100)] || 1;
    // A small group stays close together; from about sixty posts it fills its island.
    const reach = island.radius * (0.45 + 0.55 * Math.min(1, count / 60));
    spots.forEach((spot, index) => {
      const scale = spot.distance ? (reach * Math.tanh(spot.distance / typical)) / spot.distance : 0;
      placed.push({ id: island.members[index].id, x: island.x + spot.x * scale, y: island.y + spot.y * scale, row: island.members[index].row, group: island.group });
    });
  }

  const near = nearestPosts(placed.map((point) => point.row));
  relax(placed, near);
  // In its category's cluster when at least a third of its closest posts that
  // could share its category do; a small category can't fill every place.
  const sizes = new Map<number, number>();
  for (const point of placed) sizes.set(point.group, (sizes.get(point.group) ?? 0) + 1);
  const fringe = near.map((list, index) => {
    const possible = Math.min(list.length, (sizes.get(placed[index].group) ?? 1) - 1);
    return possible === 0 || list.filter((other) => placed[other].group === placed[index].group).length * 3 < possible;
  });

  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const point of placed) {
    minX = Math.min(minX, point.x);
    maxX = Math.max(maxX, point.x);
    minY = Math.min(minY, point.y);
    maxY = Math.max(maxY, point.y);
  }
  const margin = 0.03 * Math.max(maxX - minX, maxY - minY);
  minX -= margin;
  minY -= margin;
  maxX += margin;
  maxY += margin;
  const width = Math.max(maxX - minX, 1e-9);
  const round = (value: number) => Math.round(value * 10000) / 10000;
  const points = placed.map((point, index) => ({ id: point.id, x: round((point.x - minX) / width), y: round((point.y - minY) / width), ...(fringe[index] ? { fringe: true } : {}) }));
  const median = (values: number[]) => [...values].sort((a, b) => a - b)[Math.floor(values.length / 2)] ?? 0;
  const islands = order.map((island) => {
    let mine = points.filter((point, index) => placed[index].group === island.group && !fringe[index]);
    if (!mine.length) mine = points.filter((_, index) => placed[index].group === island.group);
    const cx = median(mine.map((point) => point.x));
    const cy = median(mine.map((point) => point.y));
    return { category: island.group, x: round(cx), y: round(cy), radius: round(median(mine.map((point) => Math.hypot(point.x - cx, point.y - cy))) * 1.5) };
  });
  return { points, islands, aspect: round((maxY - minY) / width) };
}

// The most closest posts each post is pulled toward; a small library uses a quarter of its posts.
const NEIGHBOURS = 12;
const EPOCHS = 150;
const PUSHES = 5;
const SPREAD = 10;

function nearestPosts(rows: Float64Array[]) {
  const k = Math.min(NEIGHBOURS, Math.max(2, Math.floor(rows.length / 4)), rows.length - 1);
  return rows.map((row, index) =>
    rows
      .map((other, at) => ({ at, score: at === index ? -Infinity : dot(row, other) }))
      .sort((a, b) => b.score - a.score)
      .slice(0, Math.max(0, k))
      .map((found) => found.at),
  );
}

/* A small seeded random number generator, so the same library always gives the same map. */
function seeded(seed: number) {
  let state = seed;
  return () => {
    state = (state * 1103515245 + 12345) % 2147483648;
    return state / 2147483648;
  };
}

/* UMAP's attraction along neighbour pairs and repulsion from random posts,
   with a learning rate that cools to nothing. */
function relax(points: { x: number; y: number }[], near: number[][]) {
  const count = points.length;
  if (count < 3) return;
  let minX = Infinity;
  let maxX = -Infinity;
  let minY = Infinity;
  let maxY = -Infinity;
  for (const point of points) {
    minX = Math.min(minX, point.x);
    maxX = Math.max(maxX, point.x);
    minY = Math.min(minY, point.y);
    maxY = Math.max(maxY, point.y);
  }
  const scale = (2 * SPREAD) / Math.max(maxX - minX, maxY - minY, 1e-9);
  for (const point of points) {
    point.x = (point.x - (minX + maxX) / 2) * scale;
    point.y = (point.y - (minY + maxY) / 2) * scale;
  }
  const clip = (value: number) => Math.max(-4, Math.min(4, value));
  const random = seeded(11);
  for (let epoch = 0; epoch < EPOCHS; epoch++) {
    const rate = 1 - epoch / EPOCHS;
    near.forEach((list, index) => {
      const me = points[index];
      for (const other of list) {
        const them = points[other];
        const dx = me.x - them.x;
        const dy = me.y - them.y;
        const distance = dx * dx + dy * dy;
        if (distance > 0) {
          const pull = -2 / (1 + distance);
          me.x += rate * clip(pull * dx);
          me.y += rate * clip(pull * dy);
          them.x -= rate * clip(pull * dx);
          them.y -= rate * clip(pull * dy);
        }
        for (let push = 0; push < PUSHES; push++) {
          const far = Math.floor(random() * count);
          if (far === index) continue;
          const fx = me.x - points[far].x;
          const fy = me.y - points[far].y;
          const gap = fx * fx + fy * fy;
          const repel = 2 / ((0.01 + gap) * (1 + gap));
          me.x += rate * clip(repel * fx);
          me.y += rate * clip(repel * fy);
        }
      }
    });
  }
}

