import { Route, Routes } from "react-router-dom";
import { AppShell } from "./components/AppShell";
import { BookmarkDetailPage } from "./pages/BookmarkDetailPage";
import { BookmarksPage } from "./pages/BookmarksPage";
import { DashboardPage } from "./pages/DashboardPage";
import { SettingsPage } from "./pages/SettingsPage";
import { ActivityPage } from "./pages/ActivityPage";
import { CategoriesPage } from "./pages/CategoriesPage";
import { TagsPage } from "./pages/TagsPage";

export function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<DashboardPage />} />
        <Route path="bookmarks" element={<BookmarksPage />} />
        <Route path="bookmarks/:id" element={<BookmarkDetailPage />} />
        <Route path="categories" element={<CategoriesPage />} />
        <Route path="tags" element={<TagsPage />} />
        <Route path="activity" element={<ActivityPage />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  );
}
