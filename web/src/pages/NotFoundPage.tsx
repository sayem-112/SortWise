import { SearchX } from "lucide-react";
import { Link } from "react-router-dom";
import { EmptyState } from "../components/ui";

/* An address the app has no page for, such as an old or mistyped link. */
export function NotFoundPage() {
  return (
    <main className="page full">
      <EmptyState
        icon={SearchX}
        title="There's no page here"
        action={
          <Link to="/bookmarks" className="button secondary">
            Go to the library
          </Link>
        }
      >
        The link may be old or mistyped.
      </EmptyState>
    </main>
  );
}
