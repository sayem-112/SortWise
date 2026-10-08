import { useNavigate, useParams } from "react-router-dom";
import { BookmarkDocument } from "../components/BookmarkDocument";

export function BookmarkDetailPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();

  return (
    <main className="page bookmark-page">
      <BookmarkDocument key={id} bookmarkId={id} variant="page" onDeleted={() => navigate("/bookmarks")} />
    </main>
  );
}
