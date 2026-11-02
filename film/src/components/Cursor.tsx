import {useCurrentFrame, interpolate} from 'remotion';

export type Waypoint = {f: number; x: number; y: number; click?: boolean};

// OS arrow gliding through waypoints, arriving exactly on cue.
export const Cursor: React.FC<{path: Waypoint[]}> = ({path}) => {
	const f = useCurrentFrame();
	let x = path[0].x;
	let y = path[0].y;
	let squash = 0;
	for (let i = 1; i < path.length; i++) {
		const a = path[i - 1];
		const b = path[i];
		if (f >= b.f) {
			x = b.x;
			y = b.y;
		} else if (f > a.f) {
			const t = (f - a.f) / (b.f - a.f);
			const e = t * t * (3 - 2 * t);
			x = a.x + (b.x - a.x) * e;
			y = a.y + (b.y - a.y) * e;
			break;
		} else break;
	}
	for (const p of path) {
		if (p.click && f >= p.f && f < p.f + 6) {
			squash = Math.sin(((f - p.f) / 6) * Math.PI);
			x = p.x;
			y = p.y;
		}
	}
	return (
		<div
			style={{
				position: 'absolute',
				left: x,
				top: y,
				transform: `scale(${1 - squash * 0.25})`,
				zIndex: 50,
			}}
		>
			<svg width="44" height="44" viewBox="0 0 24 24">
				<path d="M5 3l14 7-6.5 1.5L9 18z" fill="#111" stroke="#fff" strokeWidth="1.5" />
			</svg>
			{squash > 0.5 && (
				<div
					style={{
						position: 'absolute',
						left: -14,
						top: -14,
						width: 60,
						height: 60,
						borderRadius: '50%',
						border: '4px solid #0F62FE',
						opacity: 1 - squash,
						transform: `scale(${squash})`,
					}}
				/>
			)}
		</div>
	);
};

export {interpolate};
