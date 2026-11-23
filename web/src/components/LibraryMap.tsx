import { useQuery } from "@tanstack/react-query";
import { Maximize, Minus, Plus } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { getBookmark, getCategories, getLibraryMap, type MapPoint } from "../lib/api";
import { colorFor, formatDate, postPreview, type OptionColor } from "../lib/format";
import { usePeek } from "../lib/peek";
import { AuthorAvatar } from "./Author";
import { Option } from "./ui";

// Room around the islands, so names on their rims aren't cut off.
const PAD = 28;
// A dot's radius, from the room each post has: about an eighth of the usual
// gap between posts, within these bounds at the starting zoom. It grows a
// little as you zoom in.
const DOT_MIN = 2;
const DOT_MAX = 4.5;
// Posts in mixed ground, between clusters, are drawn in this quiet grey.
const FRINGE = "#6f6f6f";
// How near the pointer has to be to a dot, in pixels, to pick it.
const REACH = 9;
const MIN_ZOOM = 1;
const MAX_ZOOM = 10;
// A group with fewer clustered posts than this goes unnamed (hover its dots
// instead); a small library needs fewer, down to 3.
const LABEL_MIN_POSTS = 8;
// The legend lists this many categories before "Show all".
const LEGEND_SHOWN = 10;
// The map's height follows its shape, within these bounds.
const MIN_HEIGHT = 280;
const MAX_HEIGHT = 640;

type View = { scale: number; x: number; y: number };
// top: the y of the group's highest post, where its name goes.
// core: how many of its posts form its cluster, rather than sitting in mixed ground.
type Group = { id: number; name: string; color: OptionColor; count: number; core: number; cx: number; cy: number; radius: number; top: number };

/* The colors the rest of the app uses for an option, read from the styles so
   the map never drifts from them. */
function useOptionColors(host: React.RefObject<HTMLElement | null>, ready: boolean) {
  const [colors, setColors] = useState<Record<string, string>>({});
  useEffect(() => {
    if (!ready || !host.current) return;
    const probe = document.createElement("span");
    host.current.appendChild(probe);
    const found: Record<string, string> = {};
    for (const color of ["default", "gray", "brown", "orange", "yellow", "green", "blue", "purple", "pink", "red"]) {
      probe.className = `opt-${color}`;
      found[color] = getComputedStyle(probe).getPropertyValue("--opt-dot").trim() || "#9b9b9b";
    }
    probe.remove();
    setColors(found);
  }, [host, ready]);
  return colors;
}

/* The library as an archipelago: an island per main category, sized by its
   posts, with islands about related things near each other and, on each
   island, posts about the same thing together. Hover a dot to see the post,
   click to open it; click a category below to pick out its island. The view
   zooms with the buttons, Ctrl and the wheel, or a double click, and pans
   by dragging. */
export function LibraryMap() {
  const map = useQuery({ queryKey: ["library-map"], queryFn: getLibraryMap, staleTime: 60_000 });
  const categories = useQuery({ queryKey: ["categories"], queryFn: getCategories });
  const peek = usePeek();
  const frame = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [width, setWidth] = useState(0);
  const [view, setView] = useState<View>({ scale: 1, x: 0, y: 0 });
  const [hovered, setHovered] = useState<MapPoint | null>(null);
  const [focus, setFocus] = useState<number | null>(null);
  const [legendOpen, setLegendOpen] = useState(false);
  const drag = useRef<{ x: number; y: number; view: View; moved: boolean } | null>(null);
  // The soft glow under the dots, kept between hovers: it changes only with the view.
  const glow = useRef<{ key: string; layer: HTMLCanvasElement } | null>(null);

  const points = useMemo(() => map.data?.points ?? [], [map.data]);
  useEffect(() => {
    glow.current = null;
  }, [points]);
  // The frame exists only once there is a map to draw.
  const colors = useOptionColors(frame, points.length > 1 && !map.data?.notice);

  // The categories, biggest cluster first, with their names and colors.
  const groups = useMemo<Group[]>(() => {
    const names = new Map((categories.data?.items ?? []).map((item) => [item.id, item.name]));
    const counts = new Map<number, number>();
    const cores = new Map<number, number>();
    const tops = new Map<number, number>();
    for (const point of points) {
      counts.set(point.category, (counts.get(point.category) ?? 0) + 1);
      if (!point.fringe) cores.set(point.category, (cores.get(point.category) ?? 0) + 1);
      tops.set(point.category, Math.min(tops.get(point.category) ?? Infinity, point.y));
    }
    return (map.data?.islands ?? [])
      .map((island) => ({
        id: island.category,
        name: island.category ? names.get(island.category) ?? "Category" : "No category",
        color: island.category ? colorFor(island.category) : ("default" as const),
        count: counts.get(island.category) ?? 0,
        core: cores.get(island.category) ?? 0,
        cx: island.x,
        cy: island.y,
        radius: island.radius,
        top: tops.get(island.category) ?? island.y - island.radius,
      }))
      .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
  }, [points, map.data, categories.data]);
  // Labels go to the biggest clusters first.
  const byCore = useMemo(() => [...groups].sort((a, b) => b.core - a.core), [groups]);
  const aspect = map.data?.aspect || 0.5;
  // The frame is as wide as the page; its height follows the map's shape.
  const height = width ? Math.round(Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, (width - 2 * PAD) * aspect + 2 * PAD))) : MIN_HEIGHT;
  const size = useMemo(() => ({ width, height }), [width, height]);
  const groupOf = useMemo(() => new Map(groups.map((group) => [group.id, group])), [groups]);

  // Measured before the first paint, so the map never draws at a guessed size.
  useLayoutEffect(() => {
    if (!frame.current) return;
    setWidth(frame.current.clientWidth);
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    observer.observe(frame.current);
    return () => observer.disconnect();
  }, [map.data]);

  // One map unit in pixels, at the starting zoom: the map fills the width,
  // or the height when the frame is too short for it. Islands stay round.
  const unit = Math.max(1, Math.min(size.width - 2 * PAD, (size.height - 2 * PAD) / aspect));
  const offsetX = (size.width - unit) / 2;
  const offsetY = (size.height - unit * aspect) / 2;

  // Where a point of the map lands on screen, in CSS pixels inside the frame.
  const place = useCallback(
    (x: number, y: number) => ({
      left: (offsetX + x * unit) * view.scale + view.x,
      top: (offsetY + y * unit) * view.scale + view.y,
    }),
    [unit, offsetX, offsetY, view],
  );
  const gap = unit * Math.sqrt(aspect / Math.max(1, points.length));
  const dot = Math.min(DOT_MAX * 1.4, Math.min(DOT_MAX, Math.max(DOT_MIN, gap * 0.12)) * Math.sqrt(view.scale));

  // Keeps the posts in view: at the starting zoom nothing moves, and zoomed
  // in, the edges of the map can't be dragged past the frame.
  const settle = useCallback(
    (next: View): View => {
      const scale = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, next.scale));
      const minX = size.width * (1 - scale);
      const minY = size.height * (1 - scale);
      return { scale, x: Math.min(0, Math.max(minX, next.x)), y: Math.min(0, Math.max(minY, next.y)) };
    },
    [size],
  );

  const zoomAt = useCallback(
    (factor: number, cx = size.width / 2, cy = size.height / 2) => {
      setView((current) => {
        const scale = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, current.scale * factor));
        const ratio = scale / current.scale;
        return settle({ scale, x: cx - (cx - current.x) * ratio, y: cy - (cy - current.y) * ratio });
      });
    },
    [size, settle],
  );

  // Draws a soft glow that follows where the posts are, then every dot.
  useEffect(() => {
    const element = canvas.current;
    if (!element || !size.width || !Object.keys(colors).length) return;
    const ratio = window.devicePixelRatio || 1;
    element.width = Math.round(size.width * ratio);
    element.height = Math.round(size.height * ratio);
    const context = element.getContext("2d");
    if (!context) return;
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
    context.clearRect(0, 0, size.width, size.height);

    // The glow: a faint spot of color under each post in a cluster, blurred
    // together, so a cluster's dense core glows and its edge fades the way
    // its posts thin out. Mixed ground stays dark.
    const key = `${size.width}x${size.height}@${ratio}|${view.scale},${view.x},${view.y}|${focus}`;
    if (glow.current?.key !== key || !glow.current.layer) {
      const spots = document.createElement("canvas");
      spots.width = element.width;
      spots.height = element.height;
      const paint = spots.getContext("2d");
      const layer = document.createElement("canvas");
      layer.width = element.width;
      layer.height = element.height;
      const blurred = layer.getContext("2d");
      if (paint && blurred) {
        paint.setTransform(ratio, 0, 0, ratio, 0, 0);
        for (const point of points) {
          const lit = focus === null ? !point.fringe : focus === point.category;
          if (!lit) continue;
          const group = groupOf.get(point.category);
          const at = place(point.x, point.y);
          const reach = dot * 4;
          paint.globalAlpha = 0.11;
          paint.fillStyle = colors[group?.color ?? "default"];
          paint.beginPath();
          paint.arc(at.left, at.top, reach, 0, Math.PI * 2);
          paint.fill();
        }
        blurred.filter = `blur(${Math.round(12 * ratio)}px)`;
        blurred.drawImage(spots, 0, 0);
      }
      glow.current = { key, layer };
    }
    context.drawImage(glow.current.layer, 0, 0, size.width, size.height);

    // Grey mixed ground first, then the clusters' colored posts on top. A
    // picked-out category shows all its posts in color, everything else dim.
    const shown = (point: MapPoint) => (focus === null ? !point.fringe : focus === point.category);
    const order = [...points.filter((point) => !shown(point)), ...points.filter(shown)];
    for (const point of order) {
      const at = place(point.x, point.y);
      if (at.left < -dot || at.top < -dot || at.left > size.width + dot || at.top > size.height + dot) continue;
      const lit = shown(point);
      const group = groupOf.get(point.category);
      context.globalAlpha = lit ? 0.9 : focus === null ? 0.55 : 0.12;
      context.fillStyle = lit ? colors[group?.color ?? "default"] : FRINGE;
      context.beginPath();
      context.arc(at.left, at.top, lit ? dot : dot * 0.85, 0, Math.PI * 2);
      context.fill();
    }
    context.globalAlpha = 1;
    if (hovered) {
      const at = place(hovered.x, hovered.y);
      context.lineWidth = 2;
      context.strokeStyle = "rgba(255, 255, 255, 0.9)";
      context.beginPath();
      context.arc(at.left, at.top, dot + 3, 0, Math.PI * 2);
      context.stroke();
    }
  }, [points, groups, groupOf, colors, size, view, unit, place, dot, focus, hovered]);

  // The post nearest the pointer, if it is close enough to mean it.
  function pick(x: number, y: number) {
    let best: MapPoint | null = null;
    let bestDistance = REACH * REACH;
    for (const point of points) {
      if (focus !== null && point.category !== focus) continue;
      const at = place(point.x, point.y);
      const distance = (at.left - x) ** 2 + (at.top - y) ** 2;
      if (distance < bestDistance) {
        best = point;
        bestDistance = distance;
      }
    }
    return best;
  }

  function local(event: { clientX: number; clientY: number }) {
    const box = canvas.current!.getBoundingClientRect();
    return { x: event.clientX - box.left, y: event.clientY - box.top };
  }

  function onPointerDown(event: ReactPointerEvent<HTMLCanvasElement>) {
    drag.current = { x: event.clientX, y: event.clientY, view, moved: false };
    event.currentTarget.setPointerCapture(event.pointerId);
  }

  function onPointerMove(event: ReactPointerEvent<HTMLCanvasElement>) {
    const held = drag.current;
    if (held && (held.moved || Math.hypot(event.clientX - held.x, event.clientY - held.y) > 4)) {
      held.moved = true;
      setHovered(null);
      setView(settle({ ...held.view, x: held.view.x + event.clientX - held.x, y: held.view.y + event.clientY - held.y }));
      return;
    }
    const { x, y } = local(event);
    const found = pick(x, y);
    if (found?.id !== hovered?.id) setHovered(found);
  }

  function onPointerUp(event: ReactPointerEvent<HTMLCanvasElement>) {
    const held = drag.current;
    drag.current = null;
    if (held?.moved) return;
    const { x, y } = local(event);
    const found = pick(x, y);
    if (!found) return;
    // The preview steps through the posts nearest this one, closest first.
    const at = place(found.x, found.y);
    const nearby = points
      .map((point) => ({ point, distance: (place(point.x, point.y).left - at.left) ** 2 + (place(point.x, point.y).top - at.top) ** 2 }))
      .sort((a, b) => a.distance - b.distance)
      .slice(0, 20)
      .map((entry) => entry.point.id);
    peek.open(found.id, nearby);
  }

  // Ctrl and the wheel (or a pinch on a trackpad) zooms; a plain wheel scrolls the page.
  useEffect(() => {
    const element = canvas.current;
    if (!element) return;
    function onWheel(event: WheelEvent) {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      const box = element!.getBoundingClientRect();
      zoomAt(Math.exp(-event.deltaY * 0.004), event.clientX - box.left, event.clientY - box.top);
    }
    element.addEventListener("wheel", onWheel, { passive: false });
    return () => element.removeEventListener("wheel", onWheel);
  }, [zoomAt, map.data]);

  if (map.isLoading) return <div className="map-frame skeleton" aria-busy="true" aria-label="Loading the library map" />;
  if (map.isError) return <p className="muted-block">The library map could not be loaded.</p>;
  if (!map.data || map.data.total === 0) return null;
  if (map.data.notice) return <p className="muted-block">{map.data.notice}</p>;
  if (points.length < 2) return <p className="muted-block">Your posts appear here once they are prepared for search by meaning.</p>;

  // Names, biggest group first; one that would cover a bigger group's name
  // is left out until zooming in makes room for it.
  const labelSize = (count: number) => Math.round(Math.min(18, 11.5 + Math.sqrt(count) / 2.6));
  const named: (Group & { left: number; top: number; fontSize: number })[] = [];
  const taken: { left: number; top: number; right: number; bottom: number }[] = [];
  for (const group of byCore) {
    if (group.core < Math.max(3, Math.min(LABEL_MIN_POSTS, Math.round(points.length / 40)))) continue;
    const at = place(group.cx, group.cy);
    const fontSize = labelSize(group.core);
    const half = (group.name.length * fontSize * 0.56) / 2 + 4;
    const box = { left: at.left - half, right: at.left + half, top: at.top - fontSize * 0.7, bottom: at.top + fontSize * 0.7 };
    // A name whose group has moved out of view would show cut in half.
    if (box.left < 4 || box.right > size.width - 4 || box.top < 4 || box.bottom > size.height - 4) continue;
    if (taken.some((other) => box.left < other.right && box.right > other.left && box.top < other.bottom && box.bottom > other.top)) continue;
    taken.push(box);
    named.push({ ...group, left: at.left, top: at.top, fontSize });
  }
  const listed = legendOpen ? groups : groups.slice(0, LEGEND_SHOWN);
  const card = hovered ? place(hovered.x, hovered.y) : null;
  return (
    <div className="library-map">
      <div className="map-frame" ref={frame} style={{ height }}>
        <canvas
          ref={canvas}
          className={`map-canvas ${hovered ? "pointing" : ""} ${view.scale > 1 ? "zoomed" : ""}`}
          style={{ width: size.width, height: size.height }}
          role="img"
          aria-label={`Map of ${points.length.toLocaleString()} posts as islands, one per category. The biggest: ${groups
            .slice(0, 5)
            .map((group) => `${group.name} (${group.count})`)
            .join(", ")}.`}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerLeave={() => !drag.current && setHovered(null)}
          onDoubleClick={(event) => {
            const { x, y } = local(event);
            zoomAt(2, x, y);
          }}
        />
        {named.map((group) => (
          <span
            key={group.id}
            className={`map-label opt-${group.color} ${focus !== null && focus !== group.id ? "faded" : ""}`}
            style={{ left: group.left, top: group.top, fontSize: `${group.fontSize}px` }}
            aria-hidden="true"
          >
            {group.name}
          </span>
        ))}
        {hovered && card && <MapPeek point={hovered} left={card.left} top={card.top} width={size.width} group={groupOf.get(hovered.category)} />}
        <div className="map-zoom" role="group" aria-label="Zoom">
          <button type="button" className="icon-button" aria-label="Zoom in" title="Zoom in (or Ctrl and scroll)" onClick={() => zoomAt(1.6)}>
            <Plus size={15} />
          </button>
          <button type="button" className="icon-button" aria-label="Zoom out" title="Zoom out" disabled={view.scale <= MIN_ZOOM} onClick={() => zoomAt(1 / 1.6)}>
            <Minus size={15} />
          </button>
          <button type="button" className="icon-button" aria-label="Show the whole map" title="Show the whole map" disabled={view.scale <= MIN_ZOOM} onClick={() => setView({ scale: 1, x: 0, y: 0 })}>
            <Maximize size={14} />
          </button>
        </div>
      </div>
      <div className="map-legend" role="group" aria-label="Pick out a category">
        {listed.map((group) => (
          <button
            key={group.id}
            type="button"
            className={`map-legend-item opt-${group.color} ${focus === group.id ? "on" : ""} ${focus !== null && focus !== group.id ? "faded" : ""}`}
            aria-pressed={focus === group.id}
            onClick={() => setFocus((current) => (current === group.id ? null : group.id))}
          >
            <span className="chip-dot" aria-hidden="true" />
            {group.name}
            <span className="map-legend-count">{group.count.toLocaleString()}</span>
          </button>
        ))}
        {groups.length > LEGEND_SHOWN && (
          <button type="button" className="map-legend-item more" aria-expanded={legendOpen} onClick={() => setLegendOpen((open) => !open)}>
            {legendOpen ? "Show fewer" : `Show all ${groups.length}`}
          </button>
        )}
      </div>
      {map.data.indexed < map.data.total && (
        <p className="map-note">
          {map.data.indexed.toLocaleString()} of {map.data.total.toLocaleString()} posts placed so far. The rest appear as they are prepared for search by meaning.
        </p>
      )}
    </div>
  );
}

/* What a dot is: who posted it, its opening, and its category. */
function MapPeek({ point, left, top, width, group }: { point: MapPoint; left: number; top: number; width: number; group?: Group }) {
  const post = useQuery({ queryKey: ["bookmark", String(point.id)], queryFn: () => getBookmark(String(point.id)), staleTime: 60_000 });
  // Opens toward whichever side has more room.
  const leftSide = left > width / 2;
  return (
    <div className={`map-peek ${leftSide ? "left" : ""}`} style={{ left, top }} role="tooltip">
      {post.data ? (
        <>
          <span className="map-peek-head">
            <AuthorAvatar name={post.data.author} username={post.data.username} src={post.data.authorAvatar} />
            <strong>{post.data.author || `@${post.data.username}`}</strong>
            {post.data.postedAt && <time>{formatDate(post.data.postedAt)}</time>}
          </span>
          <span className="map-peek-text">{postPreview(post.data)}</span>
          {group && group.id > 0 && <Option color={group.color}>{group.name}</Option>}
        </>
      ) : (
        <span className="map-peek-text muted">Loading…</span>
      )}
    </div>
  );
}
