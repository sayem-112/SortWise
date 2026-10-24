import { Bookmark, FileText, FolderTree, Sparkles, Tags, type LucideIcon } from "lucide-react";
import { motion } from "motion/react";
import { useLayoutEffect, useRef, useState, type RefObject } from "react";
import { EASE, Mark, SectionHead } from "./ui";

type Node = { key: string; icon: LucideIcon; title: string; detail: string };

const OUTPUTS: Node[] = [
  { key: "summary", icon: FileText, title: "Summary", detail: "Two lines on what it says" },
  { key: "tags", icon: Tags, title: "Tags", detail: "Specific, never “tech”" },
  { key: "category", icon: FolderTree, title: "Category", detail: "Filed where you'd look" },
  { key: "meaning", icon: Sparkles, title: "Meaning", detail: "Found by what it's about" },
];

/* A curved line between two elements of the diagram, from the right edge
   of one to the left edge of the other, in the container's coordinates. */
function curve(container: DOMRect, from: DOMRect, to: DOMRect) {
  const x1 = from.right - container.left;
  const y1 = from.top + from.height / 2 - container.top;
  const x2 = to.left - container.left;
  const y2 = to.top + to.height / 2 - container.top;
  const bend = (x2 - x1) * 0.5;
  return `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`;
}

/* Paths between the parts of the diagram, measured from the page and
   measured again whenever its size changes. */
function useBeams(container: RefObject<HTMLDivElement | null>, source: RefObject<HTMLDivElement | null>, hub: RefObject<HTMLDivElement | null>, outputs: RefObject<(HTMLDivElement | null)[]>) {
  const [paths, setPaths] = useState<string[]>([]);
  useLayoutEffect(() => {
    const box = container.current;
    if (!box) return;
    const measure = () => {
      const frame = box.getBoundingClientRect();
      const from = source.current?.getBoundingClientRect();
      const middle = hub.current?.getBoundingClientRect();
      if (!from || !middle) return;
      // Stacked on a phone: the beams run down instead of across.
      if (middle.top > from.bottom) {
        setPaths([]);
        return;
      }
      setPaths([
        curve(frame, from, middle),
        ...(outputs.current || []).filter((item): item is HTMLDivElement => Boolean(item)).map((item) => curve(frame, middle, item.getBoundingClientRect())),
      ]);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(box);
    return () => observer.disconnect();
  }, [container, source, hub, outputs]);
  return paths;
}

/* One tap on X, and the post flows through Sortwise into everything it is
   given: light travels along each line, one after another. */
export function Pipeline() {
  const container = useRef<HTMLDivElement>(null);
  const source = useRef<HTMLDivElement>(null);
  const hub = useRef<HTMLDivElement>(null);
  const outputs = useRef<(HTMLDivElement | null)[]>([]);
  const paths = useBeams(container, source, hub, outputs);

  return (
    <section className="pipeline section-orange" aria-labelledby="pipeline-title">
      <div className="container">
        <SectionHead id="pipeline-title" title="Tap bookmark. The rest is done for you." lead="The extension catches the post as you save it. Sortwise reads it, images and all, and files it before you've scrolled on." />
        <div className="beam-diagram" ref={container}>
          <svg className="beams" aria-hidden="true">
            {paths.map((path, index) => (
              <g key={index}>
                <path d={path} className="beam-track" />
                <path d={path} className="beam-light" pathLength={1} style={{ animationDelay: `${index === 0 ? 0 : 0.9 + index * 0.25}s` }} />
              </g>
            ))}
          </svg>
          <motion.div className="beam-node source" ref={source} initial={{ opacity: 0, x: -20 }} whileInView={{ opacity: 1, x: 0 }} viewport={{ once: true }} transition={{ duration: 0.6, ease: EASE }}>
            <span className="node-icon">
              <Bookmark size={20} aria-hidden="true" />
            </span>
            <span>
              <strong>Bookmark on X</strong>
              <small>From any device</small>
            </span>
          </motion.div>
          <motion.div className="beam-hub" ref={hub} initial={{ opacity: 0, scale: 0.8 }} whileInView={{ opacity: 1, scale: 1 }} viewport={{ once: true }} transition={{ duration: 0.6, ease: EASE, delay: 0.2 }}>
            <Mark size={56} tone="cream" />
            <strong>Sortwise</strong>
          </motion.div>
          <div className="beam-outputs">
            {OUTPUTS.map((node, index) => (
              <motion.div
                key={node.key}
                className="beam-node"
                ref={(element) => {
                  outputs.current[index] = element;
                }}
                initial={{ opacity: 0, x: 20 }}
                whileInView={{ opacity: 1, x: 0 }}
                viewport={{ once: true }}
                transition={{ duration: 0.6, ease: EASE, delay: 0.35 + index * 0.1 }}
              >
                <span className="node-icon">
                  <node.icon size={18} aria-hidden="true" />
                </span>
                <span>
                  <strong>{node.title}</strong>
                  <small>{node.detail}</small>
                </span>
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
