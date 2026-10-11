import { useQuery } from "@tanstack/react-query";
import { ChevronRight, ChevronsLeft, ChevronsRight, Menu, Pin, Plus, Search } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, NavLink, Outlet, matchPath, useLocation, useNavigate } from "react-router-dom";
import { getAISettings, getAuthor, getBookmark, getSetup } from "../lib/api";
import { PeekContext, type Peek } from "../lib/peek";
import { BookmarkDrawer } from "./BookmarkDrawer";
import { CommandPalette } from "./CommandPalette";
import { ListMenu, NewListForm } from "./Lists";
import { Toaster } from "./Toaster";
import { ConfirmDialog } from "./ConfirmDialog";
import { listIcon, listsQuery, listTone } from "../lib/lists";
import { navigation, type NavItem } from "../lib/navigation";
import { modKey } from "../lib/format";

const sections: { label: string; items: NavItem[] }[] = [
  { label: "Library", items: navigation.slice(0, 3) },
  { label: "Organize", items: navigation.slice(3, 5) },
  { label: "System", items: navigation.slice(5) },
];

function readCollapsed() {
  try {
    return localStorage.getItem("bw.sidebar") === "closed";
  } catch {
    return false;
  }
}

export function AppShell() {
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [commandOpen, setCommandOpen] = useState(false);
  const [creatingList, setCreatingList] = useState(false);
  // The one post preview, for every page and the finder. `ids` are the posts
  // beside it, for moving between them; `opened` is the one asked for, as
  // opposed to those merely passed through on the way.
  const [peek, setPeek] = useState<{ id: number; ids: number[]; opened: number } | null>(null);
  const peekId = peek?.id ?? null;
  const peekControl = useMemo<Peek>(
    () => ({
      current: peekId,
      open: (id, ids) => setPeek({ id, ids, opened: id }),
      close: () => setPeek(null),
    }),
    [peekId],
  );
  const lists = useQuery(listsQuery);
  const navigate = useNavigate();
  const location = useLocation();
  const setup = useQuery({ queryKey: ["setup"], queryFn: getSetup, refetchInterval: 15_000 });

  const detail = matchPath("/bookmarks/:id", location.pathname);
  const detailId = detail?.params.id;
  const authorHandle = (matchPath("/authors/:username", location.pathname)?.params.username || "").replace(/^@/, "");
  const author = useQuery({
    queryKey: ["author", authorHandle.toLowerCase()],
    queryFn: () => getAuthor(authorHandle),
    enabled: Boolean(authorHandle),
    retry: false,
  });
  const bookmark = useQuery({
    queryKey: ["bookmark", String(detailId)],
    queryFn: () => getBookmark(String(detailId)),
    enabled: Boolean(detailId),
  });

  const closeCommand = useCallback(() => setCommandOpen(false), []);

  const toggleSidebar = useCallback(() => {
    setCollapsed((value) => {
      try {
        localStorage.setItem("bw.sidebar", value ? "open" : "closed");
      } catch {
        /* storage unavailable: keep the in-memory choice */
      }
      return !value;
    });
  }, []);

  // Going anywhere, even to the same page with a different search or filter,
  // closes the sidebar drawer and the preview.
  useEffect(() => {
    setMobileOpen(false);
    setPeek(null);
  }, [location.key]);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCommandOpen((open) => !open);
      }
      if ((e.metaKey || e.ctrlKey) && e.key === "\\") {
        e.preventDefault();
        toggleSidebar();
      }
      if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey) {
        const page = navigation.find((item) => String(item.shortcut) === e.key);
        if (page) {
          e.preventDefault();
          navigate(page.to);
        }
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [navigate, toggleSidebar]);

  const listMatch = matchPath("/lists/:id", location.pathname);
  const openList = lists.data?.items.find((item) => String(item.id) === listMatch?.params.id);
  const current = openList
    ? { to: location.pathname, label: openList.name, icon: listIcon(openList), tone: listTone(openList) }
    : navigation.find((item) => item.to !== "/" && location.pathname.startsWith(item.to)) || navigation[0];
  const CurrentIcon = current.icon;
  const ai = useQuery({ queryKey: ["ai-settings"], queryFn: getAISettings, refetchInterval: 60_000 });
  const aiState = (() => {
    const data = ai.data;
    if (!data) return { text: "Checking AI…", tone: "" };
    if (!data.active) return { text: "AI needs a key", tone: "warn" };
    if (data.paused) return { text: "AI paused", tone: "idle" };
    if (data.resting?.some((item) => item.provider === data.active)) return { text: "AI waiting for limits", tone: "warn" };
    return { text: `Organizing with ${data.providers?.find((item) => item.id === data.active)?.name}`, tone: "ok" };
  })();

  return (
    <div className={`app-shell ${collapsed ? "sidebar-collapsed" : ""} ${mobileOpen ? "mobile-open" : ""}`}>
      <div className="sidebar-scrim" onClick={() => setMobileOpen(false)} aria-hidden="true" />
      <aside className="sidebar" aria-label="Sidebar">
        <div className="workspace-row">
          <NavLink to="/" className="workspace" aria-label="Sortwise home">
            <img className="workspace-logo" src="/logo.svg" alt="" width="22" height="22" />
            <span className="workspace-name">Sortwise</span>
          </NavLink>
          <button
            className="icon-button sidebar-close"
            onClick={() => (mobileOpen ? setMobileOpen(false) : toggleSidebar())}
            title={`Close sidebar (${modKey}\\)`}
            aria-label="Close sidebar"
          >
            <ChevronsLeft size={18} />
          </button>
        </div>

        <button className="sidebar-item search-item" onClick={() => setCommandOpen(true)}>
          <Search size={16} />
          <span>Search</span>
          <kbd>{modKey}K</kbd>
        </button>

        <nav aria-label="Primary navigation" className="sidebar-nav">
          {sections.map((section) => (
            <div className="sidebar-section" key={section.label}>
              <div className="sidebar-section-label">{section.label}</div>
              {section.items.map(({ to, label, icon: Icon, shortcut }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={to === "/"}
                  className={({ isActive }) => (isActive ? "sidebar-item active" : "sidebar-item")}
                  title={`${label} (${modKey}${shortcut})`}
                >
                  <Icon size={16} />
                  <span>{label}</span>
                </NavLink>
              ))}
            </div>
          ))}

          <div className="sidebar-section">
            <div className="sidebar-section-label">
              <span>Lists</span>
              <button
                type="button"
                className="icon-button sidebar-add"
                onClick={() => setCreatingList(true)}
                aria-label="New list"
                title="New list"
              >
                <Plus size={14} />
              </button>
            </div>
            {(lists.data?.items || []).map((item) => {
              const Icon = listIcon(item);
              return (
                <div key={item.id} className={`sidebar-list-row ${item.kind === "custom" ? "has-menu" : ""}`}>
                  <NavLink
                    to={`/lists/${item.id}`}
                    className={({ isActive }) => (isActive ? "sidebar-item active" : "sidebar-item")}
                  >
                    <Icon size={16} className={`tone-${listTone(item)}`} />
                    <span className="sidebar-item-label">{item.name}</span>
                    {item.pinned && <Pin size={12} className="sidebar-pin" aria-label="Pinned" />}
                    {item.count > 0 && <span className="sidebar-count">{item.count.toLocaleString()}</span>}
                  </NavLink>
                  {/* Pin and delete for the user's own lists; Favorites always stays first. */}
                  {item.kind === "custom" && <ListMenu list={item} className="sidebar-list-menu" />}
                </div>
              );
            })}
            {creatingList && (
              <div className="sidebar-new-list">
                <NewListForm
                  onCreated={(list) => {
                    setCreatingList(false);
                    navigate(`/lists/${list.id}`);
                  }}
                  onCancel={() => setCreatingList(false)}
                />
              </div>
            )}
          </div>
        </nav>

        <div className="sidebar-footer">
          <Link to="/settings" className="sidebar-status">
            <span className={`status-dot ${aiState.tone}`} aria-hidden="true" />
            <span>{aiState.text}</span>
          </Link>
          <Link to="/activity" className="sidebar-status">
            <span className="status-dot local" aria-hidden="true" />
            <span>{setup.data ? `${setup.data.bookmarks.toLocaleString()} bookmarks on this PC` : "Library"}</span>
          </Link>
        </div>
      </aside>

      <div className="main-column">
        <div className="topbar">
          <button
            className="icon-button topbar-menu"
            onClick={() => setMobileOpen(true)}
            aria-label="Open sidebar"
          >
            <Menu size={18} />
          </button>
          <button
            className="icon-button topbar-expand"
            onClick={toggleSidebar}
            aria-label="Open sidebar"
            title={`Open sidebar (${modKey}\\)`}
          >
            <ChevronsRight size={18} />
          </button>
          <nav className="breadcrumbs" aria-label="Breadcrumb">
            <Link to={current.to} className="crumb">
              <CurrentIcon size={15} className={`tone-${current.tone}`} />
              <span>{current.label}</span>
            </Link>
            {authorHandle && (
              <>
                <ChevronRight size={13} className="crumb-sep" aria-hidden="true" />
                <span className="crumb current" aria-current="page">
                  {/* The handle as its owner writes it, once known, not as it was typed in the address bar. */}
                  @{author.data?.username || authorHandle}
                </span>
              </>
            )}
            {detailId && (
              <>
                <ChevronRight size={13} className="crumb-sep" aria-hidden="true" />
                <span className="crumb current" aria-current="page">
                  {bookmark.data ? bookmark.data.author || `@${bookmark.data.username}` : "Bookmark"}
                </span>
              </>
            )}
          </nav>
          <button className="topbar-search" onClick={() => setCommandOpen(true)}>
            <Search size={15} />
            <span>Search</span>
            <kbd>{modKey}K</kbd>
          </button>
        </div>
        <PeekContext.Provider value={peekControl}>
          <Outlet />
        </PeekContext.Provider>
      </div>

      <CommandPalette
        open={commandOpen}
        onClose={closeCommand}
        onOpenBookmark={peekControl.open}
        onNewList={() => {
          setCreatingList(true);
          setMobileOpen(true);
          if (collapsed) toggleSidebar();
        }}
      />
      <BookmarkDrawer
        bookmarkId={peekId}
        ids={peek?.ids}
        openedId={peek?.opened}
        onNavigate={(id) => setPeek((current) => current && { ...current, id })}
        onClose={peekControl.close}
      />
      <Toaster />
      <ConfirmDialog />
    </div>
  );
}
