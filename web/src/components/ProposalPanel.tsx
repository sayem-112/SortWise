import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, Check, CornerDownRight, Info, LoaderCircle, Sparkles, X } from "lucide-react";
import {
  acceptProposal,
  dismissProposal,
  getProposals,
  suggestCategories,
  suggestMerges,
  type TaxonomyProposal,
} from "../lib/api";
import { colorFor } from "../lib/format";
import { Callout, Option } from "./ui";
import { useEffect, useRef } from "react";

type Kind = TaxonomyProposal["kind"];

const copy: Record<Kind, { button: string; running: string; heading: string; empty: string; intro: string }> = {
  category: {
    button: "Suggest categories",
    running: "Reading your library…",
    heading: "Suggested categories",
    empty: "No new categories to suggest. Your structure already fits your bookmarks.",
    intro: "Based on your tags and summaries. Accepting one creates it and files the matching bookmarks right away.",
  },
  merge: {
    button: "Find duplicate tags",
    running: "Comparing tags…",
    heading: "Suggested merges",
    empty: "No duplicate tags found.",
    intro: "Merging moves every bookmark to the kept tag and remembers the old name as an alias.",
  },
};

function useProposals() {
  return useQuery({ queryKey: ["taxonomy-proposals"], queryFn: getProposals });
}

/* Toolbar button that asks the AI for fresh suggestions of one kind. */
export function SuggestButton({ kind, onDone, autoStart = false }: { kind: Kind; onDone: (count: number) => void; autoStart?: boolean }) {
  const client = useQueryClient();
  const started = useRef(false);
  const suggest = useMutation({
    mutationFn: kind === "category" ? suggestCategories : suggestMerges,
    onSuccess: (data) => {
      client.setQueryData(["taxonomy-proposals"], data);
      onDone(data.items.filter((item) => item.kind === kind).length);
    },
  });
  // Links such as Overview's "Suggest categories" start a run on arrival.
  useEffect(() => {
    if (autoStart && !started.current) {
      started.current = true;
      suggest.mutate();
    }
  }, [autoStart, suggest]);
  return (
    <>
      <button className="tool-button" disabled={suggest.isPending} onClick={() => suggest.mutate()}>
        {suggest.isPending ? <LoaderCircle size={15} className="spin" /> : <Sparkles size={15} />}
        <span>{suggest.isPending ? copy[kind].running : copy[kind].button}</span>
      </button>
      {suggest.isError && (
        <span className="view-bar-error" role="alert">
          {suggest.error.message}
        </span>
      )}
    </>
  );
}

export function ProposalPanel({ kind, justRan }: { kind: Kind; justRan: boolean }) {
  const client = useQueryClient();
  const proposals = useProposals();
  const items = (proposals.data?.items || []).filter((item) => item.kind === kind);

  const refresh = (data: { items: TaxonomyProposal[] }) => {
    client.setQueryData(["taxonomy-proposals"], data);
    client.invalidateQueries({ queryKey: ["categories"] });
    client.invalidateQueries({ queryKey: ["tags"] });
    client.invalidateQueries({ queryKey: ["bookmarks"] });
    client.invalidateQueries({ queryKey: ["dashboard"] });
  };
  const accept = useMutation({ mutationFn: acceptProposal, onSuccess: refresh });
  const dismiss = useMutation({ mutationFn: dismissProposal, onSuccess: refresh });
  const busy = accept.isPending || dismiss.isPending;

  if (!items.length) {
    return justRan ? (
      <Callout icon={Info}>
        <p>{copy[kind].empty}</p>
      </Callout>
    ) : null;
  }

  return (
    <section className="proposal-panel" aria-labelledby={`${kind}-proposals`}>
      <div className="proposal-head">
        <h2 className="sub-heading" id={`${kind}-proposals`}>
          <Sparkles size={15} aria-hidden="true" /> {copy[kind].heading}
          <span className="view-tab-count">{items.length}</span>
        </h2>
        <p className="muted">{copy[kind].intro}</p>
      </div>
      <ul className="proposal-list">
        {items.map((item) => (
          <li key={item.id}>
            {item.category && (
              <div className="proposal-body">
                <div className="proposal-title">
                  {item.category.parent && (
                    <>
                      <span className="muted">{item.category.parent}</span>
                      <CornerDownRight size={13} className="tree-mark" aria-hidden="true" />
                    </>
                  )}
                  <strong>{item.category.name}</strong>
                  <span className="proposal-count">
                    {item.matches} bookmark{item.matches === 1 ? "" : "s"}
                  </span>
                </div>
                {item.category.description && <p>{item.category.description}</p>}
                <div className="cell-options wrap">
                  {item.category.tags.map((tag) => (
                    <Option key={tag}>{tag}</Option>
                  ))}
                </div>
                {item.category.reason && <p className="muted">{item.category.reason}</p>}
              </div>
            )}
            {item.merge && (
              <div className="proposal-body">
                <div className="proposal-title">
                  <Option color={colorFor(item.id)}>{item.merge.source}</Option>
                  <ArrowRight size={14} className="tree-mark" aria-hidden="true" />
                  <Option color={colorFor(item.id)}>{item.merge.target}</Option>
                  <span className="proposal-count">
                    {item.matches} bookmark{item.matches === 1 ? "" : "s"} move
                  </span>
                </div>
                {item.merge.reason && <p className="muted">{item.merge.reason}</p>}
              </div>
            )}
            <div className="proposal-actions">
              <button
                className="ghost-button"
                disabled={busy}
                onClick={() => accept.mutate(item.id)}
                aria-label={`Accept ${item.category?.name || `merge ${item.merge?.source} into ${item.merge?.target}`}`}
              >
                <Check size={14} /> Accept
              </button>
              <button className="icon-button" disabled={busy} onClick={() => dismiss.mutate(item.id)} aria-label="Dismiss suggestion">
                <X size={15} />
              </button>
            </div>
          </li>
        ))}
      </ul>
      {(accept.isError || dismiss.isError) && (
        <p className="form-error" role="alert">
          {(accept.error || dismiss.error)?.message}
        </p>
      )}
    </section>
  );
}
