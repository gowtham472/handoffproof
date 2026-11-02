import {
	Img,
	staticFile,
	useCurrentFrame,
	useVideoConfig,
	interpolate,
	spring,
} from 'remotion';

// One owl cropped out of the shared character sheet (1200x900).
export const CROPS: Record<string, [number, number, number, number]> = {
	hero: [165, 190, 145, 105], // yellow owl face, top-left
	pink: [385, 120, 195, 185], // pink bird
	scout: [125, 375, 195, 185], // yellow bird, mid-left
	crab: [595, 395, 220, 180], // orange crab
	bluebird: [900, 135, 175, 180], // blue bird
	owl2: [125, 615, 200, 195], // yellow owl, bottom-left
};

export const Owl: React.FC<{
	pose?: keyof typeof CROPS;
	size?: number;
	style?: React.CSSProperties;
	bob?: number;
}> = ({pose = 'hero', size = 220, style, bob = 0}) => {
	const f = useCurrentFrame();
	const {fps} = useVideoConfig();
	const [x, y, w, h] = CROPS[pose];
	const scale = size / w;
	const dy = bob ? Math.sin(f / 14) * bob : 0;
	// Hop in from the left at the start of every scene (SAAP-style rig).
	const hop = spring({frame: f, fps, config: {stiffness: 110, damping: 16}});
	const enterX = (1 - hop) * -90;
	const enterY = -Math.sin(Math.min(1, hop) * Math.PI) * 26;
	return (
		<div
			style={{
				width: size,
				height: (h / w) * size,
				overflow: 'hidden',
				background: '#fff',
				borderRadius: size * 0.22,
				boxShadow: '0 16px 44px rgba(11,27,61,0.28)',
				opacity: hop,
				transform: `translate(${enterX}px, ${dy + enterY}px)`,
				...style,
			}}
		>
			<Img
				src={staticFile('characters.svg')}
				style={{
					width: 1200 * scale,
					height: 900 * scale,
					marginLeft: -x * scale,
					marginTop: -y * scale,
					maxWidth: 'none',
				}}
			/>
		</div>
	);
};

export const fade = (f: number, start: number, dur = 12) =>
	interpolate(f, [start, start + dur], [0, 1], {extrapolateRight: 'clamp'});
