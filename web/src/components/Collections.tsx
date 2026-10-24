import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, MoreHorizontal, Trash2, Zap } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { createCollection, updateCollection, type Collection, type CollectionQuery } from "../lib/api";
import { addCollection, collectionsQuery, useCollectionActions } from "../lib/collections";
import { showToast } from "../lib/toast";
import { Popover } from "./ui";

/* Saves the search and filters on screen as a smart collection: a page in
   the sidebar that shows whatever matches them from then on. */
export function SaveCollection({ query, suggested }: { query: CollectionQuery; suggested: string }) {
  return (
    <Popover
      label="Save as a smart collection"
      title="Save as a smart collection: keeps this search and these filters in the sidebar"
      align="end"
      className="save-collection"
      triggerClassName="tool-button"
      trigger={
        <>
          <Zap size={15} aria-hidden="true" />
          <span>Save</span>
        </>
      }
    >
      {(close) => <SaveForm query={query} suggested={suggested} close={close} />}
    </Popover>
  );
}

function SaveForm({ query, suggested, close }: { query: CollectionQuery; suggested: string; close: () => void }) {
  const client = useQueryClient();
  const navigate = useNavigate();
  const [name, setName] = useState(suggested);
  const create = useMutation({
    mutationFn: () => createCollection(name.trim(), query),
    onSuccess: (collection) => {
      addCollection(client, collection);
      close();
      navigate(`/collections/${collection.id}`);
      showToast({ message: `Saved ${collection.name}. It's in the sidebar under Smart collections.` });
    },
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    if (name.trim() && !create.isPending) create.mutate();
  }
  return (
    <form className="save-collection-form" onSubmit={submit}>
      <p className="menu-caption">A smart collection keeps this search and these filters in the sidebar, and fills itself as posts match.</p>
      <label className="new-list-input">
        <Zap size={14} aria-hidden="true" />
        <input
          value={name}
          onChange={(event) => {
            setName(event.target.value);
            if (create.error) create.reset();
          }}
          onFocus={(event) => event.target.select()}
          placeholder="Collection name"
          aria-label="Collection name"
          maxLength={80}
          autoFocus
        />
        <button type="submit" className="new-list-save" disabled={!name.trim() || create.isPending} aria-label="Save collection" title="Save (Enter)">
          <Check size={14} />
        </button>
      </label>
      {create.error && (
        <p className="form-error" role="alert">
          {create.error.message}
        </p>
      )}
    </form>
  );
}

/* Shown on a smart collection's page once its search or filters are
   changed: keep the change, or go back to what was saved. */
export function CollectionChanges({ collection, query, canSave, onDiscard }: { collection: Collection; query: CollectionQuery; canSave: boolean; onDiscard: () => void }) {
  const client = useQueryClient();
  const save = useMutation({
    mutationFn: () => updateCollection(collection.id, { query }),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["collections"] });
      showToast({ message: `Saved changes to ${collection.name}` });
    },
    onError: (error) => showToast({ message: error.message, tone: "error" }),
  });
  return (
    <div className="collection-changes">
      <span role="status">{canSave ? `You changed what ${collection.name} shows.` : "A collection needs a search or a filter."}</span>
      <span className="collection-changes-actions">
        <button type="button" className="button secondary small" onClick={onDiscard}>
          Discard
        </button>
        <button type="button" className="button primary small" disabled={!canSave || save.isPending} onClick={() => save.mutate()}>
          Save changes
        </button>
      </span>
    </div>
  );
}

/* The ⋯ menu for a smart collection: its color, and delete (after asking).
   Used on the sidebar row and on the collection's page. */
export function CollectionMenu({ collection, className = "", align = "end" }: { collection: Collection; className?: string; align?: "start" | "end" }) {
  const client = useQueryClient();
  const collections = useQuery(collectionsQuery);
  const actions = useCollectionActions();
  const recolor = useMutation({
    mutationFn: (color: string) => updateCollection(collection.id, { color }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["collections"] }),
    onError: (error) => showToast({ message: error.message, tone: "error" }),
  });
  return (
    <Popover
      label={`${collection.name} options`}
      title="Color or delete"
      align={align}
      className={`more-menu ${className}`}
      trigger={<MoreHorizontal size={16} aria-hidden="true" />}
    >
      {(close) => (
        <div className="menu-actions">
          <p className="menu-caption">Color</p>
          <div className="look-colors" role="radiogroup" aria-label="Color">
            {(collections.data?.colors || []).map((color) => (
              <button
                key={color}
                type="button"
                role="radio"
                aria-checked={color === collection.color}
                autoFocus={color === collection.color}
                aria-label={color}
                title={color[0].toUpperCase() + color.slice(1)}
                className={`look-swatch swatch-${color} ${color === collection.color ? "on" : ""}`}
                onClick={() => recolor.mutate(color)}
              />
            ))}
          </div>
          <button
            type="button"
            className="menu-item danger"
            onClick={() => {
              close();
              actions.confirmDelete(collection);
            }}
          >
            <Trash2 size={14} aria-hidden="true" />
            Delete collection
          </button>
        </div>
      )}
    </Popover>
  );
}
