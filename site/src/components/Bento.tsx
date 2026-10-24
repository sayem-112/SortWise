import { Bookmark, Check, Search, Sparkles, Zap } from "lucide-react";
import { animate, AnimatePresence, motion, useInView, useMotionValue, useReducedMotion, useTransform } from "motion/react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import library from "../data/library.json";
import { Avatar, EASE, Mark, Reveal, SectionHead } from "./ui";

function Cell({ className = "", title, children, delay = 0 }: { className?: string; title: ReactNode; children: ReactNode; delay?: number }) {
  return (
    <Reveal className={`bento-cell ${className}`} delay={delay}>
      <div className="bento-visual">{children}</div>
      <h3>{title}</h3>
    </Reveal>
  );
}

/* Runs a looping script while the element is on screen: each step waits
   its time, then the loop starts over. */
function useLoop(steps: number[], active: boolean) {
  const [step, setStep] = useState(0);
  useEffect(() => {
    if (!active) return;
    const timer = setTimeout(() => setStep((value) => (value + 1) % steps.length), steps[step]);
    return () => clearTimeout(timer);
  }, [step, active, steps]);
  return step;
}

const HABITS = "habits";
const RESULTS = [
  { ...library.meaning[1], text: "12 must-read nonfiction books, summarized", meaning: false },
  { ...library.meaning[2], text: "BE BORING 2026: the gym, meal prep, 8,000 steps", meaning: false },
  { ...library.meaning[0], text: "The 6 things wrecking your sleep", meaning: true },
  { ...library.meaning[3], text: "Not sophisticated, but it has worked for 35 years", meaning: true },
];
const MEANING_STEPS = [700, 1300, 900, 3600];

/* Typing "habits", turning Meaning on, and the posts that never use the word joining the results. */
function MeaningSearch() {
  const ref = useRef<HTMLDivElement>(null);
  const inView = useInView(ref, { margin: "-20% 0px" });
  const still = useReducedMotion();
  const step = useLoop(MEANING_STEPS, inView && !still);
  const typed = step === 0 ? "" : HABITS;
  const meaning = still || step >= 2;
  const shown = RESULTS.filter((result) => meaning || !result.meaning);
  return (
    <div className="mini-search" ref={ref}>
      <div className="mini-input">
        <Search size={15} aria-hidden="true" />
        <span className="typed">
          {step === 1 ? (
            <motion.span initial={{ width: 0 }} animate={{ width: "auto" }} transition={{ duration: 0.8, ease: "linear" }} className="typing">
              {HABITS}
            </motion.span>
          ) : (
            typed
          )}
          <span className="caret" />
        </span>
        <span className={`mini-toggle ${meaning ? "on" : ""}`}>
          <Sparkles size={12} aria-hidden="true" /> Meaning
        </span>
      </div>
      <ul className="mini-results">
        <AnimatePresence initial={false}>
          {step > 0 &&
            shown.map((result, index) => (
              <motion.li key={result.text} layout initial={{ opacity: 0, x: -12 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0 }} transition={{ duration: 0.4, ease: EASE, delay: index * 0.06 }}>
                <Avatar src={result.avatar} name={result.author} size={24} />
                <strong>{result.author}</strong>
                <span>{result.text}</span>
                {result.meaning && (
                  <span className="mini-similar">
                    <Sparkles size={11} aria-hidden="true" /> Similar
                  </span>
                )}
              </motion.li>
            ))}
        </AnimatePresence>
      </ul>
    </div>
  );
}

/* A post on X being bookmarked, and the note that it's saved. */
function Capture() {
  const ref = useRef<HTMLDivElement>(null);
  const inView = useInView(ref, { margin: "-20% 0px" });
  const still = useReducedMotion();
  const step = useLoop([1400, 2600], inView && !still);
  const saved = still || step === 1;
  const post = library.posts.find((item) => item.username === "paulg") || library.posts[0];
  return (
    <div className="capture" ref={ref}>
      <div className="x-post">
        <header>
          <Avatar src={post.avatar} name={post.author} size={28} />
          <strong>{post.author}</strong>
          <span>@{post.username}</span>
        </header>
        <p>{post.text.slice(0, 110)}…</p>
        <footer>
          <motion.span className={`x-bookmark ${saved ? "on" : ""}`} animate={saved ? { scale: [1, 1.35, 1] } : { scale: 1 }} transition={{ duration: 0.4 }}>
            <Bookmark size={16} fill={saved ? "currentColor" : "none"} aria-hidden="true" />
          </motion.span>
        </footer>
      </div>
      <AnimatePresence>
        {saved && (
          <motion.div className="saved-toast" initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 8 }} transition={{ duration: 0.35, ease: EASE }}>
            <Check size={14} aria-hidden="true" /> Saved to Sortwise
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

const KEYS = [
  { key: "J", label: "Down" },
  { key: "K", label: "Up" },
  { key: "X", label: "Select" },
  { key: "F", label: "Star" },
  { key: "L", label: "Lists" },
];

/* The library's keys, pressed one after another. */
function Keys() {
  const ref = useRef<HTMLDivElement>(null);
  const inView = useInView(ref, { margin: "-20% 0px" });
  const still = useReducedMotion();
  const step = useLoop([600, 600, 600, 600, 600, 1200], inView && !still);
  return (
    <div className="keys" ref={ref}>
      {KEYS.map((item, index) => (
        <span className="key-group" key={item.key}>
          <motion.kbd animate={step === index ? { y: 3, backgroundColor: "var(--ink-amber)" } : { y: 0, backgroundColor: "var(--key-face)" }} transition={{ duration: 0.15 }}>
            {item.key}
          </motion.kbd>
          <small>{item.label}</small>
        </span>
      ))}
    </div>
  );
}

/* A number that counts up the first time it is seen. */
function Count({ to }: { to: number }) {
  const ref = useRef<HTMLSpanElement>(null);
  const inView = useInView(ref, { once: true });
  const value = useMotionValue(0);
  const rounded = useTransform(value, (latest) => Math.round(latest).toLocaleString());
  useEffect(() => {
    if (!inView) return;
    const controls = animate(value, to, { duration: 1.6, ease: EASE });
    return () => controls.stop();
  }, [inView, to, value]);
  return <motion.span ref={ref}>{rounded}</motion.span>;
}

const COLLECTIONS = [
  { name: "Healthy habits", count: 48, tone: "var(--ink-orange)" },
  { name: "Rust, by meaning", count: 112, tone: "var(--ink-amber)" },
  { name: "System design", count: 86, tone: "var(--ink-deep)" },
];

function Collections() {
  return (
    <ul className="collections">
      <li className="collections-label">Smart collections</li>
      {COLLECTIONS.map((item) => (
        <li key={item.name}>
          <Zap size={15} style={{ color: item.tone }} aria-hidden="true" />
          <span>{item.name}</span>
          <span className="collections-count">
            <Count to={item.count} />
          </span>
        </li>
      ))}
    </ul>
  );
}

/* The people you save from, circling your library. */
function Authors() {
  const inner = library.avatars.slice(0, 6);
  const outer = library.avatars.slice(6, 14);
  const ring = (people: typeof inner, radius: number, reverse: boolean) => (
    <div className={`orbit ${reverse ? "reverse" : ""}`} style={{ width: radius * 2, height: radius * 2 }}>
      {people.map((person, index) => {
        const angle = (index / people.length) * Math.PI * 2;
        return (
          <span className="orbiter" key={person.username} style={{ left: radius + Math.cos(angle) * radius, top: radius + Math.sin(angle) * radius }}>
            <Avatar src={person.avatar} name={person.username} size={34} />
          </span>
        );
      })}
    </div>
  );
  return (
    <div className="authors">
      {ring(outer, 118, true)}
      {ring(inner, 70, false)}
      <span className="authors-center">
        <Mark size={40} />
      </span>
    </div>
  );
}

export function Bento() {
  return (
    <section className="bento-section section-cream" aria-labelledby="bento-title">
      <div className="container">
        <SectionHead id="bento-title" title="Built for finding things again." />
        <div className="bento">
          <Cell className="wide" title={<>Search by meaning finds the post, even when you've forgotten its words.</>}>
            <MeaningSearch />
          </Cell>
          <Cell title="Saved the moment you tap it." delay={0.08}>
            <Capture />
          </Cell>
          <Cell title="Collections that fill themselves." delay={0.04}>
            <Collections />
          </Cell>
          <Cell title="Every author you've saved from, with everything you kept." delay={0.08}>
            <Authors />
          </Cell>
          <Cell title="Your hands never leave the keys." delay={0.12}>
            <Keys />
          </Cell>
        </div>
      </div>
    </section>
  );
}
