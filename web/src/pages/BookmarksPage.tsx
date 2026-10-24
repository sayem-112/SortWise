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
  List as ListIcon,
  ListPlus,
  Rows3,
  Search,
  SearchX,
  Sparkles,
  Table2,
  Tags,
  X,
  Zap,
} from "lucide-react";
import { FormEvent, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { BookmarkCard, BookmarkTable, MeaningMatch, type ListRemoval } from "../components/BookmarkCard";
import { CollectionChanges, SaveCollection } from "../components/Collections";
import { SelectionBar } from "../components/SelectionBar";
import { Select } from "../components/Select";
import { statusOptions } from "../lib/status";
import { EmptyState, Option, OptionList, PageHeader, Popover, SelectTrigger, ViewTabs } from "../components/ui";
import { colorFor, type OptionColor } from "../lib/format";
import { getAISettings, getBookmarks, getCategories, getTags, type Bookmark, type Collection, type CollectionQuery, type List } from "../lib/api";
import { collectionParams, defaultSort as collectionSort, hasCriteria, sameQuery } from "../lib/collections";
import { listIcon, listsQuery, listTone, useListChange } from "../lib/lists";
import { usePeek } from "../lib/peek";

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

// Whether searches also look for posts about the same thing; remembered,
// since someone who wants that wants it every time.
function readMeaning() {
  try {
    return localStorage.getItem("bw.searchMode") === "meaning";
  } catch {
    return false;
  }
}

// The table needs room; a phone opens on the gallery unless told otherwise.
function readView(): ViewMode {
  const narrow = typeof window !== "undefined" && window.innerWidth < 768;
  try {
    const saved = localStorage.getItem("bw.libraryView");
    if (saved === "gallery" || saved === "table") return saved;
  } catch {
    /* storage unavailable: use the default */
  }
  return narrow ? "gallery" : "table";
}

// The category and tag filters, handed to a page's header so its own
// controls (an author's topics) can switch them like the filter row does.
export type LibraryFilters = {
  categories: number[];
  tags: number[];
  toggleCategory: (id: number) => void;
  toggleTag: (id: number) => void;
};
type Heading = { title?: ReactNode; icon?: ReactNode; actions?: ReactNode | ((filters: LibraryFilters) => ReactNode) };
// What the page is narrowed to: one list, one author's handle, or a smart
// collection's saved search.
type Scope = { list?: List; author?: string; collection?: Collection };

/* The library. With a list it shows only that list's bookmarks, newest
   additions first; with an author, only their posts, newest post first;
   with a smart collection it opens on the collection's saved search.
   It remounts when the URL changes, so links from Overview and the finder
   apply their filters. */
export function BookmarksPage({ list, author, collection, ...heading }: Scope & Heading) {
  const [params] = useSearchParams();
  // Discarding changes to a collection starts its page over.
  const [revision, setRevision] = useState(0);
  return (
    <Library
      key={`${list?.id ?? ""}|${author ?? ""}|${collection ? `${collection.id}.${revision}` : ""}?${collection ? "" : params.toString()}`}
      initialParams={collection ? collectionParams(collection.query) : params}
      list={list}
      author={author}
      collection={collection}
      onDiscard={() => setRevision((value) => value + 1)}
      {...heading}
    />
  );
}

function Library({ initialParams, list, author: pageAuthor, collection, onDiscard, title, icon, actions }: { initialParams: URLSearchParams; onDiscard: () => void } & Scope & Heading) {
  // A collection saved on an author's page keeps to that author, until that is removed.
  const [collectionAuthor, setCollectionAuthor] = useState(collection?.query.author || "");
  const author = pageAuthor || collectionAuthor || undefined;
  const defaultSort = list ? "added_desc" : pageAuthor ? "posted_desc" : "imported_desc";
  const [page, setPage] = useState(1);
  const [draft, setDraft] = useState(initialParams.get("q") || "");
  const [query, setQuery] = useState(initialParams.get("q") || "");
  const [status, setStatus] = useState(initialParams.get("status") || "");
  const [sort, setSort] = useState(initialParams.get("sort") || (initialParams.get("q") ? "relevance" : defaultSort));
  // Also find posts about the same thing, in whatever words.
  const [meaning, setMeaning] = useState(() => (initialParams.get("mode") ? initialParams.get("mode") === "meaning" : readMeaning()));
  const [categories, setCategories] = useState<number[]>(
    initialParams.getAll("category").map(Number).filter(Boolean)
  );
  const [tags, setTags] = useState<number[]>(initialParams.getAll("tag").map(Number).filter(Boolean));
  // In the library: one list, or "none" for bookmarks in no list.
  const [inList, setInList] = useState(list ? "" : initialParams.get("list") || "");
  const [showFilters, setShowFilters] = useState(
    Boolean(initialParams.get("status") || initialParams.getAll("category").length || initialParams.getAll("tag").length || inList || collectionAuthor)
  );
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [anchor, setAnchor] = useState<number | null>(null);
  // The row the keyboard is on in the table (J/K), or null.
  const [cursor, setCursor] = useState<number | null>(null);
  // Rows just taken out of the list being viewed, kept in place until the view changes.
  const [removed, setRemoved] = useState<Map<number, { bookmark: Bookmark; index: number }>>(new Map());
  const listChange = useListChange();
  const [viewMode, setViewMode] = useState<ViewMode>(readView);
  // The post preview belongs to the app shell; this page opens it and watches it.
  const peek = usePeek();
  const activeBookmarkId = peek.current;

  const categoryOptions = useQuery({ queryKey: ["categories"], queryFn: getCategories });
  const tagOptions = useQuery({ queryKey: ["tags"], queryFn: getTags });
  const listOptions = useQuery(listsQuery);
  const ai = useQuery({ queryKey: ["ai-settings"], queryFn: getAISettings, staleTime: 30_000 });
  const meaningStatus = ai.data?.meaning;
  // Turned off in Settings, the toggle is hidden and nothing asks for it.
  const byMeaning = meaning && meaningStatus?.enabled !== false;

  const [pageSize, setPageSize] = useState<PageSize>(readPageSize);
  const continuous = pageSize === "continuous";
  const topRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  const filterParams = useMemo(() => {
    const value = new URLSearchParams({ sort });
    if (list) value.set("list", String(list.id));
    else if (inList) value.set("list", inList);
    if (author) value.set("author", author);
    if (query) value.set("q", query);
    if (query && byMeaning) value.set("mode", "meaning");
    if (status) value.set("status", status);
    categories.forEach((id) => value.append("category", String(id)));
    tags.forEach((id) => value.append("tag", String(id)));
    return value.toString();
  }, [list, author, inList, query, byMeaning, status, sort, categories, tags]);

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

  const fetched = useMemo(
    () => (continuous ? (stream.data?.pages.flatMap((result) => result.items) ?? []) : (paged.data?.items ?? [])),
    [continuous, stream.data, paged.data],
  );
  // Removed rows stay where they were (faded, with Undo) so the next row never
  // slides under the pointer; a row put back in the list is live again.
  const ghosts = useMemo(() => {
    const ids = new Set<number>();
    if (!list) return ids;
    removed.forEach((_, id) => {
      const live = fetched.find((item) => item.id === id);
      if (!live || !live.lists.includes(list.id)) ids.add(id);
    });
    return ids;
  }, [removed, fetched, list]);
  const items = useMemo(() => {
    const rows = [...fetched];
    [...removed.values()]
      .filter((entry) => ghosts.has(entry.bookmark.id) && !fetched.some((item) => item.id === entry.bookmark.id))
      .sort((a, b) => a.index - b.index)
      .forEach((entry) => rows.splice(Math.min(entry.index, rows.length), 0, entry.bookmark));
    return rows;
  }, [fetched, removed, ghosts]);
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

  // A selection belongs to what is on screen; it clears when the view changes.
  useEffect(() => {
    setSelected(new Set());
    setAnchor(null);
    setCursor(null);
    setRemoved(new Map());
  }, [filterParams, page, pageSize]);

  function markRemoved(bookmarks: Bookmark[]) {
    if (!list) return;
    setRemoved((current) => {
      const next = new Map(current);
      for (const bookmark of bookmarks) {
        const index = items.findIndex((item) => item.id === bookmark.id);
        next.set(bookmark.id, { bookmark: { ...bookmark, lists: bookmark.lists.filter((id) => id !== list.id) }, index });
      }
      return next;
    });
  }
  const removal: ListRemoval | undefined = list ? { ghosts, onRemoved: (bookmark) => markRemoved([bookmark]) } : undefined;

  function openBookmark(id: number) {
    setCursor(items.findIndex((item) => item.id === id));
    peek.open(id, items.filter((item) => !ghosts.has(item.id)).map((item) => item.id));
  }

  // Back in the table, the keyboard picks up from the post just read.
  const lastOpened = useRef<number | null>(null);
  useEffect(() => {
    if (activeBookmarkId !== null) {
      lastOpened.current = activeBookmarkId;
      return;
    }
    const index = items.findIndex((item) => item.id === lastOpened.current);
    lastOpened.current = null;
    if (index >= 0) setCursor(index);
    // Only when the preview opens or closes, not each time the rows refresh.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeBookmarkId]);

  // Keyboard filing in the table: J/K move, X selects (Shift+X a range),
  // Enter opens, F stars, L opens the Lists menu, Esc steps back.
  useEffect(() => {
    if (viewMode !== "table" || activeBookmarkId) return;
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target instanceof Element ? event.target : null;
      if (target?.closest("input, textarea, select, [contenteditable='true']")) return;
      if (document.querySelector(".menu.floating, .confirm-backdrop, [aria-modal='true']")) return;
      const count = items.length;
      if (!count) return;
      const key = event.key.toLowerCase();
      const row = cursor === null ? null : items[cursor];
      if (key === "j" || key === "k") {
        event.preventDefault();
        setCursor((value) => (value === null ? 0 : Math.max(0, Math.min(count - 1, value + (key === "j" ? 1 : -1)))));
      } else if (key === "x" && row) {
        event.preventDefault();
        select(row.id, event.shiftKey);
      } else if ((key === "enter" || key === "o") && row && !ghosts.has(row.id) && !target?.closest("button, a")) {
        event.preventDefault();
        openBookmark(row.id);
      } else if (key === "f") {
        const favorites = listOptions.data?.items.find((item) => item.kind === "favorites");
        const targets = selected.size ? items.filter((item) => selected.has(item.id)) : row ? [row] : [];
        if (!favorites || !targets.length) return;
        event.preventDefault();
        const star = !targets.every((item) => item.lists.includes(favorites.id));
        const ids = targets.filter((item) => item.lists.includes(favorites.id) !== star).map((item) => item.id);
        if (ids.length) listChange.apply({ list: favorites, ids, inList: star, bulk: selected.size > 0 });
      } else if (key === "l") {
        const trigger = selected.size
          ? document.querySelector<HTMLButtonElement>(".selection-menu > button")
          : cursor !== null
            ? document.querySelector<HTMLButtonElement>(`tr[data-row="${cursor}"] .cell-editor > button`)
            : null;
        if (!trigger) return;
        event.preventDefault();
        trigger.click();
      } else if (key === "escape" && cursor !== null && !selected.size) {
        setCursor(null);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  useEffect(() => {
    if (cursor !== null) document.querySelector(`tr[data-row="${cursor}"]`)?.scrollIntoView?.({ block: "nearest" });
  }, [cursor]);

  useEffect(() => {
    if (!selected.size || activeBookmarkId) return;
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape" && !(event.target instanceof Element && event.target.closest(".menu, input, textarea"))) setSelected(new Set());
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selected.size, activeBookmarkId]);

  function select(id: number, range: boolean) {
    const ids = items.map((item) => item.id);
    const next = new Set(selected);
    if (range && anchor !== null && ids.includes(anchor)) {
      const [from, to] = [ids.indexOf(anchor), ids.indexOf(id)].sort((a, b) => a - b);
      ids.slice(from, to + 1).forEach((value) => next.add(value));
    } else if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    setSelected(next);
    setAnchor(id);
  }

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
  const filterCount = categories.length + tags.length + (status ? 1 : 0) + (inList ? 1 : 0) + (collectionAuthor ? 1 : 0);
  const filtersVisible = showFilters || filterCount > 0;

  function changeView(value: ViewMode) {
    setViewMode(value);
    setSelected(new Set());
    setCursor(null);
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
    if (next && sort === defaultSort) setSort("relevance");
  }

  function toggleMeaning() {
    const next = !meaning;
    setMeaning(next);
    setPage(1);
    // Words typed but not yet searched for are searched for now.
    const words = draft.trim();
    if (words !== query) {
      setQuery(words);
      if (words && sort === defaultSort) setSort("relevance");
    }
    // On a collection's page the choice belongs to the collection.
    if (collection) return;
    try {
      localStorage.setItem("bw.searchMode", next ? "meaning" : "words");
    } catch {
      /* storage unavailable: keep the in-memory choice */
    }
  }

  function clearSearch() {
    setDraft("");
    setQuery("");
    if (sort === "relevance") setSort(defaultSort);
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
    setInList("");
    setCollectionAuthor("");
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
  const listItems = listOptions.data?.items || [];
  // A saved search can outlive the list it filters on.
  const listChoice = inList === "none" ? "In no list" : (listItems.find((item) => String(item.id) === inList)?.name ?? (inList && listOptions.data ? "Deleted list" : undefined));
  // A list with nothing in it gets a single next step instead of an empty database view.
  const emptyList = Boolean(list && loaded && items.length === 0 && !query && !filterCount);
  const selectedItems = items.filter((item) => selected.has(item.id));

  // How the search went, when it was asked to look by meaning.
  // Not while the previous search's results are still standing in.
  const searchInfo = continuous ? stream.data?.pages[0]?.search : paged.isPlaceholderData ? undefined : paged.data?.search;
  const stillIndexing = Boolean(meaningStatus && meaningStatus.available && meaningStatus.indexed < meaningStatus.total);
  // The search and filters on screen, in the form a smart collection saves.
  const criteria: CollectionQuery = {
    q: query,
    mode: query && meaning ? "meaning" : "",
    categories,
    tags,
    status,
    list: list ? String(list.id) : inList,
    author: author || "",
    // A list's own order means nothing outside the list.
    sort: sort === collectionSort(query) || sort === "added_desc" ? "" : sort,
  };
  const changed = Boolean(collection && !sameQuery(criteria, collection.query));
  // A name to offer for a new collection: the words searched for, or the first filter.
  const suggestedName = `${query || selectedCategoryNames[0]?.name || selectedTagNames[0]?.name || statusChoice?.label || listChoice || ""}${pageAuthor ? ` from @${pageAuthor}` : ""}`
    .trim()
    .replace(/^./, (first) => first.toUpperCase())
    .slice(0, 80);

  return (
    <main className="page full">
      <PageHeader
        icon={list ? listIcon(list) : collection ? Zap : BookMarked}
        tone={list ? listTone(list) : collection ? collection.color : "blue"}
        iconSlot={icon}
        title={title || (list ? list.name : collection ? collection.name : "Library")}
        description={
          emptyList
            ? undefined
            : collection
              ? loaded
                ? `${total.toLocaleString()} matching bookmark${total === 1 ? "" : "s"}. This collection fills itself as posts match its search.`
                : "A saved search that fills itself."
            : loaded
              ? pageAuthor
                ? `${total.toLocaleString()} ${query || filterCount ? "matching" : "saved"} post${total === 1 ? "" : "s"}. Click any post to preview it.`
                : `${total.toLocaleString()} ${query || filterCount ? "matching " : list ? "" : "saved "}bookmark${total === 1 ? "" : "s"}${list && !query && !filterCount ? " in this list" : ""}.${total ? " Click any bookmark to preview it." : ""}`
              : pageAuthor
                ? `Posts you saved from @${pageAuthor}.`
                : list
                  ? "Bookmarks you put in this list."
                  : "Every post you imported from X, searchable and stored locally."
        }
      >
        {typeof actions === "function"
          ? actions({
              categories,
              tags,
              toggleCategory: (id) => toggle(categories, setCategories, id),
              toggleTag: (id) => toggle(tags, setTags, id),
            })
          : actions}
      </PageHeader>

      {emptyList && (
        <EmptyState
          icon={ListPlus}
          title={list?.kind === "favorites" ? "Star posts to keep them here" : `Add posts to ${list?.name}`}
          action={
            <Link to="/bookmarks" className="button secondary">
              Go to the library
            </Link>
          }
        >
          {list?.kind === "favorites"
            ? "In the library, click the star on any post, or press F on it."
            : "In the library, click Add in a post's Lists column, or tick several posts and choose Add to list. From the keyboard: J/K to a post, then L."}
        </EmptyState>
      )}

      {!emptyList && (
      <>
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
              placeholder={byMeaning ? "Search by meaning…" : "Search bookmarks…"}
              aria-label="Search bookmarks"
            />
            {draft && (
              <button type="button" className="icon-button" aria-label="Clear search" onClick={clearSearch}>
                <X size={14} />
              </button>
            )}
            {meaningStatus?.enabled !== false && (
              <button
                type="button"
                className={`search-mode ${meaning ? "on" : ""}`}
                aria-pressed={meaning}
                onClick={toggleMeaning}
                title={
                  meaning
                    ? "Also finding posts about the same thing, even in other words. Your search words are sent to Gemini. Click to match words only."
                    : "Also find posts about the same thing, even in other words. Sends your search words to Gemini."
                }
              >
                <Sparkles size={13} aria-hidden="true" />
                <span>Meaning</span>
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
          {!collection && (query || filterCount > 0) && <SaveCollection key={suggestedName} query={criteria} suggested={suggestedName} />}
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
              ...(list ? [{ value: "added_desc", label: "Recently added to list" }] : []),
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

          {!list && (
            <Popover
              label="Filter by list"
              active={Boolean(inList)}
              trigger={
                <SelectTrigger>
                  <ListIcon size={14} aria-hidden="true" />
                  <span>{listChoice ? `List: ${listChoice}` : "List"}</span>
                </SelectTrigger>
              }
            >
              {(close) => (
                <OptionList
                  single
                  options={[
                    ...listItems.map((item) => ({ id: String(item.id), name: item.name, meta: item.count, icon: listIcon(item), iconClass: `tone-${listTone(item)}` })),
                    { id: "none", name: "In no list", icon: ListIcon, iconClass: "tone-gray" },
                  ]}
                  selected={[inList]}
                  onToggle={(id) => {
                    setInList(inList === id ? "" : String(id));
                    setPage(1);
                    close();
                  }}
                  placeholder="Search lists…"
                />
              )}
            </Popover>
          )}

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

          {collectionAuthor && (
            <Option color="pink" onRemove={() => { setCollectionAuthor(""); setPage(1); }} removeLabel={`Stop limiting to @${collectionAuthor}`}>
              From @{collectionAuthor}
            </Option>
          )}

          {filterCount > 0 && (
            <button type="button" className="text-button" onClick={clearFilters}>
              Reset
            </button>
          )}
        </div>
      )}

      {collection && changed && <CollectionChanges collection={collection} query={criteria} canSave={hasCriteria(criteria)} onDiscard={onDiscard} />}

      {query && (
        <div className="search-summary">
          <span>
            Results for <Option color="blue">{query}</Option>
            {searchInfo?.meaning && (
              <>
                {" "}
                and posts about the same thing{items.some((item) => item.match === "meaning") && <>, marked <MeaningMatch compact /></>}
              </>
            )}
          </span>
          <button type="button" className="text-button" onClick={clearSearch}>
            Clear search
          </button>
          {byMeaning && searchInfo && !searchInfo.meaning && (
            <span className="search-notice warn" role="status">
              Showing posts with these words only. {searchInfo.notice}
              {searchInfo.notice?.includes("Settings") && (
                <>
                  {" "}
                  <Link to="/settings">Open Settings</Link>
                </>
              )}
            </span>
          )}
          {searchInfo?.meaning && stillIndexing && meaningStatus && (
            <span className="search-notice" role="status">
              {meaningStatus.indexed.toLocaleString()} of {meaningStatus.total.toLocaleString()} posts are ready for search by meaning. The rest are being prepared.
            </span>
          )}
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

      {loaded && items.length === 0 && !emptyList && (
        <EmptyState
          icon={SearchX}
          title={
            collection && !changed
              ? "Nothing matches this collection yet"
              : query || filterCount
                ? pageAuthor
                  ? `No matching posts from @${pageAuthor}`
                  : "No matching bookmarks"
                : pageAuthor
                  ? `No saved posts from @${pageAuthor}`
                  : "Your library is empty"
          }
          action={
            !(collection && !changed) &&
            (query || filterCount > 0) && (
              <span className="empty-actions">
                <button
                  className="button secondary"
                  onClick={() => {
                    clearFilters();
                    clearSearch();
                  }}
                >
                  Clear search and filters
                </button>
                {/* Other words for the same thing may be what the posts use. */}
                {query && !byMeaning && meaningStatus?.enabled && meaningStatus.available && meaningStatus.indexed > 0 && (
                  <button className="button secondary" onClick={toggleMeaning}>
                    <Sparkles size={14} aria-hidden="true" />
                    Search by meaning instead
                  </button>
                )}
                {/* On one author's or list's page, the words may well match elsewhere. */}
                {query && (pageAuthor || list) && (
                  <Link to={`/bookmarks?q=${encodeURIComponent(query)}`} className="button secondary">
                    Search the whole library for “{query}”
                  </Link>
                )}
              </span>
            )
          }
        >
          {collection && !changed
            ? "Posts show up here on their own as they are saved and organized."
            : query || filterCount
            ? filterCount
              ? "Try removing a filter or using fewer search terms."
              : meaning
                ? "Try fewer or different words."
                : "Try fewer or different words, or search by meaning."
            : pageAuthor
              ? "Their posts may have been deleted or archived."
              : "Pair the extension from Overview, then start an import on X."}
        </EmptyState>
      )}

      {items.length > 0 &&
        (viewMode === "table" ? (
          <BookmarkTable
            bookmarks={items}
            onOpenDrawer={openBookmark}
            list={list}
            selected={selected}
            onSelect={select}
            onSelectAll={(all) => setSelected(new Set(all ? items.filter((item) => !ghosts.has(item.id)).map((item) => item.id) : []))}
            cursor={cursor}
            removal={removal}
            hideAuthor={Boolean(pageAuthor)}
          />
        ) : (
          <div className="gallery">
            {items.map((item) => (
              <BookmarkCard bookmark={item} key={item.id} onOpenDrawer={openBookmark} list={list} removal={removal} />
            ))}
          </div>
        ))}

      {selectedItems.length > 0 && viewMode === "table" && (
        <SelectionBar selected={selectedItems} list={list} onClear={() => setSelected(new Set())} onRemoved={markRemoved} />
      )}

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
          {viewMode === "table" && (
            <span className="kbd-hint" aria-hidden="true">
              <kbd>J</kbd>
              <kbd>K</kbd> move · <kbd>X</kbd> select · <kbd>F</kbd> star · <kbd>L</kbd> lists
            </span>
          )}
          <div className="db-footer-tools">
            {!continuous && total > perPage && <Pager page={page} pageCount={pageCount} onChange={goToPage} />}
          </div>
        </div>
      )}

      </>
      )}
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
