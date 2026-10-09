import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, KeyRound, Puzzle, Unplug } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { createPairingCode, getExtensionConnections, revokeExtensionConnections } from "../lib/api";
import { formatDateTime } from "../lib/format";
import { Callout, Toggle } from "./ui";
import { confirmAction } from "../lib/confirm";

/* Settings section for pairing the browser extension. Pairing codes used to be
   reachable only from an empty Overview, so a reinstalled extension could not
   pair again. */
export function ExtensionSettings() {
  const client = useQueryClient();
  const section = useRef<HTMLElement>(null);
  const connections = useQuery({ queryKey: ["extension-connections"], queryFn: getExtensionConnections, refetchInterval: 10_000 });
  const [pairing, setPairing] = useState<{ code: string; expiresAt: string }>();
  const [copied, setCopied] = useState(false);

  // The extension popup links to /settings#extension.
  useEffect(() => {
    if (window.location.hash === "#extension") section.current?.scrollIntoView({ block: "start" });
  }, []);

  const create = useMutation({
    mutationFn: createPairingCode,
    onSuccess: (value) => {
      setPairing(value);
      setCopied(false);
    },
  });
  const revoke = useMutation({
    mutationFn: revokeExtensionConnections,
    onSuccess: () => {
      setPairing(undefined);
      client.invalidateQueries({ queryKey: ["extension-connections"] });
    },
  });

  const items = connections.data?.items || [];
  const expired = pairing && Date.parse(pairing.expiresAt) < Date.now();

  async function copy() {
    if (!pairing) return;
    try {
      await navigator.clipboard.writeText(pairing.code);
      setCopied(true);
    } catch {
      /* clipboard unavailable: the code is on screen to type */
    }
  }

  return (
    <section className="settings-section" id="extension" aria-labelledby="extension-heading" ref={section}>
      <div className="settings-section-head">
        <h2 className="block-heading" id="extension-heading">
          Browser extension
        </h2>
        <span className={`status-pill ${items.length ? "opt-green" : "opt-gray"}`}>
          <span className="status-dot" aria-hidden="true" />
          {items.length ? `${items.length} connected` : "Not connected"}
        </span>
      </div>
      <p className="settings-lead">The Chrome or Edge extension imports your X bookmarks into this library.</p>

      <div className="setting-row">
        <div className="setting-text">
          <strong>Pair an extension</strong>
          <p>Create a one-time code and enter it in the extension popup. Codes expire after 5 minutes.</p>
        </div>
        <button className="button secondary" disabled={create.isPending} onClick={() => create.mutate()}>
          <KeyRound size={15} />
          {pairing ? "New code" : "Create code"}
        </button>
      </div>

      {pairing && (
        <Callout icon={Puzzle} tone={expired ? "default" : "blue"}>
          <div className="pairing">
            <span className="pairing-code" aria-label={`Pairing code ${pairing.code}`}>
              {pairing.code}
            </span>
            <button className="ghost-button" onClick={copy} disabled={Boolean(expired)}>
              {copied ? <Check size={14} /> : <Copy size={14} />}
              {copied ? "Copied" : "Copy"}
            </button>
          </div>
          <p className="callout-note">
            {expired
              ? "This code has expired. Create a new one."
              : `Valid until ${new Date(pairing.expiresAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}.`}
          </p>
        </Callout>
      )}
      {create.isError && (
        <p className="form-error" role="alert">
          {create.error.message}
        </p>
      )}

      {items.length > 0 && (
        <div className="properties">
          {items.map((item) => (
            <div className="property-row" key={item.id}>
              <div className="property-name">
                <Puzzle size={15} aria-hidden="true" />
                <span>{item.label}</span>
              </div>
              <div className="property-value">
                {item.lastUsedAt ? `Last used ${formatDateTime(item.lastUsedAt)}` : "Paired, not used yet"}
              </div>
            </div>
          ))}
        </div>
      )}

      <Toggle summary="Load the extension">
        <p>
          In Chrome or Edge, open <code>chrome://extensions</code>, turn on <strong>Developer mode</strong>, choose{" "}
          <strong>Load unpacked</strong>, and select the <code>extension</code> folder. Then click the extension's
          toolbar icon.
        </p>
      </Toggle>

      {items.length > 0 && (
        <div className="button-row">
          <button
            className="button secondary danger"
            disabled={revoke.isPending}
            onClick={() => {
              void confirmAction({ title: "Disconnect every extension?", message: "Each one will need a new pairing code before it can save bookmarks again.", confirmLabel: "Disconnect all", danger: true }).then((ok) => ok && revoke.mutate());
            }}
          >
            <Unplug size={15} />
            Disconnect all
          </button>
        </div>
      )}
    </section>
  );
}
