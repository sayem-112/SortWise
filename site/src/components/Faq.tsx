import { Plus } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { EASE, GITHUB_URL, SectionHead } from "./ui";

const QUESTIONS = [
  { q: "Is Sortwise free?", a: "Yes, and it works with free API keys from Google Gemini and Groq. Free keys have daily limits, so sorting a large first import can take a few days." },
  { q: "Why does Windows warn me when I first run it?", a: "Sortwise isn't signed with a paid code-signing certificate yet, so SmartScreen shows “Windows protected your PC”. Choose More info, then Run anyway. The source is on GitHub." },
  { q: "Why Developer mode for the extension?", a: "The extension isn't in the Chrome Web Store yet, so it's added from the folder that comes with Sortwise. Developer mode is the browser's switch for that." },
  { q: "Does it work on a Mac or Linux?", a: "Not yet. Sortwise is a Windows app for now, and the extension runs in Chrome and Microsoft Edge." },
  { q: "Do I give Sortwise my X password?", a: "No. The extension reads your bookmarks through X's own page, where you're already signed in." },
  { q: "Can I get my data out?", a: "Export everything to JSON or CSV from Settings, or make a full backup you can restore later." },
];

export function Faq() {
  const [open, setOpen] = useState<number | null>(0);
  return (
    <section id="faq" className="faq section-cream" aria-labelledby="faq-title">
      <div className="container faq-grid">
        <SectionHead
          id="faq-title"
          title="Questions"
          lead={
            <>
              Something else?{" "}
              <a className="text-link" href={`${GITHUB_URL}/issues`}>
                Ask on GitHub
              </a>
              .
            </>
          }
        />
        <ul className="faq-list">
          {QUESTIONS.map((item, index) => {
            const isOpen = open === index;
            return (
              <li key={item.q}>
                <button type="button" className="faq-question" aria-expanded={isOpen} aria-controls={`faq-${index}`} onClick={() => setOpen(isOpen ? null : index)}>
                  <span>{item.q}</span>
                  <motion.span animate={{ rotate: isOpen ? 45 : 0 }} transition={{ duration: 0.25, ease: EASE }} aria-hidden="true">
                    <Plus size={18} />
                  </motion.span>
                </button>
                <AnimatePresence initial={false}>
                  {isOpen && (
                    <motion.div id={`faq-${index}`} className="faq-answer" initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} exit={{ height: 0, opacity: 0 }} transition={{ duration: 0.35, ease: EASE }}>
                      <p>{item.a}</p>
                    </motion.div>
                  )}
                </AnimatePresence>
              </li>
            );
          })}
        </ul>
      </div>
    </section>
  );
}
