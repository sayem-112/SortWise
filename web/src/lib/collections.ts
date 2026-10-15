import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router-dom";
import { createCollection, deleteCollection, getCollections, updateCollection, type Collection, type CollectionQuery } from "./api";
import { confirmAction } from "./confirm";
import { showToast } from "./toast";

export const collectionsQuery = { queryKey: ["collections"], queryFn: getCollections };

/* Puts a collection just made into the loaded list right away, so its page
   can open without waiting for the list to be fetched again. */
export function addCollection(client: QueryClient, collection: Collection) {
  client.setQueryData<Awaited<ReturnType<typeof getCollections>>>(["collections"], (current) =>
    current ? { ...current, items: [...current.items.filter((item) => item.id !== collection.id), collection].sort((a, b) => a.name.localeCompare(b.name)) } : current,
  );
  client.invalidateQueries({ queryKey: ["collections"] });
}

/* A saved search as the library reads its starting state. The mode is
   always written out, so a collection saved as a word search stays one
   whatever the person last used elsewhere. */
export function collectionParams(query: CollectionQuery) {
  const params = new URLSearchParams();
  if (query.q) params.set("q", query.q);
  params.set("mode", query.mode === "meaning" ? "meaning" : "words");
  if (query.status) params.set("status", query.status);
  if (query.list) params.set("list", query.list);
  if (query.sort) params.set("sort", query.sort);
  query.categories.forEach((id) => params.append("category", String(id)));
  query.tags.forEach((id) => params.append("tag", String(id)));
  return params;
}

/* How a collection is ordered unless told otherwise: best matches first for
   a search, newest saved first without one. Saved as "". */
export function defaultSort(text: string) {
  return text.trim() ? "relevance" : "imported_desc";
}

/* Whether two saved searches find and order the same posts. The order
   categories and tags were picked in does not matter. */
export function sameQuery(a: CollectionQuery, b: CollectionQuery) {
  const ids = (values: number[]) => [...values].sort((x, y) => x - y).join(",");
  const words = (value: string) => value.trim().replace(/\s+/g, " ");
  const order = (query: CollectionQuery) => (query.sort === defaultSort(query.q) ? "" : query.sort);
  return (
    words(a.q) === words(b.q) &&
    (a.q ? a.mode === b.mode : true) &&
    ids(a.categories) === ids(b.categories) &&
    ids(a.tags) === ids(b.tags) &&
    a.status === b.status &&
    a.list === b.list &&
    a.author.toLowerCase() === b.author.toLowerCase() &&
    order(a) === order(b)
  );
}

/* Whether a search has anything in it to save. */
export function hasCriteria(query: CollectionQuery) {
  return Boolean(query.q.trim() || query.categories.length || query.tags.length || query.status || query.list || query.author);
}

/* Deleting a smart collection asks first; Undo saves it again with the same
   name, color, and search. Leaving a deleted collection's page goes back to
   the library. */
export function useCollectionActions() {
  const client = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const refresh = () => client.invalidateQueries({ queryKey: ["collections"] });

  const remove = useMutation({
    mutationFn: (collection: Collection) => deleteCollection(collection.id),
    onSuccess: (_, collection) => {
      refresh();
      // Undo returns to the collection's page only if that is where it was deleted from.
      const wasOpen = location.pathname === `/collections/${collection.id}`;
      if (wasOpen) navigate("/bookmarks");
      showToast({
        message: `Deleted ${collection.name}`,
        action: {
          label: "Undo",
          run: () => {
            void (async () => {
              const restored = await createCollection(collection.name, collection.query);
              // The color is a nicety: the collection is back either way.
              const recolored = await updateCollection(restored.id, { color: collection.color }).catch(() => restored);
              addCollection(client, recolored);
              if (wasOpen) navigate(`/collections/${restored.id}`);
            })().catch((error: Error) => showToast({ message: `Could not restore ${collection.name}: ${error.message}`, tone: "error" }));
          },
        },
      });
    },
    onError: (error) => showToast({ message: error.message, tone: "error" }),
  });

  return {
    confirmDelete: (collection: Collection) => {
      void confirmAction({
        title: `Delete ${collection.name}?`,
        message: "Only the saved search is deleted. Its posts stay in your library.",
        confirmLabel: "Delete collection",
        danger: true,
      }).then((ok) => ok && remove.mutate(collection));
    },
  };
}
