import type { Vectors } from "./search";

/* The library map, as the app's semantic package works it out
   (internal/semantic/layout.go): each post placed along the two directions
   in which the library's vectors differ most, scaled into 0 to 1. */

export type MapPoint = { id: number; x: number; y: number };

const ROUNDS = 60;
const SETTLED = 1e-7;

function dot(a: Float64Array, b: Float64Array) {
  let sum = 0;
  for (let index = 0; index < a.length; index++) sum += a[index] * b[index];
  return sum;
}

/* Removes any part of vector along after, then scales it to length one. */
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

/* Values into 0 to 1 between the 2nd and 98th percentile; the few beyond sit on the edge. */
function spread(values: number[]) {
  const sorted = [...values].sort((a, b) => a - b);
  const low = sorted[Math.floor((sorted.length * 2) / 100)];
  const high = sorted[Math.floor(((sorted.length - 1) * 98) / 100)];
  return (value: number) => (high <= low ? 0.5 : Math.round(Math.min(1, Math.max(0, (value - low) / (high - low))) * 1000) / 1000);
}

export function project(vectors: Vectors): MapPoint[] {
  const entries = [...vectors];
  if (!entries.length) return [];
  const size = entries[0][1].length;
  const mean = new Float64Array(size);
  const rows = entries.map(([, vector]) => {
    const length = Math.sqrt(vector.reduce((sum, value) => sum + value * value, 0));
    const row = new Float64Array(size);
    if (length) vector.forEach((value, index) => (row[index] = value / length));
    row.forEach((value, index) => (mean[index] += value));
    return row;
  });
  mean.forEach((_, index) => (mean[index] /= rows.length));
  for (const row of rows) row.forEach((_, index) => (row[index] -= mean[index]));

  const first = component(rows, size);
  const second = component(rows, size, first);
  const xs = rows.map((row) => dot(row, first));
  const ys = rows.map((row) => dot(row, second));
  const scaleX = spread(xs);
  const scaleY = spread(ys);
  return entries.map(([id], index) => ({ id, x: scaleX(xs[index]), y: scaleY(ys[index]) }));
}
