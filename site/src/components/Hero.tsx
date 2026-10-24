import { ArrowRight, Download } from "lucide-react";
import { motion, useScroll, useTransform } from "motion/react";
import { useRef } from "react";
import { DEMO_URL, DOWNLOAD_URL, EASE } from "./ui";

const LINES = ["Save it on X.", "Find it in seconds."];

/* The headline rises in word by word; the product sits below it, tipped
   back like a print on a table, and lies flat as the page scrolls to it. */
export function Hero() {
  const stage = useRef<HTMLDivElement>(null);
  const { scrollYProgress } = useScroll({ target: stage, offset: ["start end", "start 0.2"] });
  const rotateX = useTransform(scrollYProgress, [0, 1], [26, 0]);
  const scale = useTransform(scrollYProgress, [0, 1], [0.9, 1]);
  const y = useTransform(scrollYProgress, [0, 1], [40, 0]);
  let word = 0;

  return (
    <section className="hero section-cream" aria-labelledby="hero-title">
      <div className="dot-grid" aria-hidden="true" />
      <div className="container hero-copy">
        <motion.p className="badge" initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.6, ease: EASE }}>
          Free for Windows
        </motion.p>
        <h1 id="hero-title">
          {LINES.map((line) => (
            <span className="hero-line" key={line}>
              {line.split(" ").map((text) => {
                const index = word++;
                return (
                  <span className="word-mask" key={text + index}>
                    <motion.span className="word" initial={{ y: "110%" }} animate={{ y: 0 }} transition={{ duration: 0.8, ease: EASE, delay: 0.08 + index * 0.06 }}>
                      {text}
                    </motion.span>
                  </span>
                );
              })}
            </span>
          ))}
        </h1>
        <motion.p className="lead" initial={{ opacity: 0, y: 14 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.7, ease: EASE, delay: 0.55 }}>
          Every post you bookmark on X lands in a library on your PC, with a summary, tags, and a category written for it. Search
          by the words, or by what it meant.
        </motion.p>
        <motion.div className="actions" initial={{ opacity: 0, y: 14 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.7, ease: EASE, delay: 0.68 }}>
          <a className="button button-brand button-large" href={DOWNLOAD_URL}>
            <Download size={17} aria-hidden="true" />
            Download for Windows
          </a>
          <a className="button button-dark button-large arrow-link" href={DEMO_URL}>
            Try the live demo
            <ArrowRight size={17} aria-hidden="true" />
          </a>
        </motion.div>
        <motion.p className="meta" initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.7, delay: 0.85 }}>
          Windows 10 and 11 · Chrome or Edge
        </motion.p>
      </div>
      <div className="container hero-stage" ref={stage}>
        <motion.figure className="shot shot-hero" style={{ rotateX, scale, y }} initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.9, delay: 0.4 }}>
          <img src="/images/preview.webp" width="2560" height="1600" alt="Sortwise showing a saved post by Andrej Karpathy with its AI summary, tags, category, and related posts, open over the library." />
        </motion.figure>
      </div>
    </section>
  );
}
