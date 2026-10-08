import { Activity, BookMarked, FolderTree, House, Settings, Tags, type LucideIcon } from "lucide-react";
import type { OptionColor } from "./format";

export type NavItem = { to: string; label: string; icon: LucideIcon; tone: OptionColor };

export const navigation: NavItem[] = [
  { to: "/", label: "Overview", icon: House, tone: "gray" },
  { to: "/bookmarks", label: "Library", icon: BookMarked, tone: "blue" },
  { to: "/categories", label: "Categories", icon: FolderTree, tone: "orange" },
  { to: "/tags", label: "Tags", icon: Tags, tone: "purple" },
  { to: "/activity", label: "Activity", icon: Activity, tone: "green" },
  { to: "/settings", label: "Settings", icon: Settings, tone: "gray" },
];
