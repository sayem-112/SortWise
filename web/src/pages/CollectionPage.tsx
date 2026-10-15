import { useQuery, useQueryClient } from "@tanstack/react-query";
import { SearchX } from "lucide-react";
import { Link, useParams } from "react-router-dom";
import { CollectionMenu } from "../components/Collections";
import { EditableTitle } from "../components/EditableTitle";
import { EmptyState } from "../components/ui";
import { updateCollection } from "../lib/api";
import { collectionsQuery } from "../lib/collections";
import { BookmarksPage } from "./BookmarksPage";

/* One smart collection: the library, opened on its saved search. */
export function CollectionPage() {
  const { id } = useParams();
  const client = useQueryClient();
  const collections = useQuery(collectionsQuery);
  const collection = collections.data?.items.find((item) => String(item.id) === id);

  // Still loading, or just made and not in the fetched list yet.
  if (collections.isLoading || (!collection && collections.isFetching)) {
    return (
      <main className="page full" aria-busy="true">
        <div className="skeleton skeleton-title" />
      </main>
    );
  }
  if (!collection) {
    return (
      <main className="page full">
        <EmptyState
          icon={SearchX}
          title="This smart collection doesn't exist"
          action={
            <Link to="/bookmarks" className="button secondary">
              Go to the library
            </Link>
          }
        >
          It may have been deleted. Its posts are still in your library.
        </EmptyState>
      </main>
    );
  }
  return (
    <BookmarksPage
      collection={collection}
      title={
        <EditableTitle
          key={collection.id}
          value={collection.name}
          label="Collection name"
          onSave={(name) => updateCollection(collection.id, { name }).then(() => client.invalidateQueries({ queryKey: ["collections"] }))}
        />
      }
      actions={<CollectionMenu collection={collection} align="start" />}
    />
  );
}
