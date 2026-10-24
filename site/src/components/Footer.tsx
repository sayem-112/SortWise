import { motion } from "motion/react";
import { DEMO_URL, DOWNLOAD_URL, EASE, GITHUB_URL, Mark } from "./ui";

/* Links, then the name set huge and cut off by the page's edge. */
export function Footer() {
  return (
    <footer className="site-footer section-orange">
      <div className="container footer-top">
        <a className="brand brand-cream" href="/">
          <Mark tone="cream" />
          <span>Sortwise</span>
        </a>
        <ul className="footer-links">
          <li>
            <a href={DOWNLOAD_URL}>Download</a>
          </li>
          <li>
            <a href={DEMO_URL}>Live demo</a>
          </li>
          <li>
            <a href={GITHUB_URL}>GitHub</a>
          </li>
          <li>
            <a href={`${GITHUB_URL}/issues`}>Report a problem</a>
          </li>
        </ul>
      </div>
      <div className="container">
        <p className="fine-print">Not affiliated with X Corp. Posts in the demo are public posts, shown as their authors wrote them.</p>
      </div>
      <motion.p className="wordmark" aria-hidden="true" initial={{ y: "35%" }} whileInView={{ y: "0%" }} viewport={{ once: true }} transition={{ duration: 1.1, ease: EASE }}>
        Sortwise
      </motion.p>
    </footer>
  );
}
