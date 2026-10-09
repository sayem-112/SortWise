import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ListX } from "lucide-react";
import { useState } from "react";
import { useParams } from "react-router-dom";
import { EmptyState } from "../components/ui";
import { ListLook, ListMenu } from "../components/Lists";
import { updateList, type List } from "../lib/api";
import { listsQuery } from "../lib/lists";
import { BookmarksPage } from "./BookmarksPage";

/* One list: the library, showing only the bookmarks in it. */
export function ListPage() {
  const { id } = useParams();
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
      title={<ListTitle key={list.id} list={list} />}
      actions={<ListMenu list={list} />}
    />
  );
}

/* The page title is the list's name and is edited in place, like a Notion
   page title: click, type, Enter or click away to save, Escape to cancel. */
function ListTitle({ list }: { list: List }) {
  const client = useQueryClient();
  const [name, setName] = useState(list.name);
  const rename = useMutation({
    mutationFn: (value: string) => updateList(list.id, { name: value }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["lists"] }),
    onError: () => setName(list.name),
  });

  function save() {
    const value = name.trim();
    if (!value) setName(list.name);
    else if (value !== list.name) rename.mutate(value);
  }

  return (
    <>
      <input
        className="page-title-input"
        value={name}
        onChange={(event) => {
          setName(event.target.value);
          if (rename.error) rename.reset();
        }}
        onBlur={save}
        onKeyDown={(event) => {
          if (event.key === "Enter") event.currentTarget.blur();
          if (event.key === "Escape") {
            setName(list.name);
            requestAnimationFrame(() => (event.target as HTMLInputElement).blur());
          }
        }}
        aria-label="List name"
        title="Click to rename"
        maxLength={80}
        size={Math.max(name.length, 4)}
      />
      {rename.error && (
        <span className="form-error title-error" role="alert">
          {rename.error.message}
        </span>
      )}
    </>
  );
}
