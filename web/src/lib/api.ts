export type Media = {
  kind: "image" | "video_poster";
  url: string;
  previewUrl: string;
  altText: string;
  width?: number;
  height?: number;
  // The video file for a video or GIF, when captured from X's own data.
  videoUrl?: string;
};

export type TaxonomyItem = { id: number; name: string; confidence?: number; manual: boolean };

export type Bookmark = {
  id: number;
  postId: string;
  author: string;
  username: string;
  text: string;
  url: string;
  postedAt?: string;
  importedAt: string;
  language?: string;
  visibleContext: {
    quotedPost?: { postId: string; username: string; url: string; text: string };
    card?: { url: string; title: string; description: string };
    // Set when the post is an X Article filled in from X's embed data.
    article?: boolean;
  };
  media: Media[];
  summary: string;
  mediaDescription: string;
  processingStatus: "pending" | "processing" | "completed" | "failed" | "blocked";
  archived: boolean;
  // Set when the post was unbookmarked on X; it stays in the library.
  removedOnXAt?: string;
  categories: TaxonomyItem[];
  tags: TaxonomyItem[];
};

export type BookmarkPage = { items: Bookmark[]; page: number; pageSize: number; total: number };
export type Setup = { bookmarks: number; pendingJobs: number; aiConfigured: boolean };
export type Category = { id: number; name: string; parentId?: number; description: string; active: boolean; count: number };
export type TagKind = "topic" | "tool" | "entity" | "format";
export type Tag = { id: number; name: string; kind: TagKind; count: number; aliases: string[] };
export type ProviderId = "gemini" | "groq";
export type AIProviderOption = { id: ProviderId; name: string; model: string; configured: boolean; source: string; keyUrl: string };
export type AISettings = {
  provider: ProviderId;
  backup: boolean;
  paused: boolean;
  language: string;
  providers: AIProviderOption[];
  // The provider that would organize right now, or "" when none has a key.
  active: ProviderId | "";
  resting: { provider: ProviderId; restingUntil: string }[];
};
export type Job = { id: number; bookmarkId: number; postId: string; author: string; status: string; attempts: number; maxAttempts: number; lastError: string; updatedAt: string };
export type DashboardStats = { total: number; completed: number; pending: number; failed: number; needsReview: number; categories: { id: number; name: string; count: number }[]; topTags: { id: number; name: string; count: number }[] };

export class APIError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new APIError(response.status, data?.error?.message || "Something went wrong.");
  return data as T;
}

export const getSetup = () => api<Setup>("/setup");
export const getBookmarks = (params = "") => api<BookmarkPage>(`/bookmarks${params ? `?${params}` : ""}`);
export const getBookmark = (id: string) => api<Bookmark>(`/bookmarks/${id}`);
export const createPairingCode = () => api<{ code: string; expiresAt: string }>("/extension/pairing-codes", { method: "POST", body: "{}" });
export const getCategories = () => api<{ items: Category[] }>("/categories");
export const getTags = () => api<{ items: Tag[] }>("/tags");
export const setArchived = (id: number, archived: boolean) => api<Bookmark>(`/bookmarks/${id}`, { method: "PATCH", body: JSON.stringify({ archived }) });
export const deleteBookmark = (id: number) => api<void>(`/bookmarks/${id}?confirm=true`, { method: "DELETE" });
export const getAISettings = () => api<AISettings>("/settings/ai");
export const updateAISettings = (input: { provider?: ProviderId; backup?: boolean; paused?: boolean; language?: string }) =>
  api<AISettings>("/settings/ai", { method: "PATCH", body: JSON.stringify(input) });
export const saveAIKey = (provider: ProviderId, apiKey: string) =>
  api<AISettings>(`/settings/ai/keys/${provider}`, { method: "PUT", body: JSON.stringify({ apiKey }) });
export const removeAIKey = (provider: ProviderId) => api<AISettings>(`/settings/ai/keys/${provider}`, { method: "DELETE" });
export const testAI = (provider?: ProviderId) => api<{ status: string; provider: ProviderId }>("/settings/ai/test", { method: "POST", body: JSON.stringify(provider ? { provider } : {}) });
export const AI_LANGUAGES = [
  "English", "Arabic", "Bengali", "Chinese (Simplified)", "Chinese (Traditional)", "Dutch", "French", "German",
  "Hindi", "Indonesian", "Italian", "Japanese", "Korean", "Polish", "Portuguese", "Russian", "Spanish",
  "Thai", "Turkish", "Ukrainian", "Vietnamese",
];
export type JobCounts = { pending: number; processing: number; completed: number; failed: number; blocked: number; total: number };
export const getJobs = (status = "") => api<{ items: Job[]; counts: JobCounts }>(`/jobs${status ? `?status=${status}` : ""}`);
export const retryJob = (id: number) => api<void>(`/jobs/${id}/retry`, { method: "POST", body: "{}" });
export const reprocessBookmark = (id: number) => api<{ status: string }>(`/bookmarks/${id}/reprocess`, { method: "POST", body: "{}" });
export const updateBookmarkMetadata = (id: number, input: { summary?: string; resetSummary?: boolean; categoryDecisions?: { id: number; state: string }[]; tagDecisions?: { id: number; state: string }[] }) => api<Bookmark>(`/bookmarks/${id}`, { method: "PATCH", body: JSON.stringify(input) });
export const createCategory = (input: { name: string; description: string; parentId?: number }) => api<Category>("/categories", { method: "POST", body: JSON.stringify(input) });
export const updateCategory = (id: number, input: { name: string; description: string; parentId?: number; active?: boolean }) => api<Category>(`/categories/${id}`, { method: "PATCH", body: JSON.stringify(input) });
export const deactivateCategory = (id: number) => api<Category>(`/categories/${id}`, { method: "DELETE" });
export const mergeCategory = (id: number, targetId: number) => api<void>(`/categories/${id}/merge`, { method: "POST", body: JSON.stringify({ targetId }) });
export const createTag = (name: string, kind: TagKind) => api<Tag>("/tags", { method: "POST", body: JSON.stringify({ name, kind }) });
export const updateTag = (id: number, name: string, kind: TagKind) => api<Tag>(`/tags/${id}`, { method: "PATCH", body: JSON.stringify({ name, kind }) });
export const deleteTag = (id: number) => api<void>(`/tags/${id}`, { method: "DELETE" });
export const addTagAlias = (id: number, alias: string) => api<Tag>(`/tags/${id}/aliases`, { method: "POST", body: JSON.stringify({ alias }) });
export const deleteTagAlias = (id: number, alias: string) => api<Tag>(`/tags/${id}/aliases?alias=${encodeURIComponent(alias)}`, { method: "DELETE" });
export const mergeTag = (id: number, targetId: number) => api<void>(`/tags/${id}/merge`, { method: "POST", body: JSON.stringify({ targetId }) });
export const getDashboard = () => api<DashboardStats>("/dashboard");
export const createBackup = () => api<{ name: string; downloadUrl: string }>("/backups", { method: "POST", body: "{}" });

export type CategoryProposal = { name: string; parent: string; description: string; tags: string[]; reason: string };
export type MergeProposal = { source: string; target: string; reason: string };
export type TaxonomyProposal = { id: number; kind: "category" | "merge"; category?: CategoryProposal; merge?: MergeProposal; matches: number };
export const getProposals = () => api<{ items: TaxonomyProposal[] }>("/taxonomy/proposals");
export const suggestCategories = () => api<{ items: TaxonomyProposal[] }>("/taxonomy/proposals/categories", { method: "POST", body: "{}" });
export const suggestMerges = () => api<{ items: TaxonomyProposal[] }>("/taxonomy/proposals/merges", { method: "POST", body: "{}" });
export const acceptProposal = (id: number) => api<{ items: TaxonomyProposal[] }>(`/taxonomy/proposals/${id}/accept`, { method: "POST", body: "{}" });
export const dismissProposal = (id: number) => api<{ items: TaxonomyProposal[] }>(`/taxonomy/proposals/${id}/dismiss`, { method: "POST", body: "{}" });

export type ExtensionConnection = { id: number; label: string; createdAt: string; lastUsedAt: string | null };
export const getExtensionConnections = () => api<{ items: ExtensionConnection[] }>("/extension/connections");
export const revokeExtensionConnections = () => api<void>("/extension/connections", { method: "DELETE" });
