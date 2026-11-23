import { useQuery } from "@tanstack/react-query";
import { Maximize, Minus, Plus } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { getBookmark, getCategories, getLibraryMap, type MapPoint } from "../lib/api";
import { colorFor, formatDate, postPreview, type OptionColor } from "../lib/format";
import { usePeek } from "../lib/peek";
import { AuthorAvatar } from "./Author";
import { Option } from "./ui";

// Room around the posts, so dots on the edge aren't cut in half.
const PAD = 28;
const DOT = 3.5;
// How near the pointer has to be to a dot, in pixels, to pick it.
const REACH = 9;
const MIN_ZOOM = 1;
const MAX_ZOOM = 8;
// Groups with fewer posts get no name on the map; their posts still show.
const LABEL_MIN = 3;
const LABELS_SHOWN = 6;

type View = { scale: number; x: number; y: number };
type Group = { id: number; name: string; color: OptionColor; count: number; cx: number; cy: number; radius: number };

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

function median(values: number[]) {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.floor(sorted.length / 2)] ?? 0.5;
}

/* Every post placed by meaning, so posts about the same thing sit together,
   each colored by its main category. Hover a dot to see the post, click to
   open it; click a category below to pick out its posts. The view zooms
   with the buttons, Ctrl and the wheel, or a double click, and pans by dragging. */
export function LibraryMap() {
  const map = useQuery({ queryKey: ["library-map"], queryFn: getLibraryMap, staleTime: 60_000 });
  const categories = useQuery({ queryKey: ["categories"], queryFn: getCategories });
  const peek = usePeek();
  const frame = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [size, setSize] = useState({ width: 0, height: 0 });
  const [view, setView] = useState<View>({ scale: 1, x: 0, y: 0 });
  const [hovered, setHovered] = useState<MapPoint | null>(null);
  const [focus, setFocus] = useState<number | null>(null);
  const drag = useRef<{ x: number; y: number; view: View; moved: boolean } | null>(null);

  const points = useMemo(() => map.data?.points ?? [], [map.data]);
  // The frame exists only once there is a map to draw.
  const colors = useOptionColors(frame, points.length > 1 && !map.data?.notice);

  // The groups, biggest first, with where each one sits and how far it spreads.
  const groups = useMemo<Group[]>(() => {
    const names = new Map((categories.data?.items ?? []).map((item) => [item.id, item.name]));
    const byCategory = new Map<number, MapPoint[]>();
    for (const point of points) byCategory.set(point.category, [...(byCategory.get(point.category) ?? []), point]);
    return [...byCategory]
      .map(([id, members]) => {
        const cx = median(members.map((point) => point.x));
        const cy = median(members.map((point) => point.y));
        const radius = Math.sqrt(members.reduce((sum, point) => sum + (point.x - cx) ** 2 + (point.y - cy) ** 2, 0) / members.length);
        return { id, name: id ? names.get(id) ?? "Category" : "No category", color: id ? colorFor(id) : ("default" as const), count: members.length, cx, cy, radius };
      })
      .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
  }, [points, categories.data]);
  const groupOf = useMemo(() => new Map(groups.map((group) => [group.id, group])), [groups]);

  useEffect(() => {
    if (!frame.current) return;
    const observer = new ResizeObserver(([entry]) => setSize({ width: entry.contentRect.width, height: entry.contentRect.height }));
    observer.observe(frame.current);
    return () => observer.disconnect();
  }, [map.data]);

  // Where a post lands on screen, in CSS pixels inside the frame.
  const place = useCallback(
    (x: number, y: number) => ({
      left: (PAD + x * (size.width - 2 * PAD)) * view.scale + view.x,
      top: (PAD + y * (size.height - 2 * PAD)) * view.scale + view.y,
    }),
    [size, view],
  );

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

  // Draws the halos behind the biggest groups, then every dot.
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

    for (const group of groups.slice(0, LABELS_SHOWN)) {
      if (!group.id || group.count < LABEL_MIN) continue;
      const centre = place(group.cx, group.cy);
      const reach = Math.max(48, group.radius * Math.min(size.width, size.height) * view.scale * 1.25);
      const glow = context.createRadialGradient(centre.left, centre.top, 0, centre.left, centre.top, reach);
      const strength = focus === null || focus === group.id ? 0.11 : 0.03;
      glow.addColorStop(0, withAlpha(colors[group.color], strength));
      glow.addColorStop(0.6, withAlpha(colors[group.color], strength * 0.35));
      glow.addColorStop(1, withAlpha(colors[group.color], 0));
      context.fillStyle = glow;
      context.fillRect(centre.left - reach, centre.top - reach, reach * 2, reach * 2);
    }

    // The picked-out group is drawn last, on top of the others.
    const order = focus === null ? points : [...points.filter((point) => point.category !== focus), ...points.filter((point) => point.category === focus)];
    for (const point of order) {
      const at = place(point.x, point.y);
      if (at.left < -DOT || at.top < -DOT || at.left > size.width + DOT || at.top > size.height + DOT) continue;
      const group = groupOf.get(point.category);
      context.globalAlpha = focus === null || focus === point.category ? 0.92 : 0.14;
      context.fillStyle = colors[group?.color ?? "default"];
      context.beginPath();
      context.arc(at.left, at.top, DOT, 0, Math.PI * 2);
      context.fill();
    }
    context.globalAlpha = 1;
    if (hovered) {
      const at = place(hovered.x, hovered.y);
      context.lineWidth = 2;
      context.strokeStyle = "rgba(255, 255, 255, 0.9)";
      context.beginPath();
      context.arc(at.left, at.top, DOT + 3, 0, Math.PI * 2);
      context.stroke();
    }
  }, [points, groups, groupOf, colors, size, view, place, focus, hovered]);

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

  const named = groups.filter((group) => group.id && group.count >= LABEL_MIN).slice(0, LABELS_SHOWN);
  const card = hovered ? place(hovered.x, hovered.y) : null;
  return (
    <div className="library-map">
      <div className="map-frame" ref={frame}>
        <canvas
          ref={canvas}
          className={`map-canvas ${hovered ? "pointing" : ""} ${view.scale > 1 ? "zoomed" : ""}`}
          style={{ width: size.width, height: size.height }}
          role="img"
          aria-label={`Map of ${points.length.toLocaleString()} posts, grouped by what they are about. The biggest groups: ${named.map((group) => group.name).join(", ")}.`}
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
          const at = place(group.cx, group.cy);
          // A name whose group has moved out of view would show cut in half.
          if (at.left < 40 || at.left > size.width - 40 || at.top < 14 || at.top > size.height - 14) return null;
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
        {groups.map((group) => (
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

/* A CSS color with its opacity changed; the option colors are hex. */
function withAlpha(color: string | undefined, alpha: number) {
  const hex = (color || "#9b9b9b").replace("#", "");
  const full = hex.length === 3 ? hex.replace(/./g, "$&$&") : hex.slice(0, 6);
  const value = Number.parseInt(full, 16);
  if (Number.isNaN(value)) return `rgba(155, 155, 155, ${alpha})`;
  return `rgba(${(value >> 16) & 255}, ${(value >> 8) & 255}, ${value & 255}, ${alpha})`;
}
