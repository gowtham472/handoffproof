import {useCurrentFrame, interpolate} from 'remotion';

// Big kinetic punchline word(s), one per beat, like the reference film.
export const Punch: React.FC<{words: string[]; accent?: number[]}> = ({
	words,
	accent = [],
}) => {
	const f = useCurrentFrame();
	return (
		<div
			style={{
				flex: 1,
				display: 'flex',
				alignItems: 'center',
				justifyContent: 'center',
				gap: 36,
				background: '#0B1B3D',
			}}
		>
			{words.map((w, i) => {
				const t = interpolate(f - i * 9, [0, 9, 30], [0, 1, 1], {
					extrapolateLeft: 'clamp',
					extrapolateRight: 'clamp',
				});
				return (
					<div
						key={w + i}
						style={{
							fontSize: 170,
							fontWeight: 800,
							fontFamily: 'Plus Jakarta Sans, system-ui, sans-serif',
							color: accent.includes(i) ? '#7AA2FF' : '#fff',
							opacity: t,
							filter: `blur(${(1 - t) * 16}px)`,
							transform: `translateY(${(1 - t) * 60}px)`,
						}}
					>
						{w}
					</div>
				);
			})}
		</div>
	);
};
