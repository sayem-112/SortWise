import { ChevronRight, Database, Folder, HardDrive, KeyRound, Lock } from "lucide-react";
import { motion } from "motion/react";
import { EASE, Reveal, SectionHead } from "./ui";

const FACTS = ["No account", "No cloud database", "No tracking", "Export any time"];
const PATH = ["This PC", "Local Disk (C:)", "Users", "you", "AppData", "Local", "Sortwise"];

/* Where the library actually is: a folder on your own disk. */
export function Privacy() {
  return (
    <section id="privacy" className="privacy section-cream" aria-labelledby="privacy-title">
      <div className="container privacy-grid">
        <div>
          <SectionHead id="privacy-title" title="Your library lives on your PC." lead="One file on your own disk, readable without Sortwise. Your API keys stay in Windows Credential Manager." />
          <ul className="fact-chips">
            {FACTS.map((fact, index) => (
              <motion.li key={fact} initial={{ opacity: 0, scale: 0.9 }} whileInView={{ opacity: 1, scale: 1 }} viewport={{ once: true }} transition={{ duration: 0.4, ease: EASE, delay: 0.2 + index * 0.08 }}>
                {fact}
              </motion.li>
            ))}
          </ul>
        </div>
        <Reveal className="explorer" delay={0.1}>
          <div className="explorer-bar">
            <HardDrive size={14} aria-hidden="true" />
            {PATH.map((part, index) => (
              <span key={part} className="crumb">
                {index > 0 && <ChevronRight size={12} aria-hidden="true" />}
                {part}
              </span>
            ))}
          </div>
          <div className="explorer-head">
            <span>Name</span>
            <span>Size</span>
          </div>
          <ul className="explorer-files">
            {[
              { icon: Database, name: "sortwise.db", size: "34 MB", main: true },
              { icon: Folder, name: "avatars", size: "1,502 items" },
              { icon: Folder, name: "backups", size: "3 items" },
            ].map((file, index) => (
              <motion.li key={file.name} className={file.main ? "main" : ""} initial={{ opacity: 0, x: 16 }} whileInView={{ opacity: 1, x: 0 }} viewport={{ once: true }} transition={{ duration: 0.5, ease: EASE, delay: 0.25 + index * 0.1 }}>
                <file.icon size={16} aria-hidden="true" />
                <span>{file.name}</span>
                <span className="size">{file.size}</span>
              </motion.li>
            ))}
          </ul>
          <div className="explorer-foot">
            <Lock size={13} aria-hidden="true" />
            <span>Keys</span>
            <KeyRound size={13} aria-hidden="true" />
            <span>Windows Credential Manager</span>
          </div>
        </Reveal>
      </div>
    </section>
  );
}
