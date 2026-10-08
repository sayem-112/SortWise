import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { GitMerge, Hash, Plus, Replace, Save, Shapes, Table2, Tags, Trash2, Type } from "lucide-react";
import { FormEvent, useEffect, useState } from "react";
import {
  addTagAlias,
  createTag,
  deleteTag,
  deleteTagAlias,
  getTags,
  mergeTag,
  updateTag,
  type TagKind,
} from "../lib/api";
import { ProposalPanel, SuggestButton } from "../components/ProposalPanel";
import { Select } from "../components/Select";
import { EmptyState, Option, PageHeader, PropertyRow, SidePeek, ViewTabs } from "../components/ui";
import type { OptionColor } from "../lib/format";

const kindLabel: Record<TagKind, string> = { topic: "Topic", tool: "Tool", entity: "Person or org", format: "Format" };
const kindColor: Record<TagKind, OptionColor> = { topic: "gray", tool: "blue", entity: "purple", format: "orange" };

export function TagsPage() {
  const client = useQueryClient();
  const result = useQuery({ queryKey: ["tags"], queryFn: getTags });
  const [editorOpen, setEditorOpen] = useState(false);
  const [selectedID, setSelectedID] = useState<number>();
  const items = result.data?.items || [];
  const selected = items.find((item) => item.id === selectedID);
  const [name, setName] = useState("");
  const [alias, setAlias] = useState("");
  const [target, setTarget] = useState("");
  const [error, setError] = useState("");
  const [kind, setKind] = useState<TagKind>("topic");
  const [suggested, setSuggested] = useState(false);

  useEffect(() => {
    setName(selected?.name || "");
    setKind(selected?.kind || "topic");
    setAlias("");
    setTarget("");
    setError("");
  }, [selected]);

  const refresh = () => {
    client.invalidateQueries({ queryKey: ["tags"] });
    client.invalidateQueries({ queryKey: ["bookmarks"] });
  };

  const save = useMutation({
    mutationFn: () => (selected ? updateTag(selected.id, name, kind) : createTag(name, kind)),
    onSuccess: (item) => {
      setSelectedID(item.id);
      refresh();
    },
    onError: (e) => setError(e.message),
  });

  const addAlias = useMutation({
    mutationFn: () => addTagAlias(selected!.id, alias),
    onSuccess: () => {
      setAlias("");
      refresh();
    },
    onError: (e) => setError(e.message),
  });

  const removeAlias = useMutation({
    mutationFn: (value: string) => deleteTagAlias(selected!.id, value),
    onSuccess: refresh,
    onError: (e) => setError(e.message),
  });

  const merge = useMutation({
    mutationFn: () => mergeTag(selected!.id, Number(target)),
    onSuccess: () => {
      setSelectedID(undefined);
      setEditorOpen(false);
      refresh();
    },
    onError: (e) => setError(e.message),
  });

  const remove = useMutation({
    mutationFn: () => deleteTag(selected!.id),
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
      setKind("topic");
      setError("");
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    save.mutate();
  }

  return (
    <main className="page full">
      <PageHeader
        icon={Tags}
        tone="purple"
        title="Tags"
        description="Canonical concepts and aliases keep small wording differences from fragmenting your library."
      />

      <div className="view-bar">
        <ViewTabs
          label="Tag views"
          value="all"
          onChange={() => undefined}
          tabs={[{ value: "all", label: "All tags", icon: Table2, count: items.length }]}
        />
        <div className="view-bar-tools">
          <SuggestButton kind="merge" onDone={() => setSuggested(true)} />
          <button className="button primary" onClick={() => openEditor()}>
            <Plus size={15} /> New
          </button>
        </div>
      </div>

      <ProposalPanel kind="merge" justRan={suggested} />

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
                <th className="col-kind"><span><Shapes size={14} />Type</span></th>
                <th className="col-aliases"><span><Replace size={14} />Aliases</span></th>
                <th className="col-num"><span><Hash size={14} />Bookmarks</span></th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr
                  key={item.id}
                  className={item.id === selectedID && editorOpen ? "selected" : ""}
                  onClick={() => openEditor(item.id)}
                >
                  <td className="col-name">
                    <button
                      type="button"
                      className="row-title"
                      onClick={(event) => {
                        event.stopPropagation();
                        openEditor(item.id);
                      }}
                    >
                      <Option>{item.name}</Option>
                    </button>
                  </td>
                  <td className="col-kind">
                    <Option color={kindColor[item.kind]}>{kindLabel[item.kind]}</Option>
                  </td>
                  <td className="col-aliases">
                    <span className="cell-options">
                      {item.aliases.map((value) => (
                        <Option key={value} color="gray">
                          {value}
                        </Option>
                      ))}
                    </span>
                  </td>
                  <td className="col-num">{item.count.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {result.data && items.length === 0 && (
        <EmptyState
          icon={Tags}
          title="No tags yet"
          action={
            <button className="button secondary" onClick={() => openEditor()}>
              <Plus size={15} /> Create a tag
            </button>
          }
        >
          Tags appear here after AI analysis, or you can create your own.
        </EmptyState>
      )}

      <SidePeek open={editorOpen} onClose={() => setEditorOpen(false)} label={selected ? `Edit ${selected.name}` : "New tag"}>
        <form className="document doc-peek" onSubmit={submit}>
          <div className="document-icon tone-purple" aria-hidden="true">
            <Tags size={48} strokeWidth={1.5} />
          </div>
          <label className="visually-hidden" htmlFor="tag-name">
            Name
          </label>
          <input
            id="tag-name"
            className="title-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            maxLength={80}
            placeholder="Untitled tag"
            autoFocus={!selected}
          />

          <div className="properties">
            <PropertyRow icon={Shapes} name="Type">
              <Select
                label="Tag type"
                value={kind}
                onChange={setKind}
                options={(Object.keys(kindLabel) as TagKind[]).map((value) => ({ value, label: kindLabel[value], color: kindColor[value] }))}
              />
            </PropertyRow>
          </div>

          {selected && (
            <div className="properties">
              <PropertyRow icon={Hash} name="Bookmarks">
                {selected.count.toLocaleString()}
              </PropertyRow>
              <PropertyRow icon={Replace} name="Aliases">
                {selected.aliases.length ? (
                  selected.aliases.map((value) => (
                    <Option
                      key={value}
                      color="gray"
                      onRemove={() => removeAlias.mutate(value)}
                      removeLabel={`Remove ${value}`}
                    >
                      {value}
                    </Option>
                  ))
                ) : (
                  <span className="placeholder">No aliases yet</span>
                )}
              </PropertyRow>
            </div>
          )}

          <div className="form-actions">
            <button className="button primary" type="submit" disabled={!name.trim() || save.isPending}>
              <Save size={14} />
              {save.isPending ? "Saving…" : selected ? "Save canonical name" : "Create tag"}
            </button>
          </div>

          {selected && (
            <>
              <section className="block-section tight" aria-labelledby="alias-heading">
                <h3 id="alias-heading" className="sub-heading">
                  Add an alias
                </h3>
                <p className="muted">Posts tagged with an alias are filed under #{selected.name}.</p>
                <div className="inline-form">
                  <input
                    className="inline-input"
                    aria-label="New alias"
                    value={alias}
                    onChange={(e) => setAlias(e.target.value)}
                    placeholder="e.g. golang"
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        if (alias.trim()) addAlias.mutate();
                      }
                    }}
                  />
                  <button
                    type="button"
                    className="button secondary"
                    disabled={!alias.trim() || addAlias.isPending}
                    onClick={() => addAlias.mutate()}
                  >
                    <Plus size={14} /> Add
                  </button>
                </div>
              </section>

              <section className="danger-zone" aria-labelledby="tag-merge-heading">
                <h3 id="tag-merge-heading">Merge or delete</h3>
                <p className="muted">Merging moves this tag and its aliases into the target.</p>
                <div className="inline-form">
                  <Select
                    label="Merge into"
                    placeholder="Merge into…"
                    value={target}
                    onChange={setTarget}
                    options={items
                      .filter((item) => item.id !== selected.id)
                      .map((item) => ({ value: String(item.id), label: item.name, hint: String(item.count) }))}
                  />
                  <button
                    type="button"
                    className="button secondary"
                    disabled={!target || merge.isPending}
                    onClick={() => {
                      if (window.confirm(`Merge #${selected.name} and its aliases into the selected tag?`)) {
                        merge.mutate();
                      }
                    }}
                  >
                    <GitMerge size={14} /> Merge
                  </button>
                </div>
                <button
                  type="button"
                  className="button secondary danger"
                  disabled={remove.isPending}
                  onClick={() => remove.mutate()}
                  title="Only tags with no bookmarks can be deleted"
                >
                  <Trash2 size={14} /> Delete unused tag
                </button>
              </section>
            </>
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
