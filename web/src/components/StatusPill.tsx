import { statuses } from "../lib/status";

/* Notion's Status property: a rounded pill with a leading dot. */
export function StatusPill({ status }: { status: string }) {
  const value = statuses[status] || statuses.pending;
  return (
    <span className={`status-pill opt-${value.tone} ${status === "processing" ? "live" : ""}`}>
      <span className="status-dot" aria-hidden="true" />
      <span>{value.label}</span>
    </span>
  );
}
