import { Activity, BookMarked, FolderTree, House, Settings, Tags, Users, type LucideIcon } from "lucide-react";
import type { OptionColor } from "./format";

// `shortcut` is the number pressed with Ctrl/⌘. Pages keep their number when
// a new one joins, so Authors, added later, took the next free one.
export type NavItem = { to: string; label: string; icon: LucideIcon; tone: OptionColor; shortcut: number };

export const navigation: NavItem[] = [
  { to: "/", label: "Overview", icon: House, tone: "gray", shortcut: 1 },
  { to: "/bookmarks", label: "Library", icon: BookMarked, tone: "blue", shortcut: 2 },
  { to: "/authors", label: "Authors", icon: Users, tone: "pink", shortcut: 7 },
  { to: "/categories", label: "Categories", icon: FolderTree, tone: "orange", shortcut: 3 },
  { to: "/tags", label: "Tags", icon: Tags, tone: "purple", shortcut: 4 },
  { to: "/activity", label: "Activity", icon: Activity, tone: "green", shortcut: 5 },
  { to: "/settings", label: "Settings", icon: Settings, tone: "gray", shortcut: 6 },
];
