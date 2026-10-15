import { Route, Routes } from "react-router-dom";
import { AppShell } from "./components/AppShell";
import { BookmarkDetailPage } from "./pages/BookmarkDetailPage";
import { BookmarksPage } from "./pages/BookmarksPage";
import { DashboardPage } from "./pages/DashboardPage";
import { ListPage } from "./pages/ListPage";
import { SettingsPage } from "./pages/SettingsPage";
import { ActivityPage } from "./pages/ActivityPage";
import { AuthorPage } from "./pages/AuthorPage";
import { AuthorsPage } from "./pages/AuthorsPage";
import { CategoriesPage } from "./pages/CategoriesPage";
import { CollectionPage } from "./pages/CollectionPage";
import { TagsPage } from "./pages/TagsPage";

export function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<DashboardPage />} />
        <Route path="bookmarks" element={<BookmarksPage />} />
        <Route path="bookmarks/:id" element={<BookmarkDetailPage />} />
        <Route path="lists/:id" element={<ListPage />} />
        <Route path="collections/:id" element={<CollectionPage />} />
        <Route path="authors" element={<AuthorsPage />} />
        <Route path="authors/:username" element={<AuthorPage />} />
        <Route path="categories" element={<CategoriesPage />} />
        <Route path="tags" element={<TagsPage />} />
        <Route path="activity" element={<ActivityPage />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  );
}
