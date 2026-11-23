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
// Dots shrink on crowded islands, between these radii in pixels.
const DOT_MIN = 1.4;
const DOT_MAX = 6;
// How near the pointer has to be to a dot, in pixels, to pick it.
const REACH = 9;
const MIN_ZOOM = 1;
const MAX_ZOOM = 10;
// An island smaller than this on screen (radius, pixels) goes unnamed until zoomed in.
const LABEL_MIN_RADIUS = 24;
// The legend lists this many categories before "Show all".
const LEGEND_SHOWN = 10;
// The map's height follows its shape, within these bounds.
const MIN_HEIGHT = 280;
const MAX_HEIGHT = 640;

type View = { scale: number; x: number; y: number };
// top: the y of the group's highest post, where its name goes.
type Group = { id: number; name: string; color: OptionColor; count: number; cx: number; cy: number; radius: number; top: number };

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

  // The islands, biggest first, with their names and colors.
  const groups = useMemo<Group[]>(() => {
    const names = new Map((categories.data?.items ?? []).map((item) => [item.id, item.name]));
    const counts = new Map<number, number>();
    const tops = new Map<number, number>();
    for (const point of points) {
      counts.set(point.category, (counts.get(point.category) ?? 0) + 1);
      tops.set(point.category, Math.min(tops.get(point.category) ?? Infinity, point.y));
    }
    return (map.data?.islands ?? [])
      .map((island) => ({
        id: island.category,
        name: island.category ? names.get(island.category) ?? "Category" : "No category",
        color: island.category ? colorFor(island.category) : ("default" as const),
        count: counts.get(island.category) ?? 0,
        cx: island.x,
        cy: island.y,
        radius: island.radius,
        top: tops.get(island.category) ?? island.y - island.radius,
      }))
      .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
  }, [points, map.data, categories.data]);
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
  // How big a dot is on an island: smaller where it is crowded.
  const dotSize = useMemo(() => {
    const sizes = new Map<number, number>();
    for (const group of groups) {
      const spacing = group.radius * unit * view.scale * Math.sqrt(Math.PI / Math.max(1, group.count));
      sizes.set(group.id, Math.min(DOT_MAX, Math.max(DOT_MIN, spacing * 0.3)));
    }
    return sizes;
  }, [groups, unit, view.scale]);

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

    // The glow: a wide, faint spot of color under each post, blurred
    // together, so dense parts of a group glow brighter and its edge fades
    // the way the posts thin out.
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
          const group = groupOf.get(point.category);
          const at = place(point.x, point.y);
          const reach = Math.max(7, (dotSize.get(point.category) ?? DOT_MIN) * 3.2);
          paint.globalAlpha = focus === null || focus === point.category ? 0.13 : 0.03;
          paint.fillStyle = colors[group?.color ?? "default"];
          paint.beginPath();
          paint.arc(at.left, at.top, reach, 0, Math.PI * 2);
          paint.fill();
        }
        blurred.filter = `blur(${Math.round(14 * ratio)}px)`;
        blurred.drawImage(spots, 0, 0);
      }
      glow.current = { key, layer };
    }
    context.drawImage(glow.current.layer, 0, 0, size.width, size.height);

    // The picked-out group is drawn last, on top of the others.
    const order = focus === null ? points : [...points.filter((point) => point.category !== focus), ...points.filter((point) => point.category === focus)];
    for (const point of order) {
      const at = place(point.x, point.y);
      const group = groupOf.get(point.category);
      const dot = dotSize.get(point.category) ?? DOT_MIN;
      if (at.left < -dot || at.top < -dot || at.left > size.width + dot || at.top > size.height + dot) continue;
      context.globalAlpha = focus === null || focus === point.category ? 0.88 : 0.1;
      context.fillStyle = colors[group?.color ?? "default"];
      context.beginPath();
      context.arc(at.left, at.top, dot, 0, Math.PI * 2);
      context.fill();
    }
    context.globalAlpha = 1;
    if (hovered) {
      const at = place(hovered.x, hovered.y);
      context.lineWidth = 2;
      context.strokeStyle = "rgba(255, 255, 255, 0.9)";
      context.beginPath();
      context.arc(at.left, at.top, (dotSize.get(hovered.category) ?? DOT_MIN) + 3, 0, Math.PI * 2);
      context.stroke();
    }
  }, [points, groups, groupOf, colors, size, view, unit, place, dotSize, focus, hovered]);

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

  const named = groups.filter((group) => group.radius * unit * view.scale >= LABEL_MIN_RADIUS);
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
        {named.map((group) => {
          // The name floats just above the group's highest post.
          const at = place(group.cx, group.top);
          // A name whose group has moved out of view would show cut in half.
          if (at.left < 40 || at.left > size.width - 40 || at.top < 18 || at.top > size.height - 4) return null;
          return (
            <span
              key={group.id}
              className={`map-label opt-${group.color} ${focus !== null && focus !== group.id ? "faded" : ""}`}
              style={{ left: at.left, top: at.top }}
              aria-hidden="true"
            >
              {group.name}
            </span>
          );
        })}
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
