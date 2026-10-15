import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ListX } from "lucide-react";
import { useParams } from "react-router-dom";
import { EditableTitle } from "../components/EditableTitle";
import { EmptyState } from "../components/ui";
import { ListLook, ListMenu } from "../components/Lists";
import { updateList } from "../lib/api";
import { listsQuery } from "../lib/lists";
import { BookmarksPage } from "./BookmarksPage";

/* One list: the library, showing only the bookmarks in it. */
export function ListPage() {
  const { id } = useParams();
  const client = useQueryClient();
  const lists = useQuery(listsQuery);
  const list = lists.data?.items.find((item) => String(item.id) === id);

  if (lists.isLoading) {
    return (
      <main className="page full" aria-busy="true">
        <div className="skeleton skeleton-title" />
      </main>
    );
  }
  if (!list) {
    return (
      <main className="page full">
        <EmptyState icon={ListX} title="This list doesn't exist">
          It may have been deleted.
        </EmptyState>
      </main>
    );
  }
  if (list.kind === "favorites") return <BookmarksPage list={list} />;
  return (
    <BookmarksPage
      list={list}
      icon={<ListLook list={list} />}
      title={
        <EditableTitle
          key={list.id}
          value={list.name}
          label="List name"
          onSave={(name) => updateList(list.id, { name }).then(() => client.invalidateQueries({ queryKey: ["lists"] }))}
        />
      }
      actions={<ListMenu list={list} />}
    />
  );
}
