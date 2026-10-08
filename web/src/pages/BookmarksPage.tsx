import { keepPreviousData, useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  ArrowUpDown,
  BookMarked,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  LoaderCircle,
  CircleDot,
  FolderTree,
  LayoutGrid,
  ListFilter,
  Rows3,
  Search,
  SearchX,
  Table2,
  Tags,
  X,
} from "lucide-react";
import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { BookmarkCard, BookmarkTable } from "../components/BookmarkCard";
import { BookmarkDrawer } from "../components/BookmarkDrawer";
import { Select } from "../components/Select";
import { statusOptions } from "../lib/status";
import { EmptyState, Option, OptionList, PageHeader, Popover, SelectTrigger, ViewTabs } from "../components/ui";
import { colorFor, type OptionColor } from "../lib/format";
import { getBookmarks, getCategories, getTags } from "../lib/api";

type ViewMode = "table" | "gallery";
type PageSize = 25 | 50 | 100 | "continuous";

const CONTINUOUS_BATCH = 50;

function readPageSize(): PageSize {
  try {
    const value = localStorage.getItem("bw.pageSize");
    if (value === "continuous") return value;
    const number = Number(value);
    if (number === 25 || number === 50 || number === 100) return number;
  } catch {
    /* storage unavailable: use the default */
  }
  return 50;
}

// "Needs review" is not a processing state: it finds analyzed bookmarks the AI
// could not confidently file into a category.
const filterStatuses = [...statusOptions, { value: "review", label: "Needs review", tone: "yellow" }];

function readView(): ViewMode {
  try {
    return localStorage.getItem("bw.libraryView") === "gallery" ? "gallery" : "table";
  } catch {
    return "table";
  }
}

/* Remount when the URL changes so links from Overview and the search palette apply their filters. */
export function BookmarksPage() {
  const [params] = useSearchParams();
  return <Library key={params.toString()} initialParams={params} />;
}

function Library({ initialParams }: { initialParams: URLSearchParams }) {
  const [page, setPage] = useState(1);
  const [draft, setDraft] = useState(initialParams.get("q") || "");
  const [query, setQuery] = useState(initialParams.get("q") || "");
  const [status, setStatus] = useState(initialParams.get("status") || "");
  const [sort, setSort] = useState(initialParams.get("sort") || (initialParams.get("q") ? "relevance" : "imported_desc"));
  const [categories, setCategories] = useState<number[]>(
    initialParams.getAll("category").map(Number).filter(Boolean)
  );
  const [tags, setTags] = useState<number[]>(initialParams.getAll("tag").map(Number).filter(Boolean));
  const [showFilters, setShowFilters] = useState(
    Boolean(initialParams.get("status") || initialParams.getAll("category").length || initialParams.getAll("tag").length)
  );
  const [viewMode, setViewMode] = useState<ViewMode>(readView);
  const [activeBookmarkId, setActiveBookmarkId] = useState<number | null>(null);

  const categoryOptions = useQuery({ queryKey: ["categories"], queryFn: getCategories });
  const tagOptions = useQuery({ queryKey: ["tags"], queryFn: getTags });

  const [pageSize, setPageSize] = useState<PageSize>(readPageSize);
  const continuous = pageSize === "continuous";
  const topRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  const filterParams = useMemo(() => {
    const value = new URLSearchParams({ sort });
    if (query) value.set("q", query);
    if (status) value.set("status", status);
    categories.forEach((id) => value.append("category", String(id)));
    tags.forEach((id) => value.append("tag", String(id)));
    return value.toString();
  }, [query, status, sort, categories, tags]);

  const pagedParams = `${filterParams}&page=${page}&pageSize=${continuous ? CONTINUOUS_BATCH : pageSize}`;
  const paged = useQuery({
    queryKey: ["bookmarks", pagedParams],
    queryFn: () => getBookmarks(pagedParams),
    enabled: !continuous,
    placeholderData: keepPreviousData,
  });
  const stream = useInfiniteQuery({
    queryKey: ["bookmarks", "continuous", filterParams],
    queryFn: ({ pageParam }) => getBookmarks(`${filterParams}&page=${pageParam}&pageSize=${CONTINUOUS_BATCH}`),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page * last.pageSize < last.total ? last.page + 1 : undefined),
    enabled: continuous,
  });

  const items = continuous ? (stream.data?.pages.flatMap((result) => result.items) ?? []) : (paged.data?.items ?? []);
  const loaded = continuous ? Boolean(stream.data) : Boolean(paged.data);
  const isLoading = continuous ? stream.isLoading : paged.isLoading;
  const isError = continuous ? stream.isError : paged.isError;

  // Continuous mode loads the next batch as the end of the list scrolls into view.
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = stream;
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!continuous || !sentinel) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && hasNextPage && !isFetchingNextPage) void fetchNextPage();
      },
      { rootMargin: "600px 0px" },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [continuous, hasNextPage, isFetchingNextPage, fetchNextPage, items.length]);

  function goToPage(next: number) {
    setPage(next);
    topRef.current?.scrollIntoView({ block: "start" });
  }

  function changePageSize(value: PageSize) {
    setPageSize(value);
    setPage(1);
    try {
      localStorage.setItem("bw.pageSize", String(value));
    } catch {
      /* storage unavailable: keep the in-memory choice */
    }
  }
  const filterCount = categories.length + tags.length + (status ? 1 : 0);
  const filtersVisible = showFilters || filterCount > 0;

  function changeView(value: ViewMode) {
    setViewMode(value);
    try {
      localStorage.setItem("bw.libraryView", value);
    } catch {
      /* storage unavailable: keep the in-memory choice */
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    setPage(1);
    const next = draft.trim();
    setQuery(next);
    if (next && sort === "imported_desc") setSort("relevance");
  }

  function clearSearch() {
    setDraft("");
    setQuery("");
    if (sort === "relevance") setSort("imported_desc");
    setPage(1);
  }

  function toggle(list: number[], setter: (value: number[]) => void, id: number) {
    setter(list.includes(id) ? list.filter((value) => value !== id) : [...list, id]);
    setPage(1);
  }

  function clearFilters() {
    setCategories([]);
    setTags([]);
    setStatus("");
    setPage(1);
  }

  const categoryItems = categoryOptions.data?.items || [];
  const tagItems = tagOptions.data?.items || [];
  const selectedCategoryNames = categoryItems.filter((item) => categories.includes(item.id));
  const selectedTagNames = tagItems.filter((item) => tags.includes(item.id));
  const statusChoice = filterStatuses.find((item) => item.value === status);
  const total = (continuous ? stream.data?.pages[0]?.total : paged.data?.total) ?? 0;
  const perPage = continuous ? CONTINUOUS_BATCH : pageSize;
  const pageCount = Math.max(1, Math.ceil(total / perPage));

  return (
    <main className="page full">
      <PageHeader
        icon={BookMarked}
        tone="blue"
        title="Library"
        description={
          loaded
            ? `${total.toLocaleString()} ${query || filterCount ? "matching" : "saved"} bookmark${total === 1 ? "" : "s"}. Click any bookmark to preview it.`
            : "Every post you imported from X, searchable and stored locally."
        }
      />

      <div className="view-bar" ref={topRef}>
        <ViewTabs
          label="Library views"
          value={viewMode}
          onChange={changeView}
          tabs={[
            { value: "table", label: "Table", icon: Table2 },
            { value: "gallery", label: "Gallery", icon: LayoutGrid },
          ]}
        />
        <div className="view-bar-tools">
          <form className="db-search" onSubmit={submit} role="search">
            <Search size={15} aria-hidden="true" />
            <input
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              placeholder="Search bookmarks…"
              aria-label="Search bookmarks"
            />
            {draft && (
              <button type="button" className="icon-button" aria-label="Clear search" onClick={clearSearch}>
                <X size={14} />
              </button>
            )}
          </form>
          <button
            type="button"
            className={`tool-button ${filtersVisible ? "active" : ""}`}
            aria-expanded={filtersVisible}
            onClick={() => setShowFilters((value) => !value)}
          >
            <ListFilter size={15} />
            <span>Filter</span>
            {filterCount > 0 && <span className="filter-count">{filterCount}</span>}
          </button>
          <Select
            variant="tool"
            icon={ArrowUpDown}
            label="Sort bookmarks"
            align="end"
            value={sort}
            onChange={(value) => {
              setSort(value);
              setPage(1);
            }}
            options={[
              { value: "imported_desc", label: "Recently imported" },
              { value: "imported_asc", label: "Oldest imported" },
              { value: "posted_desc", label: "Newest post date" },
              { value: "posted_asc", label: "Oldest post date" },
              ...(query ? [{ value: "relevance", label: "Search relevance" }] : []),
            ]}
          />
          {/* In the toolbar rather than the footer: continuous scroll keeps pushing the footer away. */}
          <Select
            variant="tool"
            icon={Rows3}
            label="Bookmarks per page"
            align="end"
            value={String(pageSize)}
            onChange={(value) => changePageSize((value === "continuous" ? "continuous" : Number(value)) as PageSize)}
            options={[
              { value: "25", label: "25 per page" },
              { value: "50", label: "50 per page" },
              { value: "100", label: "100 per page" },
              { value: "continuous", label: "Continuous scroll" },
            ]}
          />
        </div>
      </div>

      {filtersVisible && (
        <div className="filter-row" aria-label="Filters">
          <Popover
            label="Filter by status"
            active={Boolean(status)}
            trigger={
              <SelectTrigger>
                <CircleDot size={14} aria-hidden="true" />
                <span>{statusChoice ? `Status: ${statusChoice.label}` : "Status"}</span>
              </SelectTrigger>
            }
          >
            {(close) => (
              <OptionList
                single
                options={filterStatuses.map((item) => ({ id: item.value, name: item.label, color: item.tone as OptionColor }))}
                selected={[status]}
                onToggle={(id) => {
                  setStatus(status === id ? "" : String(id));
                  setPage(1);
                  close();
                }}
              />
            )}
          </Popover>

          <Popover
            label="Filter by category"
            active={categories.length > 0}
            trigger={
              <SelectTrigger>
                <FolderTree size={14} aria-hidden="true" />
                <span>
                  {selectedCategoryNames.length
                    ? `Categories: ${selectedCategoryNames.map((item) => item.name).join(", ")}`
                    : "Categories"}
                </span>
              </SelectTrigger>
            }
          >
            {() => (
              <>
                <p className="menu-caption">Bookmarks must match every selected category.</p>
                <OptionList
                  options={categoryItems
                    .filter((item) => item.active)
                    .map((item) => ({ id: item.id, name: item.name, color: colorFor(item.id), meta: item.count }))}
                  selected={categories}
                  onToggle={(id) => toggle(categories, setCategories, Number(id))}
                  placeholder="Search categories…"
                  emptyText="No matching categories"
                />
              </>
            )}
          </Popover>

          <Popover
            label="Filter by tag"
            active={tags.length > 0}
            trigger={
              <SelectTrigger>
                <Tags size={14} aria-hidden="true" />
                <span>
                  {selectedTagNames.length ? `Tags: ${selectedTagNames.map((item) => item.name).join(", ")}` : "Tags"}
                </span>
              </SelectTrigger>
            }
          >
            {() => (
              <>
                <p className="menu-caption">Bookmarks must match every selected tag.</p>
                <OptionList
                  options={tagItems.map((item) => ({ id: item.id, name: item.name, meta: item.count }))}
                  selected={tags}
                  onToggle={(id) => toggle(tags, setTags, Number(id))}
                  placeholder="Search tags…"
                  emptyText={tagItems.length ? "No matching tags" : "Tags appear after AI analysis."}
                />
              </>
            )}
          </Popover>

          {filterCount > 0 && (
            <button type="button" className="text-button" onClick={clearFilters}>
              Reset
            </button>
          )}
        </div>
      )}

      {query && (
        <div className="search-summary">
          <span>
            Results for <Option color="blue">{query}</Option>
          </span>
          <button type="button" className="text-button" onClick={clearSearch}>
            Clear search
          </button>
        </div>
      )}

      {isLoading && (
        <div className="skeleton-table" aria-busy="true" aria-label="Loading bookmarks">
          {Array.from({ length: 8 }).map((_, index) => (
            <div key={index} className="skeleton skeleton-row" />
          ))}
        </div>
      )}

      {isError && (
        <EmptyState icon={SearchX} title="Bookmarks could not be loaded">
          The local server did not answer. Check that Sortwise is still running, or try a simpler search.
        </EmptyState>
      )}

      {loaded && items.length === 0 && (
        <EmptyState
          icon={SearchX}
          title={query || filterCount ? "No matching bookmarks" : "Your library is empty"}
          action={
            (query || filterCount > 0) && (
              <button
                className="button secondary"
                onClick={() => {
                  clearFilters();
                  clearSearch();
                }}
              >
                Clear search and filters
              </button>
            )
          }
        >
          {query || filterCount
            ? "Try removing a filter or using fewer search terms."
            : "Pair the extension from Overview, then start an import on X."}
        </EmptyState>
      )}

      {items.length > 0 &&
        (viewMode === "table" ? (
          <BookmarkTable bookmarks={items} onOpenDrawer={setActiveBookmarkId} />
        ) : (
          <div className="gallery">
            {items.map((item) => (
              <BookmarkCard bookmark={item} key={item.id} onOpenDrawer={setActiveBookmarkId} />
            ))}
          </div>
        ))}

      {continuous && <div ref={sentinelRef} className="scroll-sentinel" aria-hidden="true" />}

      {total > 0 && (
        <div className="db-footer">
          <span className="db-count">
            Count <strong>{total.toLocaleString()}</strong>
            {continuous && items.length < total && (
              <span className="muted">
                {" "}
                · showing {items.length.toLocaleString()}
                {isFetchingNextPage && <LoaderCircle size={13} className="spin inline-spinner" aria-label="Loading more" />}
              </span>
            )}
          </span>
          <div className="db-footer-tools">
            {!continuous && total > perPage && <Pager page={page} pageCount={pageCount} onChange={goToPage} />}
          </div>
        </div>
      )}

      <BookmarkDrawer bookmarkId={activeBookmarkId} onClose={() => setActiveBookmarkId(null)} ids={items.map((item) => item.id)} onNavigate={setActiveBookmarkId} />
    </main>
  );
}

/* First / previous / typed page number / next / last. */
function Pager({ page, pageCount, onChange }: { page: number; pageCount: number; onChange: (page: number) => void }) {
  function commit(input: HTMLInputElement) {
    const next = Math.min(pageCount, Math.max(1, Number.parseInt(input.value, 10) || page));
    input.value = String(next);
    if (next !== page) onChange(next);
  }

  return (
    <nav className="pager" aria-label="Bookmark pages">
      <button className="icon-button" disabled={page === 1} onClick={() => onChange(1)} aria-label="First page" title="First page">
        <ChevronsLeft size={16} />
      </button>
      <button className="icon-button" disabled={page === 1} onClick={() => onChange(page - 1)} aria-label="Previous page" title="Previous page">
        <ChevronLeft size={16} />
      </button>
      <label className="pager-jump">
        <span>Page</span>
        {/* Uncontrolled so a typed number survives until Enter or blur; remounts when the page changes. */}
        <input
          key={page}
          defaultValue={String(page)}
          inputMode="numeric"
          aria-label={`Page number, 1 to ${pageCount}`}
          onFocus={(event) => event.target.select()}
          onBlur={(event) => commit(event.currentTarget)}
          onKeyDown={(event) => {
            if (event.key === "Enter") commit(event.currentTarget);
            if (event.key === "Escape") {
              event.currentTarget.value = String(page);
              event.currentTarget.blur();
            }
          }}
          style={{ width: `${Math.max(2, String(pageCount).length) + 1.5}ch` }}
        />
        <span>of {pageCount}</span>
      </label>
      <button className="icon-button" disabled={page >= pageCount} onClick={() => onChange(page + 1)} aria-label="Next page" title="Next page">
        <ChevronRight size={16} />
      </button>
      <button className="icon-button" disabled={page >= pageCount} onClick={() => onChange(pageCount)} aria-label="Last page" title="Last page">
        <ChevronsRight size={16} />
      </button>
    </nav>
  );
}
