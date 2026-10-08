import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  CircleDot,
  Clock3,
  ListChecks,
  Loader,
  RefreshCw,
  RotateCcw,
  Type,
  CheckCircle2,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { StatusPill } from "../components/StatusPill";
import { EmptyState, PageHeader, ViewTabs } from "../components/ui";
import { getJobs, retryJob } from "../lib/api";
import { formatDateTime } from "../lib/format";

type View = "all" | "active" | "failed" | "completed";

// Each view asks the server for its own list, so tabs stay correct past the 200-row cap.
const viewStatuses: Record<View, string> = {
  all: "",
  active: "pending,processing,blocked",
  failed: "failed",
  completed: "completed",
};

export function ActivityPage() {
  const client = useQueryClient();
  const [view, setView] = useState<View>("all");
  const [retrying, setRetrying] = useState<number>();
  const jobs = useQuery({
    queryKey: ["jobs", view],
    queryFn: () => getJobs(viewStatuses[view]),
    refetchInterval: 5000,
    placeholderData: (previous) => previous,
  });

  const retry = useMutation({
    mutationFn: retryJob,
    onMutate: (id) => setRetrying(id),
    onSettled: () => setRetrying(undefined),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["jobs"] });
      client.invalidateQueries({ queryKey: ["bookmarks"] });
    },
  });

  const visible = jobs.data?.items || [];
  const counts = jobs.data?.counts;
  const total = counts?.total ?? 0;

  return (
    <main className="page full">
      <PageHeader
        icon={Activity}
        tone="green"
        title="Activity"
        description="Every AI analysis runs as a durable job: observable, retryable, and paced to respect provider limits."
      />

      <div className="view-bar">
        <ViewTabs
          label="Job views"
          value={view}
          onChange={setView}
          tabs={[
            { value: "all", label: "All jobs", icon: ListChecks, count: counts?.total },
            { value: "active", label: "Waiting", icon: Clock3, count: counts && counts.pending + counts.processing + counts.blocked },
            { value: "failed", label: "Needs attention", icon: AlertTriangle, count: counts?.failed },
            { value: "completed", label: "Completed", icon: CheckCircle2, count: counts?.completed },
          ]}
        />
        <div className="view-bar-tools">
          <span className="view-bar-note" aria-live="polite">
            {visible.length >= 200 && `Showing the latest 200 · `}
            {jobs.isFetching ? "Refreshing…" : "Updates every 5 seconds"}
          </span>
        </div>
      </div>

      {jobs.isLoading && (
        <div className="skeleton-table" aria-busy="true">
          {Array.from({ length: 6 }).map((_, index) => (
            <div key={index} className="skeleton skeleton-row" />
          ))}
        </div>
      )}

      {visible.length > 0 && (
        <div className="table-scroll">
          <table className="db-table">
            <thead>
              <tr>
                <th className="col-title"><span><Type size={14} />Bookmark</span></th>
                <th className="col-status"><span><CircleDot size={14} />Status</span></th>
                <th className="col-num"><span><RotateCcw size={14} />Attempts</span></th>
                <th className="col-date"><span><Clock3 size={14} />Updated</span></th>
                <th className="col-error"><span><AlertTriangle size={14} />Last error</span></th>
                <th className="col-actions"><span className="visually-hidden">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {visible.map((job) => (
                <tr key={job.id} className="static">
                  <td className="col-title">
                    <Link className="row-link" to={`/bookmarks/${job.bookmarkId}`} title={`Post ${job.postId}`}>
                      {job.author || "Untitled post"}
                    </Link>
                  </td>
                  <td className="col-status">
                    <StatusPill status={job.status} />
                  </td>
                  <td className="col-num">
                    {job.attempts}/{job.maxAttempts}
                  </td>
                  <td className="col-date">
                    <time dateTime={job.updatedAt}>{formatDateTime(job.updatedAt)}</time>
                  </td>
                  <td className="col-error">
                    {job.lastError && (
                      <span className="cell-text error-text" title={job.lastError}>
                        {job.lastError}
                      </span>
                    )}
                  </td>
                  <td className="col-actions">
                    {job.status === "failed" && (
                      <button
                        className="ghost-button"
                        disabled={retry.isPending}
                        onClick={() => retry.mutate(job.id)}
                        title="Retry analysis"
                      >
                        {retrying === job.id ? <Loader size={13} className="spin" /> : <RefreshCw size={13} />}
                        Retry
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {jobs.data && visible.length === 0 && (
        <EmptyState icon={Activity} title={total ? "Nothing in this view" : "No processing activity"}>
          {total
            ? "Jobs move between views as they are processed."
            : "Analysis jobs appear here automatically when bookmarks are imported."}
        </EmptyState>
      )}

      {jobs.isError && (
        <EmptyState icon={AlertTriangle} title="Activity could not be loaded">
          The local server did not answer. Check that Sortwise is still running.
        </EmptyState>
      )}
    </main>
  );
}
