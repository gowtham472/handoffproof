import {
	AbsoluteFill,
	Audio,
	Sequence,
	staticFile,
	useCurrentFrame,
	interpolate,
} from 'remotion';
import {Owl, fade} from './components/Owl';
import './tokens';
import {Punch} from './components/Punch';
import {Cursor} from './components/Cursor';
import {Window, Big, Note, FaultRow, INK, ACCENT, GREEN, RED} from './components/Window';

const BG = '#EEF2FB';

const Caption: React.FC<{text: string}> = ({text}) => {
	const f = useCurrentFrame();
	return (
		<div
			style={{
				position: 'absolute',
				bottom: 36,
				left: 0,
				right: 0,
				textAlign: 'center',
				opacity: fade(f, 6),
			}}
		>
			<span
				style={{
					background: INK,
					color: '#fff',
					fontSize: 26,
					padding: '10px 28px',
					borderRadius: 30,
					fontFamily: 'Plus Jakarta Sans, system-ui, sans-serif',
				}}
			>
				{text}
			</span>
		</div>
	);
};

const ProofCard: React.FC<{
	big: string;
	small: string;
	color: string;
	delay: number;
}> = ({big, small, color, delay}) => {
	const f = useCurrentFrame();
	const t = interpolate(f - delay, [0, 12], [0, 1], {extrapolateRight: 'clamp'});
	return (
		<div
			style={{
				background: '#fff',
				borderRadius: 20,
				padding: '36px 48px',
				textAlign: 'center',
				minWidth: 340,
				boxShadow: '0 20px 60px rgba(11,27,61,0.18)',
				opacity: t,
				transform: `translateY(${(1 - t) * 50}px)`,
			}}
		>
			<div style={{fontSize: 84, fontWeight: 800, color}}>{big}</div>
			<div style={{fontSize: 26, color: '#4A5578', marginTop: 8}}>{small}</div>
		</div>
	);
};

const Check = ({ok, text, delay}: {ok: boolean; text: string; delay: number}) => {
	const f = useCurrentFrame();
	const t = interpolate(f - delay, [0, 8], [0, 1], {extrapolateRight: 'clamp'});
	return (
		<div
			style={{
				display: 'flex',
				gap: 14,
				alignItems: 'center',
				fontSize: 25,
				opacity: t,
			}}
		>
			<div
				style={{
					width: 34,
					height: 34,
					borderRadius: '50%',
					background: ok ? GREEN : RED,
					color: '#fff',
					textAlign: 'center',
					lineHeight: '34px',
					fontWeight: 800,
				}}
			>
				{ok ? '✓' : '✗'}
			</div>
			<div style={{fontFamily: 'monospace'}}>{text}</div>
		</div>
	);
};

export const Film: React.FC<{music: string}> = ({music}) => (
	<AbsoluteFill style={{background: BG, fontFamily: 'Plus Jakarta Sans, system-ui, sans-serif',}}>
		<Audio src={music} volume={0.5} />

		{/* 0-6s Opening */}
		<Sequence from={0} durationInFrames={240}>
			<AbsoluteFill
				style={{
					background: '#0B1B3D',
					alignItems: 'center',
					justifyContent: 'center',
				}}
			>
				<div style={{display: 'flex', alignItems: 'center', gap: 70}}>
					<Owl pose="hero" size={300} bob={10} />
					<div>
						<div style={{fontSize: 120, fontWeight: 800, color: '#fff'}}>
							HandoffProof
						</div>
						<div style={{fontSize: 40, color: '#7AA2FF', marginTop: 16}}>
							Rehearse the day your developer leaves.
						</div>
					</div>
				</div>
			</AbsoluteFill>
		</Sequence>

		{/* 6-12s Problem */}
		<Sequence from={240} durationInFrames={360}>
			<Window page="readiness" active="Readiness" title="Readiness — 4 planted faults">
				<Big>Ravi leaves Friday. What breaks on Monday?</Big>
				<FaultRow name="withdrawn_token — mail credential" caught={false} delay={30} />
				<FaultRow name="missing_configuration — scheduler token" caught={false} delay={45} />
				<FaultRow name="incomplete_backup — documents" caught={false} delay={60} />
				<Note>Handover Score 85 — measured, never asserted.</Note>
			</Window>
			<Owl pose="scout" size={200} bob={8} style={{position: 'absolute', right: 140, top: 200}} />
			<Caption text="HandoffProof rehearses the exit before it happens" />
		</Sequence>

		{/* 12-15s Punchline */}
		<Sequence from={600} durationInFrames={90}>
			<Punch words={['Rehearse.']} accent={[0]} />
		</Sequence>

		{/* 15-24s Drill run */}
		<Sequence from={690} durationInFrames={510}>
			<Window page="rehearse" active="Rehearse" title="Rehearse — withdrawn_test_token">
				<Big>withdrawn_test_token · sandbox running</Big>
				<div
					style={{
						display: 'inline-block',
						alignSelf: 'flex-start',
								background: ACCENT,
						color: '#fff',
						fontSize: 26,
						fontWeight: 700,
						padding: '14px 44px',
						borderRadius: 12,
					}}
				>
					Run drill
				</div>
				<div style={{display: 'flex', flexWrap: 'wrap', gap: '10px 22px', fontSize: 23, fontFamily: 'monospace', padding: '14px 20px', background: '#F4F7FE', borderRadius: 12}}>
					<span>registration … <b style={{color: GREEN}}>pass</b></span>
					<span>booking … <b style={{color: GREEN}}>pass</b></span>
					<span>confirmation_email … <b style={{color: RED}}>FAIL</b></span>
				</div>
				<Note>The app looks fine. The email never arrives.</Note>
			</Window>
			<Owl pose="pink" size={170} bob={8} style={{position: 'absolute', right: 120, bottom: 170}} />
			<Cursor
				path={[
					{f: 0, x: 300, y: 800},
					{f: 60, x: 600, y: 400, click: true},
					{f: 120, x: 900, y: 640},
				]}
			/>
			<Caption text="Withdraw the credential, rerun the workflows, watch it break" />
		</Sequence>

		{/* 24-30s Failure evidence */}
		<Sequence from={1200} durationInFrames={240}>
			<Window page="evidence" active="Evidence" title="Evidence — controller verdict">
				<Big>EV-ad32e8e3 · confirmation_email</Big>
				<div style={{display: 'flex', gap: 14}}>
					<div style={{background: '#FCE4E4', color: RED, fontWeight: 800, fontSize: 30, padding: '12px 30px', borderRadius: 14}}>
						passed: false
					</div>
					<div style={{fontSize: 24, alignSelf: 'center', color: '#4A5578'}}>
						artifact 5feceb66… · recorded, not generated
					</div>
				</div>
				<Note>No model involved in this verdict. Docker + assertions only.</Note>
			</Window>
			<Owl pose="crab" size={200} style={{position: 'absolute', right: 150, top: 230}} />
			<Caption text="Deterministic: the same drill fails the same way, every time" />
		</Sequence>

		{/* 30-33s Punchline */}
		<Sequence from={1440} durationInFrames={90}>
			<Punch words={['Prove.']} accent={[0]} />
		</Sequence>

		{/* 33-38s Proof cards */}
		<Sequence from={1530} durationInFrames={270}>
			<AbsoluteFill style={{alignItems: 'center', justifyContent: 'center'}}>
				<div style={{display: 'flex', gap: 36}}>
					<ProofCard big="5/5" small="faults caught" color={GREEN} delay={0} />
					<ProofCard big="85" small="Handover Score" color={ACCENT} delay={10} />
					<ProofCard big="100,749" small="tokens measured" color="#6929C4" delay={20} />
				</div>
				<Owl pose="bluebird" size={190} bob={10} style={{position: 'absolute', right: 170, bottom: 170}} />
			</AbsoluteFill>
			<Caption text="Every number on screen comes from a real artifact" />
		</Sequence>

		{/* 38-52s Diagnosis */}
		<Sequence from={1800} durationInFrames={600}>
			<Window page="insights" active="Insights" title="Diagnosis — Meta model, verified">
				<Big>Withdrawn MAIL_API_TOKEN — model states, controller checks</Big>
				<Check ok text="src/mail.ts:2-16 — token read, failure matches" delay={30} />
				<Check ok text="evidence EV-ad32e8e3 resolves to a real record" delay={60} />
				<Check ok text="5 of 5 citations verified" delay={90} />
				<Note>The model explains. The verifier decides what counts.</Note>
			</Window>
			<Owl pose="owl2" size={200} bob={8} style={{position: 'absolute', right: 120, bottom: 160}} />
			<Caption text="The Meta model reads evidence; every citation is verified" />
		</Sequence>

		{/* 52-64s Red to green */}
		<Sequence from={2400} durationInFrames={450}>
			<Window page="repair" active="Rehearse" title="Repair — rerun decides">
				<div style={{fontFamily: 'monospace', fontSize: 23, background: '#0B1B3D', color: '#fff', borderRadius: 14, padding: 26, lineHeight: 1.7}}>
					<div><span style={{color: '#FF7A7A'}}>-  api_token = ""</span></div>
					<div><span style={{color: '#7AE8A0'}}>+  api_token = vault.read("mail/api_token")</span></div>
				</div>
				<div style={{display: 'flex', gap: 14, fontSize: 24, fontFamily: 'monospace', padding: '14px 20px', background: '#F4F7FE', borderRadius: 12}}>
					<span>rerun confirmation_email … <b style={{color: GREEN}}>pass</b></span>
				</div>
				<Note>The model proposed. The sandbox decided.</Note>
			</Window>
			<Owl pose="hero" size={180} bob={10} style={{position: 'absolute', right: 120, bottom: 170}} />
			<Caption text="Repair proposed by the model, believed only after a green rerun" />
		</Sequence>

		{/* 64-68s Punchline */}
		<Sequence from={2850} durationInFrames={120}>
			<Punch words={['Hand', 'over.']} accent={[1]} />
		</Sequence>

		{/* 68-78s BYOA */}
		<Sequence from={2970} durationInFrames={480}>
			<Window page="applications" active="Applications" title="Bring Your Own App">
				<Big>Import your own Docker Compose app</Big>
				<Check ok text="profile handoffproof.json — valid" delay={30} />
				<Check ok text="baseline 3/3 — real sandbox" delay={70} />
				<Check ok text="evidence carries app ID + source hash" delay={110} />
				<Note>Tested on a bundled fixture and a real external API.</Note>
			</Window>
			<Owl pose="scout" size={200} bob={8} style={{position: 'absolute', right: 140, top: 200}} />
			<Caption text="Your app, your workflows, the same rehearsal" />
		</Sequence>

		{/* 78-88s Connect */}
		<Sequence from={3450} durationInFrames={390}>
			<Window page="settings" active="Settings" title="HOP Connect">
				<Check ok text="Slack — rehearsal summary posted (counts only)" delay={30} />
				<Check ok text="GitHub — repo connected, app imported from main" delay={70} />
				<Note>Operators connect. Everyone can read the status.</Note>
			</Window>
			<Owl pose="pink" size={190} bob={10} style={{position: 'absolute', right: 150, top: 220}} />
			<Caption text="Summaries flow to Slack; apps flow in from GitHub" />
		</Sequence>

		{/* 88-100s Proof rail */}
		<Sequence from={3840} durationInFrames={420}>
			<AbsoluteFill style={{alignItems: 'center', justifyContent: 'center'}}>
				<div style={{display: 'flex', gap: 30}}>
					<ProofCard big="3/5" small="static scan" color={ACCENT} delay={0} />
					<ProofCard big="4/5" small="scan + drills" color={ACCENT} delay={12} />
					<ProofCard big="5/5" small="full HandoffProof" color={GREEN} delay={24} />
					<ProofCard big="3/3" small="second fixture" color={GREEN} delay={36} />
				</div>
				<Note>Misses reported plainly. Nothing asserted.</Note>
				<Owl pose="crab" size={180} style={{position: 'absolute', right: 160, bottom: 150}} />
			</AbsoluteFill>
			<Caption text="Honest benchmark: misses listed, sample evidence in-repo" />
		</Sequence>

		{/* 100-110s Close */}
		<Sequence from={4260} durationInFrames={240}>
			<AbsoluteFill style={{background: '#0B1B3D', alignItems: 'center'}}>
				<Owl pose="hero" size={240} bob={12} style={{position: 'absolute', right: 150, bottom: 130}} />
				<div style={{textAlign: 'center', marginTop: 300, maxWidth: 1500}}>
					<div style={{fontSize: 84, fontWeight: 800, color: '#fff'}}>
						Rehearse the exit. Prove the handover.
					</div>
					<div style={{fontSize: 38, color: '#7AA2FF', marginTop: 24, fontFamily: 'monospace'}}>
						github.com/gowtham472/handoffproof
					</div>
					<div style={{fontSize: 24, color: '#8A94B0', marginTop: 30}}>
						Music: “Inspiring Advertising” by Rafael Krux (CC BY 4.0)
					</div>
				</div>
			</AbsoluteFill>
		</Sequence>
	</AbsoluteFill>
);
