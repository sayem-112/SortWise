import { useQuery } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowUp,
  CornerDownLeft,
  Download,
  FolderTree,
  History,
  ListPlus,
  LoaderCircle,
  Search,
  Tag,
  X,
  type LucideIcon,
} from "lucide-react";
import { Fragment, useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { getCategories, getTags, quickSearch, type QuickResults } from "../lib/api";
import { authorPath } from "../lib/authors";
import { colorFor, formatDate, modKey } from "../lib/format";
import { listIcon, listsQuery, listTone } from "../lib/lists";
import { navigation } from "../lib/navigation";
import { recentlyOpened } from "../lib/recent";
import { AuthorAvatar } from "./Author";

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
  onNewList: () => void;
  // Previews a post over the current page; `ids` are the posts beside it.
  onOpenBookmark: (id: number, ids: number[]) => void;
}

/* One row. Posts and people draw their own content in `body`; everything
   else is an icon and a label. `run` gets `true` for "open as a full page". */
type Row = {
  id: string;
  group: string;
  run: (asPage: boolean) => void;
  icon?: LucideIcon;
  tone?: string;
  label?: string;
  hint?: string;
  lead?: ReactNode;
  body?: ReactNode;
  // Came from a search for earlier words, kept on screen while the new one runs.
  stale?: boolean;
};

// The server wraps each matched word in these two characters.
const MATCH_START = String.fromCharCode(2);
const MATCH_END = String.fromCharCode(3);
// Searching starts at this many characters.
const MIN_SEARCH = 2;
const EMPTY_LISTS = 5;

/* An excerpt from the server, with the matched words highlighted. */
function Excerpt({ text }: { text: string }) {
  return (
    <>
      {text.split(MATCH_START).map((part, index) => {
        if (index === 0) return <Fragment key={index}>{part}</Fragment>;
        const [match, ...after] = part.split(MATCH_END);
        return (
          <Fragment key={index}>
            <mark>{match}</mark>
            {after.join("")}
          </Fragment>
        );
      })}
    </>
  );
}

/* A beat after typing stops, so each keystroke doesn't start a search. */
function useSettled(value: string, delay: number) {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);
  return settled;
}

/* True when the text starts the name or any word in it: "la" finds
   "Read later" but not "Relax". */
function startsWord(name: string, needle: string) {
  const lower = name.toLowerCase();
  return lower.startsWith(needle) || lower.split(/[^\p{L}\p{N}]+/u).some((word) => word.startsWith(needle));
}

/* The finder: type a few words to reach any post, person, list, category,
   tag, or page. Enter previews a post where you are; Ctrl+Enter opens it. */
export function CommandPalette({ open, onClose, onNewList, onOpenBookmark }: CommandPaletteProps) {
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  // The last results received and the words they were found for.
  const [shown, setShown] = useState<{ query: string; data: QuickResults } | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  // Enter pressed before the results for the typed words arrived: run the top
  // row once they do. Holds whether it was Ctrl+Enter.
  const pendingEnter = useRef<boolean | null>(null);
  // Set when a post was previewed, so reopening the finder returns to the search.
  const keepSearch = useRef(false);
  const navigate = useNavigate();
  const id = useId();
  const trimmed = query.trim();
  const needle = trimmed.length >= MIN_SEARCH ? trimmed.toLowerCase() : "";
  const settled = useSettled(needle, 140);

  const results = useQuery({
    queryKey: ["quick-search", settled],
    queryFn: () => quickSearch(settled),
    enabled: open && settled !== "",
    staleTime: 15_000,
  });
  const categories = useQuery({ queryKey: ["categories"], queryFn: getCategories, enabled: open });
  const tags = useQuery({ queryKey: ["tags"], queryFn: getTags, enabled: open });
  const lists = useQuery({ ...listsQuery, enabled: open });
  // Read when the finder opens, so the list is steady while it is up.
  const recent = useMemo(() => (open ? recentlyOpened() : []), [open]);

  useEffect(() => {
    if (results.data && settled) setShown({ query: settled, data: results.data });
  }, [results.data, settled]);

  useEffect(() => {
    if (open) {
      // Back from a preview: the search is still there, selected, ready to refine or replace.
      inputRef.current?.select();
      keepSearch.current = false;
    } else if (!keepSearch.current) {
      setQuery("");
      setSelectedIndex(0);
      setShown(null);
    }
    pendingEnter.current = null;
  }, [open]);

  // Results are only shown for the words they were found for, or while the
  // words are still being extended or trimmed, never for a different search.
  const related = shown && needle && (needle.startsWith(shown.query) || shown.query.startsWith(needle));
  const found = related ? shown.data : undefined;
  // If the search itself fails there is nothing to wait for.
  const failed = results.isError && settled === needle;
  const stale = needle !== "" && shown?.query !== needle && !failed;

  const rows = useMemo<Row[]>(() => {
    const go = (path: string) => () => {
      navigate(path);
      onClose();
    };
    const post = (postId: number, ids: number[]) => (asPage: boolean) => {
      if (asPage) {
        navigate(`/bookmarks/${postId}`);
      } else {
        keepSearch.current = true;
        onOpenBookmark(postId, ids);
      }
      onClose();
    };

    const allLists: Row[] = (lists.data?.items || []).map((item) => ({
      id: `l-${item.id}`,
      group: "Lists",
      label: item.name,
      hint: item.count.toLocaleString(),
      icon: listIcon(item),
      tone: listTone(item),
      run: go(`/lists/${item.id}`),
    }));
    const newList: Row = {
      id: "new-list",
      group: "Actions",
      label: "New list",
      icon: ListPlus,
      run: () => {
        onNewList();
        onClose();
      },
    };
    const download = (label: string, file: string): Row => ({
      id: `export-${file}`,
      group: "Actions",
      label,
      icon: Download,
      run: () => {
        window.location.href = `/api/v1/exports/${file}`;
        onClose();
      },
    });

    if (!needle) {
      const recentIds = recent.map((item) => item.id);
      return [
        ...recent.map((item): Row => ({
          id: `r-${item.id}`,
          group: "Recently opened",
          run: post(item.id, recentIds),
          lead: <AuthorAvatar name={item.author} username={item.username} />,
          body: (
            <span className="command-label">
              <span className="command-post-head">
                <strong>{item.author || `@${item.username}`}</strong>
                <span className="handle">@{item.username}</span>
              </span>
              <span className="command-detail">{item.preview}</span>
            </span>
          ),
        })),
        ...allLists.slice(0, EMPTY_LISTS),
        ...navigation.map((item): Row => ({
          id: `nav-${item.to}`,
          group: "Jump to",
          label: item.label,
          hint: `${modKey}${item.shortcut}`,
          icon: item.icon,
          tone: item.tone,
          run: go(item.to),
        })),
        newList,
        download("Export library as JSON", "bookmarks.json"),
        download("Export library as CSV", "bookmarks.csv"),
      ];
    }

    const ids = (found?.bookmarks || []).map((item) => item.id);
    const posts: Row[] = (found?.bookmarks || []).map((item) => {
      // Where the words matched, when it wasn't in the post's own text.
      const why = item.summary ? { label: "Summary", text: item.summary } : item.media ? { label: "In the image", text: item.media } : item.taxonomy ? { label: "Tagged", text: item.taxonomy } : null;
      return {
        id: `b-${item.id}`,
        group: "Posts",
        stale,
        run: post(item.id, ids),
        lead: <AuthorAvatar name={item.author} username={item.username} />,
        body: (
          <span className="command-label">
            <span className="command-post-head">
              <strong>{item.authorMatch ? <Excerpt text={item.authorMatch} /> : item.author || `@${item.username}`}</strong>
              <span className="handle">@{item.usernameMatch ? <Excerpt text={item.usernameMatch} /> : item.username}</span>
              <time>{formatDate(item.postedAt || item.importedAt)}</time>
            </span>
            <span className="command-detail">
              <Excerpt text={item.text} />
            </span>
            {why && (
              <span className="command-detail command-why">
                <span className="command-why-label">{why.label}</span> <Excerpt text={why.text} />
              </span>
            )}
          </span>
        ),
      };
    });
    // With nothing found there is nothing more to see in the library either.
    const seeAll: Row[] =
      found && found.total === 0
        ? []
        : [
            {
              id: "search-all",
              group: "Posts",
              label: found && !stale && found.total > posts.length ? `See all ${found.total.toLocaleString()} results in the library` : `Search the library for “${trimmed}”`,
              icon: Search,
              run: go(`/bookmarks?q=${encodeURIComponent(trimmed)}`),
            },
          ];
    const handleNeedle = needle.replace(/^@/, "");
    const people = (found?.authors || []).map((item) => ({
      row: {
        id: `a-${item.username.toLowerCase()}`,
        group: "Authors",
        stale,
        run: go(authorPath(item.username)),
        lead: <AuthorAvatar name={item.name} username={item.username} />,
        body: (
          <span className="command-label">
            <span className="command-post-head">
              <strong>{item.name || `@${item.username}`}</strong>
              <span className="handle">@{item.username}</span>
            </span>
          </span>
        ),
        hint: `${item.count.toLocaleString()} saved`,
      } as Row,
      // Typing the start of someone's name or handle means "take me to them".
      leads: handleNeedle !== "" && (startsWord(item.name, handleNeedle) || item.username.toLowerCase().startsWith(handleNeedle)),
    }));
    const pages: Row[] = navigation
      .filter((item) => startsWord(item.label, needle))
      .map((item) => ({ id: `nav-${item.to}`, group: "Pages", label: item.label, icon: item.icon, tone: item.tone, run: go(item.to) }));
    const cats: Row[] = (categories.data?.items || [])
      .filter((item) => item.count > 0 && startsWord(item.name, needle))
      .slice(0, 4)
      .map((item) => ({
        id: `c-${item.id}`,
        group: "Categories",
        label: item.name,
        hint: item.count.toLocaleString(),
        icon: FolderTree,
        tone: colorFor(item.id),
        run: go(`/bookmarks?category=${item.id}`),
      }));
    const tagRows: Row[] = (tags.data?.items || [])
      .filter((item) => item.count > 0 && startsWord(item.name, needle))
      .slice(0, 4)
      .map((item) => ({
        id: `t-${item.id}`,
        group: "Tags",
        label: item.name,
        hint: item.count.toLocaleString(),
        icon: Tag,
        tone: colorFor(item.id),
        run: go(`/bookmarks?tag=${item.id}`),
      }));
    const listRows = allLists.filter((item) => startsWord(item.label || "", needle)).slice(0, 6);
    const actions = "new list".includes(needle) ? [newList] : [];

    // Typing the start of a page, list, or person's name means "take me
    // there", so those lead. Otherwise the posts do.
    const direct = [...pages, ...listRows].filter((row) => row.label?.toLowerCase().startsWith(needle));
    const leading = people.filter((item) => item.leads).map((item) => item.row);
    const others = people.filter((item) => !item.leads).map((item) => item.row);
    const rest = [...listRows, ...cats, ...tagRows, ...pages].filter((row) => !direct.includes(row));
    return [...direct, ...leading, ...posts, ...seeAll, ...others, ...rest, ...actions];
  }, [needle, trimmed, found, stale, recent, categories.data, tags.data, lists.data, navigate, onClose, onNewList, onOpenBookmark]);

  const safeIndex = Math.min(selectedIndex, Math.max(rows.length - 1, 0));

  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>(`[data-index="${safeIndex}"]`)?.scrollIntoView?.({ block: "nearest" });
  }, [safeIndex]);

  // The results for the typed words are in: carry out an Enter that was waiting.
  useEffect(() => {
    if (!open || stale || pendingEnter.current === null) return;
    const asPage = pendingEnter.current;
    pendingEnter.current = null;
    rows[safeIndex]?.run(asPage);
  }, [open, stale, rows, safeIndex]);

  if (!open) return null;

  function move(step: number) {
    setSelectedIndex((safeIndex + step + rows.length) % Math.max(rows.length, 1));
  }

  function onKeyDown(event: React.KeyboardEvent) {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      move(event.key === "ArrowDown" ? 1 : -1);
    } else if (event.key === "Tab") {
      // Focus stays in the search box; Tab walks the results like the arrows.
      event.preventDefault();
      move(event.shiftKey ? -1 : 1);
    } else if (event.key === "Enter") {
      event.preventDefault();
      // Results for these exact words aren't in yet: wait for them rather
      // than open something found for other words.
      if (stale) pendingEnter.current = event.ctrlKey || event.metaKey;
      else rows[safeIndex]?.run(event.ctrlKey || event.metaKey);
    }
  }

  const nothing = needle !== "" && found && !stale && found.total === 0;
  let lastGroup = "";

  return (
    <div className="command-backdrop" onClick={onClose}>
      <div className="command-modal" role="dialog" aria-modal="true" aria-label="Find anything" onClick={(event) => event.stopPropagation()} onKeyDown={onKeyDown}>
        <div className="command-search">
          {stale ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : <Search size={18} aria-hidden="true" />}
          <input
            ref={inputRef}
            autoFocus
            placeholder="Find a post, person, list, tag, or page…"
            aria-label="Find a post, person, list, tag, or page"
            role="combobox"
            aria-expanded="true"
            aria-controls={`${id}-results`}
            aria-activedescendant={rows.length ? `${id}-${safeIndex}` : undefined}
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setSelectedIndex(0);
              pendingEnter.current = null;
            }}
          />
          {query && (
            <button
              type="button"
              className="icon-button"
              tabIndex={-1}
              aria-label="Clear search"
              onClick={() => {
                setQuery("");
                setSelectedIndex(0);
                inputRef.current?.focus();
              }}
            >
              <X size={15} />
            </button>
          )}
        </div>

        <div className="command-list" ref={listRef} role="listbox" id={`${id}-results`} aria-label="Results">
          {!trimmed && recent.length === 0 && (
            <p className="command-status">
              <History size={14} aria-hidden="true" /> Posts you open show up here, ready to reopen.
            </p>
          )}
          {trimmed !== "" && !needle && <p className="command-status">Keep typing to search your posts.</p>}
          {nothing && <p className="command-status">No post matches “{trimmed}”. Try fewer or different words.</p>}
          {failed && <p className="command-status">Search isn't answering. Check that Sortwise is still running.</p>}
          {rows.map((row, index) => {
            const Icon = row.icon;
            const header = row.group !== lastGroup ? row.group : null;
            lastGroup = row.group;
            return (
              <Fragment key={row.id}>
                {header && <div className="command-group-title">{header}</div>}
                <button
                  type="button"
                  role="option"
                  tabIndex={-1}
                  id={`${id}-${index}`}
                  aria-selected={index === safeIndex}
                  data-index={index}
                  className={`command-item ${row.body ? "rich" : ""} ${row.stale ? "stale" : ""} ${index === safeIndex ? "selected" : ""}`}
                  onMouseMove={() => index !== safeIndex && setSelectedIndex(index)}
                  onClick={(event) => row.run(event.ctrlKey || event.metaKey)}
                >
                  {row.lead}
                  {Icon && <Icon size={16} className={row.tone ? `tone-${row.tone}` : ""} aria-hidden="true" />}
                  {row.body || (
                    <span className="command-label">
                      <span>{row.label}</span>
                    </span>
                  )}
                  {row.hint && <span className="command-hint">{row.hint}</span>}
                </button>
              </Fragment>
            );
          })}
        </div>

        <div className="command-footer">
          <span><ArrowUp size={12} /><ArrowDown size={12} /> Select</span>
          <span><CornerDownLeft size={12} /> Open</span>
          <span><kbd>{modKey}</kbd><CornerDownLeft size={12} /> Post as full page</span>
          <span><kbd>Esc</kbd> Close</span>
        </div>
      </div>
    </div>
  );
}
