import { Pencil } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Popover } from "./ui";

export type LinkedValue = { key: number; to: string; title: string; content: ReactNode };

/* A property's values as links to the posts that share them, the way a label
   on GitHub opens everything with that label. The editor sits behind them:
   the empty space after the values and the pencil at the end of the row
   open it. `className` is added to the editor's anchor, for shortcuts that
   find it. */
export function LinkedValues({
  values,
  empty,
  editLabel,
  className = "",
  children,
}: {
  values: LinkedValue[];
  empty: ReactNode;
  editLabel: string;
  className?: string;
  children: (close: () => void) => ReactNode;
}) {
  return (
    <div className="linked-values">
      <div className="linked-values-items">
        {values.length ? (
          values.map((value) => (
            <Link key={value.key} to={value.to} title={value.title} className="linked-value">
              {value.content}
            </Link>
          ))
        ) : (
          <span className="placeholder">{empty}</span>
        )}
      </div>
      <Popover
        label={editLabel}
        title={editLabel}
        align="start"
        className={`rail-editor linked-values-editor ${className}`}
        triggerClassName="linked-values-edit"
        trigger={<Pencil size={13} aria-hidden="true" />}
      >
        {children}
      </Popover>
    </div>
  );
}
