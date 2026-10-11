/* The posts opened most recently, kept in this browser so the finder can
   offer them before anything is typed. */

export type RecentPost = { id: number; author: string; username: string; preview: string };

const KEY = "bw.recentPosts";
const LIMIT = 6;

export function recentlyOpened(): RecentPost[] {
  try {
    const value = JSON.parse(localStorage.getItem(KEY) || "[]");
    return Array.isArray(value) ? value.filter((item) => item && typeof item.id === "number").slice(0, LIMIT) : [];
  } catch {
    return [];
  }
}

function save(items: RecentPost[]) {
  try {
    localStorage.setItem(KEY, JSON.stringify(items.slice(0, LIMIT)));
  } catch {
    /* storage unavailable: recents are a convenience */
  }
}

export function rememberOpened(post: RecentPost) {
  save([{ ...post, preview: post.preview.slice(0, 160) }, ...recentlyOpened().filter((item) => item.id !== post.id)]);
}

export function forgetOpened(id: number) {
  save(recentlyOpened().filter((item) => item.id !== id));
}
