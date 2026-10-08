import { useQuery } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowUp,
  CornerDownLeft,
  Download,
  FileText,
  FolderTree,
  Search,
  Tag,
  X,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { getBookmarks, getCategories, getTags } from "../lib/api";
import { navigation } from "../lib/navigation";
import { colorFor, modKey, postPreview } from "../lib/format";
import { listIcon, listsQuery } from "../lib/lists";

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
}

type Command = {
  id: string;
  group: string;
  label: string;
  detail?: string;
  hint?: string;
  icon: LucideIcon;
  tone?: string;
  run: () => void;
};

export function CommandPalette({ open, onClose }: CommandPaletteProps) {
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const trimmed = query.trim();

  const searchResults = useQuery({
    queryKey: ["command-search", trimmed],
    queryFn: () => getBookmarks(`q=${encodeURIComponent(trimmed)}&pageSize=6`),
    enabled: open && trimmed.length > 1,
  });
  const categories = useQuery({ queryKey: ["categories"], queryFn: getCategories, enabled: open });
  const tags = useQuery({ queryKey: ["tags"], queryFn: getTags, enabled: open });
  const lists = useQuery({ ...listsQuery, enabled: open });

  useEffect(() => {
    if (!open) {
      setQuery("");
      setSelectedIndex(0);
    }
  }, [open]);

  const commands = useMemo<Command[]>(() => {
    const go = (path: string) => () => {
      navigate(path);
      onClose();
    };
    const needle = trimmed.toLowerCase();
    if (!needle) {
      return [
        ...navigation.map((item, index) => ({
          id: `nav-${item.to}`,
          group: "Jump to",
          label: item.label,
          hint: `${modKey}${index + 1}`,
          icon: item.icon,
          tone: item.tone,
          run: go(item.to),
        })),
        {
          id: "export-json",
          group: "Actions",
          label: "Export library as JSON",
          icon: Download,
          run: () => {
            window.location.href = "/api/v1/exports/bookmarks.json";
            onClose();
          },
        },
        {
          id: "export-csv",
          group: "Actions",
          label: "Export library as CSV",
          icon: Download,
          run: () => {
            window.location.href = "/api/v1/exports/bookmarks.csv";
            onClose();
          },
        },
      ];
    }
    const pages = navigation
      .filter((item) => item.label.toLowerCase().includes(needle))
      .map((item) => ({ id: `nav-${item.to}`, group: "Pages", label: item.label, icon: item.icon, tone: item.tone, run: go(item.to) }));
    const bookmarks = (searchResults.data?.items || []).map((item) => ({
      id: `b-${item.id}`,
      group: "Bookmarks",
      label: item.author || `@${item.username}`,
      detail: postPreview(item),
      icon: FileText,
      run: go(`/bookmarks/${item.id}`),
    }));
    const cats = (categories.data?.items || [])
      .filter((item) => item.name.toLowerCase().includes(needle))
      .slice(0, 4)
      .map((item) => ({
        id: `c-${item.id}`,
        group: "Categories",
        label: item.name,
        hint: `${item.count}`,
        icon: FolderTree,
        tone: colorFor(item.id),
        run: go(`/bookmarks?category=${item.id}`),
      }));
    const tagItems = (tags.data?.items || [])
      .filter((item) => item.name.toLowerCase().includes(needle))
      .slice(0, 4)
      .map((item) => ({
        id: `t-${item.id}`,
        group: "Tags",
        label: item.name,
        hint: `${item.count}`,
        icon: Tag,
        tone: colorFor(item.id),
        run: go(`/bookmarks?tag=${item.id}`),
      }));
    const listItems = (lists.data?.items || [])
      .filter((item) => item.name.toLowerCase().includes(needle))
      .slice(0, 4)
      .map((item) => ({
        id: `l-${item.id}`,
        group: "Lists",
        label: item.name,
        hint: `${item.count}`,
        icon: listIcon(item),
        tone: item.kind === "favorites" ? "yellow" : "purple",
        run: go(`/lists/${item.id}`),
      }));
    const searchAll = {
      id: "search-all",
      group: "Library",
      label: `Search library for “${trimmed}”`,
      icon: Search,
      run: go(`/bookmarks?q=${encodeURIComponent(trimmed)}`),
    };
    return [...bookmarks, searchAll, ...listItems, ...cats, ...tagItems, ...pages];
  }, [trimmed, searchResults.data, categories.data, tags.data, lists.data, navigate, onClose]);

  const safeIndex = Math.min(selectedIndex, Math.max(commands.length - 1, 0));

  useEffect(() => {
    listRef.current
      ?.querySelector<HTMLElement>(`[data-index="${safeIndex}"]`)
      ?.scrollIntoView?.({ block: "nearest" });
  }, [safeIndex]);

  if (!open) return null;

  function onKeyDown(event: React.KeyboardEvent) {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
    } else if (event.key === "ArrowDown") {
      event.preventDefault();
      setSelectedIndex((safeIndex + 1) % Math.max(commands.length, 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setSelectedIndex((safeIndex - 1 + commands.length) % Math.max(commands.length, 1));
    } else if (event.key === "Enter") {
      event.preventDefault();
      commands[safeIndex]?.run();
    }
  }

  let lastGroup = "";

  return (
    <div className="command-backdrop" onClick={onClose}>
      <div
        className="command-modal"
        role="dialog"
        aria-label="Search"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={onKeyDown}
      >
        <div className="command-search">
          <Search size={18} aria-hidden="true" />
          <input
            autoFocus
            placeholder="Search bookmarks, categories, tags, or pages…"
            aria-label="Search bookmarks, categories, tags, or pages"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelectedIndex(0);
            }}
          />
          {query && (
            <button type="button" className="icon-button" onClick={() => setQuery("")} aria-label="Clear search">
              <X size={15} />
            </button>
          )}
        </div>

        <div className="command-list" ref={listRef} role="listbox" aria-label="Results">
          {commands.map((command, index) => {
            const Icon = command.icon;
            const header = command.group !== lastGroup ? command.group : null;
            lastGroup = command.group;
            return (
              <div key={command.id}>
                {header && <div className="command-group-title">{header}</div>}
                <button
                  type="button"
                  role="option"
                  aria-selected={index === safeIndex}
                  data-index={index}
                  className={`command-item ${index === safeIndex ? "selected" : ""}`}
                  onMouseMove={() => index !== safeIndex && setSelectedIndex(index)}
                  onClick={command.run}
                >
                  <Icon size={16} className={command.tone ? `tone-${command.tone}` : ""} aria-hidden="true" />
                  <span className="command-label">
                    <span>{command.label}</span>
                    {command.detail && <span className="command-detail">{command.detail}</span>}
                  </span>
                  {command.hint && <span className="command-hint">{command.hint}</span>}
                </button>
              </div>
            );
          })}
          {trimmed.length > 1 && searchResults.isFetching && (
            <div className="command-status">Searching your library…</div>
          )}
        </div>

        <div className="command-footer">
          <span><ArrowUp size={12} /><ArrowDown size={12} /> Select</span>
          <span><CornerDownLeft size={12} /> Open</span>
          <span><kbd>Esc</kbd> Close</span>
        </div>
      </div>
    </div>
  );
}
