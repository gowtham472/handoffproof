import {useCurrentFrame, interpolate} from 'remotion';
import {Owl} from './Owl';

export const INK = '#0A1633';
export const ACCENT = '#0F62FE';
export const GREEN = '#198038';
export const RED = '#DA1E28';

const shell: React.CSSProperties = {
	width: 1560,
	height: 880,
	margin: '100px auto',
	background: '#fff',
	borderRadius: 20,
	boxShadow: '0 30px 80px rgba(11,27,61,0.25)',
	overflow: 'hidden',
	fontFamily: 'Plus Jakarta Sans, system-ui, sans-serif',
	color: INK,
	display: 'flex',
};

const Side: React.FC<{active: string}> = ({active}) => {
	const items = ['Get started', 'Applications', 'Readiness', 'Rehearse', 'Evidence', 'Insights', 'Settings'];
	return (
		<div style={{width: 280, background: '#0B1B3D', color: '#fff', padding: '28px 0'}}>
			<div style={{padding: '0 28px 24px', fontWeight: 800, fontSize: 26}}>
				HandoffProof
			</div>
			{items.map((it) => (
				<div
					key={it}
					style={{
						padding: '13px 28px',
						fontSize: 22,
						background: it === active ? '#1D3A7A' : 'transparent',
						borderLeft: it === active ? `6px solid ${ACCENT}` : '6px solid transparent',
					}}
				>
					{it}
				</div>
			))}
		</div>
	);
};

const Top: React.FC<{title: string}> = ({title}) => (
	<div
		style={{
			height: 84,
			borderBottom: '1px solid #E3E8F5',
			display: 'flex',
			alignItems: 'center',
			padding: '0 36px',
			fontSize: 30,
			fontWeight: 700,
		}}
	>
		{title}
	</div>
);

const FaultRow: React.FC<{name: string; caught: boolean; delay: number}> = ({
	name,
	caught,
	delay,
}) => {
	const f = useCurrentFrame();
	const t = interpolate(f - delay, [0, 10], [0, 1], {extrapolateRight: 'clamp'});
	return (
		<div
			style={{
				display: 'flex',
				alignItems: 'center',
				gap: 18,
				padding: '16px 22px',
				background: '#F4F7FE',
				borderRadius: 12,
				opacity: t,
				transform: `translateY(${(1 - t) * 24}px)`,
			}}
		>
			<div
				style={{
					background: caught ? '#DEFBE6' : '#FCE4E4',
					color: caught ? GREEN : RED,
					fontWeight: 700,
					padding: '6px 16px',
					borderRadius: 20,
					fontSize: 20,
				}}
			>
				{caught ? 'caught' : 'live'}
			</div>
			<div style={{fontSize: 24}}>{name}</div>
		</div>
	);
};

export const Window: React.FC<{
	page: string;
	active: string;
	title: string;
	children: React.ReactNode;
}> = ({page, active, title, children}) => {
	const f = useCurrentFrame();
	// Slow camera push-in across the scene (SAAP camera-keys idea, subtle).
	const zoom = 1 + Math.min(f, 500) / 500 * 0.035;
	return (
		<div data-page={page} style={{...shell, transform: `scale(${zoom})`}}>
			<Side active={active} />
			<div style={{flex: 1}}>
				<Top title={title} />
				<div style={{padding: 36, display: 'flex', flexDirection: 'column', gap: 18}}>
					{children}
				</div>
			</div>
		</div>
	);
};

export const Big = ({children}: {children: React.ReactNode}) => (
	<div style={{fontSize: 34, fontWeight: 700}}>{children}</div>
);

export const Note = ({children}: {children: React.ReactNode}) => (
	<div style={{fontSize: 23, color: '#4A5578'}}>{children}</div>
);

export {FaultRow, Owl};
