import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ListX, Pencil, Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { listsQuery } from "../lib/lists";
import { EmptyState } from "../components/ui";
import { deleteList, renameList, type List } from "../lib/api";
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
  return <BookmarksPage list={list} actions={list.kind === "custom" && <ListActions key={list.id} list={list} />} />;
}

/* Rename and delete, for lists the user made. Favorites has neither. */
function ListActions({ list }: { list: List }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(list.name);
  const rename = useMutation({
    mutationFn: () => renameList(list.id, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["lists"] });
      setEditing(false);
    },
  });
  const remove = useMutation({
    mutationFn: () => deleteList(list.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["lists"] });
      queryClient.invalidateQueries({ queryKey: ["bookmarks"] });
      navigate("/bookmarks");
    },
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    if (name.trim() === list.name) setEditing(false);
    else if (name.trim()) rename.mutate();
  }

  if (editing) {
    return (
      <form className="list-rename" onSubmit={submit}>
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              setName(list.name);
              setEditing(false);
            }
          }}
          aria-label="List name"
          maxLength={80}
          autoFocus
        />
        <button type="submit" className="button primary small" disabled={rename.isPending || !name.trim()}>
          Save
        </button>
        <button
          type="button"
          className="button secondary small"
          onClick={() => {
            setName(list.name);
            setEditing(false);
          }}
        >
          Cancel
        </button>
        {rename.error && (
          <p className="form-error" role="alert">
            {rename.error.message}
          </p>
        )}
      </form>
    );
  }
  return (
    <>
      <button type="button" className="ghost-button" onClick={() => setEditing(true)}>
        <Pencil size={14} aria-hidden="true" />
        Rename
      </button>
      <button
        type="button"
        className="ghost-button danger"
        disabled={remove.isPending}
        onClick={() => {
          if (window.confirm(`Delete the list "${list.name}"? The bookmarks in it stay in your library.`)) remove.mutate();
        }}
      >
        <Trash2 size={14} aria-hidden="true" />
        Delete list
      </button>
      {remove.error && (
        <p className="form-error" role="alert">
          {remove.error.message}
        </p>
      )}
    </>
  );
}
