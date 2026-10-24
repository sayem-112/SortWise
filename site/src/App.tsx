import { Bento } from "./components/Bento";
import { DemoInvite } from "./components/DemoInvite";
import { Faq } from "./components/Faq";
import { Footer } from "./components/Footer";
import { Hero } from "./components/Hero";
import { Nav } from "./components/Nav";
import { Pipeline } from "./components/Pipeline";
import { PostMarquee } from "./components/PostMarquee";
import { Privacy } from "./components/Privacy";
import { Setup } from "./components/Setup";
import { Tour } from "./components/Tour";

export function App() {
  return (
    <>
      <a className="skip-link" href="#main">
        Skip to content
      </a>
      <Nav />
      <main id="main">
        <Hero />
        <PostMarquee />
        <Pipeline />
        <Tour />
        <Bento />
        <Setup />
        <Privacy />
        <DemoInvite />
        <Faq />
      </main>
      <Footer />
    </>
  );
}
