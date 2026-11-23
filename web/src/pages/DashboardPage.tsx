import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Clock3, CirclePause, House, KeyRound, Lightbulb, Puzzle, SearchX } from "lucide-react";
import { Fragment, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { AuthorAvatar } from "../components/Author";
import { LibraryMap } from "../components/LibraryMap";
import { SavedHeatmap } from "../components/SavedHeatmap";
import { usePeek } from "../lib/peek";
import { Callout, EmptyState, Option, PageHeader } from "../components/ui";
import { colorFor, formatDate, postPreview } from "../lib/format";
import { createPairingCode, getAISettings, getBookmarks, getDashboard, getProposals, getSetup, type DashboardStats } from "../lib/api";

export function DashboardPage() {
  const setup = useQuery({ queryKey: ["setup"], queryFn: getSetup });
  const dashboard = useQuery({ queryKey: ["dashboard"], queryFn: getDashboard });
  const ai = useQuery({ queryKey: ["ai-settings"], queryFn: getAISettings });
  const proposals = useQuery({ queryKey: ["taxonomy-proposals"], queryFn: getProposals });
  const recent = useQuery({ queryKey: ["bookmarks", "recent"], queryFn: () => getBookmarks("pageSize=6") });
  const [pairing, setPairing] = useState<{ code: string; expiresAt: string }>();
  const [pairingError, setPairingError] = useState("");
  const peek = usePeek();

  async function generateCode() {
    try {
      setPairing(await createPairingCode());
      setPairingError("");
    } catch (error) {
      setPairingError(error instanceof Error ? error.message : "Could not create code.");
    }
  }

  const stats =
    dashboard.data && Array.isArray(dashboard.data.categories) && Array.isArray(dashboard.data.topTags)
      ? dashboard.data
      : undefined;
  const empty = !setup.isLoading && setup.data?.bookmarks === 0;
  const pendingCategoryProposals = (proposals.data?.items || []).filter((item) => item.kind === "category").length;
  // Many analyzed bookmarks without a fitting category means the categories do not
  // match what this person saves (the starter set is deliberately broad).
  const poorFit =
    stats && pendingCategoryProposals === 0 && stats.completed >= 25 && stats.needsReview >= 8 && stats.needsReview / stats.completed >= 0.15;
  const restingMain = ai.data?.resting?.find((item) => item.provider === ai.data?.provider);
  const mainName = ai.data?.providers?.find((item) => item.id === ai.data?.provider)?.name;
  const maxCategory = stats?.categories[0]?.count || 1;

  return (
    <main className="page overview-page">
      <PageHeader
        icon={House}
        title="Overview"
        description={stats && stats.total > 0 ? <StatusLine stats={stats} /> : "Everything you saved from X, organized locally and ready to rediscover."}
      />

      {empty && (
        <section className="block-section" aria-labelledby="connect-heading">
          <h2 className="block-heading" id="connect-heading">
            Connect the browser extension
          </h2>
          <ol className="numbered-list">
            <li>
              Load the unpacked <code>extension</code> folder in Chrome or Edge (<code>chrome://extensions</code>, Developer mode, Load unpacked).
            </li>
            <li>Generate a one-time code below and enter it in the extension popup.</li>
            <li>
              Click <strong>Sync now</strong> in the popup. Your bookmarks appear here within minutes, and every post you
              bookmark on X afterwards is saved automatically.
            </li>
          </ol>
          <Callout icon={Puzzle} tone="blue">
            {pairing ? (
              <div className="pairing">
                <span className="pairing-code" aria-label={`Pairing code ${pairing.code}`}>
                  {pairing.code}
                </span>
                <span className="muted">
                  Expires at{" "}
                  {new Date(pairing.expiresAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                </span>
              </div>
            ) : (
              <div className="pairing">
                <span>Pair this library with the extension.</span>
                <button className="button primary" onClick={generateCode}>
                  Generate pairing code
                </button>
              </div>
            )}
            {pairingError && (
              <p className="form-error" role="alert">
                {pairingError}
              </p>
            )}
          </Callout>
        </section>
      )}

      {!empty && setup.data && !setup.data.aiConfigured && (
        <Callout icon={KeyRound} tone="orange">
          <p>
            <strong>AI isn’t configured yet.</strong> {setup.data.pendingJobs.toLocaleString()} bookmark
            {setup.data.pendingJobs === 1 ? " is" : "s are"} waiting in the queue. Everything is already searchable; add a
            key to get summaries, categories, and tags.
          </p>
          <Link className="inline-link" to="/settings">
            Open settings <ArrowRight size={14} />
          </Link>
        </Callout>
      )}

      {!empty && setup.data?.aiConfigured && ai.data?.paused && (
        <Callout icon={CirclePause} tone="yellow">
          <p>
            <strong>AI processing is paused.</strong> Queued bookmarks stay on this machine until you resume it.
          </p>
          <Link className="inline-link" to="/settings">
            Review in settings <ArrowRight size={14} />
          </Link>
        </Callout>
      )}

      {!empty && setup.data?.aiConfigured && !ai.data?.paused && restingMain && !ai.data?.active && (
        <Callout icon={Clock3} tone="yellow">
          <p>
            <strong>{mainName} reached its free limit.</strong> Organizing continues at{" "}
            {new Date(restingMain.restingUntil).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}. Add a key for
            the other AI service to keep going in the meantime.
          </p>
          <Link className="inline-link" to="/settings">
            Open settings <ArrowRight size={14} />
          </Link>
        </Callout>
      )}

      {poorFit && (
        <Callout icon={Lightbulb} tone="blue">
          <p>
            <strong>{stats.needsReview.toLocaleString()} bookmarks don’t fit your categories yet.</strong> Sortwise can
            suggest categories based on what you actually save.
          </p>
          <Link className="inline-link" to="/categories?suggest=1">
            Suggest categories <ArrowRight size={14} />
          </Link>
        </Callout>
      )}

      {stats && stats.total > 0 && (
        <>
          <section className="block-section" aria-labelledby="map-heading">
            <h2 className="block-heading" id="map-heading">
              Library map
            </h2>
            <LibraryMap />
          </section>

          <section className="block-section" aria-labelledby="days-heading">
            <h2 className="block-heading" id="days-heading">
              Bookmarks by day
            </h2>
            <SavedHeatmap />
          </section>
        </>
      )}

      {pendingCategoryProposals > 0 && (
        <Callout icon={Lightbulb}>
          <p>
            <strong>
              {pendingCategoryProposals} suggested categor{pendingCategoryProposals === 1 ? "y" : "ies"}
            </strong>{" "}
            {pendingCategoryProposals === 1 ? "is" : "are"} waiting for your review.
          </p>
          <Link className="inline-link" to="/categories">
            Review on Categories <ArrowRight size={14} />
          </Link>
        </Callout>
      )}

      <div className="column-list overview-columns">
        <section className="block-section" aria-labelledby="recent-heading">
          <div className="linked-view-header">
            <h2 className="block-heading" id="recent-heading">
              Recently saved
            </h2>
            <Link to="/bookmarks" className="inline-link push">
              Open library <ArrowRight size={14} />
            </Link>
          </div>

          {recent.isLoading && (
            <div aria-busy="true">
              {Array.from({ length: 4 }).map((_, index) => (
                <div key={index} className="skeleton skeleton-row" />
              ))}
            </div>
          )}

          {recent.isError && (
            <Callout icon={SearchX} tone="red" role="alert">
              The local library could not be loaded. Check that Sortwise is still running.
            </Callout>
          )}

          {recent.data?.items?.length === 0 && (
            <EmptyState icon={SearchX} title="No bookmarks yet">
              Your first imports appear here straight away, without waiting for AI.
            </EmptyState>
          )}

          {recent.data && recent.data.items?.length > 0 && (
            <ul className="recent-list">
              {recent.data.items.map((item) => (
                <li key={item.id}>
                  <Link
                    to={`/bookmarks/${item.id}`}
                    className="recent-row"
                    onClick={(event) => {
                      if (event.metaKey || event.ctrlKey || event.shiftKey) return;
                      event.preventDefault();
                      peek.open(item.id, recent.data?.items.map((entry) => entry.id) ?? []);
                    }}
                  >
                    <AuthorAvatar name={item.author || item.username} username={item.username} src={item.authorAvatar} size="post" />
                    <span className="recent-text">
                      <span className="recent-head">
                        <strong>{item.author || `@${item.username}`}</strong>
                        <time>{formatDate(item.importedAt)}</time>
                      </span>
                      <span className="recent-preview">{postPreview(item)}</span>
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </section>

        {stats && stats.total > 0 && (
          <section className="block-section" aria-labelledby="categories-heading">
            <h2 className="block-heading" id="categories-heading">
              Top categories
            </h2>
            {stats.categories.length ? (
              <ul className="bar-list">
                {stats.categories.slice(0, 8).map((item) => (
                  <li key={item.id}>
                    <Link to={`/bookmarks?category=${item.id}`}>
                      <Option color={colorFor(item.id)}>{item.name}</Option>
                      <span className="bar-track" aria-hidden="true">
                        <span className={`bar-fill seg-${colorFor(item.id)}`} style={{ width: `${Math.max(4, (item.count / maxCategory) * 100)}%` }} />
                      </span>
                      <span className="bar-count">{item.count}</span>
                    </Link>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="muted-block">Categories appear here once bookmarks are analyzed.</p>
            )}
          </section>
        )}
      </div>
    </main>
  );
}

/* The library in one line under the title: how much is saved, and anything
   that needs a look, each a link to those posts. Quiet when all is well. */
function StatusLine({ stats }: { stats: DashboardStats }) {
  const parts: ReactNode[] = [<span key="saved">{stats.total.toLocaleString()} saved</span>];
  const link = (key: string, count: number, words: string, status: string, tone = "") =>
    parts.push(
      <Link key={key} className={`status-link ${tone}`} to={`/bookmarks?status=${status}`}>
        {count.toLocaleString()} {words}
      </Link>,
    );
  if (stats.pending) link("pending", stats.pending, "waiting for AI", "pending");
  if (stats.failed) link("failed", stats.failed, stats.failed === 1 ? "needs attention" : "need attention", "failed", "warn");
  if (stats.needsReview) link("review", stats.needsReview, stats.needsReview === 1 ? "needs a category" : "need a category", "review");
  if (!stats.pending && !stats.failed) parts.push(<span key="done">all organized</span>);
  return (
    <span className="status-line">
      {parts.map((part, index) => (
        <Fragment key={index}>
          {index > 0 && <span aria-hidden="true"> · </span>}
          {part}
        </Fragment>
      ))}
    </span>
  );
}
