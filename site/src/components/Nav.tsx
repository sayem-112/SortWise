import { AnimatePresence, motion, useMotionValueEvent, useScroll } from "motion/react";
import { Menu, X } from "lucide-react";
import { useState } from "react";
import { DEMO_URL, DOWNLOAD_URL, EASE, GITHUB_URL, Mark } from "./ui";

const LINKS = [
  { href: "#features", label: "Features" },
  { href: "#setup", label: "Setup" },
  { href: "#privacy", label: "Privacy" },
  { href: "#faq", label: "FAQ" },
  { href: GITHUB_URL, label: "GitHub" },
];

/* The header tightens once the page scrolls. On a phone the links fold into a menu. */
export function Nav() {
  const { scrollY } = useScroll();
  const [scrolled, setScrolled] = useState(false);
  const [open, setOpen] = useState(false);
  useMotionValueEvent(scrollY, "change", (value) => setScrolled(value > 24));

  return (
    <header className={`site-header ${scrolled ? "scrolled" : ""}`}>
      <nav className="container nav" aria-label="Main">
        <a className="brand" href="/" aria-label="Sortwise home">
          <Mark />
          <span>Sortwise</span>
        </a>
        <ul className="nav-links">
          {LINKS.map((link) => (
            <li key={link.href}>
              <a href={link.href}>{link.label}</a>
            </li>
          ))}
        </ul>
        <div className="nav-actions">
          <a className="button button-ghost button-small nav-download" href={DOWNLOAD_URL}>
            Download
          </a>
          <a className="button button-dark button-small" href={DEMO_URL}>
            Try the demo
          </a>
          <button type="button" className="nav-toggle" aria-expanded={open} aria-controls="mobile-menu" aria-label={open ? "Close menu" : "Open menu"} onClick={() => setOpen((value) => !value)}>
            {open ? <X size={20} /> : <Menu size={20} />}
          </button>
        </div>
      </nav>
      <AnimatePresence>
        {open && (
          <motion.ul
            id="mobile-menu"
            className="mobile-menu"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.35, ease: EASE }}
          >
            {[...LINKS, { href: DOWNLOAD_URL, label: "Download for Windows" }].map((link) => (
              <li key={link.href}>
                <a href={link.href} onClick={() => setOpen(false)}>
                  {link.label}
                </a>
              </li>
            ))}
          </motion.ul>
        )}
      </AnimatePresence>
    </header>
  );
}
