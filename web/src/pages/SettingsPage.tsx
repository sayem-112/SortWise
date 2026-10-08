import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUpRight,
  Check,
  DatabaseBackup,
  FileJson,
  FileSpreadsheet,
  Info,
  KeyRound,
  Languages,
  LoaderCircle,
  Settings,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { FormEvent, useState } from "react";
import { ExtensionSettings } from "../components/ExtensionSettings";
import { Select } from "../components/Select";
import { Callout, PageHeader, Switch, Toggle } from "../components/ui";
import {
  AI_LANGUAGES,
  createBackup,
  getAISettings,
  removeAIKey,
  saveAIKey,
  testAI,
  updateAISettings,
  type AIProviderOption,
  type AISettings,
  type ProviderId,
} from "../lib/api";

const providerCopy: Record<ProviderId, { blurb: string; keyHelp: string }> = {
  gemini: {
    blurb: "Google's Gemini. Reads images well. A free key covers a steady trickle of bookmarks each day.",
    keyHelp: "Get a free key from Google AI Studio",
  },
  groq: {
    blurb: "Fast, with a generous free tier. A good backup when Gemini's free daily limit runs out.",
    keyHelp: "Get a free key from GroqCloud",
  },
};

function time(value: string) {
  return new Date(value).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

/* One line saying what the AI is doing right now, in plain words. */
function aiStatus(settings: AISettings): { text: string; tone: "green" | "orange" | "gray" } {
  const name = (id: string) => settings.providers.find((item) => item.id === id)?.name || id;
  if (!settings.active) return { text: "Needs a key", tone: "orange" };
  if (settings.paused) return { text: "Paused", tone: "gray" };
  const resting = settings.resting.map((item) => item.provider);
  if (resting.includes(settings.active)) return { text: "Waiting for limits to reset", tone: "orange" };
  return { text: `Organizing with ${name(settings.active)}`, tone: "green" };
}

export function SettingsPage() {
  const client = useQueryClient();
  const settings = useQuery({ queryKey: ["ai-settings"], queryFn: getAISettings, refetchInterval: 30_000 });
  const [message, setMessage] = useState<{ text: string; error?: boolean }>();
  const update = (value: AISettings) => {
    client.setQueryData(["ai-settings"], value);
    client.invalidateQueries({ queryKey: ["setup"] });
  };
  const fail = (error: Error) => setMessage({ text: error.message, error: true });

  const change = useMutation({
    mutationFn: updateAISettings,
    onSuccess: update,
    onError: fail,
  });
  const backup = useMutation({
    mutationFn: createBackup,
    onSuccess: (value) => {
      setMessage({ text: `Backup ${value.name} created.` });
      window.location.href = value.downloadUrl;
    },
    onError: fail,
  });

  const data = settings.data;
  const status = data ? aiStatus(data) : null;
  const bothKeys = Boolean(data?.providers.every((item) => item.configured));
  const main = data?.providers.find((item) => item.id === data.provider);
  const restingMain = data?.resting.find((item) => item.provider === data.provider);

  return (
    <main className="page">
      <PageHeader icon={Settings} title="Settings" description="How your bookmarks are organized, and where your data lives." />

      {message && (
        <Callout icon={message.error ? Info : Check} tone={message.error ? "red" : "green"} role="status">
          {message.text}
        </Callout>
      )}

      <section className="settings-section" aria-labelledby="provider-heading">
        <div className="settings-section-head">
          <h2 className="block-heading" id="provider-heading">
            AI organization
          </h2>
          {status && (
            <span className={`status-pill opt-${status.tone}`}>
              <span className="status-dot" aria-hidden="true" />
              {status.text}
            </span>
          )}
        </div>
        <p className="settings-lead">
          An AI service reads each new bookmark and writes its summary, tags, and categories. Both services below work with a
          free API key. Keys are stored in Windows Credential Manager, never in the app's files.
        </p>

        {restingMain && main && (
          <Callout icon={Info} tone={data?.active && data.active !== data.provider ? "blue" : "yellow"}>
            {data?.active && data.active !== data.provider ? (
              <p>
                {main.name} reached its free limit, so {data.providers.find((item) => item.id === data.active)?.name} is organizing until {main.name} is ready again at {time(restingMain.restingUntil)}.
              </p>
            ) : (
              <p>
                {main.name} reached its free limit. Organizing continues at {time(restingMain.restingUntil)}.
                {!bothKeys && " Add a key for the other service below to keep going in the meantime."}
              </p>
            )}
          </Callout>
        )}

        <div className="provider-list" role="radiogroup" aria-label="Main AI service">
          {data?.providers.map((provider) => (
            <ProviderRow
              key={provider.id}
              provider={provider}
              main={data.provider === provider.id}
              onSelect={() => change.mutate({ provider: provider.id })}
              onChanged={(value, text) => {
                if (value) update(value);
                setMessage(text ? { text } : undefined);
              }}
              onError={fail}
            />
          ))}
          {settings.isLoading && <div className="skeleton skeleton-row" />}
        </div>

        <div className="setting-row">
          <div className="setting-text">
            <strong>Use the other service as a backup</strong>
            <p>
              {bothKeys
                ? "When the main service reaches its free limit or is down, the other keeps organizing, then hands back."
                : "Add keys for both services to keep organizing when one reaches its free limit."}
            </p>
          </div>
          <Switch
            checked={Boolean(data?.backup) && bothKeys}
            label="Use the other service as a backup"
            disabled={!bothKeys || change.isPending}
            onChange={(value) => change.mutate({ backup: value })}
          />
        </div>

        <div className="setting-row">
          <div className="setting-text">
            <strong>Language</strong>
            <p>Summaries and topic tags are written in this language. Posts in any language are understood.</p>
          </div>
          <Select
            label="Language for summaries and tags"
            icon={Languages}
            align="end"
            value={data?.language || "English"}
            disabled={!data || change.isPending}
            onChange={(value) => change.mutate({ language: value })}
            options={AI_LANGUAGES.map((language) => ({ value: language, label: language }))}
          />
        </div>

        <div className="setting-row">
          <div className="setting-text">
            <strong>Pause organizing</strong>
            <p>
              {data?.paused
                ? "Paused. New bookmarks are saved and searchable but not sent to AI until you resume."
                : "New bookmarks are sent to the AI service shortly after they arrive."}
            </p>
          </div>
          <Switch
            checked={Boolean(data?.paused)}
            label="Pause organizing"
            disabled={!data || change.isPending}
            onChange={(value) => change.mutate({ paused: value })}
          />
        </div>

        <Callout icon={ShieldCheck}>
          Only a post's text, its images, and your tag and category names are sent to the service you use. Videos, your
          library, and your keys are never uploaded.
        </Callout>

        {data && <AdvancedAI settings={data} />}
      </section>

      <ExtensionSettings />

      <section className="settings-section" aria-labelledby="data-heading">
        <div className="settings-section-head">
          <h2 className="block-heading" id="data-heading">
            Your data
          </h2>
        </div>
        <p className="settings-lead">Everything stays in one file on this computer. Export or back it up any time.</p>

        <div className="setting-row">
          <div className="setting-text">
            <strong>Full JSON export</strong>
            <p>Every bookmark with summaries, categories, tags, and your corrections.</p>
          </div>
          <a className="button secondary" href="/api/v1/exports/bookmarks.json">
            <FileJson size={15} />
            Export JSON
          </a>
        </div>
        <div className="setting-row">
          <div className="setting-text">
            <strong>Bookmark CSV</strong>
            <p>A flat table for spreadsheets.</p>
          </div>
          <a className="button secondary" href="/api/v1/exports/bookmarks.csv">
            <FileSpreadsheet size={15} />
            Export CSV
          </a>
        </div>
        <div className="setting-row">
          <div className="setting-text">
            <strong>Database backup</strong>
            <p>A complete copy of your library you can restore later.</p>
          </div>
          <button className="button secondary" disabled={backup.isPending} onClick={() => backup.mutate()}>
            {backup.isPending ? <LoaderCircle className="spin" size={15} /> : <DatabaseBackup size={15} />}
            Create backup
          </button>
        </div>

        <Toggle summary="How to restore a backup">
          <p>
            Close Sortwise, then run <code>sortwise.exe restore path-to-backup.db</code>. Your current library
            is kept as a <code>.pre-restore</code> copy first, and the backup is checked before it replaces anything.
          </p>
        </Toggle>
      </section>
    </main>
  );
}

function ProviderRow({
  provider,
  main,
  onSelect,
  onChanged,
  onError,
}: {
  provider: AIProviderOption;
  main: boolean;
  onSelect: () => void;
  onChanged: (value: AISettings | undefined, message?: string) => void;
  onError: (error: Error) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [key, setKey] = useState("");
  const fromEnvironment = provider.source.endsWith("_API_KEY");
  const copy = providerCopy[provider.id];

  const save = useMutation({
    mutationFn: () => saveAIKey(provider.id, key.trim()),
    onSuccess: (value) => {
      setKey("");
      setEditing(false);
      onChanged(value, `${provider.name} key saved.`);
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: () => removeAIKey(provider.id),
    onSuccess: (value) => onChanged(value, `${provider.name} key removed.`),
    onError,
  });
  const test = useMutation({
    mutationFn: () => testAI(provider.id),
    onSuccess: () => onChanged(undefined, `${provider.name} is working.`),
    onError,
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    if (key.trim().length >= 10) save.mutate();
  }

  return (
    <div className={`provider ${main ? "selected" : ""}`}>
      <button type="button" role="radio" aria-checked={main} className="provider-select" onClick={() => !main && onSelect()}>
        <span className="choice-radio" aria-hidden="true" />
        <span className="choice-text">
          <strong>
            {provider.name}
            {main && <span className="provider-role">Main</span>}
            {provider.id === "gemini" && !main && <span className="provider-role muted">Recommended</span>}
          </strong>
          <span>{copy.blurb}</span>
        </span>
      </button>
      <div className="provider-key">
        {provider.configured ? (
          <>
            <span className="key-state ok">
              <KeyRound size={13} aria-hidden="true" />
              {fromEnvironment ? "Key from environment" : "Key saved"}
            </span>
            <button type="button" className="text-button" disabled={test.isPending} onClick={() => test.mutate()}>
              {test.isPending ? "Testing…" : "Test"}
            </button>
            {!fromEnvironment && (
              <>
                <button type="button" className="text-button" onClick={() => setEditing((value) => !value)}>
                  Replace
                </button>
                <button
                  type="button"
                  className="text-button danger"
                  disabled={remove.isPending}
                  onClick={() => {
                    if (window.confirm(`Remove the saved ${provider.name} key?`)) remove.mutate();
                  }}
                >
                  <Trash2 size={13} aria-hidden="true" />
                  Remove
                </button>
              </>
            )}
          </>
        ) : (
          !editing && (
            <button type="button" className="button secondary small" onClick={() => setEditing(true)}>
              <KeyRound size={14} />
              Add key
            </button>
          )
        )}
      </div>
      {editing && (
        <form className="provider-form" onSubmit={submit}>
          <input
            className="inline-input"
            type="password"
            autoComplete="off"
            autoFocus
            value={key}
            onChange={(event) => setKey(event.target.value)}
            placeholder={`Paste your ${provider.name} API key`}
            aria-label={`${provider.name} API key`}
          />
          <button type="submit" className="button primary" disabled={key.trim().length < 10 || save.isPending}>
            {save.isPending ? <LoaderCircle className="spin" size={15} /> : <Check size={15} />}
            Save
          </button>
          <button type="button" className="button secondary" onClick={() => setEditing(false)}>
            Cancel
          </button>
          <a className="inline-link" href={provider.keyUrl} target="_blank" rel="noreferrer">
            {copy.keyHelp} <ArrowUpRight size={13} />
          </a>
        </form>
      )}
    </div>
  );
}

/* Model names and overrides matter only to people who want them. */
function AdvancedAI({ settings }: { settings: AISettings }) {
  return (
    <Toggle summary="Advanced">
      <div className="properties compact">
        {settings.providers.map((provider) => (
          <div className="property-row" key={provider.id}>
            <div className="property-name">
              <span>{provider.name} model</span>
            </div>
            <div className="property-value">
              <code>{provider.model}</code>
            </div>
          </div>
        ))}
      </div>
      <p className="muted">
        Set <code>SORTWISE_GEMINI_MODEL</code> or <code>SORTWISE_GROQ_MODEL</code> to use another model, or{" "}
        <code>GEMINI_API_KEY</code> and <code>GROQ_API_KEY</code> to supply keys from the environment. Environment keys take
        priority over saved ones.
      </p>
    </Toggle>
  );
}
