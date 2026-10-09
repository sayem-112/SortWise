import { useMutation, useQueryClient, type InfiniteData, type QueryClient } from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router-dom";
import {
  BookMarked,
  BookOpen,
  Briefcase,
  ChefHat,
  Code,
  Dumbbell,
  Film,
  FlaskConical,
  GraduationCap,
  Heart,
  Lightbulb,
  List as ListIcon,
  Music,
  Newspaper,
  Palette,
  Plane,
  Rocket,
  Sparkles,
  Star,
  Target,
  Wallet,
  type LucideIcon,
} from "lucide-react";
import { createList, deleteList, getLists, setManyInList, updateList, type Bookmark, type BookmarkPage, type List } from "./api";
import { confirmAction } from "./confirm";
import type { OptionColor } from "./format";
import { showToast } from "./toast";

export const listsQuery = { queryKey: ["lists"], queryFn: getLists };

// The icons a list can have, by the name the app stores.
export const LIST_ICONS: Record<string, LucideIcon> = {
  list: ListIcon,
  bookmark: BookMarked,
  "book-open": BookOpen,
  lightbulb: Lightbulb,
  code: Code,
  palette: Palette,
  "chef-hat": ChefHat,
  dumbbell: Dumbbell,
  wallet: Wallet,
  briefcase: Briefcase,
  "graduation-cap": GraduationCap,
  flask: FlaskConical,
  music: Music,
  film: Film,
  plane: Plane,
  heart: Heart,
  rocket: Rocket,
  target: Target,
  newspaper: Newspaper,
  sparkles: Sparkles,
  star: Star,
};

export function listIcon(list: Pick<List, "kind" | "icon">) {
  if (list.kind === "favorites") return Star;
  return LIST_ICONS[list.icon] || ListIcon;
}

export function listTone(list: Pick<List, "kind" | "color">): OptionColor {
  return list.kind === "favorites" ? "yellow" : list.color || "purple";
}

/* Writes new list membership into every loaded copy of the bookmarks, so
   stars and list chips change the moment they are clicked. */
function patchLoaded(client: QueryClient, ids: number[], listId: number, inList: boolean) {
  const patch = (bookmark: Bookmark) => {
    if (!ids.includes(bookmark.id)) return bookmark;
    const others = bookmark.lists.filter((id) => id !== listId);
    return { ...bookmark, lists: inList ? [...others, listId] : others };
  };
  const patchPage = (page: BookmarkPage) => ({ ...page, items: page.items.map(patch) });
  client.setQueriesData<BookmarkPage | InfiniteData<BookmarkPage>>({ queryKey: ["bookmarks"] }, (value) => {
    if (!value) return value;
    if ("pages" in value) return { ...value, pages: value.pages.map(patchPage) };
    return "items" in value ? patchPage(value) : value;
  });
  for (const id of ids) client.setQueryData<Bookmark>(["bookmark", String(id)], (value) => (value ? patch(value) : value));
}

// `bulk` marks an action on a selection, which always says what it did.
type Change = { list: Pick<List, "id" | "name">; ids: number[]; inList: boolean; quiet?: boolean; bulk?: boolean };

/* Adds bookmarks to a list or takes them out. Taking anything out offers an
   undo; adding several at once says how many went in. */
export function useListChange() {
  const client = useQueryClient();
  const change = useMutation({
    mutationFn: ({ list, ids, inList }: Change) => setManyInList(list.id, ids, inList),
    onMutate: ({ list, ids, inList }) => patchLoaded(client, ids, list.id, inList),
    onSuccess: (_, input) => {
      const { list, ids, inList, quiet, bulk } = input;
      const count = ids.length === 1 ? "1 bookmark" : `${ids.length} bookmarks`;
      if (!inList) {
        showToast({
          message: `Removed ${ids.length === 1 && !bulk ? "" : `${count} `}from ${list.name}`,
          action: { label: "Undo", run: () => change.mutate({ ...input, inList: true, quiet: true }) },
        });
      } else if (!quiet && (ids.length > 1 || bulk)) {
        showToast({ message: `Added ${count} to ${list.name}` });
      }
    },
    onError: (error) => showToast({ message: error.message, tone: "error" }),
    onSettled: () => {
      client.invalidateQueries({ queryKey: ["lists"] });
      client.invalidateQueries({ queryKey: ["bookmarks"] });
      client.invalidateQueries({ queryKey: ["bookmark"] });
    },
  });
  return { pending: change.isPending, apply: (input: Change) => change.mutate(input) };
}

/* Pin and delete for a list, from the sidebar or the list's own page.
   Deleting asks first, and Undo rebuilds the list with its name, look, pin,
   and bookmarks. Leaving a deleted list's page goes back to the library. */
export function useListActions() {
  const client = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const refresh = () => {
    client.invalidateQueries({ queryKey: ["lists"] });
    client.invalidateQueries({ queryKey: ["bookmarks"] });
    client.invalidateQueries({ queryKey: ["bookmark"] });
  };

  const pin = useMutation({
    mutationFn: (list: List) => updateList(list.id, { pinned: !list.pinned }),
    onSuccess: (list) => {
      refresh();
      showToast({ message: list.pinned ? `Pinned ${list.name} to the top` : `Unpinned ${list.name}` });
    },
    onError: (error) => showToast({ message: error.message, tone: "error" }),
  });

  const remove = useMutation({
    mutationFn: (list: List) => deleteList(list.id),
    onSuccess: ({ bookmarkIds }, list) => {
      refresh();
      if (location.pathname === `/lists/${list.id}`) navigate("/bookmarks");
      showToast({
        message: `Deleted ${list.name}`,
        action: {
          label: "Undo",
          run: () => {
            void (async () => {
              const restored = await createList(list.name);
              await updateList(restored.id, { icon: list.icon, color: list.color, pinned: list.pinned || undefined });
              if (bookmarkIds.length) await setManyInList(restored.id, bookmarkIds, true);
              refresh();
              navigate(`/lists/${restored.id}`);
            })().catch((error: Error) => showToast({ message: `Could not restore ${list.name}: ${error.message}`, tone: "error" }));
          },
        },
      });
    },
    onError: (error) => showToast({ message: error.message, tone: "error" }),
  });

  return {
    togglePin: (list: List) => pin.mutate(list),
    confirmDelete: (list: List) => {
      const count = list.count === 1 ? "1 bookmark" : `${list.count.toLocaleString()} bookmarks`;
      void confirmAction({
        title: `Delete ${list.name}?`,
        message: list.count ? `Its ${count} stay in your library.` : "It has no bookmarks in it.",
        confirmLabel: "Delete list",
        danger: true,
      }).then((ok) => ok && remove.mutate(list));
    },
  };
}
