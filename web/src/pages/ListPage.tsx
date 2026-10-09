import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ListX, MoreHorizontal, Trash2 } from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { EmptyState, Popover } from "../components/ui";
import { ListLook } from "../components/Lists";
import { createList, deleteList, setManyInList, updateList, type List } from "../lib/api";
import { showToast } from "../lib/toast";
import { listsQuery } from "../lib/lists";
import { BookmarksPage } from "./BookmarksPage";
import { confirmAction } from "../lib/confirm";

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
      actions={<DeleteList list={list} />}
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

/* Deleting sits behind the ⋯ menu, so a new list doesn't greet you with it.
   Undo rebuilds the list with its name, look, and bookmarks. */
function DeleteList({ list }: { list: List }) {
  const client = useQueryClient();
  const navigate = useNavigate();
  const remove = useMutation({
    mutationFn: () => deleteList(list.id),
    onSuccess: ({ bookmarkIds }) => {
      client.invalidateQueries({ queryKey: ["lists"] });
      client.invalidateQueries({ queryKey: ["bookmarks"] });
      navigate("/bookmarks");
      showToast({
        message: `Deleted ${list.name}`,
        action: {
          label: "Undo",
          run: () => {
            void (async () => {
              const restored = await createList(list.name);
              await updateList(restored.id, { icon: list.icon, color: list.color });
              if (bookmarkIds.length) await setManyInList(restored.id, bookmarkIds, true);
              await client.invalidateQueries({ queryKey: ["lists"] });
              client.invalidateQueries({ queryKey: ["bookmarks"] });
              navigate(`/lists/${restored.id}`);
            })().catch((error: Error) => showToast({ message: `Could not restore ${list.name}: ${error.message}`, tone: "error" }));
          },
        },
      });
    },
  });
  const count = list.count === 1 ? "1 bookmark" : `${list.count.toLocaleString()} bookmarks`;
  return (
    <>
      <Popover label="List actions" className="more-menu" trigger={<MoreHorizontal size={16} aria-hidden="true" />}>
        {(close) => (
          <div className="menu-actions" role="menu">
            <button
              type="button"
              role="menuitem"
              className="menu-item danger"
              disabled={remove.isPending}
              onClick={() => {
                close();
                void confirmAction({
                  title: `Delete ${list.name}?`,
                  message: list.count ? `Its ${count} stay in your library.` : "It has no bookmarks in it.",
                  confirmLabel: "Delete list",
                  danger: true,
                }).then((ok) => ok && remove.mutate());
              }}
            >
              <Trash2 size={14} aria-hidden="true" />
              Delete list
            </button>
          </div>
        )}
      </Popover>
      {remove.error && (
        <p className="form-error" role="alert">
          {remove.error.message}
        </p>
      )}
    </>
  );
}
