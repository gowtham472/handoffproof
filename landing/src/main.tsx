import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react';
import { createRoot } from 'react-dom/client';
import { ArrowUpRight, ArrowDown, ArrowRight, Fingerprint, GearSix, Database, ShieldCheck, TreeStructure, Flask, Files, GithubLogo, SlackLogo, UserPlus, CalendarCheck, EnvelopeSimple, CheckCircle, Play } from '@phosphor-icons/react';
import LaserFlow from './reactbits/LaserFlow';
import ShinyText from './reactbits/ShinyText';
import Magnet from './reactbits/Magnet';
import StarBorder from './reactbits/StarBorder';
import SpotlightCard from './reactbits/SpotlightCard';
import GlareHover from './reactbits/GlareHover';
import ClickSpark from './reactbits/ClickSpark';

declare const __STATIC_LANDING__: boolean;
const staticLanding = typeof __STATIC_LANDING__ !== 'undefined' && __STATIC_LANDING__;
const repository = 'https://github.com/gowtham472/handoffproof';
const demoVideo = 'https://youtu.be/LIAsrzvvelM';
const homeLink = staticLanding ? '/' : '/landing';
function workspaceLink(page = '') {
  if (!staticLanding) return '/#' + page;
  if (page === 'evidence' || page === 'overview') return repository + '/blob/main/docs/VALIDATION.md';
  if (page === 'settings') return repository + '/tree/main/docs/guide';
  return repository + '#quick-start';
}

function Mark() { return <ShieldCheck weight="duotone" aria-hidden="true"/>; }
function CTA({ children = 'Open Control Room', large = false }: {children?: ReactNode; large?: boolean}) {
  return <Magnet padding={8} magnetStrength={24} maxOffset={2} tilt={0} scale={1} spring={{stiffness:180,damping:30,mass:1}}><StarBorder as="a" href={staticLanding ? demoVideo : "/"} theme="dark" color="#0f62fe" backgroundColor="#0f62fe" textColor="#fff" radius={0} className={large ? 'cta cta-large' : 'cta'}>{staticLanding ? "Watch demo" : children}<ArrowUpRight size={18} aria-hidden="true"/></StarBorder></Magnet>;
}
function Heading({label, title, children}: {label:string; title:ReactNode; children:ReactNode}) {
  return <header className="section-heading"><div><p className="label">{label}</p><h2>{title}</h2></div><p>{children}</p></header>;
}
const views = [
  { label:'Readiness', image:'readiness.jpg', icon:ShieldCheck, description:'Review the handover score, workflow assertions, and dependency faults from a recorded demo run.', page:'overview' },
  { label:'Evidence', image:'evidence.jpg', icon:Files, description:'Inspect controller-recorded assertions and evidence IDs from the bundled demo application.', page:'evidence' },
  { label:'Connections', image:'connections.jpg', icon:GearSix, description:'Keep connection status in Settings. Open a dedicated workspace to configure GitHub or Slack.', page:'settings' },
];
function ProductPreview({panelRef}: {panelRef:React.RefObject<HTMLDivElement | null>}) {
  const [selected, setSelected] = useState(0);
  const view = views[selected];
  return <div ref={panelRef} className="product-preview panel ticks" id="preview">
    <div className="preview-toolbar"><div className="preview-brand"><Mark/><span>Inside HandoffProof</span></div><div className="product-tabs" role="tablist" aria-label="Product screens">{views.map((v,i)=><button key={v.label} id={'view-tab-'+i} role="tab" aria-selected={selected===i} aria-controls="product-screen" tabIndex={selected===i?0:-1} onClick={()=>setSelected(i)} onKeyDown={e=>{if(e.key==='ArrowRight'||e.key==='ArrowLeft'){e.preventDefault(); const next=(selected+(e.key==='ArrowRight'?1:2))%3;setSelected(next);document.getElementById('view-tab-'+next)?.focus();}}}><v.icon size={16} aria-hidden="true"/>{v.label}</button>)}</div><a href={workspaceLink(view.page)} className="preview-open" aria-label={(staticLanding ? 'Read documentation for ' : 'Open ')+view.label}><ArrowUpRight size={19}/></a></div>
    <div role="tabpanel" id="product-screen" aria-labelledby={'view-tab-'+selected} className="product-screen"><img loading="eager" fetchPriority="high" decoding="sync" src={'/landing/screens/'+view.image} width="2545" height="1440" alt={'HandoffProof '+view.label+' screen: '+view.description}/></div>
    <div className="preview-caption"><p>{view.description}</p><span><CheckCircle size={14} aria-hidden="true"/>Captured from the application</span></div>
  </div>;
}
const process = [
  {n:'01',title:'Map the application',copy:'Scan configuration, credentials, storage paths, and backup coverage.',icon:TreeStructure},
  {n:'02',title:'Run a handover drill',copy:'Establish a baseline, then withdraw a test token, remove configuration, or attempt recovery.',icon:Flask},
  {n:'03',title:'Review the evidence',copy:'Read assertions and source references. Review the proposed repair, then rerun the drill.',icon:Files},
];
function Landing() {
  const panel = useRef<HTMLDivElement>(null);
  const finalPanel = useRef<HTMLDivElement>(null);
  const [reduced, setReduced] = useState(()=>matchMedia('(prefers-reduced-motion: reduce)').matches);
  useEffect(()=>{const m=matchMedia('(prefers-reduced-motion: reduce)');const fn=()=>setReduced(m.matches);m.addEventListener('change',fn);return()=>m.removeEventListener('change',fn);},[]);
  const cardStyle = {'--spotlight-card-surface':'#262626','--spotlight-card-border':'#393939'} as CSSProperties;
  return <div className="lp"><ClickSpark sparkColor={['#0f62fe','#6929c4','#8d8d8d']} sparkCount={8} sparkRadius={22} disabled={reduced}>
    <a className="skip-link" href="#main">Skip to content</a>
    <nav className="navigation" aria-label="Main navigation"><a className="wordmark" href={homeLink}><Mark/>HandoffProof<span className="wordmark-dot">.</span></a><div className="nav-links"><a href="#why">What to test</a><a href="#rehearsal">How it works</a><a href="#principles">Evidence & controls</a></div><CTA/></nav>
    <main id="main">
      <section className="hero"><div className="grid-background" aria-hidden="true"/><div className="hero-beam" aria-hidden="true">{!reduced&&<LaserFlow theme="dark" surfaceRef={panel} beamPosition={0.5} color="#0f62fe" intensity={1.1} fog={0.04} dust={0.12} flare={0.7} spill={0.4} mouseInteraction/>}</div>
        <div className="hero-copy"><div className="hero-label"><span className="signal-dot"/><ShinyText text="HandoffProof / Developer handover rehearsals" color="#a8a8a8" shineColor="#ffffff" speed={3}/></div><h1>Rehearse your next<br/><span className="gradient-text">developer handover.</span></h1><p>Test the credentials, configuration, and backups your application depends on before the developer who knows them leaves.</p><div className="hero-actions"><CTA large>Open your workspace</CTA><a className="quiet-link" href="#preview"><Play size={16} weight="fill" aria-hidden="true"/>Explore the application<ArrowDown size={16} aria-hidden="true"/></a></div><div className="workflow-chips"><span><UserPlus size={17}/>Registration</span><span><CalendarCheck size={17}/>Booking</span><span><EnvelopeSimple size={17}/>Confirmation email</span></div></div>
        <ProductPreview panelRef={panel}/>
      </section>
      <section className="content-section" id="why"><Heading label="What to test" title={<>Find the dependencies<br/>a handover can miss.</>}>HandoffProof rehearses three common failures in an isolated environment, using the workflows in your application.</Heading>
        <div className="problem-grid"><div className="problem-story"><img src="/assets/illustrations/handover.svg" width="340" height="300" alt="Storyset illustration of two people preparing a developer handover"/><h3>Prepare with the developer,<br/>before their last day.</h3><p>Use the rehearsal to review account ownership, setup instructions, and recovery procedures together.</p><a href={workspaceLink("start")} className="quiet-link">{staticLanding ? "Get the source and setup guide" : "Start the handover checklist"}<ArrowRight size={18}/></a></div>
          <div className="feature-grid"><SpotlightCard theme="dark" spotlightColor="rgba(15,98,254,.14)" style={cardStyle} className="feature feature-wide"><Fingerprint className="feature-icon" size={30} weight="duotone"/><span className="feature-index">01 / Credentials</span><h3>Withdraw a developer’s test token.</h3><p>Run the workflows again to find which ones depend on that credential.</p><div className="dependency-diagram"><span><GithubLogo size={17}/>Developer account</span><ArrowRight size={20}/><span><EnvelopeSimple size={17}/>Confirmation email</span></div></SpotlightCard><SpotlightCard theme="dark" spotlightColor="rgba(105,41,196,.12)" style={cardStyle} className="feature"><GearSix className="feature-icon" size={28} weight="duotone"/><span className="feature-index">02 / Configuration</span><h3>Rebuild the setup.</h3><p>Remove configuration to check what a fresh environment needs to run.</p></SpotlightCard><SpotlightCard theme="dark" spotlightColor="rgba(15,98,254,.12)" style={cardStyle} className="feature"><Database className="feature-icon" size={28} weight="duotone"/><span className="feature-index">03 / Recovery</span><h3>Attempt a restore.</h3><p>Check whether the backup covers database rows and uploaded files.</p></SpotlightCard></div>
        </div>
      </section>
      <section className="content-section" id="rehearsal"><Heading label="How it works" title={<>From source code<br/>to a recorded rehearsal.</>}>Import your application, describe its workflows, and review the results in the Control Room.</Heading><div className="process-grid">{process.map(step=><div className="process-step" key={step.n}><span className="process-number">{step.n}<step.icon size={28} weight="duotone" aria-hidden="true"/></span><h3>{step.title}</h3><p>{step.copy}</p></div>)}</div>
        <GlareHover width="100%" height="auto" background="#262626" borderColor="#393939" borderRadius="16px" glareColor="#0f62fe" glareOpacity={.12} className="evidence-glare"><div className="evidence-preview"><div><p className="label signal">Recorded evidence</p><h3>Inspect the assertion<br/>behind a finding.</h3><p>The Go controller records workflow outcomes with evidence IDs. The Meta model uses those records to explain a failure and propose a repair.</p><a className="quiet-link" href={workspaceLink("evidence")}>{staticLanding ? "Read the validation evidence" : "Open the evidence view"}<ArrowUpRight size={18}/></a></div><a className="evidence-image" href={workspaceLink("evidence")}><img src="/landing/screens/evidence.jpg" alt="The HandoffProof evidence view showing recorded workflow assertions and their evidence IDs" width="2545" height="1440" loading="lazy"/></a></div></GlareHover>
      </section>
      <section className="content-section" id="principles"><Heading label="Evidence & controls" title={<>Review what happened<br/>and what still needs testing.</>}>Observed outcomes link to controller evidence. Inferred risks and dependencies outside the rehearsal remain separate.</Heading><div className="control-layout"><div className="control-list">{[{title:'Controller-owned verdicts',copy:'The model proposes diagnoses and repairs. The controller evaluates the workflow assertions.',icon:ShieldCheck},{title:'Protected acceptance tests',copy:'Repair proposals cannot modify the acceptance suite or the controller.',icon:Files},{title:'A rerun after repair',copy:'Review the proposed change, then use a controller rerun to check its effect.',icon:Flask}].map(item=><div className="control-row" key={item.title}><item.icon size={26} weight="duotone"/><div><h3>{item.title}</h3><p>{item.copy}</p></div></div>)}</div><div className="team-panel"><div className="team-icons"><GithubLogo size={36} weight="fill"/><SlackLogo size={36} weight="duotone"/></div><h3>Connect the tools<br/>your team uses.</h3><p>Choose a GitHub repository for remediation work and send report summaries to a Slack channel.</p><a href={workspaceLink("settings")} className="quiet-link">{staticLanding ? "Read the integration guides" : "Manage connections"}<ArrowUpRight size={18}/></a></div></div></section>
      <section className="final-section"><div className="final-beam" aria-hidden="true">{!reduced&&<LaserFlow theme="dark" surfaceRef={finalPanel} color="#0f62fe" beamPosition={.5} intensity={1.1} fog={0.04} dust={0.12} flare={0.7} spill={0.4}/>}</div><div ref={finalPanel} className="final-panel panel ticks"><div><p className="label signal">Prepare for the handover</p><h2>Start with a baseline.</h2><p>{staticLanding ? "Watch the recorded demo, then explore the source and setup instructions." : "Open the Control Room and run your first rehearsal."}</p></div><CTA large/></div></section>
    </main><footer className="footer"><a className="wordmark" href={homeLink}><Mark/>HandoffProof<span className="wordmark-dot">.</span></a><p>Developer handover rehearsals.</p><a href={staticLanding ? repository : "/#about"}>About the project<ArrowUpRight size={15}/></a><span>Built for the Meta AI Developer Hackathon · Illustrations by <a href="https://storyset.com" target="_blank" rel="noopener">Storyset</a></span></footer>
  </ClickSpark></div>;
}
createRoot(document.getElementById('root')!).render(<Landing/>);
