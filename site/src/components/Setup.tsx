import { ArrowRight, Download } from "lucide-react";
import { motion, useScroll, useSpring } from "motion/react";
import { useRef } from "react";
import { DOWNLOAD_URL, GITHUB_URL, Reveal, SectionHead } from "./ui";

const STEPS = [
  {
    title: "Download and run",
    text: (
      <>
        Unzip Sortwise and run <code>sortwise.exe</code>. If Windows warns about an unknown publisher, choose More info, then Run
        anyway.
      </>
    ),
  },
  {
    title: "Add the extension",
    text: (
      <>
        On the extensions page in Chrome or Edge, turn on Developer mode, choose Load unpacked, and pick the <code>extension</code>{" "}
        folder. Pair it with the code Sortwise shows you.
      </>
    ),
  },
  {
    title: "Add a free Gemini key",
    text: <>Paste it in Settings. Then bring in everything you've already bookmarked, or start from today.</>,
  },
];

/* The steps on a line that draws itself as the page scrolls past. */
export function Setup() {
  const list = useRef<HTMLOListElement>(null);
  const { scrollYProgress } = useScroll({ target: list, offset: ["start 0.75", "end 0.55"] });
  const drawn = useSpring(scrollYProgress, { stiffness: 120, damping: 30 });

  return (
    <section id="setup" className="setup section-orange" aria-labelledby="setup-title">
      <div className="container setup-grid">
        <div className="setup-intro">
          <SectionHead id="setup-title" title="Set up in five minutes." />
          <Reveal className="actions">
            <a className="button button-cream button-large" href={DOWNLOAD_URL}>
              <Download size={17} aria-hidden="true" />
              Download for Windows
            </a>
            <a className="text-link arrow-link" href={`${GITHUB_URL}#install`}>
              Full install guide <ArrowRight size={15} aria-hidden="true" />
            </a>
          </Reveal>
        </div>
        <ol className="timeline" ref={list}>
          <span className="timeline-rail" aria-hidden="true">
            <motion.span className="timeline-fill" style={{ scaleY: drawn }} />
          </span>
          {STEPS.map((step, index) => (
            <Reveal as="li" key={step.title} delay={index * 0.08}>
              <span className="timeline-dot" aria-hidden="true">
                {index + 1}
              </span>
              <h3>{step.title}</h3>
              <p>{step.text}</p>
            </Reveal>
          ))}
        </ol>
      </div>
    </section>
  );
}
