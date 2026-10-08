import { useMutation, useQueryClient } from "@tanstack/react-query";
import { List as ListIcon, Star } from "lucide-react";
import { getLists, setInList, type Bookmark, type List } from "./api";

export const listsQuery = { queryKey: ["lists"], queryFn: getLists };

export function listIcon(list: Pick<List, "kind">) {
  return list.kind === "favorites" ? Star : ListIcon;
}

/* Adds a bookmark to a list or takes it out. The open bookmark and every
   loaded copy of it update at once; lists and their pages refresh after. */
export function useListMembership(bookmark: Bookmark) {
  const queryClient = useQueryClient();
  const change = useMutation({
    mutationFn: ({ listId, inList }: { listId: number; inList: boolean }) => setInList(listId, bookmark.id, inList),
    onMutate: ({ listId, inList }) => {
      const lists = inList ? [...bookmark.lists, listId] : bookmark.lists.filter((id) => id !== listId);
      queryClient.setQueryData<Bookmark>(["bookmark", String(bookmark.id)], (value) => (value ? { ...value, lists } : value));
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["lists"] });
      queryClient.invalidateQueries({ queryKey: ["bookmarks"] });
      queryClient.invalidateQueries({ queryKey: ["bookmark", String(bookmark.id)] });
    },
  });
  return {
    pending: change.isPending,
    error: change.error?.message,
    set: (listId: number, inList: boolean) => change.mutate({ listId, inList }),
    toggle: (listId: number) => change.mutate({ listId, inList: !bookmark.lists.includes(listId) }),
  };
}
