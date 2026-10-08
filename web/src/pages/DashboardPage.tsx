import { useQuery } from "@tanstack/react-query";
import {
  ArrowRight,
  BookMarked,
  CircleAlert,
  CircleCheck,
  Clock3,
  CirclePause,
  House,
  KeyRound,
  LayoutGrid,
  Lightbulb,
  Puzzle,
  ScanEye,
  SearchX,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { BookmarkCard } from "../components/BookmarkCard";
import { BookmarkDrawer } from "../components/BookmarkDrawer";
import { Callout, EmptyState, Option, PageHeader, PropertyRow } from "../components/ui";
import { colorFor } from "../lib/format";
import { createPairingCode, getAISettings, getBookmarks, getDashboard, getProposals, getSetup } from "../lib/api";

export function DashboardPage() {
  const setup = useQuery({ queryKey: ["setup"], queryFn: getSetup });
  const dashboard = useQuery({ queryKey: ["dashboard"], queryFn: getDashboard });
  const ai = useQuery({ queryKey: ["ai-settings"], queryFn: getAISettings });
  const proposals = useQuery({ queryKey: ["taxonomy-proposals"], queryFn: getProposals });
  const recent = useQuery({ queryKey: ["bookmarks", "recent"], queryFn: () => getBookmarks("pageSize=4") });
  const [pairing, setPairing] = useState<{ code: string; expiresAt: string }>();
  const [pairingError, setPairingError] = useState("");
  const [activeBookmarkId, setActiveBookmarkId] = useState<number | null>(null);

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
  const segments = stats
    ? [
        { key: "completed", label: "Organized", count: stats.completed, icon: CircleCheck },
        { key: "pending", label: "Queued", count: stats.pending, icon: Clock3 },
        { key: "failed", label: "Needs attention", count: stats.failed, icon: CircleAlert },
        { key: "review", label: "Needs review", count: stats.needsReview ?? 0, icon: ScanEye },
      ]
    : [];
  const pendingCategoryProposals = (proposals.data?.items || []).filter((item) => item.kind === "category").length;
  // Many analyzed bookmarks without a fitting category means the categories do not
  // match what this person saves (the starter set is deliberately broad).
  const poorFit =
    stats && pendingCategoryProposals === 0 && stats.completed >= 25 && stats.needsReview >= 8 && stats.needsReview / stats.completed >= 0.15;
  const restingMain = ai.data?.resting?.find((item) => item.provider === ai.data?.provider);
  const mainName = ai.data?.providers?.find((item) => item.id === ai.data?.provider)?.name;
  const maxCategory = stats?.categories[0]?.count || 1;

  return (
    <main className="page">
      <PageHeader
        icon={House}
        title="Overview"
        description="Everything you saved from X, organized locally and ready to rediscover."
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
        <section className="block-section" aria-labelledby="status-heading">
          <h2 className="block-heading" id="status-heading">
            Library status
          </h2>
          <div className="properties summary-properties">
            <PropertyRow icon={BookMarked} name="Saved">
              <Link className="property-link" to="/bookmarks">
                {stats.total.toLocaleString()} bookmarks
              </Link>
            </PropertyRow>
            {segments.map((segment) => (
              <PropertyRow key={segment.key} icon={segment.icon} name={segment.label}>
                <Link className="property-link" to={`/bookmarks?status=${segment.key}`}>
                  {segment.count.toLocaleString()} bookmark{segment.count === 1 ? "" : "s"}
                </Link>
              </PropertyRow>
            ))}
          </div>
        </section>
      )}

      {stats && stats.total > 0 && (
        <div className="column-list">
          <section className="block-section" aria-labelledby="categories-heading">
            <h2 className="block-heading" id="categories-heading">
              Categories
            </h2>
            {stats.categories.length ? (
              <ul className="bar-list">
                {stats.categories.map((item) => (
                  <li key={item.id}>
                    <Link to={`/bookmarks?category=${item.id}`}>
                      <Option color={colorFor(item.id)}>{item.name}</Option>
                      <span className="bar-track" aria-hidden="true">
                        <span
                          className={`bar-fill seg-${colorFor(item.id)}`}
                          style={{ width: `${Math.max(4, (item.count / maxCategory) * 100)}%` }}
                        />
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

          <section className="block-section" aria-labelledby="tags-heading">
            <h2 className="block-heading" id="tags-heading">
              Top tags
            </h2>
            {stats.topTags.length ? (
              <div className="tag-cloud">
                {stats.topTags.map((item) => (
                  <Link to={`/bookmarks?tag=${item.id}`} key={item.id} className="tag-link">
                    <Option>{item.name}</Option>
                    <span className="tag-count">{item.count}</span>
                  </Link>
                ))}
              </div>
            ) : (
              <p className="muted-block">Tags appear here once bookmarks are analyzed.</p>
            )}
          </section>
        </div>
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

      <section className="block-section" aria-labelledby="recent-heading">
        <div className="linked-view-header">
          <h2 className="block-heading" id="recent-heading">
            Recently imported
          </h2>
          <span className="linked-view-tab">
            <LayoutGrid size={14} aria-hidden="true" /> Gallery
          </span>
          <Link to="/bookmarks" className="inline-link push">
            Open library <ArrowRight size={14} />
          </Link>
        </div>

        {recent.isLoading && (
          <div className="gallery compact" aria-busy="true">
            {Array.from({ length: 4 }).map((_, index) => (
              <div key={index} className="skeleton skeleton-card" />
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
          <div className="gallery compact">
            {recent.data.items.map((item) => (
              <BookmarkCard bookmark={item} key={item.id} onOpenDrawer={setActiveBookmarkId} />
            ))}
          </div>
        )}
      </section>

      <BookmarkDrawer bookmarkId={activeBookmarkId} onClose={() => setActiveBookmarkId(null)} ids={recent.data?.items.map((item) => item.id)} onNavigate={setActiveBookmarkId} />
    </main>
  );
}
