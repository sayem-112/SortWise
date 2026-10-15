import { useMutation } from "@tanstack/react-query";
import { useState } from "react";

/* A page title that is the thing's name and is edited in place, like a
   Notion page title: click, type, Enter or click away to save, Escape to
   cancel. Remount it (a `key`) when it should show a different thing. */
export function EditableTitle({ value, label, onSave }: { value: string; label: string; onSave: (value: string) => Promise<unknown> }) {
  const [name, setName] = useState(value);
  const rename = useMutation({
    mutationFn: onSave,
    onError: () => setName(value),
  });

  function save() {
    const next = name.trim();
    if (!next) setName(value);
    else if (next !== value) rename.mutate(next);
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
            setName(value);
            requestAnimationFrame(() => (event.target as HTMLInputElement).blur());
          }
        }}
        aria-label={label}
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
