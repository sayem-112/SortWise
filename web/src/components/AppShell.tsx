import { useQuery } from "@tanstack/react-query";
import { ChevronRight, ChevronsLeft, ChevronsRight, Menu, Plus, Search } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Link, NavLink, Outlet, matchPath, useLocation, useNavigate } from "react-router-dom";
import { getAISettings, getBookmark, getSetup } from "../lib/api";
import { CommandPalette } from "./CommandPalette";
import { NewListForm } from "./Lists";
import { Toaster } from "./Toaster";
import { ConfirmDialog } from "./ConfirmDialog";
import { listIcon, listsQuery, listTone } from "../lib/lists";
import { navigation, type NavItem } from "../lib/navigation";
import { modKey } from "../lib/format";

const sections: { label: string; items: NavItem[] }[] = [
  { label: "Library", items: navigation.slice(0, 2) },
  { label: "Organize", items: navigation.slice(2, 4) },
  { label: "System", items: navigation.slice(4) },
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
  const lists = useQuery(listsQuery);
  const navigate = useNavigate();
  const location = useLocation();
  const setup = useQuery({ queryKey: ["setup"], queryFn: getSetup, refetchInterval: 15_000 });

  const detail = matchPath("/bookmarks/:id", location.pathname);
  const detailId = detail?.params.id;
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

  useEffect(() => setMobileOpen(false), [location.pathname]);

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
        const num = parseInt(e.key, 10);
        if (num >= 1 && num <= navigation.length) {
          e.preventDefault();
          navigate(navigation[num - 1].to);
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
              {section.items.map(({ to, label, icon: Icon }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={to === "/"}
                  className={({ isActive }) => (isActive ? "sidebar-item active" : "sidebar-item")}
                  title={`${label} (${modKey}${navigation.findIndex((item) => item.to === to) + 1})`}
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
                <NavLink
                  key={item.id}
                  to={`/lists/${item.id}`}
                  className={({ isActive }) => (isActive ? "sidebar-item active" : "sidebar-item")}
                >
                  <Icon size={16} className={`tone-${listTone(item)}`} />
                  <span className="sidebar-item-label">{item.name}</span>
                  {item.count > 0 && <span className="sidebar-count">{item.count.toLocaleString()}</span>}
                </NavLink>
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
        <Outlet />
      </div>

      <CommandPalette open={commandOpen} onClose={closeCommand} onNewList={() => {
          setCreatingList(true);
          setMobileOpen(true);
          if (collapsed) toggleSidebar();
        }} />
      <Toaster />
      <ConfirmDialog />
    </div>
  );
}
