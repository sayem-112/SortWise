import library from "../data/library.json";
import { Avatar, SectionHead } from "./ui";

type Post = (typeof library.posts)[number];

function PostCard({ post }: { post: Post }) {
  const tags = post.tags.filter((tag) => tag.toLowerCase() !== post.author.toLowerCase());
  return (
    <article className="post-card">
      <header>
        <Avatar src={post.avatar} name={post.author} size={36} />
        <span className="who">
          <strong>{post.author}</strong>
          <span>@{post.username}</span>
        </span>
      </header>
      <p className="post-text">{post.text}</p>
      <footer>
        {post.category && <span className="chip chip-category">{post.category}</span>}
        {tags.map((tag) => (
          <span className="chip" key={tag}>
            {tag}
          </span>
        ))}
      </footer>
    </article>
  );
}

/* Two rows of real saved posts drifting past in opposite directions, each
   already tagged and filed. Hovering a row holds it still. */
export function PostMarquee() {
  const half = Math.ceil(library.posts.length / 2);
  const rows = [library.posts.slice(0, half), library.posts.slice(half)];
  return (
    <section className="marquee-section section-cream" aria-labelledby="marquee-title">
      <div className="container">
        <SectionHead id="marquee-title" align="center" title="Every bookmark, read and filed." lead="Each post gets a category and specific tags the moment it arrives, in whatever field you save from." />
      </div>
      <div className="marquee" aria-hidden="true">
        {rows.map((row, index) => (
          <div className={`marquee-row ${index ? "reverse" : ""}`} key={index}>
            <div className="marquee-track">
              {[...row, ...row].map((post, position) => (
                <PostCard post={post} key={`${post.username}-${position}`} />
              ))}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
