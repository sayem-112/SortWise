import { motion } from "motion/react";
import type { ReactNode } from "react";

export const DOWNLOAD_URL = "https://github.com/sayem-112/SortWise/releases/latest";
export const GITHUB_URL = "https://github.com/sayem-112/SortWise";
export const DEMO_URL = "/demo/";

export const EASE = [0.22, 1, 0.36, 1] as const;

/* Rises into place the first time it scrolls into view. */
export function Reveal({ children, delay = 0, className, as = "div" }: { children: ReactNode; delay?: number; className?: string; as?: "div" | "li" }) {
  const Tag = as === "li" ? motion.li : motion.div;
  return (
    <Tag
      className={className}
      initial={{ opacity: 0, y: 28 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "0px 0px -10% 0px" }}
      transition={{ duration: 0.7, ease: EASE, delay }}
    >
      {children}
    </Tag>
  );
}

/* A section heading with its short lead, revealed together. */
export function SectionHead({ eyebrow, title, lead, align = "left", id }: { eyebrow?: string; title: ReactNode; lead?: ReactNode; align?: "left" | "center"; id: string }) {
  return (
    <Reveal className={`section-head ${align}`}>
      {eyebrow && <p className="eyebrow">{eyebrow}</p>}
      <h2 id={id}>{title}</h2>
      {lead && <p className="lead">{lead}</p>}
    </Reveal>
  );
}

/* The app's mark in the site's colours. */
export function Mark({ size = 28, tone = "orange" }: { size?: number; tone?: "orange" | "cream" }) {
  const tile = tone === "orange" ? "var(--ink-orange)" : "var(--ink-cream)";
  const bars = tone === "orange" ? "var(--ink-cream)" : "var(--ink-orange)";
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true">
      <rect width="64" height="64" fill={tile} />
      <rect x="12" y="14" width="40" height="9" fill={bars} />
      <rect x="12" y="28" width="28" height="9" fill={bars} />
      <rect x="12" y="42" width="16" height="9" fill={bars} />
      <circle cx="45" cy="46" r="6" fill="var(--ink-amber)" />
    </svg>
  );
}

export function Avatar({ src, name, size = 32 }: { src: string; name: string; size?: number }) {
  return <img className="avatar" src={src} alt="" width={size} height={size} loading="lazy" referrerPolicy="no-referrer" title={name} />;
}
