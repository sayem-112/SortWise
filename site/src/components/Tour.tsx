import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { EASE, SectionHead } from "./ui";

const STOPS = [
  { key: "post", label: "Every post, explained", caption: "The post in full, an AI summary, its tags, and related posts from your own library.", src: "/images/post.webp", width: 1960, height: 1240, alt: "A saved post by Paul Graham open in Sortwise, with its AI summary, categories, tags, list, and two related posts." },
  { key: "meaning", label: "Search by meaning", caption: "“habits” also finds the posts on sleep, communication, and a 35-year workout routine.", src: "/images/meaning.webp", width: 1914, height: 829, alt: "A search for habits with Meaning on, finding posts about sleep, communication, and a workout routine." },
  { key: "collection", label: "Smart collections", caption: "Save any search to the sidebar. New posts that match turn up there on their own.", src: "/images/collection.webp", width: 2560, height: 1440, alt: "A smart collection called Healthy habits in the sidebar, showing the posts that match its saved search." },
  { key: "finder", label: "Ctrl+K finder", caption: "Text, summaries, image descriptions, tags, and authors, searched as you type.", src: "/images/finder.webp", width: 1280, height: 1120, alt: "The Ctrl+K finder searching for LLM, with matching words highlighted." },
  { key: "author", label: "Authors", caption: "Everyone you've saved from, and everything you kept from each of them.", src: "/images/author.webp", width: 2560, height: 1600, alt: "Andrej Karpathy's page in Sortwise, listing the five posts saved from him." },
];

const DWELL = 6.5;

/* A tour of the app: each stop shows for a few seconds, its tab filling as
   it does, until the visitor picks one. */
export function Tour() {
  const [active, setActive] = useState(0);
  const [touring, setTouring] = useState(true);
  const stop = STOPS[active];

  return (
    <section id="features" className="tour section-cream" aria-labelledby="tour-title">
      <div className="container">
        <SectionHead id="tour-title" align="center" title="One library for everything you saved." />
        <div className="tour-tabs" role="tablist" aria-label="Product tour">
          {STOPS.map((item, index) => (
            <button
              key={item.key}
              type="button"
              role="tab"
              id={`tab-${item.key}`}
              aria-selected={index === active}
              aria-controls="tour-panel"
              className={`tour-tab ${index === active ? "on" : ""}`}
              onClick={() => {
                setActive(index);
                setTouring(false);
              }}
            >
              {item.label}
              {index === active && <motion.span className="tour-underline" layoutId="tour-underline" transition={{ duration: 0.4, ease: EASE }} />}
              {index === active && touring && (
                <motion.span
                  className="tour-progress"
                  initial={{ scaleX: 0 }}
                  animate={{ scaleX: 1 }}
                  transition={{ duration: DWELL, ease: "linear" }}
                  onAnimationComplete={() => setActive((value) => (value + 1) % STOPS.length)}
                />
              )}
            </button>
          ))}
        </div>
        <div className="tour-panel" id="tour-panel" role="tabpanel" aria-labelledby={`tab-${stop.key}`}>
          <AnimatePresence mode="wait">
            <motion.div key={stop.key} initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -10 }} transition={{ duration: 0.45, ease: EASE }}>
              <p className="tour-caption">{stop.caption}</p>
              <figure className="shot tour-shot">
                <img src={stop.src} width={stop.width} height={stop.height} alt={stop.alt} />
              </figure>
            </motion.div>
          </AnimatePresence>
        </div>
      </div>
    </section>
  );
}
