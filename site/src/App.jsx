import React from 'react';
import { useLocation } from 'react-router-dom';
import { NavBar } from './sections/NavBar.jsx';
import { Hero } from './sections/Hero.jsx';
import { PipelineSection } from './sections/PipelineSection.jsx';
import { FeatureGrid } from './sections/FeatureGrid.jsx';
import { ConsolePreview } from './sections/ConsolePreview.jsx';
import { CtaSection, Footer } from './sections/CtaSection.jsx';
import './styles/page.css';

export default function App() {
  const { hash } = useLocation();
  // Scroll to an in-page section when the URL carries a hash (e.g. /#lifecycle).
  React.useEffect(() => {
    if (hash) {
      const el = document.getElementById(decodeURIComponent(hash.slice(1)));
      if (el) { el.scrollIntoView({ behavior: 'smooth' }); return; }
    }
    window.scrollTo(0, 0);
  }, [hash]);

  // Scroll-spy: reflect the section in view in the URL hash so leaving and
  // coming back (or the browser Back button) lands near the same place. Uses
  // replaceState (no history spam) and bypasses react-router (no scroll fight).
  React.useEffect(() => {
    const ids = ['lifecycle', 'features', 'control-tower'];
    let current = window.location.hash.slice(1);
    let raf = 0;
    const update = () => {
      raf = 0;
      const line = window.scrollY + window.innerHeight * 0.35;
      let active = '';
      for (const id of ids) {
        const el = document.getElementById(id);
        if (el && el.getBoundingClientRect().top + window.scrollY <= line) active = id;
      }
      if (active !== current) {
        current = active;
        const url = active ? `#${active}` : window.location.pathname + window.location.search;
        window.history.replaceState(window.history.state, '', url);
      }
    };
    const onScroll = () => { if (!raf) raf = requestAnimationFrame(update); };
    window.addEventListener('scroll', onScroll, { passive: true });
    update();
    return () => { window.removeEventListener('scroll', onScroll); if (raf) cancelAnimationFrame(raf); };
  }, []);
  return (
    <div className="tw-page-bg">
      <NavBar />
      <Hero />
      <PipelineSection />
      <FeatureGrid />
      <ConsolePreview />
      <CtaSection />
      <Footer />
    </div>
  );
}
