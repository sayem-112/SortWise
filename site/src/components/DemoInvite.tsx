import { ArrowRight } from "lucide-react";
import { motion } from "motion/react";
import { DEMO_URL, EASE, Reveal, SectionHead } from "./ui";

const CARDS = [
  { src: "/images/gallery.webp", alt: "The library in gallery view.", rotate: -7, x: "-14%", y: 24 },
  { src: "/images/authors.webp", alt: "Everyone saved from, on the Authors page.", rotate: 6, x: "14%", y: 30 },
  { src: "/images/overview.webp", alt: "The Overview page with categories and top tags.", rotate: 0, x: "0%", y: 0 },
];

/* Three screens dealt out like prints as the section comes into view. */
export function DemoInvite() {
  return (
    <section id="demo" className="demo-invite section-orange" aria-labelledby="demo-title">
      <div className="container demo-grid">
        <div>
          <SectionHead id="demo-title" title="See it with real posts." lead="A working copy of Sortwise, filled with public posts from X, already summarized, tagged, and searchable by meaning." />
          <Reveal className="actions">
            <a className="button button-cream button-large arrow-link" href={DEMO_URL}>
              Open the live demo
              <ArrowRight size={17} aria-hidden="true" />
            </a>
          </Reveal>
        </div>
        <a className="deck" href={DEMO_URL} aria-label="Open the live demo">
          {CARDS.map((card, index) => (
            <motion.figure
              key={card.src}
              className="shot deck-card"
              initial={{ opacity: 0, rotate: 0, x: "0%", y: 60 }}
              whileInView={{ opacity: 1, rotate: card.rotate, x: card.x, y: card.y }}
              whileHover={{ y: card.y - 10 }}
              viewport={{ once: true, margin: "-15% 0px" }}
              transition={{ duration: 0.9, ease: EASE, delay: index * 0.12 }}
              style={{ zIndex: index }}
            >
              <img src={card.src} width="2560" height="1600" alt={card.alt} loading="lazy" />
            </motion.figure>
          ))}
        </a>
      </div>
    </section>
  );
}
