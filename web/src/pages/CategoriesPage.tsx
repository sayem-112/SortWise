import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlignLeft,
  CornerDownRight,
  FolderTree,
  SquareCheck,
  GitMerge,
  Hash,
  Info,
  Plus,
  Save,
  Table2,
  ToggleLeft,
  Type,
} from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  createCategory,
  deactivateCategory,
  getCategories,
  mergeCategory,
  updateCategory,
  type Category,
} from "../lib/api";
import { ProposalPanel, SuggestButton } from "../components/ProposalPanel";
import { Select } from "../components/Select";
import { Callout, CheckboxValue, EmptyState, Option, PageHeader, PropertyRow, SidePeek, ViewTabs } from "../components/ui";
import { colorFor } from "../lib/format";
import { confirmAction } from "../lib/confirm";

/* Parents first, each followed by its children, so the table reads as a tree. */
function treeOrder(items: Category[]) {
  const ids = new Set(items.map((item) => item.id));
  const children = new Map<number, Category[]>();
  items.forEach((item) => {
    if (item.parentId && ids.has(item.parentId)) {
      children.set(item.parentId, [...(children.get(item.parentId) || []), item]);
    }
  });
  const ordered: { item: Category; depth: number }[] = [];
  const visit = (item: Category, depth: number) => {
    ordered.push({ item, depth });
    (children.get(item.id) || []).forEach((child) => visit(child, depth + 1));
  };
  items.filter((item) => !item.parentId || !ids.has(item.parentId)).forEach((item) => visit(item, 0));
  return ordered;
}

export function CategoriesPage() {
  const client = useQueryClient();
  const result = useQuery({ queryKey: ["categories"], queryFn: getCategories });
  const [editorOpen, setEditorOpen] = useState(false);
  const [selectedID, setSelectedID] = useState<number>();
  const items = useMemo(() => result.data?.items || [], [result.data]);
  const selected = items.find((item) => item.id === selectedID);
  const rows = useMemo(() => treeOrder(items), [items]);
  const names = new Map(items.map((item) => [item.id, item.name]));
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [parent, setParent] = useState("");
  const [mergeTarget, setMergeTarget] = useState("");
  const [error, setError] = useState("");
  const [suggested, setSuggested] = useState(false);
  const [params] = useSearchParams();
  const autoSuggest = params.get("suggest") === "1";

  useEffect(() => {
    setName(selected?.name || "");
    setDescription(selected?.description || "");
    setParent(selected?.parentId ? String(selected.parentId) : "");
    setMergeTarget("");
    setError("");
  }, [selected]);

  const refresh = () => {
    client.invalidateQueries({ queryKey: ["categories"] });
    client.invalidateQueries({ queryKey: ["bookmarks"] });
  };

  const save = useMutation({
    mutationFn: () =>
      selected
        ? updateCategory(selected.id, { name, description, parentId: parent ? Number(parent) : undefined })
        : createCategory({ name, description, parentId: parent ? Number(parent) : undefined }),
    onSuccess: (item) => {
      setSelectedID(item.id);
      refresh();
    },
    onError: (e) => setError(e.message),
  });

  const deactivate = useMutation({
    mutationFn: () => deactivateCategory(selected!.id),
    onSuccess: refresh,
    onError: (e) => setError(e.message),
  });

  const merge = useMutation({
    mutationFn: () => mergeCategory(selected!.id, Number(mergeTarget)),
    onSuccess: () => {
      setSelectedID(undefined);
      setEditorOpen(false);
      refresh();
    },
    onError: (e) => setError(e.message),
  });

  function openEditor(id?: number) {
    setSelectedID(id);
    setEditorOpen(true);
    if (id === undefined) {
      setName("");
      setDescription("");
      setParent("");
      setError("");
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    save.mutate();
  }

  const activeCount = items.filter((item) => item.active).length;

  return (
    <main className="page full">
      <PageHeader
        icon={FolderTree}
        tone="orange"
        title="Categories"
        description="Broad on purpose to start with. Shape it to what you save: create, rename, nest, merge, or turn categories off."
      />

      <Callout icon={Info}>
        AI files each bookmark into up to two of your active categories and never adds categories by itself. When your
        library outgrows this list, <strong>Suggest categories</strong> proposes new ones from what you actually save, and
        nothing changes until you accept.
      </Callout>

      <div className="view-bar">
        <ViewTabs
          label="Category views"
          value="all"
          onChange={() => undefined}
          tabs={[{ value: "all", label: `All categories`, icon: Table2, count: items.length }]}
        />
        <div className="view-bar-tools">
          <SuggestButton kind="category" onDone={() => setSuggested(true)} autoStart={autoSuggest} />
          <span className="view-bar-note">{activeCount} active</span>
          <button className="button primary" onClick={() => openEditor()}>
            <Plus size={15} /> New
          </button>
        </div>
      </div>

      <ProposalPanel kind="category" justRan={suggested} />

      {result.isLoading && (
        <div className="skeleton-table" aria-busy="true">
          {Array.from({ length: 6 }).map((_, index) => (
            <div key={index} className="skeleton skeleton-row" />
          ))}
        </div>
      )}

      {items.length > 0 && (
        <div className="table-scroll">
          <table className="db-table">
            <thead>
              <tr>
                <th className="col-name"><span><Type size={14} />Name</span></th>
                <th className="col-desc"><span><AlignLeft size={14} />Description</span></th>
                <th className="col-parent"><span><CornerDownRight size={14} />Parent</span></th>
                <th className="col-num"><span><Hash size={14} />Bookmarks</span></th>
                <th className="col-check"><span><SquareCheck size={14} />Active</span></th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ item, depth }) => (
                <tr
                  key={item.id}
                  className={item.id === selectedID && editorOpen ? "selected" : ""}
                  onClick={() => openEditor(item.id)}
                >
                  <td className="col-name">
                    <button
                      type="button"
                      className="row-title"
                      style={{ paddingLeft: depth * 20 }}
                      onClick={(event) => {
                        event.stopPropagation();
                        openEditor(item.id);
                      }}
                    >
                      {depth > 0 && <CornerDownRight size={13} className="tree-mark" aria-hidden="true" />}
                      <Option color={item.active ? colorFor(item.id) : "default"}>{item.name}</Option>
                    </button>
                  </td>
                  <td className="col-desc">
                    <span className="cell-text">{item.description}</span>
                  </td>
                  <td className="col-parent">
                    {item.parentId ? names.get(item.parentId) : null}
                  </td>
                  <td className="col-num">{item.count.toLocaleString()}</td>
                  <td className="col-check">
                    <CheckboxValue checked={item.active} label="Active" />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {result.data && items.length === 0 && (
        <EmptyState
          icon={FolderTree}
          title="No categories yet"
          action={
            <button className="button primary" onClick={() => openEditor()}>
              <Plus size={15} /> New category
            </button>
          }
        >
          Create a handful of broad topics. AI sorts every bookmark into them.
        </EmptyState>
      )}

      <SidePeek open={editorOpen} onClose={() => setEditorOpen(false)} label={selected ? `Edit ${selected.name}` : "New category"}>
        <form className="document doc-peek" onSubmit={submit}>
          <div className={`document-icon tone-${selected ? colorFor(selected.id) : "orange"}`} aria-hidden="true">
            <FolderTree size={48} strokeWidth={1.5} />
          </div>
          <label className="visually-hidden" htmlFor="category-name">
            Name
          </label>
          <input
            id="category-name"
            className="title-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            maxLength={80}
            placeholder="Untitled category"
            autoFocus={!selected}
          />

          <div className="properties">
            <PropertyRow icon={CornerDownRight} name="Parent">
              <Select
                label="Parent category"
                value={parent}
                onChange={setParent}
                options={[
                  { value: "", label: "Top level (no parent)" },
                  ...items
                    .filter((item) => item.id !== selected?.id && item.active)
                    .map((item) => ({ value: String(item.id), label: item.name, color: colorFor(item.id) })),
                ]}
              />
            </PropertyRow>
            {selected && (
              <>
                <PropertyRow icon={Hash} name="Bookmarks">
                  {selected.count.toLocaleString()}
                </PropertyRow>
                <PropertyRow icon={SquareCheck} name="Active">
                  <CheckboxValue checked={selected.active} label="Active" />
                </PropertyRow>
              </>
            )}
          </div>

          <label className="block-label" htmlFor="category-description">
            Description
          </label>
          <textarea
            id="category-description"
            className="block-textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            placeholder="A short description helps the AI classify accurately…"
          />

          <div className="form-actions">
            <button className="button primary" type="submit" disabled={!name.trim() || save.isPending}>
              <Save size={14} />
              {save.isPending ? "Saving…" : selected ? "Save changes" : "Create category"}
            </button>
          </div>

          {selected && (
            <section className="danger-zone" aria-labelledby="category-merge-heading">
              <h3 id="category-merge-heading">Merge or deactivate</h3>
              <p className="muted">Merging moves every assignment into the target and removes this category.</p>
              <div className="inline-form">
                <Select
                  label="Merge into"
                  placeholder="Merge into…"
                  value={mergeTarget}
                  onChange={setMergeTarget}
                  options={items
                    .filter((item) => item.id !== selected.id && item.active)
                    .map((item) => ({ value: String(item.id), label: item.name, color: colorFor(item.id) }))}
                />
                <button
                  type="button"
                  className="button secondary"
                  disabled={!mergeTarget || merge.isPending}
                  onClick={() => {
                    void confirmAction({ title: `Merge ${selected.name}?`, message: "Its bookmarks move to the selected category, and this category is removed.", confirmLabel: "Merge" }).then((ok) => ok && merge.mutate());
                  }}
                >
                  <GitMerge size={14} /> Merge
                </button>
              </div>
              {selected.active && (
                <button
                  type="button"
                  className="button secondary"
                  disabled={deactivate.isPending}
                  onClick={() => deactivate.mutate()}
                >
                  <ToggleLeft size={14} /> Deactivate
                </button>
              )}
            </section>
          )}

          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
        </form>
      </SidePeek>
    </main>
  );
}
