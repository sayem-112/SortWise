import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { getSavedDays } from "../lib/api";

const WEEKS = 53;
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const WEEKDAYS = ["Mon", "", "Wed", "", "Fri", "", ""];
// Room for the weekday names, and the gap after them, in pixels.
const LABELS = 34;
const GAP = 3;

function dayKey(date: Date) {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

function dayLabel(date: Date) {
  return date.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric", year: "numeric" });
}

/* Four shades for days with saves: by where the day falls among the busy
   days, so one huge day doesn't wash out the rest. */
function levels(counts: number[]) {
  const sorted = counts.filter((count) => count > 0).sort((a, b) => a - b);
  const at = (share: number) => sorted[Math.floor((sorted.length - 1) * share)] ?? 1;
  const steps = [at(0.25), at(0.5), at(0.75)];
  return (count: number) => (count <= 0 ? 0 : count <= steps[0] ? 1 : count <= steps[1] ? 2 : count <= steps[2] ? 3 : 4);
}

/* A year of saving, one square per day, weeks running left to right and the
   newest at the right. */
export function SavedHeatmap() {
  const saved = useQuery({ queryKey: ["saved-days"], queryFn: getSavedDays, staleTime: 60_000 });
  const scroller = useRef<HTMLDivElement>(null);
  // Squares grow to fill the width, between 9 and 16 pixels; below that the year scrolls.
  const [cell, setCell] = useState(11);
  useEffect(() => {
    const element = scroller.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => {
      const fit = Math.floor((entry.contentRect.width - LABELS - (WEEKS - 1) * GAP) / WEEKS);
      setCell(Math.max(9, Math.min(16, fit)));
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [saved.data]);

  const grid = useMemo(() => {
    const counts = new Map((saved.data?.days ?? []).map((day) => [day.date, day.count]));
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    // Columns are weeks starting on Monday; the last column holds today.
    const start = new Date(today);
    start.setDate(start.getDate() - ((start.getDay() + 6) % 7) - (WEEKS - 1) * 7);
    const shade = levels([...counts.values()]);
    const cells: { key: string; label: string; count: number; level: number; future: boolean }[] = [];
    const months: { column: number; name: string }[] = [];
    for (let index = 0; index < WEEKS * 7; index++) {
      const date = new Date(start);
      date.setDate(start.getDate() + index);
      const key = dayKey(date);
      const count = counts.get(key) ?? 0;
      cells.push({ key, label: dayLabel(date), count, level: shade(count), future: date > today });
      // A month is named over its first week; none over the last two weeks, where the name would run off the edge.
      if (index % 7 === 0 && date.getDate() <= 7) {
        const column = index / 7;
        const last = months.at(-1);
        if (column < WEEKS - 2 && (!last || column - last.column >= 3)) months.push({ column, name: MONTHS[date.getMonth()] });
      }
    }
    const year = [...counts.values()].reduce((sum, count) => sum + count, 0);
    return { cells, months, year };
  }, [saved.data]);

  // The newest weeks are the interesting ones: start scrolled to them on a narrow screen.
  useEffect(() => {
    if (scroller.current) scroller.current.scrollLeft = scroller.current.scrollWidth;
  }, [saved.data]);

  if (saved.isLoading) return <div className="heatmap-skeleton skeleton" aria-busy="true" aria-label="Loading saves by day" />;
  if (saved.isError || !saved.data) return <p className="muted-block">Saves by day could not be loaded.</p>;
  const back = saved.data.backCatalogue;

  return (
    <div className="heatmap" style={{ ["--cell" as string]: `${cell}px`, ["--gap" as string]: `${GAP}px` }}>
      <div className="heatmap-scroll" ref={scroller}>
        <div className="heatmap-grid" style={{ ["--weeks" as string]: WEEKS }}>
          <span className="heatmap-corner" aria-hidden="true" />
          <div className="heatmap-months" aria-hidden="true">
            {grid.months.map((month) => (
              <span key={`${month.column}-${month.name}`} style={{ gridColumn: month.column + 1 }}>
                {month.name}
              </span>
            ))}
          </div>
          <div className="heatmap-weekdays" aria-hidden="true">
            {WEEKDAYS.map((day, index) => (
              <span key={index}>{day}</span>
            ))}
          </div>
          <div className="heatmap-days" role="img" aria-label={`${grid.year.toLocaleString()} posts saved in the past year`}>
            {grid.cells.map((cell) => (
              <span
                key={cell.key}
                className={`heatmap-day level-${cell.level} ${cell.future ? "future" : ""}`}
                title={cell.future ? undefined : `${cell.count ? cell.count.toLocaleString() : "Nothing"} saved on ${cell.label}`}
              />
            ))}
          </div>
        </div>
      </div>
      <div className="heatmap-foot">
        <span>
          {grid.year.toLocaleString()} saved in the past year
          {back && (
            <span className="muted">
              {" "}
              · {back.count.toLocaleString()} more arrived together on {new Date(`${back.date}T12:00:00`).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}, your back catalogue, and are left out of the shading
            </span>
          )}
        </span>
        <span className="heatmap-scale" aria-hidden="true">
          Fewer
          {[0, 1, 2, 3, 4].map((level) => (
            <span key={level} className={`heatmap-day level-${level}`} />
          ))}
          More
        </span>
      </div>
    </div>
  );
}
